package futures

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestConsecutivePeriodConditionNeedsThreePointsAndGapIsUnknown(t *testing.T) {
	condition := CheckCondition{Kind: ConditionConsecutive, Operator: "lt", Threshold: decimal.Zero, Periods: 2}
	got := EvaluateCondition(condition, []ObservationRecord{
		{Period: "2026-10-01", Value: decimal.RequireFromString("3")},
		{Period: "2026-10-02", Value: decimal.RequireFromString("2")},
		{Period: "2026-10-03", Value: decimal.RequireFromString("1")},
	})
	if got.Result != "met" {
		t.Fatalf("three points result=%+v", got)
	}
	got = EvaluateCondition(condition, []ObservationRecord{
		{Period: "2026-10-01", Value: decimal.RequireFromString("3")},
		{Period: "2026-10-03", Value: decimal.RequireFromString("1")},
		{Period: "2026-10-04", Value: decimal.RequireFromString("0")},
	})
	if got.Result != "unknown" {
		t.Fatalf("missing period result=%+v", got)
	}
}

func TestInitialCheckDoesNotNotifyButRevisionFlipDoes(t *testing.T) {
	tracker := NewCheckTracker()
	first := tracker.Record("h1", 1, "c1", 1, "met")
	if first.Notify {
		t.Fatal("initial check must not emit change notification")
	}
	second := tracker.Record("h1", 1, "c1", 2, "not_met")
	if !second.Notify || second.Type != "revision" {
		t.Fatalf("revision flip=%+v", second)
	}
}

func TestHypothesisLifecycleAndVersionedUserDecision(t *testing.T) {
	h := HypothesisState{Version: 1, Lifecycle: "draft"}
	var err error
	if h, err = TransitionHypothesis(h, "activate", 1); err != nil || h.Lifecycle != "active" {
		t.Fatalf("activate=%+v err=%v", h, err)
	}
	if _, err = TransitionHypothesis(h, "resume", 1); err == nil {
		t.Fatal("active cannot resume")
	}
	h, err = TransitionHypothesis(h, "pause", h.Version)
	if err != nil {
		t.Fatal(err)
	}
	h, _ = TransitionHypothesis(h, "resume", h.Version)
	h, err = DecideHypothesis(h, "rejected", "reason", 3)
	if err != nil || h.UserView != "rejected" || h.ReviewedCheckVersion == nil || *h.ReviewedCheckVersion != 3 || h.Version != 5 {
		t.Fatalf("decision=%+v err=%v", h, err)
	}
	if _, err = DecideHypothesis(h, "retained", "stale", 2); err == nil {
		t.Fatal("stale check version must not clear review")
	}
}

func TestExpiryStopsTrackingAndRecapDoesNotUsePriceDirection(t *testing.T) {
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	state := HypothesisState{Lifecycle: "paused", ExpiresAt: now.Add(-time.Second), UserView: "undetermined"}
	got := ExpireHypothesis(state, now)
	if got.Lifecycle != "expired" || got.UserView != "undetermined" {
		t.Fatalf("expire=%+v", got)
	}
	if err := ValidateRecap(RecapState{Facts: "事实", Transmission: "传导", ContractPerformance: "价格方向错误", DataSufficiency: "不足", Reason: "理由"}); err == nil {
		t.Fatal("price direction cannot be sole recap result")
	}
}
