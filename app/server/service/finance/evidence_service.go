package finance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	modelfinance "zhigu/server/model/finance"
)

type EvidenceService struct {
	DB *gorm.DB
}

func NewEvidenceService(db *gorm.DB) *EvidenceService { return &EvidenceService{DB: db} }

type Metric struct {
	Metric      string `json:"metric"`
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
	Value       string `json:"value"`
	Unit        string `json:"unit"`
	ValueType   string `json:"value_type"`
}

type EvidenceIn struct {
	SourceID    string    `json:"source_id"`
	Title       string    `json:"title"`
	SourceURL   string    `json:"source_url"`
	SourceKind  string    `json:"source_kind"`
	Locator     string    `json:"locator"`
	Text        string    `json:"text"`
	Metrics     []Metric  `json:"metrics"`
	PublishedAt time.Time `json:"published_at"`
	AvailableAt time.Time `json:"available_at"`
	RetrievedAt time.Time `json:"retrieved_at"`
	ContentHash string    `json:"content_hash"`
	DataVersion string    `json:"data_version"`
	Mode        string    `json:"mode"`
}

func ContentHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func CanonicalRecordHash(rec EvidenceIn) (string, error) {
	metrics := make([]map[string]string, 0, len(rec.Metrics))
	for _, m := range rec.Metrics {
		metrics = append(metrics, map[string]string{
			"metric": m.Metric, "period_start": m.PeriodStart, "period_end": m.PeriodEnd,
			"value": m.Value, "unit": m.Unit, "value_type": m.ValueType,
		})
	}
	body := map[string]any{
		"source_id": rec.SourceID, "source_url": rec.SourceURL, "source_kind": rec.SourceKind,
		"locator": rec.Locator, "text": rec.Text, "metrics": metrics,
		"published_at": rec.PublishedAt.UTC().Format(time.RFC3339),
		"available_at": rec.AvailableAt.UTC().Format(time.RFC3339),
		"retrieved_at": rec.RetrievedAt.UTC().Format(time.RFC3339),
		"data_version": rec.DataVersion, "mode": rec.Mode, "title": rec.Title,
	}
	return HashCanonical(body)
}

func CanonicalRecordHashMap(payload map[string]any) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	var rec EvidenceIn
	if err := json.Unmarshal(raw, &rec); err != nil {
		return "", err
	}
	return CanonicalRecordHash(rec)
}

func (s *EvidenceService) Register(ctx context.Context, grantID string, recordIDs []string) ([]string, error) {
	if len(recordIDs) == 0 {
		return nil, NewError(400, "validation", "EVIDENCE_REJECTED", "须提供 record_ids")
	}
	var ids []string
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var grant modelfinance.ToolGrant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", grantID).Take(&grant).Error; err != nil {
			return NewError(404, "not_found", "GRANT_NOT_FOUND", "授权不存在")
		}
		if grant.Status != "granted" && grant.Status != "succeeded" {
			return NewError(409, "conflict", "GRANT_REVOKED", "授权已失效")
		}
		var run modelfinance.ResearchRun
		if err := tx.Where("id = ?", grant.RunID).Take(&run).Error; err != nil {
			return err
		}
		if runIsCanceled(run) {
			return NewError(409, "conflict", "RUN_CLOSED", "拒绝登记")
		}
		if !runAllowsToolsAt(run, time.Now().UTC()) {
			return NewError(409, "conflict", "RUN_CLOSED", "研究已结束，拒绝登记")
		}
		for _, rid := range recordIDs {
			var stored modelfinance.ProviderRecordRow
			if err := tx.Where("id = ? AND grant_id = ?", rid, grantID).Take(&stored).Error; err != nil {
				return NewError(400, "validation", "UNTRUSTED_RECORD", "证据必须是本 grant 下已保存的 record_id")
			}
			var rec ProviderRecord
			if err := json.Unmarshal(stored.Payload, &rec); err != nil {
				return NewError(400, "validation", "UNTRUSTED_RECORD", "已保存记录无法解码")
			}
			trusted := rec.ToEvidenceIn()
			if err := s.validateRecord(run, trusted); err != nil {
				return err
			}
			textHash := ContentHash(trusted.Text)
			now := time.Now().UTC()
			metrics, _ := json.Marshal(trusted.Metrics)
			ev := modelfinance.Evidence{
				ID: "ev_" + uuid.NewString(), RunID: run.ID, InstrumentID: run.InstrumentID,
				SourceID: trusted.SourceID, SourceURL: trusted.SourceURL, SourceKind: trusted.SourceKind,
				Title: trusted.Title, Locator: trusted.Locator, Text: trusted.Text, Metrics: datatypes.JSON(metrics),
				PublishedAt: trusted.PublishedAt, AvailableAt: trusted.AvailableAt, RetrievedAt: trusted.RetrievedAt,
				ContentHash: textHash, DataVersion: trusted.DataVersion, Mode: run.Mode,
				CreatedAt: now, UpdatedAt: now,
			}
			if ev.SourceKind == "" {
				ev.SourceKind = "fixture"
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "run_id"}, {Name: "source_id"}, {Name: "content_hash"}, {Name: "locator"}},
				DoNothing: true,
			}).Create(&ev).Error; err != nil {
				return err
			}
			var storedEv modelfinance.Evidence
			if err := tx.Where("run_id = ? AND source_id = ? AND content_hash = ? AND locator = ?", run.ID, trusted.SourceID, textHash, trusted.Locator).Take(&storedEv).Error; err != nil {
				return err
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&modelfinance.TaskEvidence{TaskID: grant.TaskID, EvidenceID: storedEv.ID}).Error; err != nil {
				return err
			}
			ids = append(ids, storedEv.ID)
		}
		return nil
	})
	return ids, err
}

func (s *EvidenceService) validateRecord(run modelfinance.ResearchRun, rec EvidenceIn) error {
	if rec.AvailableAt.IsZero() || rec.PublishedAt.IsZero() {
		return NewError(400, "validation", "TIME_GATE", "披露或可用时间未知")
	}
	if rec.AvailableAt.After(run.AsOf) || rec.PublishedAt.After(run.AsOf) {
		return NewError(400, "validation", "FUTURE_EVIDENCE", "拒绝未来证据")
	}
	if rec.AvailableAt.Before(rec.PublishedAt) {
		return NewError(400, "validation", "INVALID_CHRONOLOGY", "取得时间早于发布")
	}
	if rec.Mode != "" && rec.Mode != run.Mode {
		return NewError(400, "validation", "MODE_MISMATCH", "mode 必须与 run 一致")
	}
	for _, m := range rec.Metrics {
		if _, err := decimal.NewFromString(m.Value); err != nil {
			return NewError(400, "validation", "INVALID_DECIMAL", "指标必须为十进制字符串")
		}
	}
	if strings.Contains(strings.ToLower(rec.Text), "ignore previous") || strings.Contains(rec.Text, "忽略规则") {
		// still stored as data, never executed; no extra side effect
	}
	return nil
}

func (s *EvidenceService) ValidateRole(ctx context.Context, run RunSnapshot, result ResearchResult) (ValidatedRole, error) {
	expectedTask := run.TaskID
	if expectedTask != "" {
		if err := validateExecutorResult(run.ID, expectedTask, &result); err != nil {
			return ValidatedRole{}, err
		}
	}
	hasContent := len(result.Arguments)+len(result.EvidenceIDs)+len(result.Counterevidence) > 0
	if hasContent {
		if result.RunID != "" && result.RunID != run.ID {
			return ValidatedRole{}, NewError(409, "conflict", "RESULT_RUN_MISMATCH", "结果不属于当前研究")
		}
		if expectedTask != "" && result.TaskID != "" && result.TaskID != expectedTask {
			return ValidatedRole{}, NewError(409, "conflict", "RESULT_TASK_MISMATCH", "结果不属于当前任务")
		}
	}
	lookupTask := expectedTask
	if lookupTask == "" {
		lookupTask = result.TaskID
	}
	allowed := map[string]struct{}{}
	if s.DB != nil && lookupTask != "" {
		var links []modelfinance.TaskEvidence
		s.DB.WithContext(ctx).Where("task_id = ?", lookupTask).Find(&links)
		for _, l := range links {
			allowed[l.EvidenceID] = struct{}{}
		}
	}
	for _, id := range result.EvidenceIDs {
		if _, ok := allowed[id]; !ok {
			return ValidatedRole{}, NewError(400, "validation", "UNREGISTERED_CITATION", "未知引用")
		}
	}
	for _, arg := range append(append([]Argument{}, result.Arguments...), result.Counterevidence...) {
		for _, id := range arg.EvidenceIDs {
			if _, ok := allowed[id]; !ok {
				return ValidatedRole{}, NewError(400, "validation", "UNREGISTERED_CITATION", "未知引用")
			}
		}
		if (arg.ClaimType == "fact" || arg.ClaimType == "inference") && len(arg.EvidenceIDs) == 0 {
			return ValidatedRole{}, NewError(400, "validation", "MISSING_CITATION", "事实/推断须引用证据")
		}
	}
	return NewValidatedRole("", result), nil
}

func Calculate(operation string, left, right decimal.Decimal) (decimal.Decimal, string, error) {
	switch operation {
	case "growth_rate":
		if !right.IsPositive() {
			return decimal.Zero, "", NewError(422, "validation", "INSUFFICIENT_DENOMINATOR", "零分母或非正基数")
		}
		v := left.Sub(right).Div(right).Mul(decimal.NewFromInt(100)).Round(2)
		return v, "(current-previous)/previous×100", nil
	case "ratio":
		if right.IsZero() {
			return decimal.Zero, "", NewError(422, "validation", "INSUFFICIENT_DENOMINATOR", "零分母")
		}
		return left.Div(right), "a/b", nil
	case "difference":
		return left.Sub(right), "a-b", nil
	default:
		return decimal.Zero, "", NewError(400, "validation", "INVALID_OPERATION", "operation 不支持")
	}
}

func FixtureFinancials() map[string]any {
	text := "Fixture only: revenue 2025 = 100 CNY_million; 2024 = 80 CNY_million."
	return map[string]any{
		"source_id":    "source_fixture",
		"title":        "虚构测试财务数据，不用于投资",
		"source_url":   "https://example.invalid/fixture/annual-report",
		"source_kind":  "fixture",
		"locator":      "fixture:financials:revenue",
		"text":         text,
		"content_hash": ContentHash(text),
		"data_version": "fixture_annual_v1",
		"mode":         ModeFixture,
		"published_at": "2026-03-31T00:00:00Z",
		"available_at": "2026-03-31T00:00:00Z",
		"retrieved_at": "2026-09-17T00:00:00Z",
		"metrics": []Metric{
			{Metric: "revenue", PeriodStart: "2025-01-01", PeriodEnd: "2025-12-31", Value: "100", Unit: "CNY_million", ValueType: "actual"},
			{Metric: "revenue", PeriodStart: "2024-01-01", PeriodEnd: "2024-12-31", Value: "80", Unit: "CNY_million", ValueType: "actual"},
		},
	}
}

func FixtureFilings() map[string]any {
	text := "Fixture only: no authorized counter-filing body is available for DEMO:COMPANY."
	return map[string]any{
		"source_id":    "source_fixture_filing",
		"title":        "虚构测试公告摘要，不是真实披露",
		"source_url":   "https://example.invalid/fixture/filing",
		"source_kind":  "fixture",
		"locator":      "fixture:filings:none",
		"text":         text,
		"content_hash": ContentHash(text),
		"data_version": "fixture_filing_v1",
		"mode":         ModeFixture,
		"published_at": "2026-03-31T00:00:00Z",
		"available_at": "2026-03-31T00:00:00Z",
		"retrieved_at": "2026-09-17T00:00:00Z",
		"metrics":      []Metric{},
	}
}

func HasPromptInjection(s string) bool {
	low := strings.ToLower(s)
	return strings.Contains(low, "ignore previous") || strings.Contains(s, "上传密钥") || strings.Contains(s, "忽略截止")
}

func StripControl(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
}
