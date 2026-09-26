package finance

import (
	"context"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	modelfinance "zhigu/server/model/finance"
)

func TestStageBConfigCapabilityActivation(t *testing.T) {
	s := setup(t)
	cfg, err := NewConfigService(s.DB).SaveModel(context.Background(), map[string]any{
		"protocol": ProtocolChatCompletions,
		"base_url": "https://api.openai.com/v1",
		"model":    "finance-research",
	}, "sk-test")
	if err != nil {
		t.Fatal(err)
	}
	out, err := NewConfigService(s.DB).StartTest(context.Background(), cfg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if out["status"] == "passed" {
		t.Fatal("capability test must not pass without an upstream roundtrip")
	}
	if err := NewConfigService(s.DB).Activate(context.Background(), cfg.ID); err == nil {
		t.Fatal("untested config must not activate")
	}
}

func TestStageBConfigSnapshotIsolation(t *testing.T) {
	s := setup(t)
	first := createMinimalRun(t, s, 1001, "snap-old")
	var before modelfinance.ResearchRun
	if err := s.DB.Where("id = ?", first.RunID).Take(&before).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(before.ConfigVersions), "model_config_id") {
		t.Fatal("run created before a model config must not invent one")
	}
	activateTestModel(t, s.DB)
	if err := s.DB.Model(&modelfinance.ResearchRun{}).Where("id = ?", first.RunID).Update("status", StatusCanceled).Error; err != nil {
		t.Fatal(err)
	}
	second := createMinimalRun(t, s, 1001, "snap-new")
	var oldAgain modelfinance.ResearchRun
	if err := s.DB.Where("id = ?", first.RunID).Take(&oldAgain).Error; err != nil {
		t.Fatal(err)
	}
	if string(oldAgain.ConfigVersions) != string(before.ConfigVersions) {
		t.Fatal("activating a new model rewrote the frozen config of the old run")
	}
	var created modelfinance.ResearchRun
	if err := s.DB.Where("id = ?", second.RunID).Take(&created).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(created.ConfigVersions), "model_config_id") {
		t.Fatal("new run did not freeze the active model config")
	}
}

func TestStageBGrantScopeAndSingleExecution(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "grant-scope")
	task := reviewTaskID(t, s, r.RunID)
	args := mustArgsHash(t, map[string]any{"metrics": []string{"revenue"}})
	first, err := s.CreateGrant(context.Background(), r.RunID, task, GrantIn{RequestID: "grant-scope-1", ToolName: "get_financials", ArgsHash: args})
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.CreateGrant(context.Background(), r.RunID, task, GrantIn{RequestID: "grant-scope-1", ToolName: "get_financials", ArgsHash: args})
	if err != nil || again.ID != first.ID {
		t.Fatalf("same request must return the same grant: %v %+v", err, again)
	}
	if _, err := s.CreateGrant(context.Background(), r.RunID, task, GrantIn{RequestID: "grant-scope-1", ToolName: "search_filings", ArgsHash: args}); ErrorCode(err) != "GRANT_PAYLOAD_CONFLICT" {
		t.Fatalf("changed tool: %v", err)
	}
}

func TestStageBEvidenceRecordHandleIntegrity(t *testing.T) {
	left := ProviderRecord{Text: "营业收入 100", Metrics: []Metric{{Metric: "revenue", Value: "100", Unit: "CNY", ValueType: "actual"}}}
	right := left
	right.Metrics = []Metric{{Metric: "revenue", Value: "101", Unit: "CNY", ValueType: "actual"}}
	h1, err := HashCanonical(left)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := HashCanonical(right)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Fatal("same text with a changed metric must not keep the same record hash")
	}
}

func TestStageBRequestIdempotencyAndUnknown(t *testing.T) {
	s := setup(t)
	draft := parseDemo(t, s, 1001)
	body := createReqFrom(t, draft)
	a, err := s.CreateResearch(ctxUser(1001), "idem-matrix", body)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateResearch(ctxUser(1001), "idem-matrix", body)
	if err != nil || a.RunID != b.RunID {
		t.Fatalf("same body must reuse the run: %v %s %s", err, a.RunID, b.RunID)
	}
	other := body
	other.ParentRunID = "run_other"
	if _, err := s.CreateResearch(ctxUser(1001), "idem-matrix", other); ErrorCode(err) != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("different body: %v", err)
	}
}

func TestStageBRepairOnceAndRejectedDraftHidden(t *testing.T) {
	s := setup(t)
	or := NewOrchestrator(s, NewEvidenceService(s.DB))
	run := createMinimalRun(t, s, 1001, "repair-hidden")
	st := wfState{RunID: run.RunID, Report: VerifiedReport{SchemaVersion: "0.9", Summary: "", QualityStatus: "completed"}}
	_, _ = or.verify(context.Background(), st)
	if or.RepairN != 1 {
		t.Fatalf("repair count %d", or.RepairN)
	}
	st.Repaired = false
	_, _ = or.verify(context.Background(), st)
	if or.RepairN != 1 {
		t.Fatal("repair ran more than once")
	}
	view, err := s.GetResearch(ctxUser(1001), run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Report != nil {
		t.Fatal("rejected draft was visible")
	}
}

func TestStageBParallelRolesAndOutcomeSemantics(t *testing.T) {
	s := setup(t)
	or := NewOrchestrator(s, NewEvidenceService(s.DB))
	run := createMinimalRun(t, s, 1001, "outcome")
	sup := roleTaskID(t, s, run.RunID, RoleSupporter)
	chal := roleTaskID(t, s, run.RunID, RoleChallenger)
	st := wfState{
		RunID: run.RunID,
		Supporter: &ResearchResult{
			SchemaVersion: "1.0", RunID: run.RunID, TaskID: sup, Status: "insufficient",
			Unknowns: []string{"无法取数：缺少足以判断该主张的已披露资料。"},
		},
		Challenger: &ResearchResult{
			SchemaVersion: "1.0", RunID: run.RunID, TaskID: chal, Status: "insufficient",
			Unknowns: []string{"无法取数：缺少足以判断该主张的已披露资料。"},
		},
	}
	out, err := or.synthesize(context.Background(), st)
	if err != nil {
		t.Fatal(err)
	}
	if out.Report.QualityStatus != "completed" || out.Report.Verdict == nil || *out.Report.Verdict != "insufficient" {
		t.Fatalf("both insufficient: %+v", out.Report)
	}
	other := createMinimalRun(t, s, 1002, "outcome-fail")
	failedState := wfState{
		RunID:        other.RunID,
		SupporterErr: NewError(503, "unavailable", "HARNESS_UNAVAILABLE", "支持方不可用"),
		Challenger: &ResearchResult{
			SchemaVersion: "1.0", RunID: other.RunID, TaskID: roleTaskID(t, s, other.RunID, RoleChallenger), Status: "insufficient",
			Unknowns: []string{"无法取数：缺少足以判断该主张的已披露资料。"},
		},
	}
	failed, err := or.synthesize(context.Background(), failedState)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Report.QualityStatus == "completed" || failed.Report.Verdict != nil {
		t.Fatalf("one side failed must not publish a verdict: %+v", failed.Report)
	}
}

func TestStageBAtomicBudgetAndBilling(t *testing.T) {
	budget := NewMemoryBudget()
	var last error
	for i := 0; i < 5; i++ {
		_, last = budget.Reserve(context.Background(), BudgetRequest{
			RequestID: "role-" + string(rune('a'+i)), RunID: "run_budget", TaskID: "task_budget", Kind: "model",
		})
	}
	if last == nil || ErrorCode(last) != "BUDGET_EXCEEDED" {
		t.Fatalf("5th model call on one role: %v", last)
	}
}

func TestStageBCancelLeaseAndLatePublish(t *testing.T) {
	s := setup(t)
	created := createMinimalRun(t, s, 1001, "late-publish")
	if _, err := s.CancelResearch(ctxUser(1001), created.RunID); err != nil {
		t.Fatal(err)
	}
	run, err := s.loadRun(created.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Publish(context.Background(), created.RunID, run.Version, sampleReport(created.RunID)); err == nil {
		t.Fatal("cancel must reject a late publish")
	}
}

func TestStageBDeadlineQueueAndConcurrency(t *testing.T) {
	s := setup(t)
	createMinimalRun(t, s, 1001, "queue-1")
	draft := parseDemo(t, s, 1001)
	if _, err := s.CreateResearch(ctxUser(1001), "queue-2", createReqFrom(t, draft)); ErrorCode(err) != "ACTIVE_RUN_EXISTS" && ErrorCode(err) != "DRAFT_ALREADY_CONFIRMED" {
		t.Fatalf("second active run: %v", err)
	}
}

func TestStageBInjectionSSRFAndSecretRedaction(t *testing.T) {
	if err := ValidateUpstreamURL("http://127.0.0.1:9/v1"); ErrorCode(err) != "SSRF_BLOCKED" {
		t.Fatalf("loopback model url: %v", err)
	}
	httpc := NewCountedHTTP(1, 0)
	if _, _, err := httpc.Get(context.Background(), "http://169.254.169.254/latest"); ErrorCode(err) != "SSRF_BLOCKED" {
		t.Fatalf("link local: %v", err)
	}
	if yuan, err := ScaleYuan("1", "万元"); err != nil || !yuan.Equal(decimal.NewFromInt(10000)) {
		t.Fatalf("scale drifted: %s %v", yuan, err)
	}
}

func TestStageBQuestionsEvidenceOnly(t *testing.T) {
	s := setup(t)
	created := createMinimalRun(t, s, 1001, "ask-1")
	openVerifying(t, s, created.RunID)
	if err := s.Publish(context.Background(), created.RunID, 1, sampleReport(created.RunID)); err != nil {
		t.Fatal(err)
	}
	var grants int64
	for i := 0; i < 3; i++ {
		out, err := s.Ask(ctxUser(1001), created.RunID, "ask-"+string(rune('a'+i)), "依据是什么")
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Limitations) == 0 || !strings.Contains(out.Limitations[0], "追问不调用外部工具") {
			t.Fatalf("question called out of the report: %+v", out)
		}
	}
	if _, err := s.Ask(ctxUser(1001), created.RunID, "ask-d", "再问一次"); ErrorCode(err) != "QUESTION_LIMIT" {
		t.Fatalf("fourth question: %v", err)
	}
	s.DB.Model(&modelfinance.ToolGrant{}).Where("run_id = ?", created.RunID).Count(&grants)
	if grants != 0 {
		t.Fatalf("questions created tool grants: %d", grants)
	}
	again, err := s.Ask(ctxUser(1001), created.RunID, "ask-a", "依据是什么")
	if err != nil || again.Answer == "" {
		t.Fatalf("replay: %v %+v", err, again)
	}
	var n int64
	s.DB.Model(&modelfinance.Question{}).Where("run_id = ?", created.RunID).Count(&n)
	if n != 3 {
		t.Fatalf("replay inserted another question: %d", n)
	}
}
