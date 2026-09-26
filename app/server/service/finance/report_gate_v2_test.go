package finance

import (
	"strings"
	"testing"

	modelfinance "zhigu/server/model/finance"
)

func sampleReportV2(runID string) VerifiedReport {
	claim := Claim{Text: "收入增长会证明盈利质量改善", Horizon: "2026-01-01/2026-12-31", Items: []ClaimItem{
		{ClaimID: "c1", ClaimType: "inference", Text: "收入增长会证明盈利质量改善"},
	}}
	report := BuildReportV2(runID, claim,
		[]Argument{{ClaimID: "c1", ClaimType: "fact", Text: "收入增长有已披露证据", EvidenceIDs: []string{"ev_1"}}},
		[]Argument{{ClaimID: "c1", ClaimType: "fact", Title: "现金流反证", Text: "现金流下降削弱盈利质量", EvidenceIDs: []string{"ev_2"}}},
		[]string{"无法取数：缺少独立现金流证据。"}, []string{"ev_1", "ev_2"}, ModeFixture, "2026-01-01T00:00:00Z")
	report.QualityStatus = "completed"
	v := "partially_supported"
	report.Verdict = &v
	report.EvidenceIndex = []EvidenceRef{
		{EvidenceID: "ev_1", SourceGrade: "structured_data", VerificationStatus: "independent_verified", Relation: "support", Title: "source1"},
		{EvidenceID: "ev_2", SourceGrade: "user_report", VerificationStatus: "reported_only", Relation: "challenge", Title: "source2"},
	}
	return report
}

func evidenceRowsForReport() []modelfinance.Evidence {
	return []modelfinance.Evidence{
		{ID: "ev_1", SourceKind: "financials", SourceGrade: "structured_data", VerificationStatus: "independent_verified", Title: "source1", Text: "收入增长 1", Metrics: []byte("[]")},
		{ID: "ev_2", SourceKind: "user_report", SourceGrade: "user_report", VerificationStatus: "reported_only", Title: "source2", Text: "现金流下降", Metrics: []byte("[]")},
	}
}

func TestReportV2RejectsMissingRequiredSection(t *testing.T) {
	report := sampleReportV2("run_gate")
	report.TailRisks = nil
	claim := Claim{Items: []ClaimItem{{ClaimID: "c1", ClaimType: "inference", Text: "收入增长会证明盈利质量改善"}}}
	err := ValidateReportV2(report, evidenceRowsForReport(), claim)
	if err == nil || ErrorCode(err) != "REPORT_SECTION_MISSING" {
		t.Fatalf("got %v", err)
	}
}

func TestReportV2RejectsUnregisteredFactEvidence(t *testing.T) {
	report := sampleReportV2("run_gate")
	report.FactChecks[0].EvidenceIDs = []string{"ev_missing"}
	claim := Claim{Items: []ClaimItem{{ClaimID: "c1", ClaimType: "inference", Text: "收入增长会证明盈利质量改善"}}}
	err := ValidateReportV2(report, evidenceRowsForReport(), claim)
	if err == nil || ErrorCode(err) != "UNREGISTERED_CITATION" {
		t.Fatalf("got %v", err)
	}
}

func TestExportHTMLHasNoScriptAndHasDisclaimer(t *testing.T) {
	report := sampleReportV2("run_export")
	claim := Claim{Text: "收入增长会证明盈利质量改善", Items: []ClaimItem{{ClaimID: "c1", ClaimType: "inference", Text: "收入增长会证明盈利质量改善"}}}
	raw := RenderReportHTML(report, claim, evidenceRowsForReport(), nil)
	if strings.Contains(strings.ToLower(raw), "<script") {
		t.Fatal("script forbidden")
	}
	for _, text := range []string{"核心判断", "事实核验", "逐条质疑", "推理链缺口", "被忽略的风险", "证实与证伪条件", "证据清单", "本报告仅供研究参考，不构成投资建议"} {
		if !strings.Contains(raw, text) {
			t.Fatalf("missing %s", text)
		}
	}
}
