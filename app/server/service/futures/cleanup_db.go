package futures

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DeletionJobValue struct {
	ID            string    `gorm:"column:id;primaryKey"`
	OwnerID       uint      `gorm:"column:owner_id"`
	Mode          string    `gorm:"column:mode"`
	TargetType    string    `gorm:"column:target_type"`
	TargetID      string    `gorm:"column:target_id"`
	ImpactVersion string    `gorm:"column:impact_version"`
	PurgeDueAt    time.Time `gorm:"column:purge_due_at"`
	EligibleAt    time.Time `gorm:"column:eligible_at"`
	RetryCount    int       `gorm:"column:retry_count"`
	LastError     *string   `gorm:"column:last_error"`
	State         string    `gorm:"column:state"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

func (DeletionJobValue) TableName() string { return "futures_deletion_jobs" }

// PurgeDueDeletionJobs hard-deletes hidden copies promptly and retries failures
// before purge_due_at, the 24-hour completion deadline. It runs in off/read_only.
func PurgeDueDeletionJobs(ctx context.Context, db *gorm.DB, now time.Time, limit int) (int, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	purged := 0
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var jobs []DeletionJobValue
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where(`state IN ('scheduled','retry') AND eligible_at<=?`, now).Order(`purge_due_at`).Limit(limit).Find(&jobs).Error; err != nil {
			return err
		}
		for _, job := range jobs {
			var err error
			switch job.TargetType {
			case "run":
				err = purgeRunBody(tx, job.OwnerID, job.Mode, job.TargetID)
			case "document":
				err = purgeDocumentBody(tx, job.OwnerID, job.Mode, job.TargetID)
			case "hypothesis":
				err = purgeHypothesisBody(tx, job.OwnerID, job.Mode, job.TargetID)
			case "draft":
				err = purgeDraftBody(tx, job.OwnerID, job.Mode, job.TargetID)
			default:
				err = ErrInvalidInput
			}
			if err != nil {
				message := err.Error()
				if updateErr := tx.Exec(`UPDATE futures_deletion_jobs SET state='retry', retry_count=retry_count+1, last_error=? WHERE id=?`, message, job.ID).Error; updateErr != nil {
					return updateErr
				}
				continue
			}
			if err := tx.Exec(`UPDATE futures_deletion_jobs SET state='purged', last_error=NULL WHERE id=?`, job.ID).Error; err != nil {
				return err
			}
			purged++
		}
		return nil
	})
	return purged, err
}

func purgeRunBody(tx *gorm.DB, ownerID uint, mode, runID string) error {
	statements := []struct {
		sql  string
		args []any
	}{
		{`DELETE FROM futures_checks WHERE owner_id=? AND mode=? AND hypothesis_id IN (SELECT id FROM futures_hypotheses WHERE owner_id=? AND mode=? AND run_id=?)`, []any{ownerID, mode, ownerID, mode, runID}},
		{`DELETE FROM futures_recaps WHERE owner_id=? AND mode=? AND hypothesis_id IN (SELECT id FROM futures_hypotheses WHERE owner_id=? AND mode=? AND run_id=?)`, []any{ownerID, mode, ownerID, mode, runID}},
		{`DELETE FROM futures_hypothesis_versions WHERE owner_id=? AND mode=? AND hypothesis_id IN (SELECT id FROM futures_hypotheses WHERE owner_id=? AND mode=? AND run_id=?)`, []any{ownerID, mode, ownerID, mode, runID}},
		{`DELETE FROM futures_hypotheses WHERE owner_id=? AND mode=? AND run_id=?`, []any{ownerID, mode, runID}},
		{`DELETE FROM futures_reports WHERE owner_id=? AND mode=? AND run_id=?`, []any{ownerID, mode, runID}},
		{`DELETE FROM futures_evidence_links WHERE owner_id=? AND mode=? AND run_id=?`, []any{ownerID, mode, runID}},
		{`DELETE FROM futures_calculations WHERE owner_id=? AND mode=? AND run_id=?`, []any{ownerID, mode, runID}},
		{`DELETE FROM futures_runs WHERE owner_id=? AND mode=? AND id=?`, []any{ownerID, mode, runID}},
		{`DELETE FROM futures_notifications WHERE owner_id=? AND mode=? AND object_id=?`, []any{ownerID, mode, runID}},
		{`DELETE FROM futures_outbox WHERE owner_id=? AND mode=? AND payload->>'object_id'=?`, []any{ownerID, mode, runID}},
		{`DELETE FROM futures_idempotency_keys WHERE owner_id=? AND mode=? AND response->>'id'=?`, []any{ownerID, mode, runID}},
	}
	for _, statement := range statements {
		if err := tx.Exec(statement.sql, statement.args...).Error; err != nil {
			return err
		}
	}
	return nil
}

func purgeDocumentBody(tx *gorm.DB, ownerID uint, mode, documentID string) error {
	var storageKey string
	_ = tx.Raw(`SELECT storage_key FROM futures_documents WHERE owner_id=? AND mode=? AND id=?`, ownerID, mode, documentID).Scan(&storageKey).Error
	if storageDir := os.Getenv("ZHIGU_FUTURES_STORAGE_DIR"); storageDir != "" && storageKey != "" {
		full := filepath.Join(storageDir, filepath.FromSlash(storageKey))
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := tx.Exec(`DELETE FROM futures_documents WHERE owner_id=? AND mode=? AND id=?`, ownerID, mode, documentID).Error; err != nil {
		return err
	}
	if err := deleteObjectNotifications(tx, ownerID, mode, documentID); err != nil {
		return err
	}
	return purgeIdempotentCopies(tx, ownerID, mode, documentID)
}

func purgeDraftBody(tx *gorm.DB, ownerID uint, mode, draftID string) error {
	if err := tx.Exec(`DELETE FROM futures_draft_versions WHERE owner_id=? AND mode=? AND draft_id=?`, ownerID, mode, draftID).Error; err != nil {
		return err
	}
	if err := tx.Exec(`DELETE FROM futures_drafts WHERE owner_id=? AND mode=? AND id=?`, ownerID, mode, draftID).Error; err != nil {
		return err
	}
	if err := deleteObjectNotifications(tx, ownerID, mode, draftID); err != nil {
		return err
	}
	return purgeIdempotentCopies(tx, ownerID, mode, draftID)
}

func purgeHypothesisBody(tx *gorm.DB, ownerID uint, mode, hypothesisID string) error {
	for _, statement := range []string{
		`DELETE FROM futures_checks WHERE owner_id=? AND mode=? AND hypothesis_id=?`,
		`DELETE FROM futures_recaps WHERE owner_id=? AND mode=? AND hypothesis_id=?`,
		`DELETE FROM futures_hypothesis_versions WHERE owner_id=? AND mode=? AND hypothesis_id=?`,
		`DELETE FROM futures_hypotheses WHERE owner_id=? AND mode=? AND id=?`,
	} {
		if err := tx.Exec(statement, ownerID, mode, hypothesisID).Error; err != nil {
			return err
		}
	}
	if err := deleteObjectNotifications(tx, ownerID, mode, hypothesisID); err != nil {
		return err
	}
	return purgeIdempotentCopies(tx, ownerID, mode, hypothesisID)
}

func deleteObjectNotifications(tx *gorm.DB, ownerID uint, mode, objectID string) error {
	if err := tx.Exec(`DELETE FROM futures_notifications WHERE owner_id=? AND mode=? AND object_id=?`, ownerID, mode, objectID).Error; err != nil {
		return err
	}
	return tx.Exec(`DELETE FROM futures_outbox WHERE owner_id=? AND mode=? AND payload->>'object_id'=?`, ownerID, mode, objectID).Error
}

func purgeIdempotentCopies(tx *gorm.DB, ownerID uint, mode, objectID string) error {
	return tx.Exec(`DELETE FROM futures_idempotency_keys WHERE owner_id=? AND mode=? AND response->>'id'=?`, ownerID, mode, objectID).Error
}

func PurgeDueDocuments(ctx context.Context, db *gorm.DB, now time.Time) (int, error) {
	return 0, db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(`UPDATE futures_documents SET deleted_at=? WHERE deleted_at IS NULL AND purge_at<=?`, now, now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		return tx.Exec(`INSERT INTO futures_deletion_jobs(id,owner_id,mode,target_type,target_id,impact_version,purge_due_at,state)
SELECT 'del_'||md5(owner_id::text||mode||id),owner_id,mode,'document',id,content_hash,?,'scheduled'
FROM futures_documents WHERE deleted_at=? AND purge_at<=?
ON CONFLICT (owner_id,mode,target_type,target_id) DO NOTHING`, now.Add(24*time.Hour), now, now).Error
	})
}
