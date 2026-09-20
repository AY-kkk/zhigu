package finance

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"zhigu/server/testdb"
)

func TestQueueTimeoutFailsQueuedRun(t *testing.T) {
	db := testdb.Start(t)
	clk := NewFrozenClock(time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC))
	svc := NewService(db, NewFakeResearchClient(), NewMemoryBudget(), NewFixtureConfig())
	svc.Clock = clk
	seedUsers(t, db)
	created := createMinimalRun(t, svc, 1001, "q-timeout")
	clk.Advance(31 * time.Second)
	svc.tick(context.Background())
	view, err := svc.GetResearch(ctxUser(1001), created.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != StatusFailed {
		t.Fatalf("status %s", view.Status)
	}
}

func TestParseRateLimit(t *testing.T) {
	svc := setup(t)
	old := ParsePerMinuteMax
	ParsePerMinuteMax = 1
	t.Cleanup(func() { ParsePerMinuteMax = old })
	if _, err := svc.ParseClaim(ctxUser(1001), ParseInput{Text: "演示公司的收入增长能否支持未来一年股价上涨？这是一条超过二十个字的测试观点。"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ParseClaim(ctxUser(1001), ParseInput{Text: "演示公司的第二份观点同样超过二十个字符以便通过校验。"}); err == nil {
		t.Fatal("expected rate limit")
	}
}

func TestDailyResearchLimit(t *testing.T) {
	svc := setup(t)
	old := DailyResearchMax
	DailyResearchMax = 1
	t.Cleanup(func() { DailyResearchMax = old })
	d1 := parseDemo(t, svc, 1001)
	created, err := svc.CreateResearch(ctxUser(1001), "day-1", createReqFrom(t, d1))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DB.Exec("UPDATE finance_research_runs SET status='completed', stage='done' WHERE id=?", created.RunID).Error; err != nil {
		t.Fatal(err)
	}
	d2 := parseDemo(t, svc, 1001)
	_, err = svc.CreateResearch(ctxUser(1001), "day-2", createReqFrom(t, d2))
	if err == nil {
		t.Fatal("expected daily limit")
	}
	if ErrorCode(err) != "DAILY_RESEARCH_LIMIT" {
		t.Fatalf("code %s", ErrorCode(err))
	}
}

func TestGrantDataQueryEvidenceRoundTrip(t *testing.T) {
	svc := setup(t)
	created := createMinimalRun(t, svc, 1001, "grant-rt")
	var taskID string
	if err := svc.DB.Raw("SELECT id FROM finance_research_tasks WHERE run_id = ? AND role = ?", created.RunID, RoleSupporter).Scan(&taskID).Error; err != nil || taskID == "" {
		t.Fatalf("task: %v %s", err, taskID)
	}
	params := map[string]any{"metrics": []string{"revenue"}}
	argsHash, err := HashCanonical(params)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := svc.CreateGrant(context.Background(), created.RunID, taskID, GrantIn{
		RequestID: "req-fin-1", ToolName: "get_financials", ArgsHash: argsHash, RunID: created.RunID, TaskID: taskID,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := svc.DataQuery(context.Background(), grant.ID, "get_financials", params)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(payload)
	var rec EvidenceIn
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	ids, err := NewEvidenceService(svc.DB).Register(context.Background(), grant.ID, []EvidenceIn{rec})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("ids %v", ids)
	}
	if err := svc.CompleteGrant(context.Background(), grant.ID, "succeeded", rec.ContentHash); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerCompletesFixture(t *testing.T) {
	svc := setup(t)
	created := createMinimalRun(t, svc, 1001, "worker-eino")
	svc.tick(context.Background())
	view, err := svc.GetResearch(ctxUser(1001), created.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != StatusCompleted && view.Status != StatusIncomplete {
		t.Fatalf("status %s", view.Status)
	}
	if view.Report == nil {
		t.Fatal("expected published report")
	}
}

func TestAdminRunsRedacted(t *testing.T) {
	svc := setup(t)
	_ = createMinimalRun(t, svc, 1001, "admin-run")
	items, err := svc.AdminListRuns(WithUser(context.Background(), 1, "admin"), 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("expected runs")
	}
	for _, it := range items {
		if it.UserRef == "1001" || it.UserRef == "invitee" {
			t.Fatalf("leaked owner %s", it.UserRef)
		}
	}
	if _, err := svc.AdminListRuns(ctxUser(1001), 20); err == nil {
		t.Fatal("user must not list admin runs")
	}
}
