package futures

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type ManifestRecord struct {
	ID            string
	OwnerID       uint
	Scope         string
	UsableAt      time.Time
	RetainUntil   time.Time
	SourceType    string
	SourceStatus  string
	ModelUse      bool
	ExportAllowed bool
	QualityOK     bool
}

type CalculationRecord struct {
	ID       string
	Value    decimal.Decimal
	Unit     string
	InputIDs []string
}

var decimalPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)
var numericProsePattern = regexp.MustCompile(`[0-9]`)

// VerifyCandidate checks candidate report semantics against the frozen task and
// manifest. It never promotes a worker candidate to a published report.
func VerifyCandidate(task, report map[string]any, manifest map[string]ManifestRecord, calculations map[string]CalculationRecord) error {
	if err := validateReportSchema(report); err != nil {
		return err
	}
	for _, key := range []string{"run_id", "scope", "as_of", "product_id", "contract_id", "horizon_end", "versions"} {
		if !reflect.DeepEqual(report[key], task[key]) {
			return fmt.Errorf("%w: report %s differs from frozen task", ErrInvalidInput, key)
		}
	}
	asOf, err := time.Parse(time.RFC3339, fmt.Sprint(task["as_of"]))
	if err != nil {
		return fmt.Errorf("%w: invalid task as_of", ErrInvalidInput)
	}
	ownerID := uint(0)
	if scope, ok := task["scope"].(map[string]any); ok {
		switch value := scope["owner_id"].(type) {
		case float64:
			ownerID = uint(value)
		case int:
			ownerID = uint(value)
		case uint:
			ownerID = value
		}
	}
	claimIDs := map[string]bool{}
	if claims, ok := task["claims"].([]any); ok {
		for _, raw := range claims {
			if claim, ok := raw.(map[string]any); ok {
				claimIDs[fmt.Sprint(claim["id"])] = true
			}
		}
	}
	topEvidence := map[string]bool{}
	for _, raw := range asSlice(report["evidence_ids"]) {
		id := fmt.Sprint(raw)
		if topEvidence[id] {
			return fmt.Errorf("%w: duplicate evidence id", ErrInvalidInput)
		}
		if err := verifyEvidence(id, ownerID, asOf, manifest); err != nil {
			return err
		}
		topEvidence[id] = true
	}
	reviewedClaims := map[string]bool{}
	for _, raw := range asSlice(report["claim_reviews"]) {
		review := raw.(map[string]any)
		claimID := fmt.Sprint(review["claim_id"])
		if !claimIDs[claimID] || reviewedClaims[claimID] {
			return fmt.Errorf("%w: claim review set mismatch", ErrInvalidInput)
		}
		reviewedClaims[claimID] = true
		for _, id := range asSlice(review["evidence_ids"]) {
			recordID := fmt.Sprint(id)
			if !topEvidence[recordID] {
				return fmt.Errorf("%w: unregistered claim evidence", ErrInvalidInput)
			}
			if err := verifyEvidence(recordID, ownerID, asOf, manifest); err != nil {
				return err
			}
		}
	}
	if len(reviewedClaims) != len(claimIDs) {
		return fmt.Errorf("%w: every selected claim needs a review", ErrInvalidInput)
	}

	reportCalculations := map[string]map[string]any{}
	for _, raw := range asSlice(report["calculations"]) {
		calc := raw.(map[string]any)
		id := fmt.Sprint(calc["id"])
		registered, ok := calculations[id]
		if !ok {
			return fmt.Errorf("%w: calculation %s is not registered", ErrInvalidInput, id)
		}
		inputIDs := asSlice(calc["input_record_ids"])
		if len(inputIDs) != len(registered.InputIDs) {
			return fmt.Errorf("%w: calculation input mismatch", ErrInvalidInput)
		}
		seen := map[string]bool{}
		for _, rawID := range inputIDs {
			recordID := fmt.Sprint(rawID)
			if seen[recordID] || !topEvidence[recordID] {
				return fmt.Errorf("%w: calculation input mismatch", ErrInvalidInput)
			}
			seen[recordID] = true
			if err := verifyEvidence(recordID, ownerID, asOf, manifest); err != nil {
				return err
			}
			found := false
			for _, registeredID := range registered.InputIDs {
				if registeredID == recordID {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("%w: calculation input mismatch", ErrInvalidInput)
			}
		}
		value, _ := calc["value"].(string)
		parsed, err := decimal.NewFromString(value)
		if err != nil || !parsed.Equal(registered.Value) || calc["unit"] != registered.Unit || calc["formula_version"] != FormulaVersion {
			return fmt.Errorf("%w: calculation value or metadata mismatch", ErrInvalidInput)
		}
		reportCalculations[id] = calc
	}
	for _, section := range []string{"reasoning_chain", "counter_evidence"} {
		for _, raw := range asSlice(report[section]) {
			step := raw.(map[string]any)
			for _, id := range asSlice(step["evidence_ids"]) {
				recordID := fmt.Sprint(id)
				if !topEvidence[recordID] {
					return fmt.Errorf("%w: unregistered reasoning evidence", ErrInvalidInput)
				}
				if err := verifyEvidence(recordID, ownerID, asOf, manifest); err != nil {
					return err
				}
			}
			for _, id := range asSlice(step["calculation_ids"]) {
				if _, ok := reportCalculations[fmt.Sprint(id)]; !ok {
					return fmt.Errorf("%w: unregistered reasoning calculation", ErrInvalidInput)
				}
			}
			if step["kind"] == "fact" && len(asSlice(step["evidence_ids"])) == 0 {
				return fmt.Errorf("%w: facts require evidence", ErrInvalidInput)
			}
			if step["kind"] == "calculation" && len(asSlice(step["calculation_ids"])) == 0 {
				return fmt.Errorf("%w: calculation steps require calculations", ErrInvalidInput)
			}
		}
	}
	conditions := asSlice(report["conditions"])
	roles := map[string]int{}
	for _, raw := range conditions {
		roles[fmt.Sprint(raw.(map[string]any)["role"])]++
	}
	if len(conditions) > 10 || roles["validation"] < 1 || roles["invalidation"] < 1 {
		return fmt.Errorf("%w: conditions need validation and invalidation", ErrInvalidInput)
	}
	if report["conclusion"] == "insufficient_evidence" && len(asSlice(report["gaps"])) == 0 {
		return fmt.Errorf("%w: insufficient evidence needs gaps", ErrInvalidInput)
	}
	if containsForbiddenOutput(report) {
		return fmt.Errorf("%w: report contains returns/position/confidence output", ErrInvalidInput)
	}
	return nil
}

func verifyEvidence(id string, ownerID uint, asOf time.Time, manifest map[string]ManifestRecord) error {
	record, ok := manifest[id]
	if !ok || record.ID != id {
		return fmt.Errorf("%w: evidence %s is not in frozen manifest", ErrInvalidInput, id)
	}
	if record.Scope == "private" && record.OwnerID != ownerID {
		return fmt.Errorf("%w: private evidence owner mismatch", ErrInvalidInput)
	}
	if record.UsableAt.After(asOf) || record.RetainUntil.Before(asOf) || record.SourceStatus != "enabled" || !record.ModelUse || !record.QualityOK {
		return fmt.Errorf("%w: evidence %s is stale, revoked, or unusable", ErrInvalidInput, id)
	}
	if strings.Contains(strings.ToLower(record.SourceType), "fixture") {
		return fmt.Errorf("%w: fixture evidence in live report", ErrInvalidInput)
	}
	return nil
}

func validateReportSchema(report map[string]any) error {
	top := []string{"schema_version", "id", "run_id", "scope", "as_of", "product_id", "contract_id", "horizon_end", "question", "coverage_summary", "conclusion", "claim_reviews", "reasoning_chain", "counter_evidence", "structure_assessment", "conditions", "evidence_ids", "calculations", "gaps", "versions", "created_at"}
	if err := exactKeys(report, top); err != nil {
		return fmt.Errorf("%w: report %v", ErrInvalidInput, err)
	}
	if report["schema_version"] != "futures.report.v1" {
		return fmt.Errorf("%w: report schema version", ErrInvalidInput)
	}
	for _, key := range []string{"id", "run_id", "product_id", "question", "coverage_summary", "structure_assessment"} {
		if value, ok := report[key].(string); !ok || strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: invalid %s", ErrInvalidInput, key)
		}
	}
	for _, key := range []string{"as_of", "horizon_end", "created_at"} {
		if value, ok := report[key].(string); !ok {
			return fmt.Errorf("%w: invalid %s", ErrInvalidInput, key)
		} else if _, err := time.Parse(time.RFC3339, value); err != nil {
			return fmt.Errorf("%w: invalid %s", ErrInvalidInput, key)
		}
	}
	if _, ok := report["contract_id"].(string); report["contract_id"] != nil && !ok {
		return fmt.Errorf("%w: invalid contract_id", ErrInvalidInput)
	}
	if !oneOf(fmt.Sprint(report["conclusion"]), "supported", "partially_supported", "challenged", "insufficient_evidence") {
		return fmt.Errorf("%w: invalid conclusion", ErrInvalidInput)
	}
	scope, ok := report["scope"].(map[string]any)
	if !ok || scope["domain"] != "futures" || scope["mode"] != "live" {
		return fmt.Errorf("%w: invalid scope", ErrInvalidInput)
	}
	if err := exactKeys(scope, []string{"domain", "mode", "owner_id"}); err != nil {
		return fmt.Errorf("%w: scope %v", ErrInvalidInput, err)
	}
	switch scope["owner_id"].(type) {
	case float64, int, int64, uint:
	default:
		return fmt.Errorf("%w: invalid owner_id", ErrInvalidInput)
	}

	reviews := asSlice(report["claim_reviews"])
	if len(reviews) == 0 || len(reviews) > 12 {
		return fmt.Errorf("%w: invalid claim review count", ErrInvalidInput)
	}
	for _, raw := range reviews {
		item, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: invalid claim review", ErrInvalidInput)
		}
		if err := exactKeys(item, []string{"claim_id", "verdict", "explanation", "evidence_ids"}); err != nil {
			return fmt.Errorf("%w: claim review %v", ErrInvalidInput, err)
		}
		if nonEmptyString(item["claim_id"]) == "" || nonEmptyString(item["explanation"]) == "" || !oneOf(fmt.Sprint(item["verdict"]), "supported", "refuted", "missing_premise", "disputed", "unknown") {
			return fmt.Errorf("%w: invalid claim review", ErrInvalidInput)
		}
		if err := stringArray(item["evidence_ids"], false); err != nil {
			return err
		}
		if numericProsePattern.MatchString(nonEmptyString(item["explanation"])) {
			return fmt.Errorf("%w: unbound numeric prose", ErrInvalidInput)
		}
	}
	for _, section := range []string{"reasoning_chain", "counter_evidence"} {
		for _, raw := range asSlice(report[section]) {
			item, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("%w: invalid reasoning step", ErrInvalidInput)
			}
			if err := exactKeys(item, []string{"kind", "text", "evidence_ids", "calculation_ids"}); err != nil {
				return fmt.Errorf("%w: reasoning %v", ErrInvalidInput, err)
			}
			if !oneOf(fmt.Sprint(item["kind"]), "fact", "calculation", "inference", "assumption") || nonEmptyString(item["text"]) == "" {
				return fmt.Errorf("%w: invalid reasoning step", ErrInvalidInput)
			}
			if err := stringArray(item["evidence_ids"], false); err != nil {
				return err
			}
			if err := stringArray(item["calculation_ids"], false); err != nil {
				return err
			}
			if numericProsePattern.MatchString(nonEmptyString(item["text"])) {
				return fmt.Errorf("%w: unbound numeric prose", ErrInvalidInput)
			}
		}
	}
	for _, raw := range asSlice(report["calculations"]) {
		item, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: invalid calculation", ErrInvalidInput)
		}
		if err := exactKeys(item, []string{"id", "formula", "formula_version", "input_record_ids", "value", "unit", "unavailable_reason", "computed_at"}); err != nil {
			return fmt.Errorf("%w: calculation %v", ErrInvalidInput, err)
		}
		if nonEmptyString(item["id"]) == "" || nonEmptyString(item["formula_version"]) == "" || nonEmptyString(item["unit"]) == "" || !oneOf(fmt.Sprint(item["formula"]), "basis", "basis_rate", "calendar_spread", "inventory_change", "inventory_change_rate", "coverage") {
			return fmt.Errorf("%w: invalid calculation", ErrInvalidInput)
		}
		if err := stringArray(item["input_record_ids"], true); err != nil {
			return err
		}
		if value := item["value"]; value != nil {
			text := nonEmptyString(value)
			if text == "" || !decimalPattern.MatchString(text) {
				return fmt.Errorf("%w: invalid calculation value", ErrInvalidInput)
			}
		}
		if reason := item["unavailable_reason"]; reason != nil && nonEmptyString(reason) == "" {
			return fmt.Errorf("%w: invalid unavailable_reason", ErrInvalidInput)
		}
		if value, ok := item["computed_at"].(string); !ok {
			return fmt.Errorf("%w: invalid computed_at", ErrInvalidInput)
		} else if _, err := time.Parse(time.RFC3339, value); err != nil {
			return fmt.Errorf("%w: invalid computed_at", ErrInvalidInput)
		}
	}
	for _, raw := range asSlice(report["conditions"]) {
		item, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: invalid condition", ErrInvalidInput)
		}
		if err := exactKeys(item, []string{"id", "role", "kind", "series_id", "operator", "threshold", "unit", "periods", "instruction"}); err != nil {
			return fmt.Errorf("%w: condition %v", ErrInvalidInput, err)
		}
		kind := fmt.Sprint(item["kind"])
		if nonEmptyString(item["id"]) == "" || nonEmptyString(item["instruction"]) == "" || !oneOf(fmt.Sprint(item["role"]), "validation", "invalidation") || !oneOf(kind, "metric_compare", "metric_change", "consecutive_periods", "manual") {
			return fmt.Errorf("%w: invalid condition", ErrInvalidInput)
		}
		if kind == "manual" {
			if item["series_id"] != nil || item["operator"] != nil || item["threshold"] != nil || item["unit"] != nil || item["periods"] != nil {
				return fmt.Errorf("%w: invalid manual condition", ErrInvalidInput)
			}
		} else {
			if nonEmptyString(item["series_id"]) == "" || !oneOf(fmt.Sprint(item["operator"]), "gt", "gte", "lt", "lte", "eq", "ne") || !decimalPattern.MatchString(nonEmptyString(item["threshold"])) || nonEmptyString(item["unit"]) == "" {
				return fmt.Errorf("%w: invalid metric condition", ErrInvalidInput)
			}
		}
		if kind == "consecutive_periods" {
			value, ok := item["periods"].(float64)
			if !ok || value < 2 || value > 4 {
				return fmt.Errorf("%w: invalid condition periods", ErrInvalidInput)
			}
		} else if item["periods"] != nil {
			return fmt.Errorf("%w: invalid condition periods", ErrInvalidInput)
		}
	}
	if err := stringArray(report["evidence_ids"], true); err != nil {
		return err
	}
	if err := stringArray(report["gaps"], false); err != nil {
		return err
	}
	for _, gap := range asSlice(report["gaps"]) {
		if numericProsePattern.MatchString(fmt.Sprint(gap)) {
			return fmt.Errorf("%w: unbound numeric prose", ErrInvalidInput)
		}
	}
	for _, key := range []string{"question", "coverage_summary", "structure_assessment"} {
		if numericProsePattern.MatchString(fmt.Sprint(report[key])) {
			return fmt.Errorf("%w: unbound numeric prose", ErrInvalidInput)
		}
	}
	versions, ok := report["versions"].(map[string]any)
	if !ok {
		return fmt.Errorf("%w: invalid versions", ErrInvalidInput)
	}
	if err := exactKeys(versions, []string{"template", "formula", "policy", "model_config", "source_manifest"}); err != nil {
		return fmt.Errorf("%w: versions %v", ErrInvalidInput, err)
	}
	for _, key := range []string{"template", "formula", "policy", "model_config", "source_manifest"} {
		if nonEmptyString(versions[key]) == "" {
			return fmt.Errorf("%w: invalid version %s", ErrInvalidInput, key)
		}
	}
	return nil
}

func exactKeys(value map[string]any, allowed []string) error {
	set := map[string]bool{}
	for _, key := range allowed {
		set[key] = true
	}
	for key := range value {
		if !set[key] {
			return fmt.Errorf("unknown field %s", key)
		}
	}
	for _, key := range allowed {
		if _, ok := value[key]; !ok {
			return fmt.Errorf("missing field %s", key)
		}
	}
	return nil
}

func nonEmptyString(value any) string { result, _ := value.(string); return strings.TrimSpace(result) }

func stringArray(value any, unique bool) error {
	items, ok := value.([]any)
	if !ok {
		return fmt.Errorf("%w: expected string array", ErrInvalidInput)
	}
	seen := map[string]bool{}
	for _, raw := range items {
		id, ok := raw.(string)
		if !ok || strings.TrimSpace(id) == "" {
			return fmt.Errorf("%w: expected non-empty string array", ErrInvalidInput)
		}
		if unique && seen[id] {
			return fmt.Errorf("%w: duplicate array value", ErrInvalidInput)
		}
		seen[id] = true
	}
	return nil
}

func asSlice(value any) []any {
	switch value := value.(type) {
	case []any:
		return value
	case nil:
		return nil
	default:
		return []any{value}
	}
}

func containsForbiddenOutput(value any) bool {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "position") || strings.Contains(lower, "return") || strings.Contains(lower, "confidence") {
				return true
			}
			if containsForbiddenOutput(child) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if containsForbiddenOutput(child) {
				return true
			}
		}
	case string:
		lower := strings.ToLower(value)
		for _, phrase := range []string{"目标收益", "预期收益", "建议仓位", "自报置信度"} {
			if strings.Contains(lower, strings.ToLower(phrase)) {
				return true
			}
		}
	}
	return false
}

func decodeJSON(raw []byte) (map[string]any, error) {
	var value map[string]any
	err := json.Unmarshal(raw, &value)
	return value, err
}
