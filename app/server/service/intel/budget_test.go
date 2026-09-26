package intel

import (
	"context"
	"testing"

	"zhigu/server/testdb"
)

func TestModelBudgetReservationIsAtomicAndBounded(t *testing.T) {
	db := testdb.Start(t)
	svc := NewService(db, []byte("intel-test-cookie-secret"), testFixtureDir(t))
	ctx := context.Background()
	scope, _, err := svc.CreateDemoSession(ctx, "boot-budget", "idem-budget", "body-budget")
	if err != nil {
		t.Fatal(err)
	}
	usageID, err := svc.ReserveModelUsage(ctx, scope, "job-a", 1, 1000)
	if err != nil {
		t.Fatal(err)
	}
	same, err := svc.ReserveModelUsage(ctx, scope, "job-a", 1, 1000)
	if err != nil || same != usageID {
		t.Fatalf("same reservation id=%s err=%v", same, err)
	}
	if err := svc.SettleModelUsage(ctx, usageID, 800, false); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZHIGU_INTEL_DAILY_TOKEN_LIMIT", "1200")
	if _, err := svc.ReserveModelUsage(ctx, scope, "job-b", 1, 1000); err == nil || ErrorCode(err) != "MODEL_BUDGET_EXCEEDED" {
		t.Fatalf("budget err=%v", err)
	}
}
