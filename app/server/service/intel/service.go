package intel

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"zhigu/server/httpx"
	svcfinance "zhigu/server/service/finance"
)

type Service struct {
	DB         *gorm.DB
	CookieKey  []byte
	FixtureDir string
	Now        func() time.Time
	Configs    *svcfinance.ConfigService
	ModelCall  ModelCall
}

func NewService(db *gorm.DB, cookieKey []byte, fixtureDir string) *Service {
	return &Service{DB: db, CookieKey: append([]byte(nil), cookieKey...), FixtureDir: fixtureDir, Now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) WithConfigService(configs *svcfinance.ConfigService) *Service {
	s.Configs = configs
	return s
}

type Scope struct {
	Mode           string     `json:"mode"`
	NamespaceID    string     `json:"-"`
	PrincipalKey   string     `json:"principal_id"`
	Role           string     `json:"role"`
	NamespaceLabel string     `json:"namespace_label"`
	SessionID      string     `json:"-"`
	ExpiresAt      *time.Time `json:"expires_at"`
	Generation     int64      `json:"generation"`
	ReplayVersion  int64      `json:"replay_version"`
	Branch         string     `json:"branch,omitempty"`
	StepIndex      int        `json:"step_index,omitempty"`
	SimulatedAt    *time.Time `json:"-"`
}

type ReplayRequest struct {
	Action          string `json:"action"`
	ExpectedVersion int64  `json:"expected_version"`
	Branch          string `json:"branch,omitempty"`
}

type ReplayResult struct {
	ReplayVersion     int64      `json:"replay_version"`
	Generation        int64      `json:"generation"`
	StepIndex         int        `json:"step_index"`
	SimulatedAt       *time.Time `json:"simulated_at,omitempty"`
	EventVersion      int        `json:"event_version"`
	NotificationCount int        `json:"notification_count"`
	Changes           []string   `json:"change_ids"`
	Done              bool       `json:"done"`
}

type ListEventsResult struct {
	Items      []any  `json:"items"`
	NextCursor string `json:"next_cursor"`
}

type EventQuery struct {
	Code         string
	Type         string
	Verification string
	Cursor       string
	Limit        int
}

type NotificationQuery struct {
	Status string
	Cursor string
	Limit  int
}

type ListNotificationsResult struct {
	Items       []any  `json:"items"`
	NextCursor  string `json:"next_cursor"`
	UnreadCount int    `json:"unread_count"`
}

func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	if app, ok := err.(*svcfinance.AppError); ok {
		return app.Code
	}
	return "INTERNAL"
}

func newError(status int, code, message string) error {
	return svcfinance.NewError(status, "intel", code, message)
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) SignDemoToken(sessionID string) string {
	mac := hmac.New(sha256.New, s.CookieKey)
	mac.Write([]byte(sessionID))
	return sessionID + "." + hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) VerifyDemoToken(token string) (string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	mac := hmac.New(sha256.New, s.CookieKey)
	mac.Write([]byte(parts[0]))
	want := hex.EncodeToString(mac.Sum(nil))
	return parts[0], hmac.Equal([]byte(want), []byte(parts[1]))
}

func scanScope(row map[string]any) Scope {
	str := func(k string) string { v, _ := row[k].(string); return v }
	i64 := func(k string) int64 {
		switch v := row[k].(type) {
		case int64:
			return v
		case int32:
			return int64(v)
		case int:
			return int64(v)
		default:
			return 0
		}
	}
	i := func(k string) int { return int(i64(k)) }
	tm := func(k string) *time.Time {
		if v, ok := row[k].(time.Time); ok && !v.IsZero() {
			t := v.UTC()
			return &t
		}
		return nil
	}
	label := str("kind")
	if label == "demo" {
		label = "演示数据"
	} else {
		label = "真实数据"
	}
	return Scope{
		Mode: str("kind"), NamespaceID: str("id"), PrincipalKey: str("principal_key"),
		Role: "user", NamespaceLabel: label, ExpiresAt: tm("expires_at"),
		Generation: i64("generation"), ReplayVersion: i64("replay_version"),
		Branch: str("branch"), StepIndex: i("step_index"), SimulatedAt: tm("simulated_at"),
	}
}

func (s *Service) scope(ctx context.Context, namespaceID string) (Scope, error) {
	var row map[string]any
	err := s.DB.WithContext(ctx).Raw(`SELECT * FROM finance_intel_namespaces WHERE id = ?`, namespaceID).Scan(&row).Error
	if err != nil {
		return Scope{}, err
	}
	if len(row) == 0 {
		return Scope{}, newError(404, "NOT_FOUND", "演示空间不存在")
	}
	return scanScope(row), nil
}

func (s *Service) CreateDemoSession(ctx context.Context, bootstrapHash, idemKey, bodyHash string) (Scope, bool, error) {
	if bootstrapHash == "" || idemKey == "" || bodyHash == "" {
		return Scope{}, false, newError(400, "BOOTSTRAP_REQUIRED", "缺少演示初始化凭据")
	}
	var out Scope
	replayed := false
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing map[string]any
		err := tx.Raw(`SELECT s.id AS session_id, s.idem_body_hash::text AS existing_body_hash, n.* FROM finance_intel_demo_sessions s
JOIN finance_intel_namespaces n ON n.id = s.namespace_id
WHERE s.idem_scope = ? AND s.idem_key = ?`, bootstrapHash, idemKey).Scan(&existing).Error
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			existingBody, _ := existing["existing_body_hash"].(string)
			if existingBody != bodyHash {
				return newError(409, "IDEMPOTENCY_CONFLICT", "相同幂等键对应不同请求")
			}
			out = scanScope(existing)
			out.SessionID, _ = existing["session_id"].(string)
			out.Mode = "demo"
			out.Role = "user"
			out.NamespaceLabel = "演示数据"
			replayed = true
			return nil
		}

		sessionID := uuid.NewString()
		namespaceID := "dns_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		principal := "demo:" + sessionID
		now := s.now()
		expires := now.Add(24 * time.Hour)
		if err := tx.Exec(`INSERT INTO finance_intel_namespaces
(id, kind, principal_key, generation, replay_version, branch, step_index, simulated_at, expires_at, created_at, updated_at)
VALUES (?, 'demo', ?, 1, 0, 'main', 0, NULL, ?, ?, ?)`, namespaceID, principal, expires, now, now).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO finance_intel_demo_sessions
(id, namespace_id, bootstrap_hash, idem_scope, idem_key, idem_body_hash, idem_status, idem_response, created_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?, 201, '{}'::jsonb, ?, ?)`, sessionID, namespaceID, bootstrapHash, bootstrapHash, idemKey, bodyHash, now, expires).Error; err != nil {
			return err
		}
		instruments := []struct{ code, name string }{{"DEMO.A", "演示公司A"}, {"DEMO.B", "演示公司B"}, {"DEMO.C", "演示公司C"}}
		for _, inst := range instruments {
			if err := tx.Exec(`INSERT INTO finance_intel_instruments
(id, namespace_id, code, name, exchange, source, verified_at, created_at)
VALUES (?, ?, ?, ?, 'DEMO', 'fixture', ?, ?)`, "inst_"+strings.ReplaceAll(uuid.NewString(), "-", ""), namespaceID, inst.code, inst.name, now, now).Error; err != nil {
				return err
			}
		}
		out = Scope{
			Mode: "demo", NamespaceID: namespaceID, PrincipalKey: principal, Role: "user",
			NamespaceLabel: "演示数据", SessionID: sessionID, ExpiresAt: &expires,
			Generation: 1, ReplayVersion: 0, Branch: "main", StepIndex: 0,
		}
		return nil
	})
	return out, replayed, err
}

func (s *Service) ResolveDemoSession(ctx context.Context, sessionID string) (Scope, error) {
	var row map[string]any
	err := s.DB.WithContext(ctx).Raw(`SELECT n.*, s.id AS session_id, s.expires_at AS session_expires_at
FROM finance_intel_demo_sessions s JOIN finance_intel_namespaces n ON n.id = s.namespace_id
WHERE s.id = ? AND s.expires_at > ?`, sessionID, s.now()).Scan(&row).Error
	if err != nil {
		return Scope{}, err
	}
	if len(row) == 0 {
		return Scope{}, newError(401, "DEMO_SESSION_EXPIRED", "演示会话不存在或已过期")
	}
	scope := scanScope(row)
	scope.SessionID = sessionID
	scope.Mode = "demo"
	if t, ok := row["session_expires_at"].(time.Time); ok {
		e := t.UTC()
		scope.ExpiresAt = &e
	}
	return scope, nil
}

func assertGeneration(tx *gorm.DB, scope Scope) error {
	var generation int64
	if err := tx.Raw(`SELECT generation FROM finance_intel_namespaces WHERE id=? FOR UPDATE`, scope.NamespaceID).Scan(&generation).Error; err != nil {
		return err
	}
	if generation != scope.Generation {
		return newError(409, "VERSION_CONFLICT", "数据空间 generation 已变化")
	}
	return nil
}

func (s *Service) AddWatchlist(ctx context.Context, scope Scope, code string) (map[string]any, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil, newError(400, "INVALID_PARAM", "code 不能为空")
	}
	var out map[string]any
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := assertGeneration(tx, scope); err != nil {
			return err
		}
		var exists map[string]any
		if err := tx.Raw(`SELECT code, name, exchange FROM finance_intel_instruments WHERE namespace_id = ? AND code = ?`, scope.NamespaceID, code).Scan(&exists).Error; err != nil {
			return err
		}
		if len(exists) == 0 {
			return newError(404, "NOT_FOUND", "标的不存在")
		}
		var count int
		if err := tx.Raw(`SELECT count(*) FROM finance_intel_watchlist WHERE namespace_id = ? AND principal_key = ?`, scope.NamespaceID, scope.PrincipalKey).Scan(&count).Error; err != nil {
			return err
		}
		var current map[string]any
		if err := tx.Raw(`SELECT code, subscribed_at FROM finance_intel_watchlist WHERE namespace_id = ? AND principal_key = ? AND code = ?`, scope.NamespaceID, scope.PrincipalKey, code).Scan(&current).Error; err != nil {
			return err
		}
		if len(current) > 0 {
			out = current
			return nil
		}
		if count >= 20 {
			return newError(422, "WATCHLIST_LIMIT", "最多关注 20 个标的")
		}
		now := s.now()
		if err := tx.Exec(`INSERT INTO finance_intel_watchlist(namespace_id, principal_key, code, subscribed_at, revision)
VALUES (?, ?, ?, ?, 1)`, scope.NamespaceID, scope.PrincipalKey, code, now).Error; err != nil {
			return err
		}
		out = map[string]any{"code": code, "subscribed_at": now}
		return nil
	})
	return out, err
}

func (s *Service) RemoveWatchlist(ctx context.Context, scope Scope, code string) error {
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`DELETE FROM finance_intel_watchlist WHERE namespace_id = ? AND principal_key = ? AND code = ?`, scope.NamespaceID, scope.PrincipalKey, strings.ToUpper(code)).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE finance_intel_outbox SET status='cancelled'
WHERE namespace_id = ? AND principal_key = ? AND status='pending'`, scope.NamespaceID, scope.PrincipalKey).Error
	})
	return err
}

func (s *Service) SetMute(ctx context.Context, scope Scope, eventID string, muted bool) (map[string]any, error) {
	now := s.now()
	var out map[string]any
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := assertGeneration(tx, scope); err != nil {
			return err
		}
		internalID := eventID
		var resolved string
		if err := tx.Raw(`SELECT id FROM finance_intel_events WHERE namespace_id=? AND logical_id=?`, scope.NamespaceID, eventID).Scan(&resolved).Error; err != nil {
			return err
		}
		if resolved != "" {
			internalID = resolved
		}
		if err := execSQL(tx, `INSERT INTO finance_intel_mutes
(namespace_id, principal_key, event_id, muted, effective_at, revision)
VALUES (?, ?, ?, ?, ?, 1)
ON CONFLICT(namespace_id, principal_key, event_id)
DO UPDATE SET muted=EXCLUDED.muted, effective_at=EXCLUDED.effective_at, revision=finance_intel_mutes.revision+1`,
			scope.NamespaceID, scope.PrincipalKey, internalID, muted, now); err != nil {
			return err
		}
		out = map[string]any{"event_id": eventID, "muted": muted, "effective_at": now}
		return nil
	})
	return out, err
}

func (s *Service) PatchNotification(ctx context.Context, scope Scope, id, status string) (map[string]any, error) {
	if status != "read" && status != "ignored" {
		return nil, newError(400, "INVALID_PARAM", "status 只能是 read 或 ignored")
	}
	var out map[string]any
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := assertGeneration(tx, scope); err != nil {
			return err
		}
		var row map[string]any
		if err := tx.Raw(`SELECT * FROM finance_intel_notifications WHERE id = ? AND namespace_id = ? AND principal_key = ? FOR UPDATE`, id, scope.NamespaceID, scope.PrincipalKey).Scan(&row).Error; err != nil {
			return err
		}
		if len(row) == 0 {
			return newError(404, "NOT_FOUND", "通知不存在")
		}
		current, _ := row["status"].(string)
		if current == "ignored" && status == "read" {
			return newError(409, "NOTIFICATION_STATE_CONFLICT", "已忽略的通知不能恢复为已读")
		}
		now := s.now()
		if err := tx.Exec(`UPDATE finance_intel_notifications SET status=?, updated_at=? WHERE id=?`, status, now, id).Error; err != nil {
			return err
		}
		row["status"] = status
		row["updated_at"] = now
		if status == "read" {
			row["read_at"] = now
		}
		out = row
		return nil
	})
	return out, err
}

func (s *Service) LiveScope(ctx context.Context, userID uint, role string) (Scope, error) {
	principal := fmt.Sprintf("user:%d", userID)
	namespaceID := "live_shared"
	now := s.now()
	err := s.DB.WithContext(ctx).Exec(`INSERT INTO finance_intel_namespaces
(id,kind,principal_key,generation,replay_version,branch,step_index,simulated_at,expires_at,created_at,updated_at)
VALUES (?, 'live', ?, 1, 0, 'main', 0, NULL, NULL, ?, ?)
ON CONFLICT (kind, principal_key) DO NOTHING`, namespaceID, "shared", now, now).Error
	if err != nil {
		return Scope{}, err
	}
	var row map[string]any
	if err := s.DB.WithContext(ctx).Raw(`SELECT * FROM finance_intel_namespaces WHERE id=?`, namespaceID).Scan(&row).Error; err != nil {
		return Scope{}, err
	}
	if len(row) == 0 {
		return Scope{}, newError(503, "DATA_UNAVAILABLE", "真实数据空间未初始化")
	}
	out := scanScope(row)
	out.Mode = "live"
	out.PrincipalKey = principal
	out.Role = role
	out.NamespaceLabel = "真实数据"
	return out, nil
}

type IngestionJobRequest struct {
	Provider string   `json:"provider"`
	Codes    []string `json:"codes"`
	From     string   `json:"from"`
	To       string   `json:"to"`
}

type ImportSourceRequest struct {
	Provider     string `json:"provider"`
	Publisher    string `json:"publisher"`
	DocumentID   string `json:"document_id"`
	URL          string `json:"url"`
	Title        string `json:"title"`
	Text         string `json:"text"`
	Rights       string `json:"rights"`
	ImportReason string `json:"import_reason"`
}

func (s *Service) CreateIngestionJob(ctx context.Context, scope Scope, req IngestionJobRequest) (map[string]any, error) {
	if len(req.Codes) == 0 || len(req.Codes) > 20 {
		return nil, newError(400, "INVALID_PARAM", "codes 必须为 1–20 个")
	}
	if req.From == "" || req.To == "" {
		return nil, newError(400, "INVALID_PARAM", "from/to 不能为空")
	}
	{
		from, err := time.Parse("2006-01-02", req.From)
		if err != nil {
			return nil, newError(400, "INVALID_PARAM", "from 日期不合法")
		}
		to, err := time.Parse("2006-01-02", req.To)
		if err != nil {
			return nil, newError(400, "INVALID_PARAM", "to 日期不合法")
		}
		if to.Before(from) || to.Sub(from) > 31*24*time.Hour {
			return nil, newError(400, "INVALID_PARAM", "from/to 范围必须不超过31天")
		}
	}
	if req.Provider != "fixture" {
		enabled := false
		for _, p := range strings.Split(os.Getenv("ZHIGU_INTEL_PROVIDERS"), ",") {
			if strings.TrimSpace(p) == req.Provider {
				enabled = true
			}
		}
		if scope.Mode != "live" || !enabled {
			return nil, newError(422, "PROVIDER_DISABLED", "provider 未启用或未授权")
		}
	}
	id := "job_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	now := s.now()
	err := s.DB.WithContext(ctx).Exec(`INSERT INTO finance_intel_jobs
(id,namespace_id,kind,provider,payload,cursor,status,owner,lease_epoch,lease_until,attempts,generation,received_count,processed_count,quarantined_count,error,created_at,updated_at)
VALUES (?,?,'ingest',?,?::jsonb,'','queued',NULL,0,NULL,0,?,0,0,0,'',?,?)`,
		id, scope.NamespaceID, req.Provider, mustJSON(map[string]any{"codes": req.Codes, "from": req.From, "to": req.To}), scope.Generation, now, now).Error
	if err != nil {
		return nil, err
	}
	return map[string]any{"job_id": id, "status": "queued", "provider": req.Provider, "received_count": 0, "processed_count": 0, "quarantined_count": 0}, nil
}

func (s *Service) IngestionJob(ctx context.Context, scope Scope, id string) (map[string]any, error) {
	var row map[string]any
	if err := s.DB.WithContext(ctx).Raw(`SELECT id,kind,provider,status,attempts,generation,received_count,processed_count,quarantined_count,cursor,error,created_at,updated_at
FROM finance_intel_jobs WHERE namespace_id=? AND id=?`, scope.NamespaceID, id).Scan(&row).Error; err != nil {
		return nil, err
	}
	if len(row) == 0 {
		return nil, newError(404, "NOT_FOUND", "任务不存在")
	}
	return row, nil
}

func (s *Service) freezeModelForJob(ctx context.Context, scope Scope) (FrozenModelConfig, error) {
	if scope.Mode == "demo" {
		return FrozenModelConfig{
			ConfigID: "fixture-manual", ConfigDigest: "fixture-manual-v1", Protocol: "fixture",
			Model: "fixture-manual-extraction-v1", PromptVersion: intelPromptVersion,
		}, nil
	}
	if s.Configs != nil {
		if frozen, err := FreezeActiveModel(ctx, s.Configs); err == nil {
			return frozen, nil
		}
	}
	if frozen := envModelConfig(); NewModelExtractorV2(frozen).Enabled {
		return frozen, nil
	}
	return FrozenModelConfig{
		ConfigID: "manual-review", ConfigDigest: "manual-review-v1", Protocol: "manual",
		Model: "manual-verified-extraction", PromptVersion: intelPromptVersion,
	}, nil
}

func (s *Service) ImportSourceRevision(ctx context.Context, scope Scope, req ImportSourceRequest) (map[string]any, error) {
	if len(req.Text) == 0 || len(req.Text) > 100*1024 {
		return nil, newError(400, "INVALID_PARAM", "text 必须为 1–100KiB")
	}
	if req.Publisher == "" || req.DocumentID == "" || req.Title == "" || req.ImportReason == "" {
		return nil, newError(400, "INVALID_PARAM", "publisher/document_id/title/import_reason 不能为空")
	}
	if req.Rights == "" {
		req.Rights = "summary"
	}
	frozen, err := s.freezeModelForJob(ctx, scope)
	if err != nil {
		return nil, err
	}
	logicalID := "imported_" + sha256Text(req.Publisher + "\x00" + req.DocumentID + "\x00" + req.Text)[:16]
	var existing map[string]any
	if err := s.DB.WithContext(ctx).Raw(`SELECT r.logical_id AS revision_id,s.id AS source_id
FROM finance_intel_source_revisions r JOIN finance_intel_sources s ON s.id=r.source_id
WHERE r.namespace_id=? AND r.logical_id=?`, scope.NamespaceID, logicalID).Scan(&existing).Error; err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		existing["existing"] = true
		return existing, nil
	}
	var out map[string]any
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sourceRow map[string]any
		if err := tx.Raw(`SELECT id FROM finance_intel_sources WHERE namespace_id=? AND publisher=? AND document_id=?`, scope.NamespaceID, req.Publisher, req.DocumentID).Scan(&sourceRow).Error; err != nil {
			return err
		}
		sourceID, _ := sourceRow["id"].(string)
		if sourceID == "" {
			sourceID = "src_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			if err := execSQL(tx, `INSERT INTO finance_intel_sources(id,namespace_id,publisher,document_id,normalized_url,rights,created_at)
VALUES (?,?,?,?,?,?,?)`, sourceID, scope.NamespaceID, req.Publisher, req.DocumentID, req.URL, req.Rights, s.now()); err != nil {
				return err
			}
		}
		var revisionNo int
		if err := tx.Raw(`SELECT COALESCE(MAX(revision_no),0)+1 FROM finance_intel_source_revisions WHERE namespace_id=? AND source_id=?`, scope.NamespaceID, sourceID).Scan(&revisionNo).Error; err != nil {
			return err
		}
		revisionID := "rev_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		now := s.now()
		if err := execSQL(tx, `INSERT INTO finance_intel_source_revisions
(id,logical_id,namespace_id,source_id,revision_no,content_hash,title,allowed_excerpt,locator,disclosed_at,disclosed_date,fetched_at,source_updated_at,access_status,is_repost,supersedes_revision_id,created_at)
VALUES (?,?,?,?,?,?,?,?,?::jsonb,NULL,'',?,NULL,'available',false,NULL,?)`,
			revisionID, logicalID, scope.NamespaceID, sourceID, revisionNo, sha256Text(req.Text), req.Title, req.Text, mustJSON(map[string]any{"import_reason": req.ImportReason}), now, now); err != nil {
			return err
		}
		jobID := "job_import_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		jobPayload := map[string]any{
			"source_revision_id": logicalID,
			"model_config": map[string]any{
				"config_id": frozen.ConfigID, "config_digest": frozen.ConfigDigest,
				"protocol": frozen.Protocol, "model": frozen.Model, "prompt_version": frozen.PromptVersion,
			},
		}
		if err := execSQL(tx, `INSERT INTO finance_intel_jobs
(id,namespace_id,kind,provider,payload,cursor,status,owner,lease_epoch,lease_until,attempts,generation,received_count,processed_count,quarantined_count,error,created_at,updated_at)
VALUES (?,?,'extract','manual',?::jsonb,'','queued',NULL,0,NULL,0,?,1,0,0,'',?,?)`,
			jobID, scope.NamespaceID, mustJSON(jobPayload), scope.Generation, now, now); err != nil {
			return err
		}
		out = map[string]any{"source_id": sourceID, "revision_id": logicalID, "job_id": jobID}
		return nil
	})
	return out, err
}

type IdempotentResult struct {
	Data     any  `json:"data"`
	Replayed bool `json:"replayed"`
}

func (s *Service) Idempotent(ctx context.Context, scope Scope, key, method, path, bodyHash string, fn func(tx *gorm.DB) (any, error)) (IdempotentResult, error) {
	if len(key) < 8 || len(key) > 128 {
		return IdempotentResult{}, newError(400, "INVALID_PARAM", "Idempotency-Key 长度必须为 8–128")
	}
	scopeKey := method + " " + path + " " + key
	now := s.now()
	res := s.DB.WithContext(ctx).Exec(`INSERT INTO finance_intel_idempotency
(id,namespace_id,principal_key,scope_key,body_hash,status,response,created_at,expires_at)
VALUES (?,?,?,?,?,0,'{}'::jsonb,?,?)
ON CONFLICT(namespace_id,principal_key,scope_key) DO NOTHING`,
		"idem_"+strings.ReplaceAll(uuid.NewString(), "-", ""), scope.NamespaceID, scope.PrincipalKey,
		scopeKey, bodyHash, now, now.Add(24*time.Hour))
	if res.Error != nil {
		return IdempotentResult{}, res.Error
	}
	createdByUs := res.RowsAffected > 0
	var row map[string]any
	if err := s.DB.WithContext(ctx).Raw(`SELECT id,body_hash,status,response::text AS response_text,created_at FROM finance_intel_idempotency
WHERE namespace_id=? AND principal_key=? AND scope_key=?`, scope.NamespaceID, scope.PrincipalKey, scopeKey).Scan(&row).Error; err != nil {
		return IdempotentResult{}, err
	}
	if len(row) == 0 {
		return IdempotentResult{}, newError(500, "INTERNAL", "幂等记录创建失败")
	}
	existingHash, _ := row["body_hash"].(string)
	if existingHash != bodyHash {
		return IdempotentResult{}, newError(409, "IDEMPOTENCY_CONFLICT", "相同幂等键对应不同请求")
	}
	status := 0
	switch v := row["status"].(type) {
	case int64:
		status = int(v)
	case int32:
		status = int(v)
	case int:
		status = v
	}
	if status != 0 {
		raw, _ := row["response_text"].(string)
		var data any
		if raw != "" {
			_ = json.Unmarshal([]byte(raw), &data)
		}
		return IdempotentResult{Data: data, Replayed: true}, nil
	}
	if created, ok := row["created_at"].(time.Time); ok && !createdByUs && now.Sub(created) < 2*time.Minute {
		return IdempotentResult{}, newError(409, "IN_PROGRESS", "相同幂等键请求仍在处理")
	}
	data, err := fn(s.DB.WithContext(ctx))
	if err != nil {
		_ = s.DB.WithContext(ctx).Exec(`DELETE FROM finance_intel_idempotency WHERE id=?`, row["id"]).Error
		return IdempotentResult{}, err
	}
	encoded := mustJSON(data)
	if err := s.DB.WithContext(ctx).Exec(`UPDATE finance_intel_idempotency SET status=?, response=?::jsonb WHERE id=?`, http.StatusAccepted, encoded, row["id"]).Error; err != nil {
		return IdempotentResult{}, err
	}
	return IdempotentResult{Data: data, Replayed: false}, nil
}

type ReviewResolveRequest struct {
	Action          string         `json:"action"`
	EventID         string         `json:"event_id,omitempty"`
	Evidence        map[string]any `json:"evidence,omitempty"`
	ExpectedVersion int            `json:"expected_version"`
	Reason          string         `json:"reason"`
}

type ReassignRequest struct {
	EvidenceIDs           []string `json:"evidence_ids"`
	TargetEventID         string   `json:"target_event_id"`
	Reason                string   `json:"reason"`
	ExpectedVersion       int      `json:"expected_version"`
	TargetExpectedVersion int      `json:"target_expected_version"`
}

func (s *Service) ListReviewItems(ctx context.Context, scope Scope, kind string, limit int) (map[string]any, error) {
	sql := `SELECT id,revision,kind,status,payload::text AS payload,reason,created_at,resolved_at
FROM finance_intel_review_items WHERE namespace_id=?`
	args := []any{scope.NamespaceID}
	if kind != "" {
		sql += ` AND kind=?`
		args = append(args, kind)
	}
	sql += ` ORDER BY created_at DESC,id DESC LIMIT ?`
	args = append(args, normalizeLimit(limit))
	rows := make([]map[string]any, 0)
	if err := s.DB.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]any, len(rows))
	for i := range rows {
		rows[i]["payload"] = jsonValue(rows[i]["payload"])
		items[i] = rows[i]
	}
	return map[string]any{"items": items, "next_cursor": ""}, nil
}

func (s *Service) ResolveReviewItem(ctx context.Context, scope Scope, id string, req ReviewResolveRequest) (map[string]any, error) {
	if len(req.Reason) < 1 || len(req.Reason) > 500 {
		return nil, newError(400, "INVALID_PARAM", "reason 必须为 1–500 字")
	}
	switch req.Action {
	case "assign", "reject", "replace_extraction":
	default:
		return nil, newError(400, "INVALID_PARAM", "action 不合法")
	}
	if req.Action == "assign" && req.EventID == "" {
		return nil, newError(400, "INVALID_PARAM", "assign 需要 event_id")
	}
	if req.Action == "replace_extraction" && len(req.Evidence) == 0 {
		return nil, newError(400, "INVALID_PARAM", "replace_extraction 需要完整 evidence")
	}
	var out map[string]any
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var item map[string]any
		if err := tx.Raw(`SELECT * FROM finance_intel_review_items WHERE namespace_id=? AND id=? FOR UPDATE`, scope.NamespaceID, id).Scan(&item).Error; err != nil {
			return err
		}
		if len(item) == 0 {
			return newError(404, "NOT_FOUND", "待处理项不存在")
		}
		revision := toInt(item["revision"])
		if revision != req.ExpectedVersion {
			return newError(409, "VERSION_CONFLICT", "待处理项版本已变化")
		}
		if current, _ := item["status"].(string); current != "pending" {
			return newError(409, "VERSION_CONFLICT", "待处理项已处理")
		}
		status := "resolved"
		if req.Action == "reject" {
			status = "rejected"
		}
		payload := map[string]any{"action": req.Action, "event_id": req.EventID, "evidence": req.Evidence, "reason": req.Reason}
		if err := execSQL(tx, `UPDATE finance_intel_review_items SET status=?,revision=revision+1,payload=?::jsonb,reason=?,resolved_at=? WHERE namespace_id=? AND id=?`,
			status, mustJSON(payload), req.Reason, s.now(), scope.NamespaceID, id); err != nil {
			return err
		}
		if req.Action == "assign" {
			var event map[string]any
			if err := tx.Raw(`SELECT id FROM finance_intel_events WHERE namespace_id=? AND logical_id=?`, scope.NamespaceID, req.EventID).Scan(&event).Error; err != nil {
				return err
			}
			if len(event) == 0 {
				return newError(404, "NOT_FOUND", "目标事件不存在")
			}
			if err := execSQL(tx, `UPDATE finance_intel_events SET review_status='normal',updated_at=? WHERE namespace_id=? AND id=?`, s.now(), scope.NamespaceID, event["id"]); err != nil {
				return err
			}
		}
		if err := execSQL(tx, `INSERT INTO finance_intel_audit(id,namespace_id,actor_key,action,resource_id,before_ref,after_ref,reason,request_id,created_at)
VALUES (?,?,?,?,?,?,?,?,?,?)`, "audit_"+strings.ReplaceAll(uuid.NewString(), "-", ""), scope.NamespaceID, scope.PrincipalKey, "review."+req.Action, id, "", status, req.Reason, httpx.NewID("req"), s.now()); err != nil {
			return err
		}
		out = map[string]any{"review_item_id": id, "status": status, "version": revision + 1, "change_ids": []any{}}
		return nil
	})
	return out, err
}

func (s *Service) ReassignEvidence(ctx context.Context, scope Scope, eventID string, req ReassignRequest) (map[string]any, error) {
	if len(req.EvidenceIDs) == 0 || len(req.EvidenceIDs) > 200 {
		return nil, newError(400, "INVALID_PARAM", "evidence_ids 必须为 1–200 个")
	}
	if len(req.Reason) < 1 || len(req.Reason) > 500 {
		return nil, newError(400, "INVALID_PARAM", "reason 必须为 1–500 字")
	}
	var out map[string]any
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var source map[string]any
		if err := tx.Raw(`SELECT * FROM finance_intel_events WHERE namespace_id=? AND logical_id=? FOR UPDATE`, scope.NamespaceID, eventID).Scan(&source).Error; err != nil {
			return err
		}
		if len(source) == 0 {
			return newError(404, "NOT_FOUND", "源事件不存在")
		}
		if toInt(source["current_version"]) != req.ExpectedVersion {
			return newError(409, "VERSION_CONFLICT", "源事件版本已变化")
		}
		targetInternal := ""
		targetLogical := req.TargetEventID
		if req.TargetEventID != "" {
			var target map[string]any
			if err := tx.Raw(`SELECT * FROM finance_intel_events WHERE namespace_id=? AND logical_id=? FOR UPDATE`, scope.NamespaceID, req.TargetEventID).Scan(&target).Error; err != nil {
				return err
			}
			if len(target) == 0 {
				return newError(404, "NOT_FOUND", "目标事件不存在")
			}
			if toInt(target["current_version"]) != req.TargetExpectedVersion {
				return newError(409, "VERSION_CONFLICT", "目标事件版本已变化")
			}
			targetInternal, _ = target["id"].(string)
		} else {
			targetInternal = "evt_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			targetLogical = targetInternal
			if err := execSQL(tx, `INSERT INTO finance_intel_events
(id,logical_id,namespace_id,event_type,subject_code,subject_name,matter_key,title,core_claim_key,core_claim_text,current_version,current_snapshot_id,latest_valid_disclosed_at,review_status,redirect_event_ids,created_at,updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,0,NULL,NULL,'normal','[]'::jsonb,?,?)`,
				targetInternal, targetLogical, scope.NamespaceID, source["event_type"], source["subject_code"], source["subject_name"],
				source["matter_key"].(string)+"-reassigned-"+uuid.NewString(), source["title"], source["core_claim_key"], source["core_claim_text"], s.now(), s.now()); err != nil {
				return err
			}
		}
		for _, evidenceID := range req.EvidenceIDs {
			if err := execSQL(tx, `UPDATE finance_intel_evidence SET event_id=? WHERE namespace_id=? AND logical_id=? AND event_id=?`,
				targetInternal, scope.NamespaceID, evidenceID, source["id"]); err != nil {
				return err
			}
		}
		if err := execSQL(tx, `INSERT INTO finance_intel_audit(id,namespace_id,actor_key,action,resource_id,before_ref,after_ref,reason,request_id,created_at)
VALUES (?,?,?,?,?,?,?,?,?,?)`, "audit_"+strings.ReplaceAll(uuid.NewString(), "-", ""), scope.NamespaceID, scope.PrincipalKey, "event.reassign", eventID, eventID, targetLogical, req.Reason, httpx.NewID("req"), s.now()); err != nil {
			return err
		}
		out = map[string]any{"source_event_id": eventID, "target_event_id": targetLogical, "change_ids": []any{}}
		return nil
	})
	return out, err
}

func toInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int32:
		return int(x)
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}
