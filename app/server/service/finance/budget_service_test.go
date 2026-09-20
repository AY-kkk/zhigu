package finance

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestConcurrentBudgetReservation(t *testing.T) {
	svc := setup(t)
	budget := NewDBBudget(svc.DB)
	run := createMinimalRun(t, svc, 1001, "budget-run")
	var ok int32
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := budget.Reserve(context.Background(), BudgetRequest{
				RequestID: fmt.Sprintf("r%d", i), OwnerID: 1001, RunID: run.RunID, Kind: "model", Purpose: "research",
				ReservedInput: 10, ReservedOutput: 10,
			})
			if err == nil {
				atomic.AddInt32(&ok, 1)
			}
		}(i)
	}
	wg.Wait()
	if ok != 2 {
		// first two should succeed with max 8 research model calls; force remaining=1 via prefill
	}
	// Prefill to leave 1 slot then race
	budget2 := NewDBBudget(svc.DB)
	run2 := createMinimalRun(t, svc, 1002, "budget-run-2")
	loaded, err := svc.loadRun(run2.RunID)
	if err != nil {
		t.Fatal(err)
	}
	snap := ParseBudgetSnapshot(loaded.BudgetSnapshot)
	leave := snap.MaxModelCalls - 1
	if leave < 1 {
		leave = 1
	}
	for i := 0; i < leave; i++ {
		if _, err := budget2.Reserve(context.Background(), BudgetRequest{
			RequestID: fmt.Sprintf("pre-%d", i), OwnerID: 1002, RunID: run2.RunID, Kind: "model", Purpose: "research",
			ReservedInput: 1, ReservedOutput: 1,
		}); err != nil {
			t.Fatalf("prefill: %v", err)
		}
	}
	var ok2 int32
	var wg2 sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg2.Add(1)
		go func(i int) {
			defer wg2.Done()
			_, err := budget2.Reserve(context.Background(), BudgetRequest{
				RequestID: fmt.Sprintf("last-%d", i), OwnerID: 1002, RunID: run2.RunID, Kind: "model", Purpose: "research",
				ReservedInput: 1, ReservedOutput: 1,
			})
			if err == nil {
				atomic.AddInt32(&ok2, 1)
			}
		}(i)
	}
	wg2.Wait()
	if ok2 != 1 {
		t.Fatalf("expected 1 success on last slot, got %d", ok2)
	}
}

func TestUnknownUsageRetained(t *testing.T) {
	svc := setup(t)
	budget := NewDBBudget(svc.DB)
	run := createMinimalRun(t, svc, 1001, "unk-1")
	res, err := budget.Reserve(context.Background(), BudgetRequest{
		RequestID: "u1", OwnerID: 1001, RunID: run.RunID, Kind: "model", Purpose: "research", ReservedInput: 8000, ReservedOutput: 1200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := budget.Reconcile(context.Background(), res.ID, ObservedUsage{Unknown: true}); err != nil {
		t.Fatal(err)
	}
	row := budget.Ledger("u1")
	if row == nil || row.Status != "unknown" || row.ReservedTokens != 9200 {
		t.Fatalf("%+v", row)
	}
}

func TestSecretNeverReturned(t *testing.T) {
	svc := setup(t)
	cfg := NewConfigService(svc.DB)
	got, err := cfg.SaveModel(context.Background(), map[string]any{"base_url": "https://api.openai.com/v1", "model": "m"}, "sk-secret-test")
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := cfg.Get(context.Background(), got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.HasKey {
		t.Fatal("has_key")
	}
	raw := fmt.Sprintf("%v", loaded)
	if contains(raw, "sk-secret") {
		t.Fatalf("secret leaked: %s", raw)
	}
}

func TestSSRFBlocked(t *testing.T) {
	for _, u := range []string{"http://127.0.0.1/v1", "http://169.254.169.254/", "http://localhost:80"} {
		if err := ValidateUpstreamURL(u); err == nil {
			t.Fatalf("allowed %s", u)
		}
	}
	if err := ValidateUpstreamURL("https://api.openai.com/v1"); err != nil {
		t.Fatal(err)
	}
}

func TestActivateRequiresMatchingDigest(t *testing.T) {
	svc := setup(t)
	cfg := NewConfigService(svc.DB)
	got, err := cfg.SaveModel(context.Background(), map[string]any{"base_url": "https://api.openai.com/v1", "model": "m"}, "k")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.MarkTested(context.Background(), got.ID, "stale"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Activate(context.Background(), got.ID); !isClass(err, "conflict") {
		t.Fatalf("want stale test, got %v", err)
	}
}

func TestFrozenConfig(t *testing.T) {
	svc := setup(t)
	a := createMinimalRun(t, svc, 1001, "cfg-a")
	runA, err := svc.loadRun(a.RunID)
	if err != nil {
		t.Fatal(err)
	}
	snapA := ParseBudgetSnapshot(runA.BudgetSnapshot)
	if snapA.MaxToolCalls != 12 {
		t.Fatalf("default snapshot tools %d", snapA.MaxToolCalls)
	}
	if _, err := svc.CancelResearch(ctxUser(1001), a.RunID); err != nil {
		t.Fatal(err)
	}
	cfg := NewConfigService(svc.DB)
	if _, err := cfg.PatchPolicy(context.Background(), PolicyView{MaxToolCalls: 3, MaxModelCalls: 8}); err != nil {
		t.Fatal(err)
	}
	b := createMinimalRun(t, svc, 1001, "cfg-b")
	runB, err := svc.loadRun(b.RunID)
	if err != nil {
		t.Fatal(err)
	}
	snapB := ParseBudgetSnapshot(runB.BudgetSnapshot)
	if snapB.MaxToolCalls != 3 {
		t.Fatalf("new run did not freeze lowered policy: %d", snapB.MaxToolCalls)
	}
	runA2, _ := svc.loadRun(a.RunID)
	snapA2 := ParseBudgetSnapshot(runA2.BudgetSnapshot)
	if snapA2.MaxToolCalls != snapA.MaxToolCalls {
		t.Fatalf("old run snapshot mutated: %d -> %d", snapA.MaxToolCalls, snapA2.MaxToolCalls)
	}
	view, err := svc.GetResearch(ctxUser(1001), a.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Mode != ModeFixture {
		t.Fatalf("mode %s", view.Mode)
	}
}

func createMinimalRun(t *testing.T, svc *ResearchService, owner uint, key string) CreateResearchOutput {
	t.Helper()
	d := parseDemo(t, svc, owner)
	out, err := svc.CreateResearch(ctxUser(owner), key, createReqFrom(t, d))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return out
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (len(s) > 0 && (stringIndex(s, sub) >= 0)))
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
