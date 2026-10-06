package futures

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func verifierFixture(t *testing.T, name string, dst any) {
	t.Helper()
	raw, err := os.ReadFile("../../../../contracts/futures/v1/fixtures/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatal(err)
	}
}

func TestVerifierRejectsCrossOwnerFutureFabricationAndWrongClaim(t *testing.T) {
	var task, report map[string]any
	verifierFixture(t, "task-offline.json", &task)
	verifierFixture(t, "report-offline.json", &report)
	records := map[string]ManifestRecord{"record_test": {ID: "record_test", OwnerID: 1, UsableAt: time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC), SourceType: "industry_data", Scope: "public", ModelUse: true, SourceStatus: "enabled", QualityOK: true, RetainUntil: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)}}
	if err := VerifyCandidate(task, report, records, nil); err != nil {
		t.Fatalf("valid insufficient-evidence report rejected: %v", err)
	}
	for name, mutate := range map[string]func(){
		"cross-owner": func() {
			records["record_test"] = ManifestRecord{ID: "record_test", OwnerID: 2, UsableAt: records["record_test"].UsableAt, SourceType: "industry_data", Scope: "private", ModelUse: true, SourceStatus: "enabled", QualityOK: true, RetainUntil: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)}
		},
		"future": func() {
			records["record_test"] = ManifestRecord{ID: "record_test", OwnerID: 1, UsableAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), SourceType: "industry_data", Scope: "public", ModelUse: true, SourceStatus: "enabled", QualityOK: true, RetainUntil: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)}
		},
		"wrong-claim": func() { report["claim_reviews"].([]any)[0].(map[string]any)["claim_id"] = "other" },
		"fixture-live": func() {
			records["record_test"] = ManifestRecord{ID: "record_test", OwnerID: 1, UsableAt: records["record_test"].UsableAt, SourceType: "fixture", Scope: "public", ModelUse: true, SourceStatus: "enabled", QualityOK: true, RetainUntil: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)}
		},
	} {
		t.Run(name, func(t *testing.T) {
			var taskCopy, reportCopy map[string]any
			verifierFixture(t, "task-offline.json", &taskCopy)
			verifierFixture(t, "report-offline.json", &reportCopy)
			old := records["record_test"]
			t.Cleanup(func() { records["record_test"] = old })
			mutate()
			if name == "wrong-claim" {
				report = reportCopy
				report["claim_reviews"].([]any)[0].(map[string]any)["claim_id"] = "other"
			}
			if err := VerifyCandidate(taskCopy, report, records, nil); err == nil {
				t.Fatal("expected verifier rejection")
			}
			if name == "wrong-claim" {
				verifierFixture(t, "report-offline.json", &report)
			}
		})
	}
}

func TestVerifierRejectsFabricatedCalculationAndAllowsExplicitGap(t *testing.T) {
	var task, report map[string]any
	verifierFixture(t, "task-offline.json", &task)
	verifierFixture(t, "report-offline.json", &report)
	report["calculations"] = []any{map[string]any{
		"id": "calc-fabricated", "formula": "basis", "formula_version": "v1",
		"input_record_ids": []any{"record_test"}, "value": "1000", "unit": "tonne",
		"unavailable_reason": nil, "computed_at": "2026-10-01T08:00:00Z",
	}}
	records := map[string]ManifestRecord{"record_test": {ID: "record_test", OwnerID: 1, UsableAt: time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC), SourceType: "industry_data", Scope: "public", ModelUse: true, SourceStatus: "enabled", QualityOK: true, RetainUntil: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)}}
	if err := VerifyCandidate(task, report, records, nil); err == nil {
		t.Fatal("unregistered calculation must be rejected")
	}
	verifierFixture(t, "report-offline.json", &report)
	if err := VerifyCandidate(task, report, records, map[string]CalculationRecord{}); err != nil {
		t.Fatalf("explicit evidence gap must remain publishable: %v", err)
	}
}

func TestVerifierRejectsInvalidConclusionAndUnregisteredReasoningEvidence(t *testing.T) {
	var task, report map[string]any
	verifierFixture(t, "task-offline.json", &task)
	verifierFixture(t, "report-offline.json", &report)
	report["conclusion"] = "NOT_IN_SCHEMA"
	report["reasoning_chain"] = []any{map[string]any{
		"kind": "fact", "text": "unregistered fact", "evidence_ids": []any{"someone_elses_evidence"}, "calculation_ids": []any{},
	}}
	records := map[string]ManifestRecord{"record_test": {ID: "record_test", OwnerID: 1, UsableAt: time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC), SourceType: "industry_data", Scope: "public", ModelUse: true, SourceStatus: "enabled", QualityOK: true, RetainUntil: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)}}
	if err := VerifyCandidate(task, report, records, nil); err == nil {
		t.Fatal("invalid conclusion and unregistered reasoning evidence must be rejected")
	}
}

func TestVerifierRejectsUnboundNumericProse(t *testing.T) {
	var task, report map[string]any
	verifierFixture(t, "task-offline.json", &task)
	verifierFixture(t, "report-offline.json", &report)
	report["coverage_summary"] = "合成数值987654321"
	records := map[string]ManifestRecord{"record_test": {ID: "record_test", OwnerID: 1, UsableAt: time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC), SourceType: "industry_data", Scope: "public", ModelUse: true, SourceStatus: "enabled", QualityOK: true, RetainUntil: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)}}
	if err := VerifyCandidate(task, report, records, nil); err == nil {
		t.Fatal("unbound numeric prose must be rejected")
	}
}
