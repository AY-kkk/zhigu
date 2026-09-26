package finance

import (
	"testing"
)

func uploadModeDocument(t *testing.T, svc *ResearchService, owner uint) string {
	t.Helper()
	t.Setenv("ZHIGU_RESEARCH_DOCUMENT_DIR", t.TempDir())
	doc, err := svc.Docs.Upload(ctxUser(owner), UploadDocumentInput{
		Filename: "report.txt", MediaType: "text/plain", ByteSize: int64(len("研报认为公司收入将继续增长。")),
		Content: []byte("研报认为公司收入将继续增长。"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return doc.DocumentID
}

func TestParseInputMode(t *testing.T) {
	s := setup(t)
	docID := uploadModeDocument(t, s, 1001)
	text := "演示公司的收入增长能否支持未来一年股价上涨？这是一条超过二十个字的测试观点。"
	cases := []struct {
		in   ParseInput
		mode string
	}{
		{ParseInput{Text: text}, "claim_only"},
		{ParseInput{DocumentID: docID}, "report_only"},
		{ParseInput{Text: text, DocumentID: docID, FocusText: "重点检查现金流"}, "claim_and_report"},
	}
	for _, tc := range cases {
		out, err := s.ParseClaim(ctxUser(1001), tc.in)
		if err != nil {
			t.Fatalf("%s: %v", tc.mode, err)
		}
		if out.InputMode != tc.mode || len(out.Items) == 0 {
			t.Fatalf("mode=%s out=%+v", tc.mode, out)
		}
	}
}

func TestParseRejectsMissingInput(t *testing.T) {
	s := setup(t)
	_, err := s.ParseClaim(ctxUser(1001), ParseInput{})
	if err == nil || ErrorCode(err) != "MISSING_RESEARCH_INPUT" {
		t.Fatalf("got %v", err)
	}
}

func TestPatchRejectsChangedResearchInput(t *testing.T) {
	s := setup(t)
	out := parseDemo(t, s, 1001)
	text := "修改后的观点内容，但没有重新解析。"
	_, err := s.PatchClaim(ctxUser(1001), out.DraftID, PatchDraftInput{Revision: out.Revision, Text: &text})
	if err == nil || ErrorCode(err) != "REPARSE_REQUIRED" {
		t.Fatalf("got %v", err)
	}
}

func TestCreateResearchPreservesReportInput(t *testing.T) {
	s := setup(t)
	docID := uploadModeDocument(t, s, 1001)
	out, err := s.ParseClaim(ctxUser(1001), ParseInput{DocumentID: docID, FocusText: "现金流"})
	if err != nil {
		t.Fatal(err)
	}
	inst := InstrumentDemo
	start := "2026-01-01"
	end := "2026-12-31"
	patched, err := s.PatchClaim(ctxUser(1001), out.DraftID, PatchDraftInput{
		Revision: out.Revision, InstrumentID: &inst, HorizonStart: &start, HorizonEnd: &end,
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.CreateResearch(ctxUser(1001), "report-input", createReqFrom(t, patched))
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.GetResearch(ctxUser(1001), created.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if view.InputMode != InputModeReportOnly || view.DocumentID != docID || view.Document == nil {
		t.Fatalf("view=%+v", view)
	}
}
