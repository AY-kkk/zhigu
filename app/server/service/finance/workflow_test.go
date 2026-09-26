package finance

import (
	"context"
	"testing"
	"time"
)

func TestTwoIndependentRoles(t *testing.T) {
	svc := setup(t)
	or := NewOrchestrator(svc, NewEvidenceService(svc.DB))
	run := createMinimalRun(t, svc, 1001, "wf-1")
	// fake client queues both roles independently
	if _, err := or.freeze(context.Background(), run.RunID); err != nil {
		t.Fatal(err)
	}
	st, err := or.research(context.Background(), wfState{RunID: run.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if st.SupporterErr != nil && st.ChallengerErr != nil {
		t.Fatal("both roles failed unexpectedly")
	}
}

func TestEinoWorkflowCompiles(t *testing.T) {
	svc := setup(t)
	or := NewOrchestrator(svc, NewEvidenceService(svc.DB))
	if err := or.Run(context.Background(), "missing"); err == nil {
		t.Fatal("expected error for missing run")
	}
}
func TestRepairAtMostOnce(t *testing.T) {
	svc := setup(t)
	or := NewOrchestrator(svc, NewEvidenceService(svc.DB))
	run := createMinimalRun(t, svc, 1001, "repair-1")
	st := wfState{
		RunID: run.RunID,
		Report: VerifiedReport{
			SchemaVersion: "0.9", Summary: "", QualityStatus: "completed", Mode: ModeFixture,
			AsOf: time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC),
		},
	}
	_, _ = or.verify(context.Background(), st)
	if or.RepairN != 1 {
		t.Fatalf("first verify should repair once, RepairN=%d", or.RepairN)
	}
	st.Repaired = false
	st.Report.SchemaVersion = "0.9"
	st.Report.Summary = ""
	_, _ = or.verify(context.Background(), st)
	if or.RepairN != 1 {
		t.Fatalf("second failure must not repair again, RepairN=%d", or.RepairN)
	}
}

func TestBothInsufficientCompleted(t *testing.T) {
	svc := setup(t)
	or := NewOrchestrator(svc, NewEvidenceService(svc.DB))
	run := createMinimalRun(t, svc, 1001, "wf-ins")
	st := wfState{
		RunID: run.RunID,
		Supporter: &ResearchResult{
			SchemaVersion: "1.0", RunID: run.RunID, TaskID: roleTaskID(t, svc, run.RunID, RoleSupporter),
			Status: "insufficient", Unknowns: []string{"u1"},
		},
		Challenger: &ResearchResult{
			SchemaVersion: "1.0", RunID: run.RunID, TaskID: roleTaskID(t, svc, run.RunID, RoleChallenger),
			Status: "insufficient", Unknowns: []string{"u2"},
		},
	}
	st, err := or.synthesize(context.Background(), st)
	if err != nil {
		t.Fatal(err)
	}
	if st.Report.QualityStatus != "completed" || st.Report.Verdict == nil || *st.Report.Verdict != "insufficient" {
		t.Fatalf("%+v", st.Report)
	}
}

func TestConflictingAssumptionsKept(t *testing.T) {
	svc := setup(t)
	or := NewOrchestrator(svc, NewEvidenceService(svc.DB))
	run := createMinimalRun(t, svc, 1001, "wf-conflict")
	st := wfState{
		RunID: run.RunID,
		Supporter: &ResearchResult{
			SchemaVersion: "1.0", RunID: run.RunID, TaskID: roleTaskID(t, svc, run.RunID, RoleSupporter),
			Status:    "insufficient",
			Arguments: []Argument{{ClaimType: "assumption", Text: "source-a says up"}},
		},
		Challenger: &ResearchResult{
			SchemaVersion: "1.0", RunID: run.RunID, TaskID: roleTaskID(t, svc, run.RunID, RoleChallenger),
			Status:    "insufficient",
			Arguments: []Argument{{ClaimType: "assumption", Text: "source-b says down"}},
		},
	}
	st, err := or.synthesize(context.Background(), st)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Report.Support) == 0 || len(st.Report.Challenge) == 0 {
		t.Fatalf("conflict dropped a side: support=%+v challenge=%+v", st.Report.Support, st.Report.Challenge)
	}
	if st.Report.Support[0].Text == st.Report.Challenge[0].Text {
		t.Fatal("conflicting sources collapsed into one text")
	}
}

func TestOneSideFailureIncomplete(t *testing.T) {
	svc := setup(t)
	or := NewOrchestrator(svc, NewEvidenceService(svc.DB))
	run := createMinimalRun(t, svc, 1001, "wf-fail")
	st := wfState{
		RunID: run.RunID,
		Supporter: &ResearchResult{
			SchemaVersion: "1.0", RunID: run.RunID, TaskID: roleTaskID(t, svc, run.RunID, RoleSupporter),
			Status: "succeeded", Arguments: []Argument{{ClaimType: "assumption", Text: "a", EvidenceIDs: nil}}, Unknowns: []string{"x"},
		},
		ChallengerErr: NewError(504, "timeout", "ROLE_TIMEOUT", "timeout"),
	}
	st, err := or.synthesize(context.Background(), st)
	if err != nil {
		t.Fatal(err)
	}
	if st.Report.QualityStatus != "incomplete" || st.Report.Verdict != nil {
		t.Fatalf("%+v", st.Report)
	}
}
