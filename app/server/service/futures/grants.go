package futures

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"time"

	"gorm.io/gorm"
)

var (
	ErrGrantDenied   = errors.New("futures: grant denied")
	ErrGrantConsumed = errors.New("futures: grant already consumed")
	ErrGrantExpired  = errors.New("futures: grant expired")
)

type TaskLease struct {
	Domain     string
	OwnerID    uint
	Mode       string
	RunID      string
	TaskID     string
	Generation int64
	ManifestID string
	Active     bool
}

type GrantSpec struct {
	Domain     string
	OwnerID    uint
	Mode       string
	RunID      string
	TaskID     string
	Generation int64
	ManifestID string
	Stage      string
	ToolName   string
	ArgsHash   string
}

type GrantRecord struct {
	Token     string
	Spec      GrantSpec
	Nonce     string
	ExpiresAt time.Time
	Consumed  bool
}

type GrantStore interface {
	Create(context.Context, GrantRecord) error
	Peek(context.Context, string) (GrantRecord, error)
	Consume(context.Context, string) (GrantRecord, error)
}

type LeaseAuthorizer interface{ CheckLease(TaskLease) error }

type GrantManager struct {
	store      GrantStore
	authorizer LeaseAuthorizer
	now        func() time.Time
}

func NewGrantManager(store GrantStore, authorizer LeaseAuthorizer, now func() time.Time) *GrantManager {
	if now == nil {
		now = time.Now
	}
	return &GrantManager{store: store, authorizer: authorizer, now: now}
}

func (m *GrantManager) Issue(spec GrantSpec) (string, error) {
	if spec.Domain != "futures" || spec.Mode != "live" || spec.OwnerID == 0 || spec.RunID == "" || spec.TaskID == "" || spec.Generation < 1 || spec.ManifestID == "" || spec.ArgsHash == "" {
		return "", ErrGrantDenied
	}
	if spec.ToolName != "" && !allowedGrantTool(spec.ToolName) {
		return "", ErrGrantDenied
	}
	lease := TaskLease{Domain: spec.Domain, OwnerID: spec.OwnerID, Mode: spec.Mode, RunID: spec.RunID, TaskID: spec.TaskID, Generation: spec.Generation, ManifestID: spec.ManifestID, Active: true}
	if err := m.authorizer.CheckLease(lease); err != nil {
		return "", err
	}
	var nonceBytes [16]byte
	if _, err := rand.Read(nonceBytes[:]); err != nil {
		return "", err
	}
	token := "fg_" + hex.EncodeToString(nonceBytes[:])
	record := GrantRecord{Token: token, Nonce: token, Spec: spec, ExpiresAt: m.now().Add(60 * time.Second)}
	if err := m.store.Create(context.Background(), record); err != nil {
		return "", err
	}
	return token, nil
}

func (m *GrantManager) Use(token string, spec GrantSpec) (GrantRecord, error) {
	peek, err := m.store.Peek(context.Background(), token)
	if err != nil {
		return GrantRecord{}, err
	}
	lease := TaskLease{Domain: peek.Spec.Domain, OwnerID: peek.Spec.OwnerID, Mode: peek.Spec.Mode, RunID: peek.Spec.RunID, TaskID: peek.Spec.TaskID, Generation: peek.Spec.Generation, ManifestID: peek.Spec.ManifestID, Active: true}
	if err := m.authorizer.CheckLease(lease); err != nil {
		return GrantRecord{}, err
	}
	record, err := m.store.Consume(context.Background(), token)
	if err != nil {
		return GrantRecord{}, err
	}
	if spec.Stage == "" {
		spec.Stage = record.Spec.Stage
	}
	if spec.ToolName == "" {
		spec.ToolName = record.Spec.ToolName
	}
	if !reflect.DeepEqual(record.Spec, spec) {
		return GrantRecord{}, ErrGrantDenied
	}
	if record.Consumed {
		return GrantRecord{}, ErrGrantConsumed
	}
	if !m.now().Before(record.ExpiresAt) {
		return GrantRecord{}, ErrGrantExpired
	}
	if err := m.authorizer.CheckLease(lease); err != nil {
		return GrantRecord{}, err
	}
	return record, nil
}

func allowedGrantTool(name string) bool {
	switch name {
	case "futures_get_observations", "futures_get_evidence", "futures_calculate":
		return true
	default:
		return false
	}
}

func HashArguments(arguments any) string {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(arguments)
	raw := bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type MemoryGrantStore struct {
	mu      sync.Mutex
	records map[string]GrantRecord
}

type DBGrantStore struct{ DB *gorm.DB }

func NewDBGrantStore(db *gorm.DB) *DBGrantStore { return &DBGrantStore{DB: db} }

func (s *DBGrantStore) Create(ctx context.Context, record GrantRecord) error {
	return s.DB.WithContext(ctx).Exec(`INSERT INTO futures_grants(nonce,owner_id,run_id,task_id,generation,manifest_id,stage,tool_name,args_hash,expires_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		record.Token, record.Spec.OwnerID, record.Spec.RunID, record.Spec.TaskID, record.Spec.Generation, record.Spec.ManifestID, record.Spec.Stage, record.Spec.ToolName, record.Spec.ArgsHash, record.ExpiresAt).Error
}

func (s *DBGrantStore) Consume(ctx context.Context, token string) (GrantRecord, error) {
	var record GrantRecord
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct {
			Nonce                                                string
			OwnerID                                              uint
			RunID, TaskID, ManifestID, Stage, ToolName, ArgsHash string
			Generation                                           int64
			ExpiresAt                                            time.Time
			ConsumedAt                                           *time.Time
		}
		if err := tx.Raw(`SELECT nonce,owner_id,run_id,task_id,generation,manifest_id,stage,tool_name,args_hash,expires_at,consumed_at FROM futures_grants WHERE nonce=? FOR UPDATE`, token).Scan(&row).Error; err != nil {
			return err
		}
		if row.Nonce == "" {
			return ErrGrantDenied
		}
		record = GrantRecord{Token: row.Nonce, Nonce: row.Nonce, ExpiresAt: row.ExpiresAt, Consumed: row.ConsumedAt != nil, Spec: GrantSpec{
			Domain: "futures", OwnerID: row.OwnerID, Mode: "live", RunID: row.RunID, TaskID: row.TaskID, Generation: row.Generation,
			ManifestID: row.ManifestID, Stage: row.Stage, ToolName: row.ToolName, ArgsHash: row.ArgsHash,
		}}
		if row.ConsumedAt != nil {
			return ErrGrantConsumed
		}
		return tx.Exec(`UPDATE futures_grants SET consumed_at=? WHERE nonce=?`, time.Now().UTC(), token).Error
	})
	if err != nil {
		return record, err
	}
	record.Consumed = false
	return record, nil
}

func (s *DBGrantStore) Peek(ctx context.Context, token string) (GrantRecord, error) {
	var row struct {
		Nonce                                                string
		OwnerID                                              uint
		RunID, TaskID, ManifestID, Stage, ToolName, ArgsHash string
		Generation                                           int64
		ExpiresAt                                            time.Time
		ConsumedAt                                           *time.Time
	}
	if err := s.DB.WithContext(ctx).Raw(`SELECT nonce,owner_id,run_id,task_id,generation,manifest_id,stage,tool_name,args_hash,expires_at,consumed_at FROM futures_grants WHERE nonce=?`, token).Scan(&row).Error; err != nil {
		return GrantRecord{}, err
	}
	if row.Nonce == "" {
		return GrantRecord{}, ErrGrantDenied
	}
	return GrantRecord{Token: row.Nonce, Nonce: row.Nonce, ExpiresAt: row.ExpiresAt, Consumed: row.ConsumedAt != nil, Spec: GrantSpec{
		Domain: "futures", OwnerID: row.OwnerID, Mode: "live", RunID: row.RunID, TaskID: row.TaskID, Generation: row.Generation,
		ManifestID: row.ManifestID, Stage: row.Stage, ToolName: row.ToolName, ArgsHash: row.ArgsHash,
	}}, nil
}

func NewMemoryGrantStore() *MemoryGrantStore {
	return &MemoryGrantStore{records: map[string]GrantRecord{}}
}

func (s *MemoryGrantStore) Create(_ context.Context, record GrantRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.records[record.Token]; exists {
		return ErrGrantConsumed
	}
	s.records[record.Token] = record
	return nil
}

func (s *MemoryGrantStore) Consume(_ context.Context, token string) (GrantRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[token]
	if !ok {
		return GrantRecord{}, ErrGrantDenied
	}
	if record.Consumed {
		return record, ErrGrantConsumed
	}
	consumed := record
	consumed.Consumed = true
	s.records[token] = consumed
	return record, nil
}

func (s *MemoryGrantStore) Peek(_ context.Context, token string) (GrantRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[token]
	if !ok {
		return GrantRecord{}, ErrGrantDenied
	}
	return record, nil
}

func StaticLeaseAuthorizer(expected TaskLease) LeaseAuthorizer {
	return leaseAuthorizerFunc(func(actual TaskLease) error {
		if !actual.Active || !reflect.DeepEqual(actual, expected) {
			return ErrGrantDenied
		}
		return nil
	})
}

type leaseAuthorizerFunc func(TaskLease) error

func (f leaseAuthorizerFunc) CheckLease(lease TaskLease) error    { return f(lease) }
func LeaseAuthorizerFunc(f func(TaskLease) error) LeaseAuthorizer { return leaseAuthorizerFunc(f) }

type DBLeaseAuthorizer struct{ DB *gorm.DB }

func (a DBLeaseAuthorizer) CheckLease(lease TaskLease) error {
	var row struct {
		Status, ManifestID, SnapshotTaskID string
		Generation                         int64
		LeaseUntil, DeadlineAt             *time.Time
		CancelRequested                    bool
	}
	err := a.DB.Raw(`SELECT status, manifest_id, task_snapshot->>'task_id' AS snapshot_task_id, generation, lease_until, deadline_at, cancel_requested
FROM futures_runs WHERE id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, lease.RunID, lease.OwnerID, lease.Mode).Scan(&row).Error
	active := row.Status == "running" || row.Status == "verifying"
	now := time.Now().UTC()
	if err != nil || !active || row.CancelRequested || row.Generation != lease.Generation || row.ManifestID != lease.ManifestID ||
		row.SnapshotTaskID != lease.TaskID || row.LeaseUntil == nil || !row.LeaseUntil.After(now) ||
		row.DeadlineAt == nil || !row.DeadlineAt.After(now) {
		return ErrGrantDenied
	}
	var count int64
	if err := a.DB.Raw(`SELECT count(*) FROM futures_manifests WHERE id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, lease.ManifestID, lease.OwnerID, lease.Mode).Scan(&count).Error; err != nil || count != 1 {
		return ErrGrantDenied
	}
	return nil
}
