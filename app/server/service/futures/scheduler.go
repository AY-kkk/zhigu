package futures

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	model "zhigu/server/model/futures"
)

const (
	futuresLeaseDuration    = 30 * time.Second
	futuresExecutionTimeout = 180 * time.Second
)

func ClaimQueuedRun(ctx context.Context, db *gorm.DB, leaseOwner string, now time.Time) (*model.Run, error) {
	var run model.Run
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(7250146202645)`).Error; err != nil {
			return err
		}
		var active int64
		if err := tx.Raw(`SELECT count(*) FROM futures_runs WHERE status IN ('running','verifying') AND deleted_at IS NULL`).Scan(&active).Error; err != nil {
			return err
		}
		if active >= 2 {
			return ErrQuotaExceeded
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where(`status='queued' AND deleted_at IS NULL`).Order(`created_at`).First(&run).Error; err != nil {
			return err
		}
		leaseUntil := now.Add(futuresLeaseDuration)
		deadline := now.Add(futuresExecutionTimeout)
		res := tx.Model(&model.Run{}).Where(`id=? AND status='queued' AND generation=?`, run.ID, run.Generation).Updates(map[string]any{
			`status`: `running`, `stage`: `evidence`, `lease_until`: leaseUntil, `lease_owner`: leaseOwner, `deadline_at`: deadline,
			`generation`: gorm.Expr(`generation + 1`), `version`: gorm.Expr(`version + 1`), `updated_at`: now,
		})
		if res.RowsAffected != 1 {
			return ErrRevisionConflict
		}
		run.Status, run.Stage = "running", "evidence"
		run.LeaseUntil, run.LeaseOwner, run.DeadlineAt = &leaseUntil, &leaseOwner, &deadline
		run.Generation++
		return nil
	})
	if err == gorm.ErrRecordNotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func RenewRunLease(ctx context.Context, db *gorm.DB, identity Identity, runID, leaseOwner string, generation int64, now time.Time) error {
	leaseUntil := now.Add(futuresLeaseDuration)
	res := db.WithContext(ctx).Table(`futures_runs`).
		Where(`id=? AND owner_id=? AND mode=? AND status IN ('running','verifying') AND lease_owner=? AND generation=? AND deleted_at IS NULL`, runID, identity.OwnerID, identity.Mode, leaseOwner, generation).
		Updates(map[string]any{`lease_until`: leaseUntil, `updated_at`: now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	return nil
}

func SetRunStage(ctx context.Context, db *gorm.DB, identity Identity, runID string, generation int64, stage string) error {
	if !oneOfRunStage(stage) {
		return ErrInvalidInput
	}
	res := db.WithContext(ctx).Table(`futures_runs`).
		Where(`id=? AND owner_id=? AND mode=? AND generation=? AND status IN ('running','verifying') AND deleted_at IS NULL`, runID, identity.OwnerID, identity.Mode, generation).
		Updates(map[string]any{`stage`: stage, `updated_at`: time.Now().UTC()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	return nil
}

func FailRun(ctx context.Context, db *gorm.DB, identity Identity, runID string, generation int64, code string) error {
	if code == "" {
		return ErrInvalidInput
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Table(`futures_runs`).
			Where(`id=? AND owner_id=? AND mode=? AND generation=? AND status IN ('running','verifying') AND cancel_requested=false AND deleted_at IS NULL`, runID, identity.OwnerID, identity.Mode, generation).
			Updates(map[string]any{`status`: `failed`, `stage`: `complete`, `failure_code`: code, `lease_until`: nil, `lease_owner`: nil, `updated_at`: time.Now().UTC()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrRevisionConflict
		}
		return writeNotification(tx, identity, "run_failed", runID, 1, "run."+runID+".failed", time.Now().UTC())
	})
}

func AcknowledgeCanceledRun(ctx context.Context, db *gorm.DB, identity Identity, runID string) error {
	res := db.WithContext(ctx).Table(`futures_runs`).
		Where(`id=? AND owner_id=? AND mode=? AND status='canceling' AND deleted_at IS NULL`, runID, identity.OwnerID, identity.Mode).
		Updates(map[string]any{`status`: `canceled`, `stage`: `complete`, `lease_until`: nil, `lease_owner`: nil, `updated_at`: time.Now().UTC()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	return nil
}

func ExpireStaleRuns(ctx context.Context, db *gorm.DB, now time.Time) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table(`futures_runs`).Where(`status='queued' AND deadline_at<? AND deleted_at IS NULL`, now).
			Updates(map[string]any{`status`: `failed`, `stage`: `complete`, `failure_code`: `FUTURES_QUEUE_TIMEOUT`, `lease_until`: nil, `lease_owner`: nil, `updated_at`: now}).Error; err != nil {
			return err
		}
		if err := tx.Table(`futures_runs`).Where(`status='canceling' AND (lease_until<? OR deadline_at<?) AND deleted_at IS NULL`, now, now).
			Updates(map[string]any{`status`: `canceled`, `stage`: `complete`, `lease_until`: nil, `lease_owner`: nil, `updated_at`: now}).Error; err != nil {
			return err
		}
		return tx.Table(`futures_runs`).Where(`status IN ('running','verifying') AND (lease_until<? OR deadline_at<?) AND deleted_at IS NULL`, now, now).
			Updates(map[string]any{`status`: `failed`, `stage`: `complete`, `failure_code`: `FUTURES_EXECUTION_TIMEOUT`, `lease_until`: nil, `lease_owner`: nil, `updated_at`: now}).Error
	})
}

func oneOfRunStage(stage string) bool {
	switch stage {
	case "evidence", "support", "challenge", "verify", "complete":
		return true
	default:
		return false
	}
}
