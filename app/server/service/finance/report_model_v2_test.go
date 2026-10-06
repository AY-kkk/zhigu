package finance

import "testing"

func TestAdjudicateClaimDoesNotIgnoreUnresolvedChallenge(t *testing.T) {
	claim := ClaimItem{ClaimID: "c1", ClaimType: "inference", Text: "收入增长且现金流下降会证明盈利质量改善"}
	support := []Argument{{ClaimType: "fact", Text: "收入增长会证明盈利质量改善", EvidenceIDs: []string{"ev_s"}}}
	challenge := []Argument{{ClaimType: "fact", Text: "现金流下降会削弱盈利质量改善", EvidenceIDs: []string{"ev_c"}}}
	got := AdjudicateClaim(claim, support, challenge)
	if got.Status == "supported" {
		t.Fatalf("unresolved challenge was ignored: %+v", got)
	}
	if got.Status != "uncertain" && got.Status != "contradicted" {
		t.Fatalf("unexpected status: %+v", got)
	}
}

func TestBuildReportV2IsClaimSpecific(t *testing.T) {
	claim := Claim{Text: "现金流改善会证明盈利质量提升", Items: []ClaimItem{
		{ClaimID: "c1", ClaimType: "inference", Text: "现金流改善会证明盈利质量提升"},
	}}
	a := BuildReportV2("run_a", claim, nil, nil, nil, nil, ModeFixture, "2026-01-01T00:00:00Z")
	b := BuildReportV2("run_b", Claim{Text: "海外收入会持续增长", Items: []ClaimItem{
		{ClaimID: "c1", ClaimType: "inference", Text: "海外收入会持续增长"},
	}}, nil, nil, nil, nil, ModeFixture, "2026-01-01T00:00:00Z")
	if len(a.ReasoningGaps) != 1 || len(b.ReasoningGaps) != 1 {
		t.Fatalf("a=%+v b=%+v", a.ReasoningGaps, b.ReasoningGaps)
	}
	if a.ReasoningGaps[0].Missing == b.ReasoningGaps[0].Missing {
		t.Fatal("reasoning gap must be claim-specific")
	}
	if a.Assumptions[0] == b.Assumptions[0] && a.Assumptions[0] != "" {
		t.Fatal("assumptions must not be copied across unrelated claims")
	}
	if len(a.FactChecks) != 1 || a.FactChecks[0].ClaimID != "c1" {
		t.Fatalf("fact checks=%+v", a.FactChecks)
	}
}
