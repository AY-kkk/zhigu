package finance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	modelfinance "zhigu/server/model/finance"
)

type ConfigService struct {
	DB *gorm.DB
}

func NewConfigService(db *gorm.DB) *ConfigService { return &ConfigService{DB: db} }

type ModelConfigPublic struct {
	ID           string         `json:"id"`
	Kind         string         `json:"kind"`
	ConfigDigest string         `json:"config_digest"`
	PublicConfig map[string]any `json:"public_config"`
	HasKey       bool           `json:"has_key"`
	TestStatus   string         `json:"test_status"`
	TestDigest   *string        `json:"test_digest"`
	ActiveAt     *time.Time     `json:"active_at"`
}

func digestOf(public any, keyPresent bool) string {
	raw, _ := json.Marshal(map[string]any{"public": public, "has_key": keyPresent})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *ConfigService) SaveModel(ctx context.Context, public map[string]any, apiKey string) (ModelConfigPublic, error) {
	if rawURL, _ := public["base_url"].(string); rawURL != "" {
		if err := ValidateUpstreamURL(rawURL); err != nil {
			return ModelConfigPublic{}, err
		}
	}
	now := time.Now().UTC()
	ct, err := EncryptSecret(apiKey)
	if err != nil {
		return ModelConfigPublic{}, err
	}
	ver := KeyVersion()
	pubJSON, _ := json.Marshal(public)
	row := modelfinance.ConfigVersion{
		ID:               "cfg_" + uuid.NewString(),
		Kind:             "model",
		ConfigDigest:     digestOf(public, apiKey != ""),
		PublicConfig:     datatypes.JSON(pubJSON),
		SecretCiphertext: ct,
		SecretKeyVersion: &ver,
		TestStatus:       "untested",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.DB.WithContext(ctx).Create(&row).Error; err != nil {
		return ModelConfigPublic{}, err
	}
	return toPublic(row), nil
}

func (s *ConfigService) Get(ctx context.Context, id string) (ModelConfigPublic, error) {
	var row modelfinance.ConfigVersion
	if err := s.DB.WithContext(ctx).Where("id = ?", id).Take(&row).Error; err != nil {
		return ModelConfigPublic{}, NewError(404, "not_found", "CONFIG_NOT_FOUND", "配置不存在")
	}
	return toPublic(row), nil
}

func (s *ConfigService) MarkTested(ctx context.Context, id, digest string) error {
	now := time.Now().UTC()
	return s.DB.WithContext(ctx).Model(&modelfinance.ConfigVersion{}).Where("id = ?", id).Updates(map[string]any{
		"test_status": "passed", "test_digest": digest, "updated_at": now,
	}).Error
}

func (s *ConfigService) Activate(ctx context.Context, id string) error {
	var row modelfinance.ConfigVersion
	if err := s.DB.WithContext(ctx).Where("id = ?", id).Take(&row).Error; err != nil {
		return NewError(404, "not_found", "CONFIG_NOT_FOUND", "配置不存在")
	}
	if row.TestStatus != "passed" || row.TestDigest == nil || *row.TestDigest != row.ConfigDigest {
		return NewError(409, "conflict", "STALE_TEST", "须使用相同 digest 的通过测试才能启用")
	}
	now := time.Now().UTC()
	if err := s.DB.WithContext(ctx).Model(&modelfinance.ConfigVersion{}).Where("kind = ? AND active_at IS NOT NULL", row.Kind).
		Update("active_at", nil).Error; err != nil {
		return err
	}
	return s.DB.Model(&row).Updates(map[string]any{"active_at": now, "updated_at": now}).Error
}

func (s *ConfigService) Active(ctx context.Context, kind string) (modelfinance.ConfigVersion, error) {
	var row modelfinance.ConfigVersion
	err := s.DB.WithContext(ctx).Where("kind = ? AND active_at IS NOT NULL", kind).Order("active_at desc").Take(&row).Error
	return row, err
}

func (s *ConfigService) List(ctx context.Context, kind string) ([]ModelConfigPublic, error) {
	q := s.DB.WithContext(ctx).Order("created_at desc")
	if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	var rows []modelfinance.ConfigVersion
	if err := q.Limit(50).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ModelConfigPublic, 0, len(rows))
	for _, row := range rows {
		out = append(out, toPublic(row))
	}
	return out, nil
}

func (s *ConfigService) SaveDataSource(ctx context.Context, public map[string]any, apiKey string) (ModelConfigPublic, error) {
	publicCopy := map[string]any{}
	for k, v := range public {
		publicCopy[k] = v
	}
	if publicCopy["protocol"] == nil {
		publicCopy["protocol"] = "fixture"
	}
	now := time.Now().UTC()
	ct, err := EncryptSecret(apiKey)
	if err != nil {
		return ModelConfigPublic{}, err
	}
	ver := KeyVersion()
	pubJSON, _ := json.Marshal(publicCopy)
	row := modelfinance.ConfigVersion{
		ID: "cfg_" + uuid.NewString(), Kind: "source", ConfigDigest: digestOf(publicCopy, apiKey != ""),
		PublicConfig: datatypes.JSON(pubJSON), SecretCiphertext: ct, SecretKeyVersion: &ver,
		TestStatus: "untested", CreatedAt: now, UpdatedAt: now,
	}
	if err := s.DB.WithContext(ctx).Create(&row).Error; err != nil {
		return ModelConfigPublic{}, err
	}
	return toPublic(row), nil
}

func (s *ConfigService) StartTest(ctx context.Context, id string) (map[string]any, error) {
	row, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	checks := []map[string]any{}
	pass := true
	proto, _ := row.PublicConfig["protocol"].(string)
	if row.Kind == "model" {
		if _, err := NormalizeProtocol(proto); err != nil {
			pass = false
			checks = append(checks, map[string]any{"name": "protocol", "ok": false})
		} else {
			checks = append(checks, map[string]any{"name": "protocol", "ok": true})
		}
	} else {
		checks = append(checks, map[string]any{"name": "protocol", "ok": true})
	}
	if !row.HasKey {
		pass = false
		checks = append(checks, map[string]any{"name": "has_key", "ok": false})
	} else {
		checks = append(checks, map[string]any{"name": "has_key", "ok": true})
	}
	if rawURL, _ := row.PublicConfig["base_url"].(string); rawURL != "" {
		if err := ValidateUpstreamURL(rawURL); err != nil {
			pass = false
			checks = append(checks, map[string]any{"name": "ssrf", "ok": false, "error": ErrorCode(err)})
		} else {
			checks = append(checks, map[string]any{"name": "ssrf", "ok": true})
		}
	}
	checks = append(checks, map[string]any{"name": "structured_parse_fixture", "ok": false, "skipped": true, "note": "未执行真实 Go→Python 调用"})
	checks = append(checks, map[string]any{"name": "tool_call_schema", "ok": false, "skipped": true, "note": "未执行真实工具调用"})
	checks = append(checks, map[string]any{"name": "upstream_connection", "ok": false, "skipped": true, "note": "阶段 A 仅格式校验，未验证上游连接"})
	testID := "test_" + id
	status := "format_checked"
	if !pass {
		status = "failed"
	}
	return map[string]any{
		"test_id": testID, "status": status, "checks": checks, "config_id": id, "config_digest": row.ConfigDigest,
		"connection_verified": false, "note": "仅保存/格式校验，未验证连接。真实模型能力测试属于阶段 B。",
	}, nil
}

func (s *ConfigService) GetTest(ctx context.Context, testID string) (map[string]any, error) {
	id := strings.TrimPrefix(testID, "test_")
	row, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"test_id": testID, "status": row.TestStatus, "config_digest": row.ConfigDigest, "config_id": id}, nil
}

func DefaultPolicy() PolicyView {
	return PolicyView{
		MaxModelCalls: 14, MaxToolCalls: 12, RunTimeoutSeconds: 180, QueueTimeoutSeconds: 30,
		Note: "UI 只允许降低硬上限",
	}
}

func (s *ConfigService) GetPolicy(ctx context.Context) (PolicyView, error) {
	row, err := s.Active(ctx, "policy")
	if err != nil {
		return DefaultPolicy(), nil
	}
	var p PolicyView
	_ = json.Unmarshal(row.PublicConfig, &p)
	if p.MaxModelCalls == 0 {
		p = DefaultPolicy()
	}
	p.Note = "UI 只允许降低硬上限"
	return p, nil
}

func (s *ConfigService) PatchPolicy(ctx context.Context, in PolicyView) (PolicyView, error) {
	cur, _ := s.GetPolicy(ctx)
	if in.MaxModelCalls == 0 {
		in.MaxModelCalls = cur.MaxModelCalls
	}
	if in.MaxToolCalls == 0 {
		in.MaxToolCalls = cur.MaxToolCalls
	}
	if in.MaxModelCalls > cur.MaxModelCalls || in.MaxToolCalls > cur.MaxToolCalls {
		return PolicyView{}, NewError(400, "validation", "POLICY_RAISE_FORBIDDEN", "提升上限需要代码与回归测试")
	}
	if in.RunTimeoutSeconds < 0 || in.QueueTimeoutSeconds < 0 {
		return PolicyView{}, NewError(400, "validation", "POLICY_INVALID", "超时必须为正数")
	}
	if in.RunTimeoutSeconds == 0 {
		in.RunTimeoutSeconds = cur.RunTimeoutSeconds
	}
	if in.QueueTimeoutSeconds == 0 {
		in.QueueTimeoutSeconds = cur.QueueTimeoutSeconds
	}
	if in.RunTimeoutSeconds > 180 || in.QueueTimeoutSeconds > 30 {
		return PolicyView{}, NewError(400, "validation", "POLICY_RAISE_FORBIDDEN", "提升超时上限需要代码与回归测试")
	}
	if in.RunTimeoutSeconds == 0 || in.QueueTimeoutSeconds == 0 {
		return PolicyView{}, NewError(400, "validation", "POLICY_INVALID", "超时必须为正数")
	}
	in.Note = "UI 只允许降低硬上限"
	now := time.Now().UTC()
	raw, _ := json.Marshal(in)
	row := modelfinance.ConfigVersion{
		ID: "cfg_" + uuid.NewString(), Kind: "policy", ConfigDigest: digestOf(in, false),
		PublicConfig: datatypes.JSON(raw), TestStatus: "passed", CreatedAt: now, UpdatedAt: now, ActiveAt: &now,
	}
	td := row.ConfigDigest
	row.TestDigest = &td
	if err := s.DB.WithContext(ctx).Model(&modelfinance.ConfigVersion{}).Where("kind = ? AND active_at IS NOT NULL", "policy").
		Update("active_at", nil).Error; err != nil {
		return PolicyView{}, err
	}
	if err := s.DB.Create(&row).Error; err != nil {
		return PolicyView{}, err
	}
	return in, nil
}

func toPublic(row modelfinance.ConfigVersion) ModelConfigPublic {
	var pub map[string]any
	_ = json.Unmarshal(row.PublicConfig, &pub)
	return ModelConfigPublic{
		ID: row.ID, Kind: row.Kind, ConfigDigest: row.ConfigDigest, PublicConfig: pub,
		HasKey: len(row.SecretCiphertext) > 0, TestStatus: row.TestStatus, TestDigest: row.TestDigest, ActiveAt: row.ActiveAt,
	}
}
