package futures

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestBudgetConcurrentLastYuanHasOneWinnerAndUnknownUsageIsNotReleased(t *testing.T) {
	db := repoTestDB(t)
	budget := NewDBBudget(db)
	ctx := context.Background()
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	if err := budget.SeedAccount(ctx, UserAccount, 1, day, decimal.RequireFromString("1"), 8, 24, 48000); err != nil {
		t.Fatal(err)
	}
	if err := budget.SeedAccount(ctx, ModuleAccount, 0, day, decimal.RequireFromString("50"), 8, 24, 48000); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	type reserveResult struct {
		attempt string
		err     error
	}
	results := make(chan reserveResult, 2)
	var wg sync.WaitGroup
	for _, attempt := range []string{"attempt-a", "attempt-b"} {
		wg.Add(1)
		go func(attempt string) {
			defer wg.Done()
			<-start
			_, err := budget.Reserve(ctx, Reservation{AttemptID: attempt, OwnerID: 1, RunID: "run", BudgetDay: day, AmountCNY: decimal.RequireFromString("1"), Tokens: 1, Kind: "model"})
			results <- reserveResult{attempt: attempt, err: err}
		}(attempt)
	}
	close(start)
	wg.Wait()
	close(results)
	successes, winner := 0, ""
	for result := range results {
		if result.err == nil {
			successes++
			winner = result.attempt
		} else if !errors.Is(result.err, ErrQuotaExceeded) {
			t.Fatalf("unexpected reserve error: %v", result.err)
		}
	}
	if successes != 1 {
		t.Fatalf("successes=%d want 1", successes)
	}
	if err := budget.SettleUnknown(ctx, 1, winner, decimal.RequireFromString("1"), 1); err != nil {
		t.Fatal(err)
	}
	account, err := budget.Account(ctx, UserAccount, 1, day)
	if err != nil || account.ReservedCNY.Equal(decimal.Zero) {
		t.Fatalf("unknown usage must retain reservation account=%+v err=%v", account, err)
	}
}
