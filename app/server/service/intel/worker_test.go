package intel

import (
	"context"
	"testing"

	"zhigu/server/testdb"
)

func TestJobLeaseRenewAndEpochFencing(t *testing.T) {
	db := testdb.Start(t)
	svc := NewService(db, []byte("intel-test-cookie-secret"), testFixtureDir(t))
	ctx := context.Background()
	scope, _, err := svc.CreateDemoSession(ctx, "boot-worker", "idem-worker", "body-worker")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateIngestionJob(ctx, scope, IngestionJobRequest{Provider: "fixture", Codes: []string{"DEMO.A"}, From: "2026-09-01", To: "2026-09-02"}); err != nil {
		t.Fatal(err)
	}
	lease, err := svc.ClaimJob(ctx, "worker-a")
	if err != nil {
		t.Fatal(err)
	}
	if lease.LeaseEpoch != 1 || lease.Generation != scope.Generation {
		t.Fatalf("lease = %#v", lease)
	}
	if _, err := svc.ClaimJob(ctx, "worker-b"); err == nil {
		t.Fatal("leased job must not be claimable")
	}
	if err := svc.RenewJob(ctx, lease); err != nil {
		t.Fatal(err)
	}
	stale := lease
	stale.LeaseEpoch = 0
	if err := svc.CompleteJob(ctx, stale, "succeeded", 1, 1, 0, ""); err == nil || ErrorCode(err) != "VERSION_CONFLICT" {
		t.Fatalf("stale complete err=%v", err)
	}
	if err := svc.CompleteJob(ctx, lease, "partial", 1, 0, 1, "fixture job requires explicit replay"); err != nil {
		t.Fatal(err)
	}
}

func TestOutboxLeaseAndCompletionAreFenced(t *testing.T) {
	db := testdb.Start(t)
	svc := NewService(db, []byte("intel-test-cookie-secret"), testFixtureDir(t))
	ctx := context.Background()
	scope, _, err := svc.CreateDemoSession(ctx, "boot-outbox", "idem-outbox", "body-outbox")
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"DEMO.A", "DEMO.B"} {
		if _, err := svc.AddWatchlist(ctx, scope, code); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.ReplayAction(ctx, scope, ReplayRequest{Action: "step", ExpectedVersion: 0}); err != nil {
		t.Fatal(err)
	}
	lease, err := svc.ClaimOutbox(ctx, "outbox-a")
	if err != nil {
		t.Fatal(err)
	}
	stale := lease
	stale.LeaseEpoch = 0
	if err := svc.CompleteOutbox(ctx, stale, "sent"); err == nil || ErrorCode(err) != "VERSION_CONFLICT" {
		t.Fatalf("stale outbox err=%v", err)
	}
	if err := svc.CompleteOutbox(ctx, lease, "sent"); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := db.Raw(`SELECT status FROM finance_intel_outbox WHERE id=?`, lease.ID).Scan(&status).Error; err != nil {
		t.Fatal(err)
	}
	if status != "sent" {
		t.Fatalf("outbox status=%q", status)
	}
}
