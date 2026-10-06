package futures

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type InternalToolCall struct {
	GrantSpec
	Name      string
	Arguments map[string]any
}

func (d *Domain) ExecuteInternalTool(ctx context.Context, call InternalToolCall) (map[string]any, error) {
	if !allowedGrantTool(call.Name) {
		return nil, ErrForbidden
	}
	if err := d.AuthorizeExecution(ctx, Identity{OwnerID: call.OwnerID, Mode: call.Mode}); err != nil {
		return nil, err
	}
	budget := NewDBBudget(d.DB)
	day := beijingDay(time.Now().UTC())
	if err := budget.SeedAccount(ctx, UserAccount, call.OwnerID, day, decimal.RequireFromString("5"), 0, 0, 0); err != nil {
		return nil, err
	}
	if err := budget.SeedAccount(ctx, ModuleAccount, 0, day, decimal.RequireFromString("50"), 0, 0, 0); err != nil {
		return nil, err
	}
	attemptID := "tool_" + uuid.NewString()
	maxModel, maxTools, maxTokens := executionBudgetLimits(ctx, d.DB, call.OwnerID, call.Mode, call.RunID)
	reservation, err := budget.Reserve(ctx, Reservation{AttemptID: attemptID, OwnerID: call.OwnerID, RunID: call.RunID, BudgetDay: day, AmountCNY: decimal.Zero, Tokens: 0, Kind: "tool", MaxModelCalls: maxModel, MaxToolCalls: maxTools, MaxTokens: maxTokens})
	if err != nil {
		return nil, err
	}
	result, executeErr := d.executeInternalTool(ctx, call)
	if executeErr != nil {
		_ = budget.SettleUnknown(ctx, call.OwnerID, attemptID, reservation.AmountCNY, reservation.Tokens)
		return nil, executeErr
	}
	if err := budget.SettleKnown(ctx, call.OwnerID, attemptID, decimal.Zero, 0); err != nil {
		return nil, err
	}
	return result, nil
}

func (d *Domain) executeInternalTool(ctx context.Context, call InternalToolCall) (map[string]any, error) {
	switch call.Name {
	case "futures_get_observations":
		ids := stringSlice(call.Arguments["record_ids"])
		asOf, err := time.Parse(time.RFC3339, fmt.Sprint(call.Arguments["as_of"]))
		if err != nil {
			return nil, ErrInvalidInput
		}
		return d.frozenRecords(ctx, call, ids, asOf)
	case "futures_get_evidence":
		ids := stringSlice(call.Arguments["evidence_ids"])
		return d.frozenRecords(ctx, call, ids, time.Now().UTC())
	case "futures_calculate":
		roleRows, ok := call.Arguments["inputs"].([]any)
		if !ok || len(roleRows) < 2 {
			return nil, ErrInvalidInput
		}
		ids := make([]string, 0, len(roleRows))
		roles := make(map[string]string, len(roleRows))
		for _, raw := range roleRows {
			item, ok := raw.(map[string]any)
			if !ok {
				return nil, ErrInvalidInput
			}
			recordID, role := fmt.Sprint(item["record_id"]), fmt.Sprint(item["role"])
			if recordID == "" || role == "" || roles[recordID] != "" {
				return nil, ErrInvalidInput
			}
			ids, roles[recordID] = append(ids, recordID), role
		}
		asOf, err := time.Parse(time.RFC3339, fmt.Sprint(call.Arguments["as_of"]))
		if err != nil {
			return nil, ErrInvalidInput
		}
		records, err := d.frozenRecords(ctx, call, ids, asOf)
		if err != nil {
			return nil, err
		}
		recordRows := recordMaps(records)
		byID := make(map[string]map[string]any, len(recordRows))
		for _, record := range recordRows {
			byID[fmt.Sprint(record["id"])] = record
		}
		inputs := make([]FormulaInput, 0, len(roleRows))
		for _, recordID := range ids {
			record := byID[recordID]
			value, err := decimal.NewFromString(fmt.Sprint(record["value"]))
			if err != nil {
				return nil, ErrInvalidInput
			}
			expiry := time.Time{}
			if raw := fmt.Sprint(record["expiry_at"]); raw != "" {
				expiry, _ = time.Parse(time.RFC3339, raw)
			}
			tradingDay := fmt.Sprint(record["trading_day"])
			if len(tradingDay) >= 10 {
				tradingDay = tradingDay[:10]
			}
			inputs = append(inputs, FormulaInput{
				RecordID: recordID, Role: roles[recordID], Value: value,
				Unit: fmt.Sprint(record["unit"]), Currency: fmt.Sprint(record["currency"]), CaliberID: fmt.Sprint(record["caliber_id"]),
				SeriesID: fmt.Sprint(record["series_id"]), ContractKind: fmt.Sprint(record["contract_kind"]), TradingDay: tradingDay,
				ExpiryAt: expiry, PeriodStart: fmt.Sprint(record["period_start"]), PeriodEnd: fmt.Sprint(record["period_end"]),
			})
		}
		formula := Formula(fmt.Sprint(call.Arguments["formula"]))
		result, err := Calculate(formula, inputs)
		if err != nil {
			return nil, err
		}
		calculationID := "calc_" + hashJSON(map[string]any{"run_id": call.RunID, "formula": formula, "inputs": roleRows})
		computedAt := d.Now().UTC()
		inputRaw, _ := json.Marshal(result.InputRecordIDs)
		if err := d.DB.WithContext(ctx).Exec(`INSERT INTO futures_calculations(id,owner_id,mode,run_id,formula,formula_version,input_record_ids,value,unit,unavailable_reason,computed_at,created_at) VALUES(?,?,?,?,?,?,?,?,?,NULL,?,?)`,
			calculationID, call.OwnerID, call.Mode, call.RunID, result.Formula, result.FormulaVersion, datatypes.JSON(inputRaw), result.Value, result.Unit, computedAt, computedAt).Error; err != nil {
			return nil, err
		}
		return map[string]any{"id": calculationID, "formula": result.Formula, "formula_version": result.FormulaVersion, "input_record_ids": result.InputRecordIDs, "value": result.Value.String(), "unit": result.Unit, "unavailable_reason": nil, "computed_at": computedAt}, nil
	default:
		return nil, ErrForbidden
	}
}

func (d *Domain) frozenRecords(ctx context.Context, call InternalToolCall, ids []string, asOf time.Time) (map[string]any, error) {
	var rows []struct {
		ID, Unit, SourceType, SeriesID, CaliberID, ContractKind, TradingDay, PeriodStart, PeriodEnd string
		Currency                                                                                    *string
		Value, ExpiryAt                                                                             string
		UsableAt                                                                                    time.Time
	}
	query := `SELECT o.id, o.unit, o.source_type, o.series_id, o.caliber_id, s.currency,
CASE WHEN o.contract_id IS NULL THEN 'spot' ELSE c.kind END AS contract_kind,
coalesce(c.last_trading_at::text,'') AS expiry_at, coalesce(o.trading_day::text,'') AS trading_day,
o.period_start::text AS period_start, o.period_end::text AS period_end,
coalesce(o.numeric_value::text,o.text_value,'') AS value, o.usable_at
FROM futures_observations o
JOIN futures_manifest_records m ON m.record_id=o.id
JOIN futures_series s ON s.id=o.series_id
JOIN futures_sources src ON src.source_id=o.source_id
LEFT JOIN futures_contracts c ON c.id=o.contract_id
WHERE m.manifest_id=? AND o.usable_at<=? AND o.deleted_at IS NULL
  AND src.status='enabled' AND src.deleted_at IS NULL
  AND coalesce((src.rights->>'model_use')::boolean,false)
  AND coalesce((src.rights->>'retain_until')::timestamptz,to_timestamp(0)) >= ?
  AND (o.scope='public' OR (o.scope='private' AND o.owner_id=? AND o.mode=?))`
	args := []any{call.ManifestID, asOf, asOf, call.OwnerID, call.Mode}
	if len(ids) > 0 {
		query += ` AND o.id IN (?)`
		args = append(args, ids)
	}
	query += ` ORDER BY o.usable_at, o.id LIMIT 500`
	if err := d.DB.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(ids) > 0 && len(rows) != len(ids) {
		return nil, ErrForbidden
	}
	records := make([]any, 0, len(rows))
	for _, row := range rows {
		currency := any(nil)
		if row.Currency != nil {
			currency = *row.Currency
		}
		records = append(records, map[string]any{"id": row.ID, "value": row.Value, "unit": row.Unit, "currency": currency,
			"source_type": row.SourceType, "usable_at": row.UsableAt, "series_id": row.SeriesID, "caliber_id": row.CaliberID,
			"contract_kind": row.ContractKind, "expiry_at": row.ExpiryAt, "trading_day": row.TradingDay,
			"period_start": row.PeriodStart, "period_end": row.PeriodEnd})
	}
	return map[string]any{"records": records}, nil
}

func stringSlice(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		if encoded, err := json.Marshal(value); err == nil {
			var out []string
			if json.Unmarshal(encoded, &out) == nil {
				return out
			}
		}
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		out = append(out, fmt.Sprint(item))
	}
	return out
}

func recordMaps(value map[string]any) []map[string]any {
	raw, _ := value["records"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if record, ok := item.(map[string]any); ok {
			out = append(out, record)
		}
	}
	return out
}

func executionBudgetLimits(ctx context.Context, db *gorm.DB, ownerID uint, mode, runID string) (int, int, int) {
	var row struct {
		ConsumedModelCalls int
		ConsumedTokens     int
	}
	_ = db.WithContext(ctx).Raw(`SELECT d.consumed_model_calls,d.consumed_tokens FROM futures_runs r JOIN futures_drafts d ON d.id=r.draft_id AND d.owner_id=r.owner_id AND d.mode=r.mode WHERE r.id=? AND r.owner_id=? AND r.mode=?`, runID, ownerID, mode).Scan(&row).Error
	modelCalls := 8 - row.ConsumedModelCalls
	tokens := 48000 - row.ConsumedTokens
	if modelCalls < 0 {
		modelCalls = 0
	}
	if tokens < 0 {
		tokens = 0
	}
	return modelCalls, 24, tokens
}
