package finance

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	modelfinance "zhigu/server/model/finance"
)

func roleTaskID(t *testing.T, s *ResearchService, runID, role string) string {
	t.Helper()
	var task modelfinance.ResearchTask
	if err := s.DB.Where("run_id = ? AND role = ?", runID, role).Take(&task).Error; err != nil {
		t.Fatal(err)
	}
	return task.ID
}

func reviewTaskID(t *testing.T, s *ResearchService, runID string) string {
	return roleTaskID(t, s, runID, RoleSupporter)
}

func mustArgsHash(t *testing.T, params map[string]any) string {
	t.Helper()
	h, err := HashCanonical(params)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestResearchClientRequiresURL(t *testing.T) {
	t.Setenv("ZHIGU_RESEARCH_MODE", "")
	t.Setenv("ZHIGU_RESEARCH_URL", "")
	if _, err := NewHTTPResearchClient(); err == nil {
		t.Fatal("missing ZHIGU_RESEARCH_URL must fail outside unit-test mode")
	}
	t.Setenv("ZHIGU_RESEARCH_MODE", "unit-test")
	c, err := NewHTTPResearchClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*FakeResearchClient); !ok {
		t.Fatalf("unit-test mode must return fake client, got %T", c)
	}
}

func TestReviewCancelQueuedReleasesSlot(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "review-cancel")
	if _, e := s.CancelResearch(ctxUser(1001), r.RunID); e != nil {
		t.Fatal(e)
	}
	s.tick(context.Background())
	v, _ := s.GetResearch(ctxUser(1001), r.RunID)
	d := parseDemo(t, s, 1001)
	_, err := s.CreateResearch(ctxUser(1001), "review-next", createReqFrom(t, d))
	if v.Status != StatusCanceled || err != nil {
		t.Fatalf("cancel did not converge: status=%s next_run_error=%v", v.Status, err)
	}
}

func TestReviewDeleteActiveReleasesSlot(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "review-delete")
	if _, e := s.DeleteResearch(ctxUser(1001), r.RunID); e != nil {
		t.Fatal(e)
	}
	s.tick(context.Background())
	d := parseDemo(t, s, 1001)
	if _, err := s.CreateResearch(ctxUser(1001), "review-delete-next", createReqFrom(t, d)); err != nil {
		t.Fatalf("delete did not release slot: %v", err)
	}
}

func TestReviewCancelCannotBeOverwrittenBySynthesis(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "review-race")
	id := reviewTaskID(t, s, r.RunID)
	o := NewOrchestrator(s, NewEvidenceService(s.DB))
	if _, e := o.freeze(context.Background(), r.RunID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.CancelResearch(ctxUser(1001), r.RunID); e != nil {
		t.Fatal(e)
	}
	st := wfState{RunID: r.RunID, Supporter: &ResearchResult{TaskID: id, Status: "insufficient"}, Challenger: &ResearchResult{Status: "insufficient"}}
	st, e := o.synthesize(context.Background(), st)
	if e != nil {
		return
	}
	_, e = o.verify(context.Background(), st)
	v, _ := s.GetResearch(ctxUser(1001), r.RunID)
	if e == nil && v.Report != nil {
		t.Fatalf("CANCELED run published: status=%s quality=%s", v.Status, v.Report.QualityStatus)
	}
}

func TestReviewRejectedCitationMustNotPublish(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "review-cite")
	o := NewOrchestrator(s, NewEvidenceService(s.DB))
	st := wfState{
		RunID: r.RunID,
		Supporter: &ResearchResult{
			Status: "succeeded", TaskID: reviewTaskID(t, s, r.RunID), EvidenceIDs: []string{"ev_forged"},
			Arguments: []Argument{{ClaimType: "fact", Text: "Unsupported material fact", EvidenceIDs: []string{"ev_forged"}}},
		},
		Challenger: &ResearchResult{Status: "insufficient"},
	}
	st, e := o.synthesize(context.Background(), st)
	if e != nil {
		t.Fatal(e)
	}
	_, e = o.verify(context.Background(), st)
	if e != nil {
		return
	}
	v, _ := s.GetResearch(ctxUser(1001), r.RunID)
	if v.Report != nil && len(v.Report.Support) > 0 {
		t.Fatalf("rejected fact still published: quality=%s support=%+v", v.Report.QualityStatus, v.Report.Support)
	}
}

func TestReviewMetricForgeryRejected(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "review-metric")
	id := reviewTaskID(t, s, r.RunID)
	params := map[string]any{}
	h := mustArgsHash(t, params)
	g, e := s.CreateGrant(context.Background(), r.RunID, id, GrantIn{RequestID: "review-grant", ToolName: "get_financials", ArgsHash: h, RunID: r.RunID, TaskID: id})
	if e != nil {
		t.Fatal(e)
	}
	p, e := s.DataQuery(context.Background(), g.ID, "get_financials", params)
	if e != nil {
		t.Fatal(e)
	}
	ids, e := NewEvidenceService(s.DB).Register(context.Background(), g.ID, []string{"prec_forged"})
	if e == nil {
		t.Fatalf("forged record_id accepted: evidence_ids=%v records=%d", ids, len(p.Records))
	}
}

func TestReviewRoleToolLimitEnforced(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "review-budget")
	id := reviewTaskID(t, s, r.RunID)
	budget := NewDBBudget(s.DB)
	for i := 0; i < 7; i++ {
		_, e := budget.Reserve(context.Background(), BudgetRequest{RequestID: fmt.Sprintf("review-budget-%d", i), RunID: r.RunID, TaskID: id, OwnerID: 1001, Kind: "tool", Purpose: "research"})
		if i == 6 {
			if e == nil {
				t.Fatal("seventh tool call accepted for same role; role limit is six")
			}
			return
		}
		if e != nil {
			t.Fatal(e)
		}
	}
}

func TestReviewPolicyLoweringAffectsNewRun(t *testing.T) {
	s := setup(t)
	cfg := NewConfigService(s.DB)
	if _, e := cfg.PatchPolicy(context.Background(), PolicyView{MaxToolCalls: 1, MaxModelCalls: 1}); e != nil {
		t.Fatal(e)
	}
	r := createMinimalRun(t, s, 1001, "review-policy")
	id := reviewTaskID(t, s, r.RunID)
	budget := NewDBBudget(s.DB)
	for i := 0; i < 2; i++ {
		_, e := budget.Reserve(context.Background(), BudgetRequest{RequestID: fmt.Sprintf("review-policy-%d", i), RunID: r.RunID, TaskID: id, OwnerID: 1001, Kind: "tool", Purpose: "research"})
		if i == 1 && e == nil {
			t.Fatal("second tool call accepted despite admin policy max_tool_calls=1")
		}
		if i == 0 && e != nil {
			t.Fatal(e)
		}
	}
}

func TestReviewGrantCannotBindForeignTask(t *testing.T) {
	s := setup(t)
	a := createMinimalRun(t, s, 1001, "review-grant-a")
	b := createMinimalRun(t, s, 1002, "review-grant-b")
	foreign := reviewTaskID(t, s, b.RunID)
	g, e := s.CreateGrant(context.Background(), a.RunID, foreign, GrantIn{RequestID: "review-cross-grant", ToolName: "get_financials", ArgsHash: "x", RunID: a.RunID, TaskID: foreign})
	if e == nil {
		t.Fatalf("cross-user run/task combination accepted: run=%s task=%s grant=%s", a.RunID, foreign, g.ID)
	}
}

func TestReviewEmptyArgsHashRejected(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "review-empty-hash")
	id := reviewTaskID(t, s, r.RunID)
	if _, e := s.CreateGrant(context.Background(), r.RunID, id, GrantIn{RequestID: "review-empty-hash", ToolName: "get_financials", ArgsHash: "", RunID: r.RunID, TaskID: id}); e == nil {
		t.Fatal("empty args_hash accepted")
	}
}

func TestReviewRevokedGrantCannotQuery(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "review-grant-revoke")
	id := reviewTaskID(t, s, r.RunID)
	params := map[string]any{}
	h := mustArgsHash(t, params)
	g, e := s.CreateGrant(context.Background(), r.RunID, id, GrantIn{RequestID: "review-revoked", ToolName: "get_financials", ArgsHash: h, RunID: r.RunID, TaskID: id})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CancelResearch(ctxUser(1001), r.RunID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DataQuery(context.Background(), g.ID, "get_financials", params); e == nil {
		t.Fatal("revoked grant still executes data query after run cancellation")
	}
}

func TestReviewChangedParamsRejected(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "review-args")
	id := reviewTaskID(t, s, r.RunID)
	granted := map[string]any{"metrics": []string{"revenue"}}
	g, e := s.CreateGrant(context.Background(), r.RunID, id, GrantIn{RequestID: "review-args", ToolName: "get_financials", ArgsHash: mustArgsHash(t, granted), RunID: r.RunID, TaskID: id})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DataQuery(context.Background(), g.ID, "get_financials", map[string]any{"metrics": []string{"profit"}}); e == nil {
		t.Fatal("changed params accepted")
	}
}

func TestReviewConsumedGrantRejected(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "review-once")
	id := reviewTaskID(t, s, r.RunID)
	params := map[string]any{"metrics": []string{"revenue"}}
	h := mustArgsHash(t, params)
	g, e := s.CreateGrant(context.Background(), r.RunID, id, GrantIn{RequestID: "review-once", ToolName: "get_financials", ArgsHash: h, RunID: r.RunID, TaskID: id})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DataQuery(context.Background(), g.ID, "get_financials", params); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DataQuery(context.Background(), g.ID, "get_financials", params); e == nil {
		t.Fatal("consumed grant executed twice")
	}
}

type delayedSubmitClient struct {
	inner *FakeResearchClient
	delay time.Duration
}

func (d *delayedSubmitClient) Submit(ctx context.Context, task ResearchTask) (TaskReceipt, error) {
	select {
	case <-ctx.Done():
		return TaskReceipt{}, ctx.Err()
	case <-time.After(d.delay):
	}
	return d.inner.Submit(ctx, task)
}

func (d *delayedSubmitClient) Get(ctx context.Context, taskID string) (TaskSnapshot, error) {
	return d.inner.Get(ctx, taskID)
}

func (d *delayedSubmitClient) Cancel(ctx context.Context, taskID string) (TaskSnapshot, error) {
	return d.inner.Cancel(ctx, taskID)
}

func (d *delayedSubmitClient) Purge(ctx context.Context, taskID string) error {
	return d.inner.Purge(ctx, taskID)
}

func TestLeaseHeartbeatKeepsSlowTask(t *testing.T) {
	s := setup(t)
	inner := NewFakeResearchClient()
	s.Client = &delayedSubmitClient{inner: inner, delay: 20 * time.Second}
	r := createMinimalRun(t, s, 1001, "lease-hb")
	o := NewOrchestrator(s, NewEvidenceService(s.DB))
	if _, err := o.freeze(context.Background(), r.RunID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				s.reapExpiredLeases(ctx, s.Clock.Now())
			}
		}
	}()
	if _, err := o.research(ctx, wfState{RunID: r.RunID}); err != nil {
		t.Fatal(err)
	}
	run, err := s.loadRun(r.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status == StatusIncomplete && run.Stage == "lease_lost" {
		t.Fatal("slow task reaped despite heartbeat")
	}
}

func TestRolesRunConcurrently(t *testing.T) {
	s := setup(t)
	probe := &parallelClient{FakeResearchClient: NewFakeResearchClient()}
	s.Client = probe
	r := createMinimalRun(t, s, 1001, "parallel-roles")
	o := NewOrchestrator(s, NewEvidenceService(s.DB))
	if _, err := o.freeze(context.Background(), r.RunID); err != nil {
		t.Fatal(err)
	}
	if _, err := o.research(context.Background(), wfState{RunID: r.RunID}); err != nil {
		t.Fatal(err)
	}
	if probe.maxInflight() < 2 {
		t.Fatalf("roles were serial; max inflight=%d", probe.maxInflight())
	}
}

type parallelClient struct {
	*FakeResearchClient
	mu        sync.Mutex
	inflight  int
	maxSeen   int
}

func (p *parallelClient) Submit(ctx context.Context, task ResearchTask) (TaskReceipt, error) {
	p.mu.Lock()
	p.inflight++
	if p.inflight > p.maxSeen {
		p.maxSeen = p.inflight
	}
	p.mu.Unlock()
	time.Sleep(40 * time.Millisecond)
	rec, err := p.FakeResearchClient.Submit(ctx, task)
	p.mu.Lock()
	p.inflight--
	p.mu.Unlock()
	return rec, err
}

func (p *parallelClient) maxInflight() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.maxSeen
}

func TestConfigStartTestIsFormatOnly(t *testing.T) {
	s := setup(t)
	cfg := NewConfigService(s.DB)
	got, err := cfg.SaveModel(context.Background(), map[string]any{"base_url": "https://api.openai.com/v1", "model": "m", "protocol": "openai-chat-completions"}, "k")
	if err != nil {
		t.Fatal(err)
	}
	out, err := cfg.StartTest(context.Background(), got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if out["status"] == "passed" {
		t.Fatal("format-only test must not be labeled passed")
	}
	loaded, _ := cfg.Get(context.Background(), got.ID)
	if loaded.TestStatus == "passed" {
		t.Fatal("saved config marked passed without connection test")
	}
}

func TestRetentionClearsPrivateCopies(t *testing.T) {
	s := setup(t)
	clk := NewFrozenClock(time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC))
	s.Clock = clk
	r := createMinimalRun(t, s, 1001, "retention")
	id := reviewTaskID(t, s, r.RunID)
	params := map[string]any{}
	h := mustArgsHash(t, params)
	g, err := s.CreateGrant(context.Background(), r.RunID, id, GrantIn{RequestID: "ret-g", ToolName: "get_financials", ArgsHash: h, RunID: r.RunID, TaskID: id})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := s.DataQuery(context.Background(), g.ID, "get_financials", params)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := NewEvidenceService(s.DB).Register(context.Background(), g.ID, issuedIDs(payload))
	if err != nil {
		t.Fatal(err)
	}
	now := s.Clock.Now()
	until := now.Add(LeaseTTL)
	if err := s.DB.Model(&modelfinance.ResearchRun{}).Where("id = ?", r.RunID).Updates(map[string]any{
		"status": StatusVerifying, "stage": "verifying", "lease_owner": "zhigu-worker", "lease_until": until,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.Publish(context.Background(), r.RunID, 1, sampleReport(r.RunID)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteResearch(ctxUser(1001), r.RunID); err != nil {
		t.Fatal(err)
	}
	clk.Advance(25 * time.Hour)
	if err := s.PurgeRetention(context.Background()); err != nil {
		t.Fatal(err)
	}
	var ev modelfinance.Evidence
	if err := s.DB.Where("id = ?", ids[0]).Take(&ev).Error; err != nil {
		t.Fatal(err)
	}
	if ev.Text != "" {
		t.Fatal("evidence text retained")
	}
	var report modelfinance.Report
	if err := s.DB.Where("run_id = ?", r.RunID).Take(&report).Error; err == nil && len(report.Body) > 2 {
		t.Fatalf("report body retained: %s", report.Body)
	}
	var recRow modelfinance.DataRecord
	if err := s.DB.Where("grant_id = ?", g.ID).Take(&recRow).Error; err == nil && len(recRow.Payload) > 2 {
		t.Fatalf("data record payload retained")
	}
}
