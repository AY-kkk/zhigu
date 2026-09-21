package finance

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	modelfinance "zhigu/server/model/finance"
)

func (s *ResearchService) AuthorizeTaskToken(ctx context.Context, raw, runID, taskID, purpose string) error {
	if strings.TrimSpace(raw) == "" {
		return NewError(401, "forbidden", "MISSING_TASK_TOKEN", "缺少任务凭据")
	}
	var row modelfinance.InternalToken
	if err := s.DB.WithContext(ctx).Where("token_hash = ?", SHA256Text(raw)).Take(&row).Error; err != nil {
		return NewError(401, "forbidden", "INVALID_TASK_TOKEN", "任务凭据无效")
	}
	if row.RevokedAt != nil || s.Clock.Now().After(row.ExpiresAt) {
		return NewError(401, "forbidden", "TASK_TOKEN_REVOKED", "任务凭据已失效")
	}
	if row.RunID != runID || row.TaskID != taskID {
		return NewError(403, "forbidden", "TASK_TOKEN_MISMATCH", "任务凭据与请求不一致")
	}
	if purpose != "" && row.Purpose != purpose {
		return NewError(403, "forbidden", "TASK_TOKEN_PURPOSE", "任务凭据用途不匹配")
	}
	var run modelfinance.ResearchRun
	if err := s.DB.WithContext(ctx).Where("id = ?", runID).Take(&run).Error; err != nil {
		return NewError(404, "not_found", "RUN_NOT_FOUND", "研究不存在")
	}
	if !runAllowsToolsAt(run, s.Clock.Now()) {
		return NewError(409, "conflict", "RUN_CLOSED", "研究已结束，拒绝新工具请求")
	}
	return nil
}

func (s *ResearchService) IssueTaskToken(runID, taskID, purpose string) (string, error) {
	raw := uuid.NewString()
	now := s.Clock.Now()
	ttl := RunTimeout
	if run, err := s.loadRun(runID); err == nil {
		ttl, _ = s.snapshotTimeouts(run)
		now := s.Clock.Now()
		if run.DeadlineAt != nil {
			remain := run.DeadlineAt.Sub(now)
			if remain <= 0 {
				return "", NewError(409, "conflict", "DEADLINE", "已过截止时间")
			}
			if remain < ttl {
				ttl = remain
			}
		}
	}
	row := modelfinance.InternalToken{
		TokenHash: SHA256Text(raw),
		RunID:     runID,
		TaskID:    taskID,
		Purpose:   purpose,
		ExpiresAt: now.Add(ttl),
		CreatedAt: now,
	}
	if err := s.DB.Create(&row).Error; err != nil {
		return "", err
	}
	return raw, nil
}

func (s *ResearchService) CompleteGrant(ctx context.Context, grantID, status, outputHash string) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var grant modelfinance.ToolGrant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", grantID).Take(&grant).Error; err != nil {
			return NewError(404, "not_found", "GRANT_NOT_FOUND", "授权不存在")
		}
		if grant.Status == "unknown" && status == "succeeded" {
			return NewError(409, "conflict", "GRANT_UNKNOWN", "unknown 不得改成虚假 succeeded")
		}
		if grant.OutputHash != nil && *grant.OutputHash != "" {
			return nil
		}
		now := s.Clock.Now()
		updates := map[string]any{"status": status, "updated_at": now}
		if outputHash != "" {
			updates["output_hash"] = outputHash
		}
		return tx.Model(&grant).Updates(updates).Error
	})
}

type CalcInput struct {
	EvidenceID string `json:"evidence_id"`
	Metric     string `json:"metric"`
	Period     string `json:"period"`
}

func (s *ResearchService) CalculateMetric(ctx context.Context, grantID, operation string, inputs map[string]CalcInput) (map[string]any, error) {
	if tok := TaskTokenFrom(ctx); tok != "" {
		var g modelfinance.ToolGrant
		if err := s.DB.Where("id = ?", grantID).Take(&g).Error; err != nil {
			return nil, NewError(404, "not_found", "GRANT_NOT_FOUND", "授权不存在")
		}
		if err := s.AuthorizeTaskToken(ctx, tok, g.RunID, g.TaskID, "research"); err != nil {
			return nil, err
		}
	}
	leftKey, rightKey, err := calcInputNames(operation)
	if err != nil {
		return nil, err
	}
	if _, ok := inputs[leftKey]; !ok || len(inputs) != 2 {
		return nil, NewError(400, "validation", "INVALID_INPUTS", "计算须使用恰好两个有名输入")
	}
	if _, ok := inputs[rightKey]; !ok {
		return nil, NewError(400, "validation", "INVALID_INPUTS", "计算须使用恰好两个有名输入")
	}
	var out map[string]any
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		grant, err := s.consumeGrant(tx, grantID, "calculate_metric", map[string]any{"operation": operation, "inputs": inputs})
		if err != nil {
			return err
		}
		left, err := s.metricPoint(tx, grant.RunID, grant.TaskID, inputs[leftKey])
		if err != nil {
			return err
		}
		right, err := s.metricPoint(tx, grant.RunID, grant.TaskID, inputs[rightKey])
		if err != nil {
			return err
		}
		if err := comparableMetrics(operation, left, right); err != nil {
			return err
		}
		lv, err := decimal.NewFromString(left.Value)
		if err != nil {
			return NewError(400, "validation", "INVALID_DECIMAL", "指标必须为十进制字符串")
		}
		rv, err := decimal.NewFromString(right.Value)
		if err != nil {
			return NewError(400, "validation", "INVALID_DECIMAL", "指标必须为十进制字符串")
		}
		result, formula, err := Calculate(operation, lv, rv)
		if err != nil {
			return err
		}
		unit := "ratio"
		precision := 6
		if operation == "difference" {
			unit = left.Unit
		}
		if operation == "growth_rate" {
			unit = "percent"
			precision = 2
		}
		now := s.Clock.Now()
		inJSON, _ := json.Marshal(inputs)
		row := modelfinance.Calculation{
			ID: "calc_" + uuid.NewString(), RunID: grant.RunID, TaskID: grant.TaskID, GrantID: grantID,
			Operation: operation, Inputs: datatypes.JSON(inJSON), Formula: formula, Precision: precision,
			Result: result.StringFixed(int32(precision)), Unit: unit, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&row).Error; err != nil {
			if isUnique(err) {
				var old modelfinance.Calculation
				if tx.Where("grant_id = ?", grantID).Take(&old).Error == nil {
					out = map[string]any{"calculation_id": old.ID, "value": old.Result, "formula": old.Formula, "unit": old.Unit, "precision": old.Precision, "evidence_ids": []string{inputs[leftKey].EvidenceID, inputs[rightKey].EvidenceID}}
					return nil
				}
			}
			return err
		}
		out = map[string]any{
			"calculation_id": row.ID, "value": row.Result, "formula": formula, "unit": unit, "precision": precision,
			"evidence_ids": []string{inputs[leftKey].EvidenceID, inputs[rightKey].EvidenceID},
		}
		return nil
	})
	return out, err
}

func calcInputNames(operation string) (string, string, error) {
	switch operation {
	case "growth_rate":
		return "current", "previous", nil
	case "ratio":
		return "numerator", "denominator", nil
	case "difference":
		return "left", "right", nil
	default:
		return "", "", NewError(400, "validation", "INVALID_OPERATION", "operation 不支持")
	}
}

func comparableMetrics(operation string, left, right Metric) error {
	if left.Metric != right.Metric {
		return NewError(400, "validation", "INCOMPARABLE_METRIC", "指标名称不一致")
	}
	if left.Unit != right.Unit {
		return NewError(400, "validation", "UNIT_MISMATCH", "币种或单位不可比，拒绝换算")
	}
	if left.ValueType != right.ValueType {
		return NewError(400, "validation", "VALUE_TYPE_MISMATCH", "实际值与预测值不可直接运算")
	}
	if periodGranularity(left) != periodGranularity(right) || periodSpanDays(left) != periodSpanDays(right) {
		return NewError(400, "validation", "PERIOD_MISMATCH", "期间粒度或跨度不可比")
	}
	if operation == "growth_rate" || operation == "ratio" || operation == "difference" {
		return nil
	}
	return nil
}

func (s *ResearchService) metricPoint(tx *gorm.DB, runID, taskID string, in CalcInput) (Metric, error) {
	if strings.TrimSpace(in.Period) == "" {
		return Metric{}, NewError(400, "validation", "PERIOD_REQUIRED", "计算必须指定完整期间")
	}
	var link modelfinance.TaskEvidence
	if err := tx.Where("task_id = ? AND evidence_id = ?", taskID, in.EvidenceID).Take(&link).Error; err != nil {
		return Metric{}, NewError(403, "forbidden", "EVIDENCE_NOT_GRANTED", "证据未授权给当前任务")
	}
	var ev modelfinance.Evidence
	if err := tx.Where("id = ? AND run_id = ?", in.EvidenceID, runID).Take(&ev).Error; err != nil {
		return Metric{}, NewError(400, "validation", "UNREGISTERED_CITATION", "计算输入未登记")
	}
	var metrics []Metric
	_ = json.Unmarshal(ev.Metrics, &metrics)
	for _, m := range metrics {
		if m.Metric != in.Metric {
			continue
		}
		if metricPeriodMatches(m, in.Period) {
			return m, nil
		}
	}
	return Metric{}, NewError(400, "validation", "METRIC_NOT_FOUND", "指标不存在")
}

func metricPeriodMatches(m Metric, period string) bool {
	period = strings.TrimSpace(period)
	if period == "" {
		return false
	}
	if m.PeriodStart == period || m.PeriodEnd == period {
		return true
	}
	if len(period) == 4 {
		return m.PeriodStart == period+"-01-01" && m.PeriodEnd == period+"-12-31"
	}
	return false
}

func periodGranularity(m Metric) string {
	days := periodSpanDays(m)
	switch {
	case days >= 360:
		return "year"
	case days >= 170 && days <= 190:
		return "half"
	case days >= 85 && days <= 100:
		return "quarter"
	default:
		return "other:" + m.PeriodStart + ":" + m.PeriodEnd
	}
}

func periodSpanDays(m Metric) int {
	start, err1 := time.Parse("2006-01-02", m.PeriodStart)
	end, err2 := time.Parse("2006-01-02", m.PeriodEnd)
	if err1 != nil || err2 != nil {
		return -1
	}
	return int(end.Sub(start).Hours() / 24)
}

func (s *ResearchService) AdminListRuns(ctx context.Context, limit int) ([]AdminRunItem, error) {
	if RoleFrom(ctx) != "admin" {
		return nil, NewError(403, "forbidden", "FORBIDDEN", "无管理员权限")
	}
	if limit <= 0 || limit > MaxHistory {
		limit = DefaultHistory
	}
	var rows []modelfinance.ResearchRun
	if err := s.DB.WithContext(ctx).Order("created_at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]AdminRunItem, 0, len(rows))
	for _, r := range rows {
		dur := int64(0)
		if r.StartedAt != nil {
			dur = r.UpdatedAt.Sub(*r.StartedAt).Milliseconds()
		}
		code := ""
		if r.Status == StatusFailed {
			code = "RUN_FAILED"
		}
		var usage int64
		_ = s.DB.WithContext(ctx).Model(&modelfinance.UsageLedger{}).
			Where("run_id = ?", r.ID).
			Select("COALESCE(SUM(COALESCE(actual_tokens, reserved_tokens)),0)").
			Scan(&usage)
		out = append(out, AdminRunItem{
			RunID: r.ID, UserRef: "user_" + SHA256Text(strconv.FormatUint(uint64(r.OwnerID), 10))[:8],
			Stage: r.Stage, Status: r.Status, Mode: r.Mode, DurationMS: dur, UsageTokens: usage, ErrorCode: code,
		})
	}
	return out, nil
}
