package futures

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	model "zhigu/server/model/futures"
)

const releaseGateTTL = 24 * time.Hour

type SourceProbeInput struct {
	SourceID          string `json:"source_id"`
	SourceVersion     int64  `json:"source_version"`
	AdapterVersion    string `json:"adapter_version"`
	Result            string `json:"result"`
	Reason            string `json:"reason"`
	SecretRevision    int64  `json:"secret_revision"`
	SecretFingerprint string `json:"secret_fingerprint"`
	EvidenceHash      string `json:"evidence_hash"`
}

type SourceProbeFilter struct {
	SourceID string
	Result   string
}

func (f SourceProbeFilter) String() string {
	return "source_id=" + f.SourceID + ";result=" + f.Result
}

type SourceProbeValue struct {
	ID                string    `gorm:"column:id;primaryKey"`
	OwnerID           uint      `gorm:"column:owner_id"`
	SourceID          string    `gorm:"column:source_id"`
	SourceVersion     int64     `gorm:"column:source_version"`
	AdapterVersion    string    `gorm:"column:adapter_version"`
	ConfigDigest      string    `gorm:"column:config_digest"`
	SecretRevision    int64     `gorm:"column:secret_revision"`
	SecretFingerprint string    `gorm:"column:secret_fingerprint"`
	Result            string    `gorm:"column:result"`
	Reason            string    `gorm:"column:reason"`
	EvidenceHash      string    `gorm:"column:evidence_hash"`
	CreatedAt         time.Time `gorm:"column:created_at"`
}

func (SourceProbeValue) TableName() string { return "futures_source_probes" }

type ReleaseGateValue struct {
	ID                        string     `gorm:"column:id;primaryKey"`
	OwnerID                   uint       `gorm:"column:owner_id"`
	Kind                      string     `gorm:"column:kind"`
	ProbeID                   string     `gorm:"column:probe_id"`
	ConfigDigest              string     `gorm:"column:config_digest"`
	SecretRevision            int64      `gorm:"column:secret_revision"`
	SecretFingerprint         string     `gorm:"column:secret_fingerprint"`
	Status                    string     `gorm:"column:status"`
	Reason                    string     `gorm:"column:reason"`
	CreatedAt                 time.Time  `gorm:"column:created_at"`
	ExpiresAt                 time.Time  `gorm:"column:expires_at"`
	ConsumedAt                *time.Time `gorm:"column:consumed_at"`
	ConsumedOperationsVersion *int64     `gorm:"column:consumed_operations_version"`
	InvalidatedAt             *time.Time `gorm:"column:invalidated_at"`
	InvalidatedReason         *string    `gorm:"column:invalidated_reason"`
}

func (ReleaseGateValue) TableName() string { return "futures_release_gates" }

func (d *Domain) WithNow(now func() time.Time) *Domain {
	if now != nil {
		d.Now = now
	}
	return d
}

func ComputeSourceConfigDigest(config any, secretRevision int64, secretFingerprint string) string {
	payload := map[string]any{
		"schema_version":     "futures.source-config.v1",
		"config":             config,
		"secret_revision":    secretRevision,
		"secret_fingerprint": secretFingerprint,
	}
	return hashJSON(payload)
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && strings.ToLower(value) == value
}

func validSecretFingerprint(value string) bool {
	if len(value) < 32 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func newSourceReleaseGate(id Identity, probeID, configDigest string, secretRevision int64, secretFingerprint string, now time.Time) ReleaseGateValue {
	return ReleaseGateValue{
		ID: "gate_" + uuid.NewString(), OwnerID: id.OwnerID, Kind: "source", ProbeID: probeID,
		ConfigDigest: configDigest, SecretRevision: secretRevision, SecretFingerprint: secretFingerprint,
		Status: "active", Reason: "SOURCE_PROBE_PASS", CreatedAt: now.UTC(), ExpiresAt: now.UTC().Add(releaseGateTTL),
	}
}

func sourceProbeMap(probe SourceProbeValue, releaseGateID any) map[string]any {
	return map[string]any{
		"id": probe.ID, "source_id": probe.SourceID, "source_version": probe.SourceVersion,
		"adapter_version": probe.AdapterVersion, "config_digest": probe.ConfigDigest,
		"secret_revision": probe.SecretRevision, "secret_fingerprint": probe.SecretFingerprint,
		"result": probe.Result, "reason": probe.Reason, "evidence_hash": probe.EvidenceHash,
		"created_at": probe.CreatedAt, "release_gate_id": releaseGateID,
	}
}

func validateSourceProbe(input SourceProbeInput) error {
	if input.SourceID == "" || input.SourceVersion < 1 || input.AdapterVersion == "" || input.Reason == "" ||
		input.SecretRevision < 1 || !validSecretFingerprint(input.SecretFingerprint) || !validSHA256(input.EvidenceHash) ||
		!oneOf(input.Result, "PASS", "FAIL", "NOT_RUN", "BLOCKED") {
		return ErrInvalidInput
	}
	return nil
}

func sourceConfigWithoutSecret(source SourceValue) map[string]any {
	var config map[string]any
	_ = json.Unmarshal(source.Manifest, &config)
	if config == nil {
		config = map[string]any{}
	}
	delete(config, "credential_ref")
	config["source_id"] = source.SourceID
	config["source_version"] = source.Version
	config["adapter_version"] = source.AdapterVersion
	return config
}

func (d *Domain) CreateSourceProbe(ctx context.Context, id Identity, idempotencyKey string, input SourceProbeInput) (map[string]any, error) {
	if id.OwnerID == 0 || idempotencyKey == "" {
		return nil, ErrInvalidInput
	}
	if err := validateSourceProbe(input); err != nil {
		return nil, err
	}
	now := d.Now().UTC()
	var result map[string]any
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		requestHash := hashJSON(input)
		var existing model.Idempotency
		err := tx.Where(`owner_id=? AND mode=? AND operation=? AND key=?`, id.OwnerID, id.Mode, "source.probe", idempotencyKey).First(&existing).Error
		if err == nil {
			if existing.RequestHash != requestHash {
				return ErrIdempotencyConflict
			}
			return json.Unmarshal(existing.Response, &result)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var source SourceValue
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(`source_id=? AND deleted_at IS NULL`, input.SourceID).First(&source).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if source.Version != input.SourceVersion || source.AdapterVersion != input.AdapterVersion {
			return ErrRevisionConflict
		}
		digest := ComputeSourceConfigDigest(sourceConfigWithoutSecret(source), input.SecretRevision, input.SecretFingerprint)
		probe := SourceProbeValue{
			ID: "probe_" + uuid.NewString(), OwnerID: id.OwnerID, SourceID: input.SourceID, SourceVersion: input.SourceVersion,
			AdapterVersion: input.AdapterVersion, ConfigDigest: digest, SecretRevision: input.SecretRevision,
			SecretFingerprint: input.SecretFingerprint, Result: input.Result, Reason: input.Reason,
			EvidenceHash: input.EvidenceHash, CreatedAt: now,
		}
		if err := tx.Create(&probe).Error; err != nil {
			return err
		}
		var releaseGateID any
		if input.Result == "PASS" {
			gate := newSourceReleaseGate(id, probe.ID, digest, input.SecretRevision, input.SecretFingerprint, now)
			if err := tx.Create(&gate).Error; err != nil {
				return err
			}
			releaseGateID = gate.ID
		}
		result = sourceProbeMap(probe, releaseGateID)
		response, _ := json.Marshal(result)
		return tx.Create(&model.Idempotency{
			OwnerID: id.OwnerID, Mode: id.Mode, Operation: "source.probe", Key: idempotencyKey,
			RequestHash: requestHash, Response: datatypes.JSON(response), Status: "complete",
			ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (d *Domain) ListSourceProbesPage(ctx context.Context, id Identity, filter SourceProbeFilter, options PageOptions) (Page[map[string]any], error) {
	if options.Limit < 1 || options.Limit > 100 {
		return Page[map[string]any]{}, ErrInvalidInput
	}
	query := d.DB.WithContext(ctx).Where(`owner_id=?`, id.OwnerID)
	if filter.SourceID != "" {
		query = query.Where(`source_id=?`, filter.SourceID)
	}
	if filter.Result != "" {
		query = query.Where(`result=?`, filter.Result)
	}
	if options.Cursor != nil {
		cursorTime, err := time.Parse(time.RFC3339Nano, options.Cursor.Primary)
		if err != nil {
			return Page[map[string]any]{}, ErrInvalidCursor
		}
		query = query.Where(`created_at < ? OR (created_at = ? AND id < ?)`, cursorTime, cursorTime, options.Cursor.Secondary)
	}
	var values []SourceProbeValue
	if err := query.Order(`created_at DESC, id DESC`).Limit(options.Limit + 1).Find(&values).Error; err != nil {
		return Page[map[string]any]{}, err
	}
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		out = append(out, sourceProbeMap(value, nil))
	}
	return BuildPage(id, "source-probes", filter.String(), options, out, func(value map[string]any) (string, string, string) {
		return TimePagePosition(value["created_at"].(time.Time), fmt.Sprint(value["id"]), 0)
	})
}

func consumeReleaseGate(tx *gorm.DB, id Identity, gateID, configDigest string, operationsVersion int64, now time.Time) error {
	if gateID == "" || configDigest == "" {
		return ErrReleaseGate
	}
	var gate ReleaseGateValue
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(`id=? AND owner_id=?`, gateID, id.OwnerID).First(&gate).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrReleaseGate
	}
	if err != nil {
		return err
	}
	if gate.Status != "active" || gate.ConfigDigest != configDigest || !gate.ExpiresAt.After(now) {
		return ErrReleaseGate
	}
	consumedAt := now.UTC()
	res := tx.Model(&ReleaseGateValue{}).
		Where(`id=? AND owner_id=? AND status='active' AND config_digest=? AND expires_at>?`, gateID, id.OwnerID, configDigest, now).
		Updates(map[string]any{"status": "consumed", "consumed_at": consumedAt, "consumed_operations_version": operationsVersion})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrReleaseGate
	}
	return nil
}

func invalidateSourceGates(tx *gorm.DB, sourceID, reason string, now time.Time) error {
	if reason == "" {
		reason = "SOURCE_CONFIG_CHANGED"
	}
	return tx.Exec(`UPDATE futures_release_gates SET status='invalidated', invalidated_at=?, invalidated_reason=?
WHERE status='active' AND probe_id IN (SELECT id FROM futures_source_probes WHERE source_id=?)`, now.UTC(), reason, sourceID).Error
}
