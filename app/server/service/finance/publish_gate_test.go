package finance

import (
	"context"
	"testing"
	"time"

	modelfinance "zhigu/server/model/finance"
)

func TestStageBWholeReportEvidenceGate(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "gate-ok")
	openVerifying(t, s, r.RunID)
	if err := s.Publish(context.Background(), r.RunID, 1, sampleReport(r.RunID)); err != nil {
		t.Fatal(err)
	}
	var n int64
	if err := s.DB.Model(&modelfinance.ReportCheck{}).Where("run_id = ? AND program_result = ?", r.RunID, "pass").Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("publish did not write finance_report_checks")
	}

	s2 := setup(t)
	r2 := createMinimalRun(t, s2, 1001, "gate-bad")
	openVerifying(t, s2, r2.RunID)
	bad := sampleReport(r2.RunID)
	bad.Summary = "营收同比增长 20%"
	if err := s2.Publish(context.Background(), r2.RunID, 1, bad); ErrorCode(err) != "UNBOUND_NUMBER" {
		t.Fatalf("unbound number: %v", err)
	}
	var published int64
	s2.DB.Model(&modelfinance.Report{}).Where("run_id = ?", r2.RunID).Count(&published)
	if published != 0 {
		t.Fatal("unbound number was published")
	}
	bare := sampleReport(r2.RunID)
	bare.Unknowns = []string{"待核实"}
	if err := s2.Publish(context.Background(), r2.RunID, 1, bare); ErrorCode(err) != "UNKNOWN_GATE" {
		t.Fatalf("bare unknown: %v", err)
	}
}

func TestStageBTimeAndRevisionGate(t *testing.T) {
	gate := NewEvidenceService(nil)
	asOf := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	run := modelfinance.ResearchRun{AsOf: asOf, Mode: ModeFixture}
	if err := gate.validateRecord(run, EvidenceIn{PublishedAt: asOf.Add(-time.Hour)}); ErrorCode(err) != "TIME_GATE" {
		t.Fatalf("unknown available_at: %v", err)
	}
	if err := gate.validateRecord(run, EvidenceIn{
		PublishedAt: asOf.Add(-time.Hour),
		AvailableAt: asOf.Add(time.Hour),
	}); ErrorCode(err) != "FUTURE_EVIDENCE" {
		t.Fatalf("future available_at: %v", err)
	}
}

func openVerifying(t *testing.T, s *ResearchService, runID string) {
	t.Helper()
	until := s.Clock.Now().Add(LeaseTTL)
	if err := s.DB.Model(&modelfinance.ResearchRun{}).Where("id = ?", runID).Updates(map[string]any{
		"status": StatusVerifying, "stage": "verifying", "lease_owner": "zhigu-worker", "lease_until": until,
	}).Error; err != nil {
		t.Fatal(err)
	}
}
