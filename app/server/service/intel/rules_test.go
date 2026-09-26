package intel

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestRulesEffectiveWeightsAndStableSupport(t *testing.T) {
	evidence := []EvidenceRule{
		{ID: "rumor-2", ClaimKey: "amount", SourceCluster: "community", SourceWeight: decimal.RequireFromString("0.1"), Grade: GradeRumor, Stance: StanceSupport, Status: EvidenceActive},
		{ID: "rumor-1", ClaimKey: "amount", SourceCluster: "community", SourceWeight: decimal.RequireFromString("0.1"), Grade: GradeRumor, Stance: StanceSupport, Status: EvidenceActive},
		{ID: "media", ClaimKey: "amount", SourceCluster: "root-media", SourceWeight: decimal.RequireFromString("0.6"), Grade: GradeFact, Stance: StanceSupport, Status: EvidenceActive},
		{ID: "repost", ClaimKey: "amount", SourceCluster: "root-media", SourceWeight: decimal.RequireFromString("0.6"), Grade: GradeFact, Stance: StanceSupport, Status: EvidenceActive},
		{ID: "invalid", ClaimKey: "amount", SourceCluster: "other", SourceWeight: decimal.RequireFromString("1.0"), Grade: GradeFact, Stance: StanceSupport, Status: EvidenceSuperseded},
	}
	got := EvaluateClaim(evidence)
	if !got.SupportScore.Equal(decimal.RequireFromString("0.6")) {
		t.Fatalf("support score = %s, want 0.6", got.SupportScore)
	}
	if !got.RefuteScore.Equal(decimal.Zero) {
		t.Fatalf("refute score = %s, want 0", got.RefuteScore)
	}
	if got.SupportLevel != SupportMedium {
		t.Fatalf("support level = %q, want medium", got.SupportLevel)
	}
	if len(got.SupportEvidenceIDs) != 2 || got.SupportEvidenceIDs[0] != "media" || got.SupportEvidenceIDs[1] != "repost" {
		t.Fatalf("support ids = %#v, want stable [media repost]", got.SupportEvidenceIDs)
	}
	if got.Uncertain {
		t.Fatal("claim with evidence must not be uncertain")
	}
}

func TestRulesRumorCountCannotRaiseSupport(t *testing.T) {
	evidence := make([]EvidenceRule, 0, 80)
	for i := 0; i < 80; i++ {
		evidence = append(evidence, EvidenceRule{
			ID:            "rumor-" + string(rune('A'+i%26)) + decimal.NewFromInt(int64(i)).String(),
			ClaimKey:      "deal_exists",
			SourceCluster: "community-" + decimal.NewFromInt(int64(i)).String(),
			SourceWeight:  decimal.RequireFromString("0.1"),
			Grade:         GradeRumor,
			Stance:        StanceSupport,
			Status:        EvidenceActive,
		})
	}
	got := EvaluateClaim(evidence)
	if !got.SupportScore.Equal(decimal.RequireFromString("0.01")) {
		t.Fatalf("rumor score = %s, want 0.01", got.SupportScore)
	}
	if got.SupportLevel != SupportLow {
		t.Fatalf("rumor level = %q, want low", got.SupportLevel)
	}
}

func TestRulesEqualAuthoritativeConflictCapsSupportAndKeepsValues(t *testing.T) {
	evidence := []EvidenceRule{
		{ID: "n2", ClaimKey: "transaction_amount", Value: "800000000", SourceCluster: "a", SourceWeight: decimal.RequireFromString("1.0"), Grade: GradeFact, Stance: StanceSupport, Status: EvidenceActive, Authoritative: true, Direct: true},
		{ID: "n3", ClaimKey: "transaction_amount", Value: "1000000000", SourceCluster: "b", SourceWeight: decimal.RequireFromString("1.0"), Grade: GradeFact, Stance: StanceSupport, Status: EvidenceActive, Authoritative: true, Direct: true},
	}
	got := EvaluateClaim(evidence)
	if !got.ConflictOpen {
		t.Fatal("mutually exclusive normalized values must conflict")
	}
	if got.SupportLevel != SupportMedium {
		t.Fatalf("conflict level = %q, want capped medium", got.SupportLevel)
	}
	if got.CapReason == "" {
		t.Fatal("conflict cap reason must be recorded")
	}
	if len(got.ConflictingEvidenceIDs) != 2 {
		t.Fatalf("conflicting ids = %#v", got.ConflictingEvidenceIDs)
	}
}

func TestRulesContextDoesNotCountAndMissingIsUnknown(t *testing.T) {
	contextOnly := []EvidenceRule{{ID: "c", ClaimKey: "impact", SourceCluster: "x", SourceWeight: decimal.RequireFromString("1"), Grade: GradeFact, Stance: StanceContext, Status: EvidenceActive}}
	got := EvaluateClaim(contextOnly)
	if !got.SupportScore.Equal(decimal.Zero) || !got.RefuteScore.Equal(decimal.Zero) || !got.Uncertain {
		t.Fatalf("context-only result = %#v, want zero/uncertain", got)
	}
	empty := EvaluateClaim(nil)
	if !empty.Uncertain || empty.SupportLevel != SupportUnknown {
		t.Fatalf("empty result = %#v, want unknown", empty)
	}
}

func TestRulesVerificationRequiresDirectAuthoritativeCoreEvidence(t *testing.T) {
	cases := []struct {
		name string
		in   []EvidenceRule
		want Verification
	}{
		{"rumor unverified", []EvidenceRule{{ID: "r", ClaimKey: CoreClaimKey, SourceCluster: "r", SourceWeight: decimal.RequireFromString("0.1"), Grade: GradeRumor, Stance: StanceSupport, Status: EvidenceActive, Direct: true}}, VerificationUnverified},
		{"official confirms", []EvidenceRule{{ID: "n", ClaimKey: CoreClaimKey, SourceCluster: "n", SourceWeight: decimal.RequireFromString("1"), Grade: GradeFact, Stance: StanceSupport, Status: EvidenceActive, Authoritative: true, Direct: true, SubjectUnique: true, ModalityExplicit: true}}, VerificationConfirmed},
		{"official denies", []EvidenceRule{{ID: "d", ClaimKey: CoreClaimKey, SourceCluster: "d", SourceWeight: decimal.RequireFromString("1"), Grade: GradeFact, Stance: StanceRefute, Status: EvidenceActive, Authoritative: true, Direct: true, SubjectUnique: true, ModalityExplicit: true}}, VerificationDenied},
		{"equal split disputed", []EvidenceRule{
			{ID: "d", ClaimKey: CoreClaimKey, SourceCluster: "d", SourceWeight: decimal.RequireFromString("1"), Grade: GradeFact, Stance: StanceRefute, Status: EvidenceActive, Authoritative: true, Direct: true, SubjectUnique: true, ModalityExplicit: true},
			{ID: "n", ClaimKey: CoreClaimKey, SourceCluster: "n", SourceWeight: decimal.RequireFromString("1"), Grade: GradeFact, Stance: StanceSupport, Status: EvidenceActive, Authoritative: true, Direct: true, SubjectUnique: true, ModalityExplicit: true},
		}, VerificationDisputed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EvaluateVerification(tc.in); got != tc.want {
				t.Fatalf("verification = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRulesSupportOrderDoesNotAffectResult(t *testing.T) {
	a := []EvidenceRule{
		{ID: "a", ClaimKey: "x", Value: "1", SourceCluster: "one", SourceWeight: decimal.RequireFromString("0.5"), Grade: GradeOpinion, Stance: StanceSupport, Status: EvidenceActive},
		{ID: "b", ClaimKey: "x", Value: "2", SourceCluster: "two", SourceWeight: decimal.RequireFromString("0.6"), Grade: GradeFact, Stance: StanceSupport, Status: EvidenceActive},
	}
	b := []EvidenceRule{a[1], a[0]}
	ga, gb := EvaluateClaim(a), EvaluateClaim(b)
	if !ga.SupportScore.Equal(gb.SupportScore) || ga.SupportLevel != gb.SupportLevel {
		t.Fatalf("order changed result: %#v vs %#v", ga, gb)
	}
}
