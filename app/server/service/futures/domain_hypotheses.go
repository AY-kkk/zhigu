package futures

import (
	"context"
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

type HypothesisCreateInput struct {
	RunID       string           `json:"run_id"`
	ReportID    string           `json:"report_id"`
	ClaimIDs    []string         `json:"claim_ids"`
	Proposition string           `json:"proposition"`
	ExpiresAt   time.Time        `json:"expires_at"`
	Conditions  []map[string]any `json:"conditions"`
	Activate    bool             `json:"activate"`
}

type HypothesisPatchInput struct {
	ExpectedVersion      int              `json:"expected_version"`
	Action               string           `json:"action"`
	Proposition          *string          `json:"proposition,omitempty"`
	ExpiresAt            *time.Time       `json:"expires_at,omitempty"`
	Conditions           []map[string]any `json:"conditions,omitempty"`
	UserView             *string          `json:"user_view,omitempty"`
	Reason               *string          `json:"reason,omitempty"`
	ReviewedCheckVersion *int             `json:"reviewed_check_version,omitempty"`
}

type CheckCreateInput struct {
	ExpectedVersion int      `json:"expected_version"`
	ConditionID     string   `json:"condition_id"`
	Result          string   `json:"result"`
	Reason          string   `json:"reason"`
	EvidenceIDs     []string `json:"evidence_ids"`
}

type RecapInput struct {
	HypothesisID        string `json:"hypothesis_id"`
	Version             int    `json:"version"`
	Facts               string `json:"facts"`
	Transmission        string `json:"transmission"`
	ContractPerformance string `json:"contract_performance"`
	DataSufficiency     string `json:"data_sufficiency"`
	Reason              string `json:"reason"`
}

type HypothesisVersion struct {
	HypothesisID string         `gorm:"column:hypothesis_id;primaryKey"`
	OwnerID      uint           `gorm:"column:owner_id"`
	Mode         string         `gorm:"column:mode"`
	Version      int64          `gorm:"column:version;primaryKey"`
	Body         datatypes.JSON `gorm:"column:body"`
	CreatedAt    time.Time      `gorm:"column:created_at"`
}

func (HypothesisVersion) TableName() string { return "futures_hypothesis_versions" }

func (d *Domain) CreateHypothesis(ctx context.Context, id Identity, idempotencyKey string, input HypothesisCreateInput) (map[string]any, error) {
	if input.RunID == "" || input.ReportID == "" || stringsTrim(input.Proposition) == "" || len(input.ClaimIDs) == 0 || len(input.ClaimIDs) > 12 {
		return nil, ErrInvalidInput
	}
	if err := validateHypothesisConditions(input.Conditions); err != nil {
		return nil, err
	}
	now := d.Now().UTC()
	value := model.Hypothesis{ID: "hypothesis_" + uuid.NewString(), OwnerID: id.OwnerID, Mode: id.Mode, Version: 1,
		RunID: input.RunID, ReportID: input.ReportID, Proposition: input.Proposition, UserView: "undetermined",
		Lifecycle: "draft", CreatedAt: now, UpdatedAt: now}
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run model.Run
		if err := tx.Where(`id=? AND owner_id=? AND mode=? AND status='succeeded' AND deleted_at IS NULL`, input.RunID, id.OwnerID, id.Mode).First(&run).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		report, ok := decodeObject(run.Report)
		if !ok || fmt.Sprint(report["id"]) != input.ReportID {
			return ErrInvalidInput
		}
		if !claimIDsAllowed(report, input.ClaimIDs) {
			return ErrInvalidInput
		}
		horizon, err := parseTime(report["horizon_end"])
		if err != nil || !input.ExpiresAt.After(now) || input.ExpiresAt.After(horizon) {
			return ErrInvalidInput
		}
		value.ProductID = fmt.Sprint(report["product_id"])
		if contract, ok := report["contract_id"].(string); ok {
			value.ContractID = &contract
		}
		if value.ContractID != nil {
			var lastTrading time.Time
			if err := tx.Raw(`SELECT last_trading_at FROM futures_contracts WHERE id=? AND product_id=? AND deleted_at IS NULL`, *value.ContractID, value.ProductID).Scan(&lastTrading).Error; err != nil {
				return ErrInvalidInput
			}
			if lastTrading.IsZero() || input.ExpiresAt.After(lastTrading) {
				return ErrInvalidInput
			}
		}
		value.ExpiresAt = input.ExpiresAt
		chainStart := now
		if createdAt, err := parseTime(report["created_at"]); err == nil {
			chainStart = createdAt
		}
		value.RetentionDeadline = minTime(input.ExpiresAt.Add(30*24*time.Hour), chainStart.Add(90*24*time.Hour))
		claimRaw, _ := json.Marshal(input.ClaimIDs)
		conditionRaw, _ := json.Marshal(input.Conditions)
		value.ClaimIDs, value.Conditions = datatypes.JSON(claimRaw), datatypes.JSON(conditionRaw)
		if input.Activate {
			var active int64
			if err := tx.Raw(`SELECT count(*) FROM futures_hypotheses WHERE owner_id=? AND mode=? AND lifecycle IN ('active','paused') AND deleted_at IS NULL`, id.OwnerID, id.Mode).Scan(&active).Error; err != nil {
				return err
			}
			if active >= 20 {
				return ErrQuotaExceeded
			}
			value.Lifecycle = "active"
		}
		if existing, err := replayHypothesisIdempotency(tx, id, idempotencyKey, hashJSON(input)); err != nil {
			return err
		} else if existing != nil {
			value = *existing
			return nil
		}
		if err := tx.Create(&value).Error; err != nil {
			return err
		}
		if err := insertHypothesisVersion(tx, value); err != nil {
			return err
		}
		return d.writeIdempotency(tx, id, "hypothesis.create", idempotencyKey, hashJSON(input), value)
	})
	if err != nil {
		return nil, err
	}
	return hypothesisMap(value), nil
}

func (d *Domain) ListHypotheses(ctx context.Context, id Identity, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var values []model.Hypothesis
	if err := d.DB.WithContext(ctx).Where(`owner_id=? AND mode=? AND deleted_at IS NULL`, id.OwnerID, id.Mode).Order(`created_at DESC, id DESC`).Limit(limit).Find(&values).Error; err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		out = append(out, hypothesisMap(value))
	}
	return out, nil
}

func (d *Domain) GetHypothesis(ctx context.Context, id Identity, hypothesisID string) (map[string]any, error) {
	var value model.Hypothesis
	err := d.DB.WithContext(ctx).Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, hypothesisID, id.OwnerID, id.Mode).First(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return hypothesisMap(value), nil
}

func (d *Domain) PatchHypothesis(ctx context.Context, id Identity, hypothesisID string, input HypothesisPatchInput) (map[string]any, error) {
	var value model.Hypothesis
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, hypothesisID, id.OwnerID, id.Mode).First(&value).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if value.Version != input.ExpectedVersion {
			return ErrRevisionConflict
		}
		state := HypothesisState{Version: value.Version, Lifecycle: value.Lifecycle, UserView: value.UserView, ViewReason: derefString(value.ViewReason), ReviewedCheckVersion: value.ReviewedCheckVersion, ExpiresAt: value.ExpiresAt, NeedsReview: value.NeedsReview}
		if input.Action == "decide" || input.Action == "acknowledge" {
			if input.ReviewedCheckVersion == nil || *input.ReviewedCheckVersion < 1 {
				return ErrInvalidInput
			}
			var latest int64
			if err := tx.Raw(`SELECT coalesce(max(version),0) FROM futures_checks WHERE owner_id=? AND mode=? AND hypothesis_id=?`, id.OwnerID, id.Mode, hypothesisID).Scan(&latest).Error; err != nil {
				return err
			}
			if latest == 0 || int64(*input.ReviewedCheckVersion) != latest {
				return ErrRevisionConflict
			}
		}
		switch input.Action {
		case "edit":
			if input.Proposition == nil || input.ExpiresAt == nil || len(input.Conditions) == 0 {
				return ErrInvalidInput
			}
			if err := validateHypothesisConditions(input.Conditions); err != nil {
				return err
			}
			if !input.ExpiresAt.After(d.Now().UTC()) {
				return ErrInvalidInput
			}
			value.Proposition, value.ExpiresAt = *input.Proposition, *input.ExpiresAt
			raw, _ := json.Marshal(input.Conditions)
			value.Conditions = datatypes.JSON(raw)
			value.Version++
		case "activate", "pause", "resume", "close":
			next, err := TransitionHypothesis(state, input.Action, input.ExpectedVersion)
			if err != nil {
				return err
			}
			value.Lifecycle, value.Version = next.Lifecycle, next.Version
		case "decide":
			if input.UserView == nil || input.Reason == nil || input.ReviewedCheckVersion == nil {
				return ErrInvalidInput
			}
			next, err := DecideHypothesis(state, *input.UserView, *input.Reason, *input.ReviewedCheckVersion)
			if err != nil {
				return err
			}
			value.UserView, value.ViewReason, value.ReviewedCheckVersion, value.Version = next.UserView, &next.ViewReason, next.ReviewedCheckVersion, next.Version
		case "acknowledge":
			if input.ReviewedCheckVersion == nil {
				return ErrInvalidInput
			}
			next, err := AcknowledgeHypothesis(state, *input.ReviewedCheckVersion)
			if err != nil {
				return err
			}
			value.ReviewedCheckVersion, value.NeedsReview, value.Version = next.ReviewedCheckVersion, next.NeedsReview, next.Version
		default:
			return ErrInvalidInput
		}
		value.UpdatedAt = d.Now().UTC()
		res := tx.Model(&model.Hypothesis{}).Where(`id=? AND owner_id=? AND mode=? AND version=?`, hypothesisID, id.OwnerID, id.Mode, input.ExpectedVersion).Updates(map[string]any{
			`version`: value.Version, `proposition`: value.Proposition, `expires_at`: value.ExpiresAt, `conditions`: value.Conditions,
			`lifecycle`: value.Lifecycle, `user_view`: value.UserView, `view_reason`: value.ViewReason,
			`reviewed_check_version`: value.ReviewedCheckVersion, `needs_review`: value.NeedsReview, `updated_at`: value.UpdatedAt,
		})
		if res.RowsAffected != 1 {
			return ErrRevisionConflict
		}
		return insertHypothesisVersion(tx, value)
	})
	if err != nil {
		return nil, err
	}
	return hypothesisMap(value), nil
}

func (d *Domain) DeleteHypothesis(ctx context.Context, id Identity, hypothesisID, idempotencyKey string) (map[string]any, error) {
	now := d.Now().UTC()
	requestHash := hashJSON(map[string]any{"id": hypothesisID, "action": "delete"})
	result := map[string]any{"deletion_id": "del_" + uuid.NewString(), "hidden_at": now, "purge_due_at": now.Add(24 * time.Hour)}
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if existing, ok, err := replayDeletion(ctx, tx, id, "hypothesis.delete", idempotencyKey, requestHash); err != nil {
			return err
		} else if ok {
			result = existing
			return nil
		}
		res := tx.Model(&model.Hypothesis{}).Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, hypothesisID, id.OwnerID, id.Mode).Update(`deleted_at`, now)
		if res.RowsAffected != 1 {
			return ErrNotFound
		}
		if err := tx.Exec(`INSERT INTO futures_deletion_jobs(id,owner_id,mode,target_type,target_id,impact_version,purge_due_at,state) VALUES(?,?,?,?,?,?,?,'scheduled')`,
			result["deletion_id"], id.OwnerID, id.Mode, "hypothesis", hypothesisID, hashJSON(map[string]any{"id": hypothesisID}), now.Add(24*time.Hour)).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO futures_tombstones(request_id,owner_id,object_type,object_id,hidden_at,purge_due_at) VALUES(?,?,?,?,?,?)`,
			uuid.NewString(), id.OwnerID, "hypothesis", hypothesisID, now, now.Add(24*time.Hour)).Error; err != nil {
			return err
		}
		if err := hideIdempotentObject(ctx, tx, id, "hypothesis.create", hypothesisID, now); err != nil {
			return err
		}
		return d.writeIdempotency(tx, id, "hypothesis.delete", idempotencyKey, requestHash, result)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (d *Domain) ListChecks(ctx context.Context, id Identity, hypothesisID string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var values []model.Check
	if err := d.DB.WithContext(ctx).Where(`hypothesis_id=? AND owner_id=? AND mode=?`, hypothesisID, id.OwnerID, id.Mode).Order(`checked_at DESC, id DESC`).Limit(limit).Find(&values).Error; err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		var evidence any = []any{}
		_ = json.Unmarshal(value.EvidenceIDs, &evidence)
		out = append(out, map[string]any{"id": value.ID, "hypothesis_id": value.HypothesisID, "hypothesis_version": value.HypothesisVersion, "condition_id": value.ConditionID, "version": value.Version, "result": value.Result, "evidence_ids": evidence, "reason": value.Reason, "checked_at": value.CheckedAt})
	}
	return out, nil
}

func (d *Domain) CreateCheck(ctx context.Context, id Identity, hypothesisID, idempotencyKey string, input CheckCreateInput) (map[string]any, error) {
	if input.ConditionID == "" || stringsTrim(input.Reason) == "" || (input.Result != "met" && input.Result != "not_met" && input.Result != "unknown") {
		return nil, ErrInvalidInput
	}
	now := d.Now().UTC()
	value := model.Check{ID: "check_" + uuid.NewString(), OwnerID: id.OwnerID, Mode: id.Mode, HypothesisID: hypothesisID, ConditionID: input.ConditionID, Result: input.Result, Reason: input.Reason, CheckedAt: now, CreatedAt: now}
	result := checkMap(value, input.EvidenceIDs)
	replayed := false
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		requestHash := hashJSON(input)
		if existing, ok, err := replayDeletion(ctx, tx, id, "check.create", idempotencyKey, requestHash); err != nil {
			return err
		} else if ok {
			result = existing
			replayed = true
			return nil
		}
		var hypothesis model.Hypothesis
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, hypothesisID, id.OwnerID, id.Mode).First(&hypothesis).Error; err != nil {
			return ErrNotFound
		}
		if hypothesis.Version != input.ExpectedVersion {
			return ErrRevisionConflict
		}
		var run model.Run
		if err := tx.Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, hypothesis.RunID, id.OwnerID, id.Mode).First(&run).Error; err != nil {
			return ErrNotFound
		}
		report, ok := decodeObject(run.Report)
		if !ok {
			return ErrInvalidInput
		}
		for _, evidenceID := range input.EvidenceIDs {
			if !evidenceAllowed(report, evidenceID) {
				return ErrInvalidInput
			}
		}
		var conditions []map[string]any
		if err := json.Unmarshal(hypothesis.Conditions, &conditions); err != nil {
			return err
		}
		condition, ok := findCondition(conditions, input.ConditionID)
		if !ok || condition["kind"] != "manual" {
			return ErrInvalidInput
		}
		var maxVersion int64
		_ = tx.Raw(`SELECT coalesce(max(version),0) FROM futures_checks WHERE owner_id=? AND mode=? AND hypothesis_id=? AND hypothesis_version=? AND condition_id=?`, id.OwnerID, id.Mode, hypothesisID, hypothesis.Version, input.ConditionID).Scan(&maxVersion).Error
		value.HypothesisVersion, value.Version = int64(hypothesis.Version), maxVersion+1
		raw, _ := json.Marshal(input.EvidenceIDs)
		value.EvidenceIDs = datatypes.JSON(raw)
		if err := tx.Create(&value).Error; err != nil {
			return err
		}
		dedupe := fmt.Sprintf("%s/%d/%s/%d/check_changed", hypothesisID, hypothesis.Version, input.ConditionID, value.Version)
		if err := writeNotification(tx, id, "check_changed", hypothesisID, int64(hypothesis.Version), dedupe, now); err != nil {
			return err
		}
		if err := tx.Model(&model.Hypothesis{}).Where(`id=? AND owner_id=? AND mode=?`, hypothesisID, id.OwnerID, id.Mode).Update(`needs_review`, true).Error; err != nil {
			return err
		}
		return d.writeIdempotency(tx, id, "check.create", idempotencyKey, requestHash, checkMap(value, input.EvidenceIDs))
	})
	if err != nil {
		return nil, err
	}
	if !replayed {
		result = checkMap(value, input.EvidenceIDs)
	}
	return result, nil
}

func checkMap(value model.Check, evidenceIDs []string) map[string]any {
	return map[string]any{"id": value.ID, "hypothesis_id": value.HypothesisID, "hypothesis_version": value.HypothesisVersion, "condition_id": value.ConditionID, "version": value.Version, "result": value.Result, "evidence_ids": evidenceIDs, "reason": value.Reason, "checked_at": value.CheckedAt}
}

func (d *Domain) GetRecap(ctx context.Context, id Identity, hypothesisID string) (map[string]any, error) {
	var value struct {
		HypothesisID                                                      string
		Version                                                           int64
		Facts, Transmission, ContractPerformance, DataSufficiency, Reason string
		CreatedAt                                                         time.Time
	}
	err := d.DB.WithContext(ctx).Raw(`SELECT hypothesis_id,version,facts,transmission,contract_performance,data_sufficiency,reason,created_at FROM futures_recaps WHERE hypothesis_id=? AND owner_id=? AND mode=? ORDER BY version DESC LIMIT 1`, hypothesisID, id.OwnerID, id.Mode).Scan(&value).Error
	if err != nil || value.HypothesisID == "" {
		return nil, ErrNotFound
	}
	return map[string]any{"hypothesis_id": value.HypothesisID, "version": value.Version, "facts": value.Facts, "transmission": value.Transmission, "contract_performance": value.ContractPerformance, "data_sufficiency": value.DataSufficiency, "reason": value.Reason}, nil
}

func (d *Domain) PutRecap(ctx context.Context, id Identity, hypothesisID string, input RecapInput) (map[string]any, error) {
	if input.HypothesisID != hypothesisID || input.Version < 1 {
		return nil, ErrInvalidInput
	}
	if err := ValidateRecap(RecapState{Facts: input.Facts, Transmission: input.Transmission, ContractPerformance: input.ContractPerformance, DataSufficiency: input.DataSufficiency, Reason: input.Reason}); err != nil {
		return nil, err
	}
	newVersion := input.Version + 1
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Raw(`SELECT count(*) FROM futures_hypotheses WHERE id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, hypothesisID, id.OwnerID, id.Mode).Scan(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return ErrNotFound
		}
		var existing int64
		_ = tx.Raw(`SELECT count(*) FROM futures_recaps WHERE hypothesis_id=? AND owner_id=? AND mode=? AND version=?`, hypothesisID, id.OwnerID, id.Mode, input.Version).Scan(&existing).Error
		if input.Version != 1 && existing != 1 {
			return ErrRevisionConflict
		}
		if input.Version == 1 && existing != 0 {
			return ErrRevisionConflict
		}
		return tx.Exec(`INSERT INTO futures_recaps(hypothesis_id,owner_id,mode,version,facts,transmission,contract_performance,data_sufficiency,reason,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			hypothesisID, id.OwnerID, id.Mode, newVersion, input.Facts, input.Transmission, input.ContractPerformance, input.DataSufficiency, input.Reason, d.Now().UTC()).Error
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"hypothesis_id": hypothesisID, "version": newVersion, "facts": input.Facts, "transmission": input.Transmission, "contract_performance": input.ContractPerformance, "data_sufficiency": input.DataSufficiency, "reason": input.Reason}, nil
}

func hypothesisMap(value model.Hypothesis) map[string]any {
	var claims any = []any{}
	var conditions any = []any{}
	_ = json.Unmarshal(value.ClaimIDs, &claims)
	_ = json.Unmarshal(value.Conditions, &conditions)
	return map[string]any{
		"schema_version": "futures.hypothesis.v1", "id": value.ID,
		"scope":   map[string]any{"domain": "futures", "mode": value.Mode, "owner_id": value.OwnerID},
		"version": value.Version, "run_id": value.RunID, "report_id": value.ReportID, "claim_ids": claims,
		"proposition": value.Proposition, "product_id": value.ProductID, "contract_id": value.ContractID,
		"expires_at": value.ExpiresAt, "retention_deadline": value.RetentionDeadline, "lifecycle": value.Lifecycle,
		"user_view": value.UserView, "view_reason": value.ViewReason, "reviewed_check_version": value.ReviewedCheckVersion,
		"needs_review": value.NeedsReview, "conditions": conditions, "created_at": value.CreatedAt,
	}
}

func insertHypothesisVersion(tx *gorm.DB, value model.Hypothesis) error {
	raw, err := json.Marshal(hypothesisMap(value))
	if err != nil {
		return err
	}
	return tx.Create(HypothesisVersion{HypothesisID: value.ID, OwnerID: value.OwnerID, Mode: value.Mode, Version: int64(value.Version), Body: datatypes.JSON(raw), CreatedAt: value.UpdatedAt}).Error
}

func replayHypothesisIdempotency(tx *gorm.DB, id Identity, key, requestHash string) (*model.Hypothesis, error) {
	if key == "" {
		return nil, ErrInvalidInput
	}
	var row model.Idempotency
	err := tx.Where(`owner_id=? AND mode=? AND operation=? AND key=?`, id.OwnerID, id.Mode, "hypothesis.create", key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.RequestHash != requestHash {
		return nil, ErrIdempotencyConflict
	}
	var value model.Hypothesis
	if err := json.Unmarshal(row.Response, &value); err != nil {
		return nil, err
	}
	if !row.ExpiresAt.After(time.Now().UTC()) {
		return nil, ErrNotFound
	}
	if err := ensureIdempotentObjectVisible(context.Background(), tx, id, value.ID, value); err != nil {
		return nil, err
	}
	return &value, nil
}

func writeNotification(tx *gorm.DB, id Identity, kind, objectID string, objectVersion int64, dedupe string, now time.Time) error {
	var count int64
	if err := tx.Raw(`SELECT count(*) FROM futures_notifications WHERE dedupe_key=?`, dedupe).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	objectType := "run"
	if strings.Contains(kind, "check") || strings.Contains(kind, "expired") {
		objectType = "hypothesis"
	}
	payload, _ := json.Marshal(NewOutboxEvent(kind, objectType, objectID, objectVersion, ""))
	if err := tx.Exec(`INSERT INTO futures_outbox(id,owner_id,mode,dedupe_key,payload,next_attempt_at,state) VALUES(?,?,?,?,?::jsonb,?,'pending')`,
		"out_"+uuid.NewString(), id.OwnerID, id.Mode, dedupe, payload, now).Error; err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO futures_notifications(id,owner_id,mode,type,object_id,object_version,dedupe_key,created_at) VALUES(?,?,?,?,?,?,?,?)`,
		"notification_"+uuid.NewString(), id.OwnerID, id.Mode, kind, objectID, objectVersion, dedupe, now).Error
}

func decodeObject(raw datatypes.JSON) (map[string]any, bool) {
	var value map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return nil, false
	}
	return value, true
}

func claimIDsAllowed(report map[string]any, claimIDs []string) bool {
	allowed := map[string]bool{}
	if reviews, ok := report["claim_reviews"].([]any); ok {
		for _, raw := range reviews {
			if review, ok := raw.(map[string]any); ok {
				allowed[fmt.Sprint(review["claim_id"])] = true
			}
		}
	}
	for _, id := range claimIDs {
		if !allowed[id] {
			return false
		}
	}
	return true
}

func evidenceAllowed(report map[string]any, evidenceID string) bool {
	for _, raw := range asAnySlice(report["evidence_ids"]) {
		if fmt.Sprint(raw) == evidenceID {
			return true
		}
	}
	return false
}

func asAnySlice(value any) []any {
	if result, ok := value.([]any); ok {
		return result
	}
	return nil
}

func parseTime(value any) (time.Time, error) {
	text, ok := value.(string)
	if !ok {
		return time.Time{}, ErrInvalidInput
	}
	return time.Parse(time.RFC3339, text)
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
func stringsTrim(value string) string { return strings.Trim(value, " \t\r\n") }
func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func findCondition(conditions []map[string]any, id string) (map[string]any, bool) {
	for _, condition := range conditions {
		if fmt.Sprint(condition["id"]) == id {
			return condition, true
		}
	}
	return nil, false
}
