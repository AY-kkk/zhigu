package finance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	modelfinance "zhigu/server/model/finance"
)

func r2Evidence(t *testing.T, s *ResearchService, runID, taskID, req string) string {
	t.Helper()
	p := map[string]any{}
	g, e := s.CreateGrant(context.Background(), runID, taskID, GrantIn{RequestID: req, ToolName: "get_financials", ArgsHash: mustArgsHash(t, p)})
	if e != nil {
		t.Fatal(e)
	}
	out, e := s.DataQuery(context.Background(), g.ID, "get_financials", p)
	if e != nil {
		t.Fatal(e)
	}
	ids, e := NewEvidenceService(s.DB).Register(context.Background(), g.ID, issuedIDs(out))
	if e != nil {
		t.Fatal(e)
	}
	return ids[0]
}

func issuedIDs(out DataQueryResult) []string {
	ids := make([]string, 0, len(out.Records))
	for _, rec := range out.Records {
		ids = append(ids, rec.RecordID)
	}
	return ids
}

func namedGrowth(eid string) map[string]CalcInput {
	return map[string]CalcInput{
		"current":  {EvidenceID: eid, Metric: "revenue", Period: "2025"},
		"previous": {EvidenceID: eid, Metric: "revenue", Period: "2024"},
	}
}

func TestR2LeaseLostCannotPublish(t *testing.T) {
	s := setup(t)
	clk := NewFrozenClock(time.Now().UTC())
	s.Clock = clk
	r := createMinimalRun(t, s, 1001, "r2-lease")
	o := NewOrchestrator(s, NewEvidenceService(s.DB))
	st, e := o.freeze(context.Background(), r.RunID)
	if e != nil {
		t.Fatal(e)
	}
	st.Supporter = &ResearchResult{Status: "insufficient"}
	st.Challenger = &ResearchResult{Status: "insufficient"}
	st, e = o.synthesize(context.Background(), st)
	if e != nil {
		t.Fatal(e)
	}
	clk.Advance(16 * time.Second)
	s.reapExpiredLeases(context.Background(), clk.Now())
	before, _ := s.loadRun(r.RunID)
	if before.Stage != "lease_lost" {
		t.Fatal("probe did not reap lease")
	}
	_, e = o.verify(context.Background(), st)
	v, _ := s.GetResearch(ctxUser(1001), r.RunID)
	if e == nil && v.Report != nil {
		t.Fatalf("expired executor resurrected run: before=%s after=%s report=%v", before.Status, v.Status, v.Report.QualityStatus)
	}
}

func TestR2ModelProxyWorksWithProductionBudget(t *testing.T) {
	s := setup(t)
	activateTestModel(t, s.DB)
	t.Setenv("ZHIGU_MODEL_FEE_CAP", "1")
	r := createMinimalRun(t, s, 1001, "r2-model")
	task := reviewTaskID(t, s, r.RunID)
	token, e := s.IssueTaskToken(r.RunID, task, "research")
	if e != nil {
		t.Fatal(e)
	}
	body := []byte(`{"model":"finance-research","messages":[]}`)
	proxy := NewModelProxy(s.DB, NewDBBudget(s.DB))
	proxy.Client = stubModelClient(`{"id":"chatcmpl_r2","choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`)
	out, e := proxy.Complete(context.Background(), "r2-model-request", NormalizeJSONHash(body), HeaderTokenHash(token), body, ProtocolChatCompletions)
	if e != nil {
		t.Fatalf("configured model call fails with production DBBudget: %v", e)
	}
	if !bytes.Contains(out, []byte("chatcmpl_r2")) {
		t.Fatalf("response did not come from upstream: %s", out)
	}
}

func TestR2GrantOperationBound(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "r2-tool")
	p := map[string]any{}
	g, e := s.CreateGrant(context.Background(), r.RunID, reviewTaskID(t, s, r.RunID), GrantIn{RequestID: "r2-tool", ToolName: "get_financials", ArgsHash: mustArgsHash(t, p)})
	if e != nil {
		t.Fatal(e)
	}
	out, e := s.DataQuery(context.Background(), g.ID, "search_filings", p)
	if e == nil {
		t.Fatalf("get_financials grant executed search_filings: records=%d", len(out.Records))
	}
}

func TestR2ChineseQueryHashRoundTrip(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "r2-chinese")
	p := map[string]any{"query": "现金流风险", "limit": 3}
	canon, err := CanonicalJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(canon, `\u`) {
		t.Fatalf("han must remain utf-8, got %s", canon)
	}
	g, e := s.CreateGrant(context.Background(), r.RunID, reviewTaskID(t, s, r.RunID), GrantIn{RequestID: "r2-chinese", ToolName: "search_filings", ArgsHash: mustArgsHash(t, p)})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DataQuery(context.Background(), g.ID, "search_filings", p); e != nil {
		t.Fatalf("unchanged Chinese query rejected: %v", e)
	}
}

func TestR2SecondRoleCanRegisterSameSource(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "r2-dedup")
	a := reviewTaskID(t, s, r.RunID)
	var b modelfinance.ResearchTask
	if e := s.DB.Where("run_id = ? AND role = ?", r.RunID, RoleChallenger).Take(&b).Error; e != nil {
		t.Fatal(e)
	}
	first := r2Evidence(t, s, r.RunID, a, "r2-first")
	second := r2Evidence(t, s, r.RunID, b.ID, "r2-second")
	if first != second {
		t.Fatalf("same source not deduplicated: %s %s", first, second)
	}
	var n int64
	s.DB.Model(&modelfinance.TaskEvidence{}).Where("task_id = ? AND evidence_id = ?", b.ID, first).Count(&n)
	if n != 1 {
		t.Fatal("second role was not granted shared evidence")
	}
}

func TestR2CalculationRejectsExpiredGrant(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "r2-calc")
	task := reviewTaskID(t, s, r.RunID)
	eid := r2Evidence(t, s, r.RunID, task, "r2-calc-source")
	inputs := namedGrowth(eid)
	h, _ := HashCanonical(map[string]any{"operation": "growth_rate", "inputs": inputs})
	g, e := s.CreateGrant(context.Background(), r.RunID, task, GrantIn{RequestID: "r2-calc-grant", ToolName: "calculate_metric", ArgsHash: h})
	if e != nil {
		t.Fatal(e)
	}
	s.DB.Model(&g).Update("expires_at", time.Now().Add(-time.Minute))
	out, e := s.CalculateMetric(context.Background(), g.ID, "growth_rate", inputs)
	if e == nil {
		t.Fatalf("expired calculation grant accepted: %v", out)
	}
}

func TestR2CalculationCannotUseOtherRolesEvidence(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "r2-calc-isolation")
	a := reviewTaskID(t, s, r.RunID)
	var b modelfinance.ResearchTask
	s.DB.Where("run_id = ? AND role = ?", r.RunID, RoleChallenger).Take(&b)
	eid := r2Evidence(t, s, r.RunID, b.ID, "r2-other-role")
	token, e := s.IssueTaskToken(r.RunID, a, "research")
	if e != nil {
		t.Fatal(e)
	}
	ctx := WithTaskToken(context.Background(), token)
	inputs := namedGrowth(eid)
	h, _ := HashCanonical(map[string]any{"operation": "growth_rate", "inputs": inputs})
	g, e := s.CreateGrant(ctx, r.RunID, a, GrantIn{RequestID: "r2-calc-isolation", ToolName: "calculate_metric", ArgsHash: h})
	if e != nil {
		t.Fatal(e)
	}
	out, e := s.CalculateMetric(ctx, g.ID, "growth_rate", inputs)
	if e == nil {
		t.Fatalf("supporter read ungranted challenger evidence: %v", out)
	}
}

func TestR2CalculationValidatesUnits(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "r2-units")
	task := reviewTaskID(t, s, r.RunID)
	eid := r2Evidence(t, s, r.RunID, task, "r2-units-source")
	var ev modelfinance.Evidence
	s.DB.Where("id = ?", eid).Take(&ev)
	var metrics []Metric
	json.Unmarshal(ev.Metrics, &metrics)
	metrics[1].Unit = "USD_million"
	raw, _ := json.Marshal(metrics)
	s.DB.Model(&ev).Update("metrics", string(raw))
	inputs := namedGrowth(eid)
	h, _ := HashCanonical(map[string]any{"operation": "growth_rate", "inputs": inputs})
	g, e := s.CreateGrant(context.Background(), r.RunID, task, GrantIn{RequestID: "r2-units", ToolName: "calculate_metric", ArgsHash: h})
	if e != nil {
		t.Fatal(e)
	}
	out, e := s.CalculateMetric(context.Background(), g.ID, "growth_rate", inputs)
	if e == nil {
		t.Fatalf("growth computed across CNY/USD without conversion: %v", out)
	}
}

func TestR2PolicyTimeoutApplied(t *testing.T) {
	s := setup(t)
	_, e := NewConfigService(s.DB).PatchPolicy(context.Background(), PolicyView{RunTimeoutSeconds: 10, QueueTimeoutSeconds: 1})
	if e != nil {
		t.Fatal(e)
	}
	r := createMinimalRun(t, s, 1001, "r2-timeout")
	o := NewOrchestrator(s, NewEvidenceService(s.DB))
	_, e = o.freeze(context.Background(), r.RunID)
	if e != nil {
		t.Fatal(e)
	}
	run, _ := s.loadRun(r.RunID)
	if run.DeadlineAt.Sub(*run.StartedAt) != 10*time.Second {
		t.Fatalf("frozen policy says 10s, executor gives %s", run.DeadlineAt.Sub(*run.StartedAt))
	}
}

type r2CancelFail struct{ *FakeResearchClient }

func (c *r2CancelFail) Cancel(context.Context, string) (TaskSnapshot, error) {
	return TaskSnapshot{}, fmt.Errorf("worker temporarily unreachable")
}

func TestR2CancelMustWaitForAckOrLeaseExpiry(t *testing.T) {
	s := setup(t)
	s.Client = &r2CancelFail{NewFakeResearchClient()}
	r := createMinimalRun(t, s, 1001, "r2-cancel")
	_, e := NewOrchestrator(s, NewEvidenceService(s.DB)).freeze(context.Background(), r.RunID)
	if e != nil {
		t.Fatal(e)
	}
	v, e := s.CancelResearch(ctxUser(1001), r.RunID)
	if e != nil {
		t.Fatal(e)
	}
	if v.Status == StatusCanceled {
		t.Fatal("canceled and slot freed although both worker cancellations failed and lease has not expired")
	}
}

func TestR2CompletedRunRejectsNewTools(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "r2-late-tool")
	task := reviewTaskID(t, s, r.RunID)
	token, e := s.IssueTaskToken(r.RunID, task, "research")
	if e != nil {
		t.Fatal(e)
	}
	s.DB.Model(&modelfinance.ResearchRun{}).Where("id = ?", r.RunID).Update("status", StatusCompleted)
	_, e = s.CreateGrant(WithTaskToken(context.Background(), token), r.RunID, task, GrantIn{RequestID: "r2-late-tool", ToolName: "get_financials", ArgsHash: mustArgsHash(t, map[string]any{})})
	if e == nil {
		t.Fatal("terminal run still authorizes fresh tool grants")
	}
}

func TestR2MalformedReportCannotPublish(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "r2-schema")
	o := NewOrchestrator(s, NewEvidenceService(s.DB))
	st, e := o.freeze(context.Background(), r.RunID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = o.verify(context.Background(), wfState{RunID: r.RunID, FenceVersion: st.FenceVersion, Report: VerifiedReport{SchemaVersion: "0.9", QualityStatus: "completed", Mode: ModeFixture, AsOf: time.Now()}})
	v, _ := s.GetResearch(ctxUser(1001), r.RunID)
	if v.Report != nil {
		t.Fatalf("failed repair published structurally invalid report: schema=%s run_id=%q version=%d", v.Report.SchemaVersion, v.Report.RunID, v.Report.Version)
	}
	if v.Status == StatusCompleted {
		t.Fatalf("malformed report marked completed: %s", v.Status)
	}
	if e == nil && v.Status != StatusFailed && v.Status != StatusIncomplete {
		t.Fatalf("expected failed/incomplete after repair miss, got %s", v.Status)
	}
}

func TestR2DeletePurgesExecutorCopies(t *testing.T) {
	s := setup(t)
	fake := NewFakeResearchClient()
	s.Client = fake
	r := createMinimalRun(t, s, 1001, "r2-purge")
	if _, e := s.DeleteResearch(ctxUser(1001), r.RunID); e != nil {
		t.Fatal(e)
	}
	var tasks []modelfinance.ResearchTask
	if e := s.DB.Where("run_id = ?", r.RunID).Find(&tasks).Error; e != nil {
		t.Fatal(e)
	}
	purged := map[string]bool{}
	for _, id := range fake.PurgedIDs() {
		purged[id] = true
	}
	if len(tasks) == 0 {
		t.Fatal("expected tasks to purge")
	}
	for _, task := range tasks {
		if !purged[task.ID] {
			t.Fatalf("python private copy not purged for %s", task.ID)
		}
	}
}

func TestR2ForeignResultCannotPublish(t *testing.T) {
	s := setup(t)
	a := createMinimalRun(t, s, 1001, "r2-result-a")
	b := createMinimalRun(t, s, 1002, "r2-result-b")
	o := NewOrchestrator(s, NewEvidenceService(s.DB))
	st, e := o.freeze(context.Background(), a.RunID)
	if e != nil {
		t.Fatal(e)
	}
	st.Supporter = &ResearchResult{SchemaVersion: "1.0", RunID: b.RunID, TaskID: reviewTaskID(t, s, b.RunID), Status: "insufficient", Arguments: []Argument{{ClaimType: "assumption", Text: "user B private research context"}}}
	st.Challenger = &ResearchResult{SchemaVersion: "1.0", RunID: a.RunID, Status: "insufficient"}
	st, e = o.synthesize(context.Background(), st)
	if e != nil {
		return
	}
	_, e = o.verify(context.Background(), st)
	v, _ := s.GetResearch(ctxUser(1001), a.RunID)
	if e == nil && v.Report != nil && len(v.Report.Support) > 0 {
		t.Fatalf("user A report includes user B result: %+v", v.Report.Support)
	}
}
