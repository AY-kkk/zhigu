package futures

import "testing"

func TestHypothesisConditionsRequireValidationAndInvalidation(t *testing.T) {
	bad := []map[string]any{
		{"id": "v", "role": "validation", "kind": "manual", "series_id": nil, "operator": nil, "threshold": nil, "unit": nil, "periods": nil, "instruction": "验证"},
	}
	if err := validateHypothesisConditions(bad); err == nil {
		t.Fatal("missing invalidation must fail")
	}
	good := append(bad, map[string]any{
		"id": "i", "role": "invalidation", "kind": "metric_compare", "series_id": "series", "operator": "lt", "threshold": "0", "unit": "tonne", "periods": nil, "instruction": "失效",
	})
	if err := validateHypothesisConditions(good); err != nil {
		t.Fatal(err)
	}
	many := make([]map[string]any, 0)
	for i := 0; i < 6; i++ {
		many = append(many, good[0])
	}
	if err := validateHypothesisConditions(many); err == nil {
		t.Fatal("more than five validation conditions must fail")
	}
}

func TestHypothesisDecisionAcknowledgeCannotClearNewReview(t *testing.T) {
	state := HypothesisState{Version: 1, Lifecycle: "active", ReviewedCheckVersion: intPtr(3), UserView: "undetermined"}
	if _, err := AcknowledgeHypothesis(state, 2); err == nil {
		t.Fatal("stale acknowledgement must fail")
	}
	next, err := AcknowledgeHypothesis(state, 3)
	if err != nil || next.NeedsReview || next.Version != 2 {
		t.Fatalf("ack=%+v err=%v", next, err)
	}
}

func intPtr(value int) *int { return &value }
