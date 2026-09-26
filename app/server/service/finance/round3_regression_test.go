package finance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	modelfinance "zhigu/server/model/finance"
)

func TestR3DeleteRetainsSlotUntilExecutorAck(t *testing.T) {
	s := setup(t)
	s.Client = &r2CancelFail{NewFakeResearchClient()}
	r := createMinimalRun(t, s, 1001, "r3-delete")
	if _, e := NewOrchestrator(s, NewEvidenceService(s.DB)).freeze(context.Background(), r.RunID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DeleteResearch(ctxUser(1001), r.RunID); e != nil {
		t.Fatal(e)
	}
	run, _ := s.loadRun(r.RunID)
	if run.Status != StatusCanceling || run.LeaseUntil == nil {
		t.Fatalf("delete freed active slot without executor acknowledgment: status=%s lease=%v", run.Status, run.LeaseUntil)
	}
}

func TestR3CancelHTTP500IsNotAcknowledgment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"detail":"executor could not stop"}`))
	}))
	defer server.Close()
	s := setup(t)
	s.Client = &HTTPResearchClient{Base: server.URL, Client: server.Client()}
	r := createMinimalRun(t, s, 1001, "r3-http-cancel")
	if _, e := NewOrchestrator(s, NewEvidenceService(s.DB)).freeze(context.Background(), r.RunID); e != nil {
		t.Fatal(e)
	}
	v, e := s.CancelResearch(ctxUser(1001), r.RunID)
	if e != nil {
		t.Fatal(e)
	}
	if v.Status != StatusCanceling {
		t.Fatalf("HTTP 500 treated as cancel ack: status=%s", v.Status)
	}
}

func TestR3GrowthRejectsHalfYearVersusFullYear(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "r3-period")
	task := reviewTaskID(t, s, r.RunID)
	eid := r2Evidence(t, s, r.RunID, task, "r3-period-source")
	var ev modelfinance.Evidence
	if e := s.DB.Where("id = ?", eid).Take(&ev).Error; e != nil {
		t.Fatal(e)
	}
	var metrics []Metric
	_ = json.Unmarshal(ev.Metrics, &metrics)
	metrics[0].PeriodEnd = "2025-06-30"
	raw, _ := json.Marshal(metrics)
	if e := s.DB.Model(&ev).Update("metrics", string(raw)).Error; e != nil {
		t.Fatal(e)
	}
	inputs := namedGrowth(eid)
	h, _ := HashCanonical(map[string]any{"operation": "growth_rate", "inputs": inputs})
	g, e := s.CreateGrant(context.Background(), r.RunID, task, GrantIn{RequestID: "r3-period-calc", ToolName: "calculate_metric", ArgsHash: h})
	if e != nil {
		t.Fatal(e)
	}
	out, e := s.CalculateMetric(context.Background(), g.ID, "growth_rate", inputs)
	if e == nil {
		t.Fatalf("H1 compared to FY without rejection: %v", out)
	}
}

type r3InvalidResultClient struct {
	*FakeResearchClient
	MissingIdentity bool
}

func (c *r3InvalidResultClient) Submit(ctx context.Context, task ResearchTask) (TaskReceipt, error) {
	receipt, e := c.FakeResearchClient.Submit(ctx, task)
	c.mu.Lock()
	snap := c.tasks[task.TaskID]
	if c.MissingIdentity {
		snap.Result = &ResearchResult{Status: "insufficient", Unknowns: []string{"unknown content without any run/task identity"}}
	} else {
		snap.Result.SchemaVersion = "9.9"
		snap.Result.Arguments = []Argument{{ClaimType: "unsupported_fact", Text: "Uncited number 999 accepted as a claim", EvidenceIDs: []string{}}}
	}
	c.tasks[task.TaskID] = snap
	c.mu.Unlock()
	return receipt, e
}

func TestR3MalformedResultsCannotComplete(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "invalid-schema-and-claim-type", true: "missing-identities-with-unknowns"}[missing], func(t *testing.T) {
			s := setup(t)
			s.Client = &r3InvalidResultClient{FakeResearchClient: NewFakeResearchClient(), MissingIdentity: missing}
			r := createMinimalRun(t, s, 1001, "r3-schema")
			e := NewOrchestrator(s, NewEvidenceService(s.DB)).Run(context.Background(), r.RunID)
			v, _ := s.GetResearch(ctxUser(1001), r.RunID)
			if v.Status == StatusCompleted {
				t.Fatalf("malformed executor results reached completed report (err=%v): %+v", e, v.Report)
			}
		})
	}
}

func TestR3ModelCacheCannotCrossRun(t *testing.T) {
	s := setup(t)
	activateTestModel(t, s.DB)
	t.Setenv("ZHIGU_MODEL_FEE_CAP", "1")
	a := createMinimalRun(t, s, 1001, "r3-cache-a")
	b := createMinimalRun(t, s, 1002, "r3-cache-b")
	ta, e := s.IssueTaskToken(a.RunID, reviewTaskID(t, s, a.RunID), "research")
	if e != nil {
		t.Fatal(e)
	}
	tb, e := s.IssueTaskToken(b.RunID, reviewTaskID(t, s, b.RunID), "research")
	if e != nil {
		t.Fatal(e)
	}
	proxy := NewModelProxy(s.DB, NewDBBudget(s.DB))
	n := 0
	proxy.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		n++
		raw := fmt.Sprintf(`{"id":"chatcmpl_%d","choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`, n)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(raw)), Header: make(http.Header)}, nil
	})}
	body := []byte(`{"model":"finance-research","messages":[]}`)
	first, e := proxy.Complete(context.Background(), "r3-cache-shared-id", NormalizeJSONHash(body), HeaderTokenHash(ta), body, ProtocolChatCompletions)
	if e != nil {
		t.Fatal(e)
	}
	second, e := proxy.Complete(context.Background(), "r3-cache-shared-id", NormalizeJSONHash(body), HeaderTokenHash(tb), body, ProtocolChatCompletions)
	if e != nil {
		t.Fatal(e)
	}
	if n != 2 {
		t.Fatalf("cross-run request reused upstream, hits=%d", n)
	}
	if NormalizeJSONHash(first) == NormalizeJSONHash(second) {
		t.Fatal("user B received user A model response")
	}
	var count int64
	s.DB.Model(&modelfinance.ModelCache{}).Where("run_id = ?", b.RunID).Count(&count)
	if count != 1 {
		t.Fatalf("user B cache rows=%d", count)
	}
}

func TestR3ExpiredRunCannotCreateToolGrant(t *testing.T) {
	s := setup(t)
	r := createMinimalRun(t, s, 1001, "r3-deadline")
	task := reviewTaskID(t, s, r.RunID)
	token, e := s.IssueTaskToken(r.RunID, task, "research")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = NewOrchestrator(s, NewEvidenceService(s.DB)).freeze(context.Background(), r.RunID); e != nil {
		t.Fatal(e)
	}
	s.DB.Model(&modelfinance.ResearchRun{}).Where("id = ?", r.RunID).Update("deadline_at", time.Now().Add(-time.Minute))
	_, e = s.CreateGrant(WithTaskToken(context.Background(), token), r.RunID, task, GrantIn{RequestID: "r3-after-deadline", ToolName: "get_financials", ArgsHash: mustArgsHash(t, map[string]any{})})
	if e == nil {
		t.Fatal("new tool grant issued after frozen run deadline")
	}
}

func TestR3CreateRejectsStaleDraftRevision(t *testing.T) {
	s := setup(t)
	d := parseDemo(t, s, 1001)
	in := createReqFrom(t, d)
	in.Revision = d.Revision + 1
	_, e := s.CreateResearch(ctxUser(1001), "r3-stale-draft", in)
	if e == nil {
		t.Fatal("create accepted a stale draft revision")
	}
	if ErrorCode(e) != "DRAFT_REVISION" {
		t.Fatalf("code %s", ErrorCode(e))
	}
}
