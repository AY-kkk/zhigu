package futures

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/datatypes"
	model "zhigu/server/model/futures"
)

func TestRunIdempotencyGenerationAndCancellationRace(t *testing.T) {
	db := repoTestDB(t)
	repo := NewDBRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	identity := Identity{OwnerID: 11, Mode: "live"}
	start := make(chan struct{})
	results := make(chan *model.Run, 20)
	errs := make(chan error, 20)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			run := &model.Run{ID: "r_idem", OwnerID: 11, Mode: "live", DraftID: "d", DraftRevision: 1,
				Status: "queued", Stage: "queued", AsOf: now, HorizonEnd: now.Add(time.Hour),
				IdempotencyKey: "same", RequestHash: "hash", CreatedAt: now, UpdatedAt: now}
			err := repo.CreateRun(ctx, identity, run)
			if err != nil {
				errs <- err
				return
			}
			results <- run
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	count := 0
	for range results {
		count++
	}
	if count != 20 {
		t.Fatalf("idempotent responses=%d want 20", count)
	}
	var rows int64
	if err := db.Raw(`SELECT count(*) FROM futures_runs WHERE idempotency_key='same'`).Scan(&rows).Error; err != nil || rows != 1 {
		t.Fatalf("run rows=%d err=%v", rows, err)
	}
	conflict := &model.Run{ID: "r_conflict", OwnerID: 11, Mode: "live", DraftID: "d", DraftRevision: 1,
		Status: "queued", Stage: "queued", AsOf: now, HorizonEnd: now.Add(time.Hour),
		IdempotencyKey: "same", RequestHash: "different", CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateRun(ctx, identity, conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("hash conflict err=%v", err)
	}

	run, err := repo.GetRun(ctx, identity, "r_idem")
	if err != nil {
		t.Fatal(err)
	}
	report := datatypes.JSON(`{"schema_version":"futures.report.v1"}`)
	if err := CompleteRun(ctx, db, identity, run.ID, 2, report); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale generation err=%v", err)
	}
	if err := CancelRun(ctx, db, identity, run.ID); err != nil {
		t.Fatal(err)
	}
	if err := CompleteRun(ctx, db, identity, run.ID, run.Generation, report); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("late completion err=%v", err)
	}
	got, err := repo.GetRun(ctx, identity, run.ID)
	if err != nil || got.Report != nil || got.Status != "canceled" {
		t.Fatalf("cancel published report run=%+v err=%v", got, err)
	}
}
