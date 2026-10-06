package futures

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	model "zhigu/server/model/futures"
)

type TaskManifest struct {
	ID        string         `gorm:"column:id;primaryKey"`
	OwnerID   uint           `gorm:"column:owner_id"`
	Mode      string         `gorm:"column:mode"`
	ProductID string         `gorm:"column:product_id"`
	AsOf      time.Time      `gorm:"column:as_of"`
	Hash      string         `gorm:"column:manifest_hash"`
	Versions  datatypes.JSON `gorm:"column:versions"`
	CreatedAt time.Time      `gorm:"column:created_at"`
}

func (TaskManifest) TableName() string { return "futures_manifests" }

func StartRunScheduler(ctx context.Context, db *gorm.DB, domain *Domain, grants *GrantManager) {
	semaphore := make(chan struct{}, 2)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
			for index := 0; index < 2; index++ {
				select {
				case semaphore <- struct{}{}:
				case <-ctx.Done():
					return
				}
				run, err := ClaimQueuedRun(ctx, db, "futures-go-scheduler", time.Now().UTC())
				if err != nil {
					<-semaphore
					break
				}
				go func(run *model.Run) {
					defer func() { <-semaphore }()
					if err := ExecuteRun(ctx, db, domain, grants, run); err != nil {
						identity := Identity{OwnerID: run.OwnerID, Mode: run.Mode}
						if ackErr := AcknowledgeCanceledRun(ctx, db, identity, run.ID); ackErr == nil {
							return
						}
						_ = FailRun(ctx, db, identity, run.ID, run.Generation, "FUTURES_EXECUTION_FAILED")
					}
				}(run)
			}
		}
	}
}

func ExecuteRun(ctx context.Context, db *gorm.DB, domain *Domain, grants *GrantManager, run *model.Run) error {
	workerURL := os.Getenv("FUTURES_WORKER_URL")
	if workerURL == "" {
		return ErrUnavailable
	}
	if err := domain.AuthorizeExecution(ctx, Identity{OwnerID: run.OwnerID, Mode: run.Mode}); err != nil {
		return err
	}
	execCtx, cancel := context.WithTimeout(ctx, futuresExecutionTimeout)
	defer cancel()
	if run.TaskSnapshot == nil || len(run.TaskSnapshot) == 0 || len(run.Versions) == 0 {
		return ErrInvalidInput
	}
	var snapshot map[string]any
	if err := json.Unmarshal(run.TaskSnapshot, &snapshot); err != nil {
		return err
	}
	input, ok := snapshot["input"].(map[string]any)
	if !ok {
		return ErrInvalidInput
	}
	var versions map[string]any
	if err := json.Unmarshal(run.Versions, &versions); err != nil {
		return err
	}
	if fmt.Sprint(versions["model_config"]) == "" {
		return ErrUnavailable
	}

	manifestID := "manifest_" + uuid.NewString()
	versions["source_manifest"] = manifestID
	versionRaw, _ := json.Marshal(versions)
	manifest := TaskManifest{ID: manifestID, OwnerID: run.OwnerID, Mode: run.Mode,
		ProductID: fmt.Sprint(input["product_id"]), AsOf: run.AsOf, Versions: datatypes.JSON(versionRaw), CreatedAt: time.Now().UTC()}
	if err := db.WithContext(execCtx).Create(&manifest).Error; err != nil {
		return err
	}
	if err := freezeManifestRecords(db.WithContext(execCtx), manifest, run); err != nil {
		return err
	}
	var recordIDs []string
	if err := db.WithContext(execCtx).Raw(`SELECT record_id FROM futures_manifest_records WHERE manifest_id=? ORDER BY position`, manifest.ID).Scan(&recordIDs).Error; err != nil {
		return err
	}
	manifest.Hash = hashJSON(map[string]any{"as_of": manifest.AsOf, "versions": versions, "run_id": run.ID, "record_ids": recordIDs})
	if err := db.WithContext(execCtx).Model(&TaskManifest{}).Where(`id=?`, manifest.ID).Update(`manifest_hash`, manifest.Hash).Error; err != nil {
		return err
	}

	taskID := "task_" + uuid.NewString()
	snapshot["task_id"] = taskID
	snapshot["manifest_id"] = manifest.ID
	snapshot["versions"] = versions
	if limits, ok := snapshot["limits"].(map[string]any); ok {
		limits["deadline"] = run.DeadlineAt
	}
	snapshotRaw, _ := json.Marshal(snapshot)
	res := db.WithContext(execCtx).Table(`futures_runs`).Where(`id=? AND generation=? AND deleted_at IS NULL`, run.ID, run.Generation).
		Updates(map[string]any{`manifest_id`: manifest.ID, `versions`: versionRaw, `task_snapshot`: snapshotRaw, `updated_at`: time.Now().UTC()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	run.ManifestID, run.Versions, run.TaskSnapshot = manifest.ID, datatypes.JSON(versionRaw), datatypes.JSON(snapshotRaw)

	task := map[string]any{
		"schema_version": "futures.task.v1",
		"scope":          map[string]any{"domain": "futures", "mode": run.Mode, "owner_id": run.OwnerID},
		"task_id":        taskID, "run_id": run.ID, "generation": run.Generation,
		"draft_id": snapshot["draft_id"], "draft_revision": snapshot["draft_revision"], "manifest_id": manifest.ID,
		"product_id": input["product_id"], "contract_id": input["contract_id"],
		"claims": snapshot["claims"], "as_of": run.AsOf, "horizon_end": run.HorizonEnd,
		"versions": versions,
		"limits":   snapshot["limits"], "allowed_tools": []string{"futures_get_observations", "futures_get_evidence", "futures_calculate"},
	}
	taskRaw, _ := json.Marshal(task)
	var taskMap map[string]any
	_ = json.Unmarshal(taskRaw, &taskMap)
	spec := GrantSpec{Domain: "futures", OwnerID: run.OwnerID, Mode: run.Mode, RunID: run.ID, TaskID: taskID,
		Generation: run.Generation, ManifestID: manifest.ID, Stage: "execute", ArgsHash: HashArguments(taskMap)}
	grantToken, err := grants.Issue(spec)
	if err != nil {
		return err
	}

	leaseOwner := "futures-go-scheduler"
	if run.LeaseOwner != nil && *run.LeaseOwner != "" {
		leaseOwner = *run.LeaseOwner
	}
	renewCtx, stopRenew := context.WithCancel(execCtx)
	defer stopRenew()
	renewDone := make(chan struct{})
	go func() {
		defer close(renewDone)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-renewCtx.Done():
				return
			case now := <-ticker.C:
				if err := RenewRunLease(renewCtx, db, Identity{OwnerID: run.OwnerID, Mode: run.Mode}, run.ID, leaseOwner, run.Generation, now.UTC()); err != nil {
					return
				}
			}
		}
	}()

	request, err := http.NewRequestWithContext(execCtx, http.MethodPost, workerURL+"/internal/futures/v1/execute", bytes.NewReader(taskRaw))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Futures-Service-Token", os.Getenv("FUTURES_SERVICE_TOKEN"))
	request.Header.Set("Authorization", "Bearer "+grantToken)
	client := &http.Client{Timeout: futuresExecutionTimeout}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	stopRenew()
	<-renewDone
	if response.StatusCode != http.StatusOK {
		return ErrUnavailable
	}
	var report map[string]any
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		return err
	}
	calculations := loadCalculationRecords(db.WithContext(execCtx), run.OwnerID, run.Mode, run.ID)
	manifestRecords := loadManifestRecords(db.WithContext(execCtx), manifest.ID, run.OwnerID)
	if err := VerifyCandidate(taskMap, report, manifestRecords, calculations); err != nil {
		return err
	}
	reportRaw, _ := json.Marshal(report)
	identity := Identity{OwnerID: run.OwnerID, Mode: run.Mode}
	return CompleteRun(ctx, db, identity, run.ID, run.Generation, reportRaw)
}

func freezeManifestRecords(db *gorm.DB, manifest TaskManifest, run *model.Run) error {
	return db.Transaction(func(tx *gorm.DB) error {
		res := tx.Exec(`WITH ranked AS (
  SELECT o.id, row_number() OVER (PARTITION BY o.series_id, o.period_start ORDER BY o.revision DESC, o.id DESC) AS rn
  FROM futures_observations o
  JOIN futures_sources src ON src.source_id=o.source_id
  WHERE o.product_id=? AND o.usable_at<=? AND o.deleted_at IS NULL
    AND src.status='enabled' AND src.deleted_at IS NULL
    AND coalesce((src.rights->>'model_use')::boolean,false)
    AND coalesce((src.rights->>'retain_until')::timestamptz,to_timestamp(0)) >= ?
    AND (o.scope='public' OR (o.scope='private' AND o.owner_id=? AND o.mode=?))
), ordered AS (
  SELECT id, row_number() OVER (ORDER BY id) AS position FROM ranked WHERE rn=1
)
INSERT INTO futures_manifest_records(manifest_id,record_id,position)
SELECT ?, id, position FROM ordered`, manifest.ProductID, manifest.AsOf, manifest.AsOf, run.OwnerID, run.Mode, manifest.ID)
		return res.Error
	})
}

func jsonRawSlice(raw datatypes.JSON) []any {
	var value []any
	_ = json.Unmarshal(raw, &value)
	if value == nil {
		return []any{}
	}
	return value
}

func loadCalculationRecords(db *gorm.DB, ownerID uint, mode, runID string) map[string]CalculationRecord {
	var rows []struct {
		ID, Value, Unit, FormulaVersion string
		InputRecordIDs                  datatypes.JSON
	}
	_ = db.Raw(`SELECT id, coalesce(value::text,'') AS value, unit, formula_version, input_record_ids FROM futures_calculations WHERE owner_id=? AND mode=? AND run_id=?`, ownerID, mode, runID).Scan(&rows).Error
	out := map[string]CalculationRecord{}
	for _, row := range rows {
		var ids []string
		_ = json.Unmarshal(row.InputRecordIDs, &ids)
		value, _ := decimal.NewFromString(row.Value)
		out[row.ID] = CalculationRecord{ID: row.ID, Value: value, Unit: row.Unit, InputIDs: ids}
	}
	return out
}

func loadManifestRecords(db *gorm.DB, manifestID string, ownerID uint) map[string]ManifestRecord {
	var rows []struct {
		ID, Scope, SourceType, SourceStatus string
		OwnerID                             *uint
		UsableAt, RetainUntil               time.Time
		ModelUse, ExportAllowed, QualityOK  bool
	}
	_ = db.Raw(`SELECT o.id, o.scope, o.owner_id, o.usable_at, o.source_type, src.status AS source_status,
coalesce((src.rights->>'model_use')::boolean,false) AS model_use,
coalesce((src.rights->>'export')::boolean,false) AS export_allowed,
coalesce((src.rights->>'retain_until')::timestamptz, to_timestamp(0)) AS retain_until,
(o.quality->>'acquisition'='available' AND o.quality->>'verification' NOT IN ('conflicted','retracted')) AS quality_ok
FROM futures_manifest_records m
JOIN futures_observations o ON o.id=m.record_id
JOIN futures_sources src ON src.source_id=o.source_id
WHERE m.manifest_id=?`, manifestID).Scan(&rows).Error
	out := map[string]ManifestRecord{}
	for _, row := range rows {
		recordOwner := uint(0)
		if row.OwnerID != nil {
			recordOwner = *row.OwnerID
		}
		out[row.ID] = ManifestRecord{ID: row.ID, OwnerID: recordOwner, Scope: row.Scope, UsableAt: row.UsableAt,
			RetainUntil: row.RetainUntil, SourceType: row.SourceType, SourceStatus: row.SourceStatus,
			ModelUse: row.ModelUse, ExportAllowed: row.ExportAllowed, QualityOK: row.QualityOK}
	}
	return out
}
