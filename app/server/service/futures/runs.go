package futures

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func CompleteRun(ctx context.Context, db *gorm.DB, identity Identity, runID string, generation int64, report datatypes.JSON) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run struct {
			Generation      int64
			CancelRequested bool
			Status          string
			LeaseUntil      *time.Time
			DeadlineAt      *time.Time
		}
		err := tx.Table(`futures_runs`).Clauses(clause.Locking{Strength: "UPDATE"}).
			Select(`generation, cancel_requested, status, lease_until, deadline_at`).
			Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, runID, identity.OwnerID, identity.Mode).
			First(&run).Error
		if err == gorm.ErrRecordNotFound {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if run.Generation != generation {
			return ErrRevisionConflict
		}
		now := time.Now().UTC()
		if run.CancelRequested || (run.Status != "running" && run.Status != "verifying") ||
			run.LeaseUntil == nil || !run.LeaseUntil.After(now) || run.DeadlineAt == nil || !run.DeadlineAt.After(now) {
			return ErrInvalidInput
		}
		var reportObject map[string]any
		if err := json.Unmarshal(report, &reportObject); err != nil || fmt.Sprint(reportObject["id"]) == "" {
			return ErrInvalidInput
		}
		if err := validateReportSchema(reportObject); err != nil {
			return err
		}
		reportID := fmt.Sprint(reportObject["id"])
		if err := tx.Exec(`INSERT INTO futures_reports(id,owner_id,mode,run_id,report_version,body,verified,created_at) VALUES(?,?,?,?,?,?,true,?)`,
			reportID, identity.OwnerID, identity.Mode, runID, 1, report, time.Now().UTC()).Error; err != nil {
			return err
		}
		evidenceIDs := stringSlice(reportObject["evidence_ids"])
		for _, recordID := range evidenceIDs {
			if err := tx.Exec(`INSERT INTO futures_evidence_links(id,owner_id,mode,run_id,record_id,created_at) VALUES(?,?,?,?,?,?) ON CONFLICT DO NOTHING`,
				"evidence_"+uuid.NewString(), identity.OwnerID, identity.Mode, runID, recordID, time.Now().UTC()).Error; err != nil {
				return err
			}
		}
		res := tx.Table(`futures_runs`).
			Where(`id=? AND owner_id=? AND mode=? AND generation=? AND status IN ('running','verifying') AND cancel_requested=false AND lease_until>? AND deleted_at IS NULL`, runID, identity.OwnerID, identity.Mode, generation, now).
			Updates(map[string]any{
				`status`: `succeeded`, `stage`: `complete`, `report`: report,
				`failure_code`: nil, `version`: gorm.Expr(`version + 1`), `updated_at`: time.Now().UTC(),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrRevisionConflict
		}
		return writeNotification(tx, identity, "run_succeeded", runID, 1, "run."+runID+".succeeded", time.Now().UTC())
	})
}

func CancelRun(ctx context.Context, db *gorm.DB, identity Identity, runID string) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var status string
		err := tx.Table(`futures_runs`).Clauses(clause.Locking{Strength: "UPDATE"}).
			Select(`status`).Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, runID, identity.OwnerID, identity.Mode).
			Scan(&status).Error
		if err != nil {
			return err
		}
		if status == "" {
			return ErrNotFound
		}
		if status == "succeeded" || status == "failed" || status == "canceled" {
			return nil
		}
		if status == "queued" {
			res := tx.Table(`futures_runs`).
				Where(`id=? AND owner_id=? AND mode=? AND status='queued' AND deleted_at IS NULL`, runID, identity.OwnerID, identity.Mode).
				Updates(map[string]any{`status`: `canceled`, `stage`: `complete`, `cancel_requested`: true, `lease_until`: nil, `lease_owner`: nil, `version`: gorm.Expr(`version + 1`), `updated_at`: time.Now().UTC()})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return ErrRevisionConflict
			}
			return nil
		}
		res := tx.Table(`futures_runs`).
			Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, runID, identity.OwnerID, identity.Mode).
			Updates(map[string]any{`cancel_requested`: true, `status`: `canceling`, `version`: gorm.Expr(`version + 1`), `updated_at`: time.Now().UTC()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrRevisionConflict
		}
		return nil
	})
}
