package futures

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"

	"gorm.io/gorm"
	model "zhigu/server/model/futures"
)

// MaintenanceAvailable preserves cleanup after disabling an installed module,
// without starting database jobs on a fresh deployment with no futures schema.
func MaintenanceAvailable(db *gorm.DB) bool {
	return db != nil && db.Migrator().HasTable(&model.Run{})
}

// StartFuturesMaintenance keeps cleanup and notification replay active even in
// off/read_only modes. It never calls a source or model.
func StartFuturesMaintenance(ctx context.Context, db *gorm.DB, domain *Domain) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := ExpireStaleRuns(ctx, db, now); err != nil {
				log.Printf("futures expire stale runs: %v", err)
			}
			if err := ExpireDueHypotheses(ctx, db, now); err != nil {
				log.Printf("futures expire hypotheses: %v", err)
			}
			if _, err := EvaluateDueConditions(ctx, db, now, 200); err != nil {
				log.Printf("futures condition checks: %v", err)
			}
			if _, err := PurgeDueDocuments(ctx, db, now); err != nil {
				log.Printf("futures document retention: %v", err)
			}
			if _, err := ProcessOutbox(ctx, db, now, 2000); err != nil {
				log.Printf("futures outbox: %v", err)
			}
			if _, err := PurgeDueDeletionJobs(ctx, db, now, 500); err != nil {
				log.Printf("futures cleanup: %v", err)
			}
			_ = domain
		}
	}
}

func ExpireDueHypotheses(ctx context.Context, db *gorm.DB, now time.Time) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []model.Hypothesis
		if err := tx.Where(`lifecycle IN ('active','paused') AND expires_at<=? AND deleted_at IS NULL`, now).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if err := tx.Model(&model.Hypothesis{}).Where(`id=? AND owner_id=? AND mode=? AND version=?`, row.ID, row.OwnerID, row.Mode, row.Version).
				Updates(map[string]any{`lifecycle`: `expired`, `needs_review`: true, `updated_at`: now}).Error; err != nil {
				return err
			}
			if err := writeNotification(tx, Identity{OwnerID: row.OwnerID, Mode: row.Mode}, "expired", row.ID, int64(row.Version), "hypothesis."+row.ID+".expired", now); err != nil {
				return err
			}
		}
		return nil
	})
}

func EvaluateDueConditions(ctx context.Context, db *gorm.DB, now time.Time, limit int) (int, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	var hypotheses []model.Hypothesis
	if err := db.WithContext(ctx).Where(`lifecycle IN ('active','paused') AND deleted_at IS NULL`).Order(`updated_at`).Limit(limit).Find(&hypotheses).Error; err != nil {
		return 0, err
	}
	evaluated := 0
	for _, hypothesis := range hypotheses {
		var raw []map[string]any
		if err := json.Unmarshal(hypothesis.Conditions, &raw); err != nil {
			continue
		}
		for _, item := range raw {
			condition, err := conditionFromMap(item)
			if err != nil {
				continue
			}
			records, evidenceIDs, maxUsable, err := conditionRecords(ctx, db, hypothesis, condition, now)
			if err != nil {
				return evaluated, err
			}
			var latest struct {
				Result    string
				CheckedAt time.Time
			}
			_ = db.WithContext(ctx).Raw(`SELECT result,checked_at FROM futures_checks WHERE owner_id=? AND mode=? AND hypothesis_id=? AND hypothesis_version=? AND condition_id=? ORDER BY version DESC LIMIT 1`,
				hypothesis.OwnerID, hypothesis.Mode, hypothesis.ID, hypothesis.Version, fmt.Sprint(item["id"])).Scan(&latest).Error
			if !latest.CheckedAt.IsZero() && !maxUsable.After(latest.CheckedAt) {
				continue
			}
			result := EvaluateCondition(condition, records)
			evidenceRaw, _ := json.Marshal(evidenceIDs)
			err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				var version int64
				if err := tx.Raw(`SELECT coalesce(max(version),0) FROM futures_checks WHERE owner_id=? AND mode=? AND hypothesis_id=? AND hypothesis_version=? AND condition_id=?`,
					hypothesis.OwnerID, hypothesis.Mode, hypothesis.ID, hypothesis.Version, fmt.Sprint(item["id"])).Scan(&version).Error; err != nil {
					return err
				}
				if err := tx.Exec(`INSERT INTO futures_checks(id,owner_id,mode,hypothesis_id,hypothesis_version,condition_id,version,result,evidence_ids,reason,checked_at,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
					"check_"+uuid.NewString(), hypothesis.OwnerID, hypothesis.Mode, hypothesis.ID, hypothesis.Version, fmt.Sprint(item["id"]), version+1,
					result.Result, datatypes.JSON(evidenceRaw), result.Reason, now, now).Error; err != nil {
					return err
				}
				if latest.Result != result.Result {
					if err := tx.Model(&model.Hypothesis{}).Where(`id=? AND owner_id=? AND mode=? AND version=?`, hypothesis.ID, hypothesis.OwnerID, hypothesis.Mode, hypothesis.Version).
						Updates(map[string]any{`needs_review`: true, `updated_at`: now}).Error; err != nil {
						return err
					}
					if err := writeNotification(tx, Identity{OwnerID: hypothesis.OwnerID, Mode: hypothesis.Mode}, "check_changed", hypothesis.ID, int64(hypothesis.Version), fmt.Sprintf("hypothesis.%s.%s.%d", hypothesis.ID, item["id"], version+1), now); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return evaluated, err
			}
			evaluated++
		}
	}
	return evaluated, nil
}

func conditionFromMap(value map[string]any) (CheckCondition, error) {
	threshold := decimal.Zero
	if raw := fmt.Sprint(value["threshold"]); raw != "" && raw != "<nil>" {
		parsed, err := decimal.NewFromString(raw)
		if err != nil {
			return CheckCondition{}, err
		}
		threshold = parsed
	}
	periods := 0
	switch raw := value["periods"].(type) {
	case float64:
		periods = int(raw)
	case int:
		periods = raw
	}
	seriesID := ""
	if raw, ok := value["series_id"].(string); ok {
		seriesID = raw
	}
	return CheckCondition{Kind: ConditionKind(fmt.Sprint(value["kind"])), SeriesID: seriesID, Operator: fmt.Sprint(value["operator"]), Threshold: threshold, Periods: periods}, nil
}

func conditionRecords(ctx context.Context, db *gorm.DB, hypothesis model.Hypothesis, condition CheckCondition, asOf time.Time) ([]ObservationRecord, []string, time.Time, error) {
	if condition.Kind == ConditionManual {
		return nil, nil, time.Time{}, nil
	}
	if condition.SeriesID == "" {
		return nil, nil, time.Time{}, fmt.Errorf("%w: condition series missing", ErrInvalidInput)
	}
	var rows []struct {
		ID, SeriesID, Period, Unit, SourceType, Frequency string
		Revision, PeriodIndex                             int64
		Value                                             string
		UsableAt                                          time.Time
	}
	limit := condition.Periods + 2
	if limit < 3 {
		limit = 3
	}
	if limit > 12 {
		limit = 12
	}
	err := db.WithContext(ctx).Raw(`WITH ranked AS (
SELECT o.id,o.series_id,o.period_start::text AS period,o.revision,o.unit,o.source_type,s.frequency,
coalesce(o.numeric_value::text,'') AS value,o.usable_at,
row_number() OVER (PARTITION BY o.series_id,o.period_start ORDER BY o.revision DESC,o.id DESC) AS revision_rank,
dense_rank() OVER (ORDER BY o.period_start) AS period_index
FROM futures_observations o JOIN futures_series s ON s.id=o.series_id JOIN futures_sources src ON src.source_id=o.source_id AND src.version::text=o.source_version
WHERE o.series_id=? AND o.product_id=? AND o.deleted_at IS NULL AND src.status='enabled' AND src.deleted_at IS NULL
AND coalesce((src.rights->>'model_use')::boolean,false)
AND coalesce((src.rights->>'retain_until')::timestamptz,to_timestamp(0)) >= ?
AND o.usable_at <= ?
AND (o.scope='public' OR (o.scope='private' AND o.owner_id=? AND o.mode=?))
), recent AS (
SELECT * FROM ranked WHERE revision_rank=1 ORDER BY period DESC,id DESC LIMIT ?
)
SELECT id,series_id,period,revision,unit,source_type,frequency,value,usable_at,period_index
FROM recent ORDER BY period,id`, condition.SeriesID, hypothesis.ProductID, asOf, asOf, hypothesis.OwnerID, hypothesis.Mode, limit).Scan(&rows).Error
	if err != nil {
		return nil, nil, time.Time{}, err
	}
	records := make([]ObservationRecord, 0, len(rows))
	ids := make([]string, 0, len(rows))
	var maxUsable time.Time
	for _, row := range rows {
		value, err := decimal.NewFromString(row.Value)
		if err != nil {
			continue
		}
		records = append(records, ObservationRecord{ID: row.ID, SeriesID: row.SeriesID, Period: row.Period, Revision: int(row.Revision),
			Value: value, Unit: row.Unit, UsableAt: row.UsableAt, SourceType: row.SourceType, Frequency: row.Frequency, PeriodIndex: row.PeriodIndex})
		ids = append(ids, row.ID)
		if row.UsableAt.After(maxUsable) {
			maxUsable = row.UsableAt
		}
	}
	return records, ids, maxUsable, nil
}
