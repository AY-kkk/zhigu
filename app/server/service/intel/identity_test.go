package intel

import "testing"

func TestIdentityExactReferenceAndMatterWinOverTime(t *testing.T) {
	old := CandidateIdentity{SubjectCode: "DEMO.A", EventType: EventAcquisition, MatterKey: "deal-1", ObjectKey: "DEMO.B", Period: "", VariableAmount: "1000000000"}
	followup := CandidateIdentity{SubjectCode: "DEMO.A", EventType: EventAcquisition, MatterKey: "", ObjectKey: "DEMO.B", Period: "", VariableAmount: "800000000", ExplicitRefs: []string{"deal-1"}}
	got := ClassifyIdentity([]CandidateIdentity{old}, followup, 20*24*3600)
	if got.Action != IdentityMerge || got.MatchedIndex != 0 {
		t.Fatalf("identity = %#v, want merge to 0", got)
	}
}

func TestIdentityAmountDateAndSevenDaysAreNotIdentity(t *testing.T) {
	old := CandidateIdentity{SubjectCode: "DEMO.A", EventType: EventAcquisition, MatterKey: "deal-1", ObjectKey: "DEMO.B", SameMatterDescription: true}
	next := CandidateIdentity{SubjectCode: "DEMO.A", EventType: EventAcquisition, MatterKey: "", ObjectKey: "DEMO.B", SameMatterDescription: true, VariableAmount: "800000000", VariableDate: "2026-10-01"}
	got := ClassifyIdentity([]CandidateIdentity{old}, next, 20*24*3600)
	if got.Action != IdentityMerge {
		t.Fatalf("changed amount/date identity = %#v, want merge", got)
	}
}

func TestIdentityDifferentMatterIsNewAndMultipleCandidatesReview(t *testing.T) {
	existing := []CandidateIdentity{
		{SubjectCode: "DEMO.A", EventType: EventAcquisition, MatterKey: "deal-1", ObjectKey: "DEMO.B"},
		{SubjectCode: "DEMO.A", EventType: EventAcquisition, MatterKey: "deal-2", ObjectKey: "DEMO.B"},
	}
	next := CandidateIdentity{SubjectCode: "DEMO.A", EventType: EventAcquisition, MatterKey: "deal-3", ObjectKey: "DEMO.B"}
	if got := ClassifyIdentity(existing, next, 0); got.Action != IdentityNew {
		t.Fatalf("different matter = %#v, want new", got)
	}
	ambiguous := CandidateIdentity{SubjectCode: "DEMO.A", EventType: EventAcquisition, ObjectKey: "DEMO.B", CandidateRefs: []string{"deal-1", "deal-2"}}
	if got := ClassifyIdentity(existing, ambiguous, 0); got.Action != IdentityReview {
		t.Fatalf("multiple candidates = %#v, want review", got)
	}
}

func TestIdentitySimilarNameWithoutVerifiedMatterIsReview(t *testing.T) {
	existing := []CandidateIdentity{{SubjectCode: "DEMO.A", EventType: EventRegulatoryInvestigation, MatterKey: "case-1", ObjectKey: "regulator"}}
	next := CandidateIdentity{SubjectCode: "DEMO.A", EventType: EventRegulatoryInvestigation, ObjectKey: "regulator", NameSimilar: true}
	got := ClassifyIdentity(existing, next, 24*3600)
	if got.Action != IdentityReview {
		t.Fatalf("name similarity = %#v, want review", got)
	}
}
