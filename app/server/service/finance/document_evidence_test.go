package finance

import (
	"context"
	"testing"

	modelfinance "zhigu/server/model/finance"
)

func TestReadDocumentSpansRegistersReportEvidence(t *testing.T) {
	s := setup(t)
	docID := uploadModeDocument(t, s, 1001)
	out, err := s.ParseClaim(ctxUser(1001), ParseInput{DocumentID: docID})
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
	created, err := s.CreateResearch(ctxUser(1001), "doc-evidence", createReqFrom(t, patched))
	if err != nil {
		t.Fatal(err)
	}
	var task modelfinance.ResearchTask
	if err := s.DB.Where("run_id = ? AND role = ?", created.RunID, RoleSupporter).Take(&task).Error; err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"document_id": docID, "query": "收入", "limit": 5}
	grant, err := s.CreateGrant(ctxUser(1001), created.RunID, task.ID, GrantIn{
		RequestID: "doc-span-1", ToolName: "read_document_spans", ArgsHash: mustArgsHash(t, args),
		RunID: created.RunID, TaskID: task.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	query, err := s.DataQuery(ctxUser(1001), grant.ID, "read_document_spans", args)
	if err != nil {
		t.Fatal(err)
	}
	if len(query.Records) == 0 || query.Records[0].Record.SourceGrade != "user_report" ||
		query.Records[0].Record.VerificationStatus != "reported_only" {
		t.Fatalf("query=%+v", query)
	}
	ids := make([]string, 0, len(query.Records))
	for _, rec := range query.Records {
		ids = append(ids, rec.RecordID)
	}
	evIDs, err := NewEvidenceService(s.DB).Register(context.Background(), grant.ID, ids)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := s.GetEvidence(ctxUser(1001), evIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if raw["source_grade"] != "user_report" || raw["verification_status"] != "reported_only" {
		t.Fatalf("evidence=%+v", raw)
	}
}
