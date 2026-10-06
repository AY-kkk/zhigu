package futures

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	model "zhigu/server/model/futures"
)

type RunCreateInput struct {
	DraftID          string           `json:"draft_id"`
	ExpectedRevision int64            `json:"expected_revision"`
	Claims           []map[string]any `json:"claims"`
	AcceptLimits     bool             `json:"accept_limits"`
}

func (d *Domain) CreateRun(ctx context.Context, id Identity, idempotencyKey string, input RunCreateInput) (map[string]any, error) {
	if !input.AcceptLimits || input.DraftID == "" || input.ExpectedRevision < 1 || len(input.Claims) == 0 || len(input.Claims) > 12 {
		return nil, ErrInvalidInput
	}
	now := d.Now().UTC()
	value := model.Run{ID: "run_" + uuid.NewString(), OwnerID: id.OwnerID, Mode: id.Mode, DraftID: input.DraftID, DraftRevision: int(input.ExpectedRevision),
		Status: "queued", Stage: "queued", AsOf: now, HorizonEnd: now.Add(14 * 24 * time.Hour), IdempotencyKey: idempotencyKey,
		RequestHash: hashJSON(input), Generation: 1, DeadlineAt: timePtr(now.Add(60 * time.Second)), CreatedAt: now, UpdatedAt: now}
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if existing, err := replayRunIdempotency(tx, id, idempotencyKey, value.RequestHash); err != nil {
			return err
		} else if existing != nil {
			value = *existing
			return nil
		}
		var draft model.Draft
		if err := tx.Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, input.DraftID, id.OwnerID, id.Mode).First(&draft).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if draft.Revision != int(input.ExpectedRevision) || draft.ParsedRevision == nil || *draft.ParsedRevision != int(input.ExpectedRevision) {
			return ErrRevisionConflict
		}
		value.HorizonEnd = now.Add(time.Duration(draft.Input.HorizonDays) * 24 * time.Hour)
		if !claimsMatchSelection(draft.Claims, input.Claims) {
			return ErrInvalidInput
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, fmt.Sprintf("futures.run.create.%d.%s", id.OwnerID, id.Mode)).Error; err != nil {
			return err
		}
		var active int64
		if err := tx.Raw(`SELECT count(*) FROM futures_runs WHERE owner_id=? AND mode=? AND status IN ('queued','running','verifying','canceling') AND deleted_at IS NULL`, id.OwnerID, id.Mode).Scan(&active).Error; err != nil {
			return err
		}
		if active >= 1 {
			return ErrQuotaExceeded
		}
		day := beijingDay(now)
		var daily int64
		if err := tx.Raw(`SELECT count(*) FROM futures_runs WHERE owner_id=? AND mode=? AND created_at>=? AND created_at<?`, id.OwnerID, id.Mode, day, day.AddDate(0, 0, 1)).Scan(&daily).Error; err != nil {
			return err
		}
		if daily >= 5 {
			return ErrQuotaExceeded
		}
		rawClaims, _ := json.Marshal(input.Claims)
		value.ClaimsSnapshot = datatypes.JSON(rawClaims)
		rawInput, _ := json.Marshal(draft.Input)
		rawVersions, _ := json.Marshal(map[string]any{
			"template":        "futures.cu.template.v1",
			"formula":         FormulaVersion,
			"policy":          "futures.policy.v1",
			"model_config":    os.Getenv("FUTURES_MODEL_CONFIG_VERSION"),
			"source_manifest": "pending",
		})
		versionMeta := map[string]any{"draft_revision": input.ExpectedRevision, "input_hash": hashJSON(draft.Input), "claims_hash": hashJSON(input.Claims)}
		rawSnapshot, _ := json.Marshal(map[string]any{
			"schema_version":  "futures.task-snapshot.v1",
			"draft_id":        input.DraftID,
			"draft_revision":  input.ExpectedRevision,
			"input":           json.RawMessage(rawInput),
			"claims":          json.RawMessage(rawClaims),
			"selected_claims": input.Claims,
			"versions":        json.RawMessage(rawVersions),
			"version_meta":    versionMeta,
			"as_of":           value.AsOf,
			"horizon_end":     value.HorizonEnd,
			"deadline_at":     value.DeadlineAt,
			"limits":          map[string]any{"max_model_calls": 8, "max_tool_calls": 24, "remaining_tokens": 48000, "deadline": value.DeadlineAt},
		})
		value.Versions = datatypes.JSON(rawVersions)
		value.TaskSnapshot = datatypes.JSON(rawSnapshot)
		value.ManifestID = "pending"
		if err := tx.Create(&value).Error; err != nil {
			return err
		}
		eventRaw, _ := json.Marshal(NewOutboxEvent("run_created", "run", value.ID, 1, value.ID))
		if err := tx.Exec(`INSERT INTO futures_outbox(id,owner_id,mode,dedupe_key,payload,next_attempt_at,state) VALUES(?,?,?,?,?::jsonb,?,'pending')`,
			"out_"+uuid.NewString(), id.OwnerID, id.Mode, "run.created."+value.ID, eventRaw, now).Error; err != nil {
			return err
		}
		return d.writeIdempotency(tx, id, "run.create", idempotencyKey, value.RequestHash, value)
	})
	if err != nil {
		return nil, err
	}
	return runMap(value), nil
}

func (d *Domain) ListRuns(ctx context.Context, id Identity, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var values []model.Run
	if err := d.DB.WithContext(ctx).Where(`owner_id=? AND mode=? AND deleted_at IS NULL`, id.OwnerID, id.Mode).Order(`created_at DESC, id DESC`).Limit(limit).Find(&values).Error; err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		out = append(out, runMap(value))
	}
	return out, nil
}

func (d *Domain) GetRun(ctx context.Context, id Identity, runID string) (map[string]any, error) {
	var value model.Run
	err := d.DB.WithContext(ctx).Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, runID, id.OwnerID, id.Mode).First(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return runMap(value), nil
}

func (d *Domain) DeleteImpact(ctx context.Context, id Identity, runID string) (map[string]any, error) {
	var hypotheses []model.Hypothesis
	if err := d.DB.WithContext(ctx).Where(`run_id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, runID, id.OwnerID, id.Mode).Order(`id`).Find(&hypotheses).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(hypotheses))
	for _, hypothesis := range hypotheses {
		ids = append(ids, hypothesis.ID)
	}
	sort.Strings(ids)
	sum := sha256.Sum256([]byte(strings.Join(ids, ",")))
	return map[string]any{"run_id": runID, "hypothesis_ids": ids, "impact_version": hex.EncodeToString(sum[:])}, nil
}

func (d *Domain) DeleteRun(ctx context.Context, id Identity, runID, impactVersion string, cascade bool, idempotencyKey string) (map[string]any, error) {
	requestHash := hashJSON(map[string]any{"id": runID, "impact_version": impactVersion, "cascade": cascade, "action": "delete"})
	var replayed map[string]any
	if err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, ok, err := replayDeletion(ctx, tx, id, "run.delete", idempotencyKey, requestHash)
		if err != nil {
			return err
		}
		if ok {
			replayed = existing
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if replayed != nil {
		return replayed, nil
	}
	impact, err := d.DeleteImpact(ctx, id, runID)
	if err != nil {
		return nil, err
	}
	if fmt.Sprint(impact["impact_version"]) != impactVersion {
		return nil, ErrRevisionConflict
	}
	if len(impact["hypothesis_ids"].([]string)) > 0 && !cascade {
		return nil, ErrInvalidInput
	}
	now := d.Now().UTC()
	deletionID := "del_" + uuid.NewString()
	result := map[string]any{"deletion_id": deletionID, "hidden_at": now, "purge_due_at": now.Add(24 * time.Hour)}
	err = d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Table(`futures_runs`).Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, runID, id.OwnerID, id.Mode).Update(`deleted_at`, now)
		if res.RowsAffected != 1 {
			return ErrNotFound
		}
		if cascade {
			if err := tx.Table(`futures_hypotheses`).Where(`run_id=? AND owner_id=? AND mode=?`, runID, id.OwnerID, id.Mode).Update(`deleted_at`, now).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec(`INSERT INTO futures_deletion_jobs(id,owner_id,mode,target_type,target_id,impact_version,purge_due_at,state) VALUES(?,?,?,?,?,?,?,'scheduled')`,
			deletionID, id.OwnerID, id.Mode, "run", runID, impactVersion, now.Add(24*time.Hour)).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO futures_tombstones(request_id,owner_id,object_type,object_id,hidden_at,purge_due_at) VALUES(?,?,?,?,?,?)`,
			uuid.NewString(), id.OwnerID, "run", runID, now, now.Add(24*time.Hour)).Error; err != nil {
			return err
		}
		if err := hideIdempotentObject(ctx, tx, id, "run.create", runID, now); err != nil {
			return err
		}
		return d.writeIdempotency(tx, id, "run.delete", idempotencyKey, requestHash, result)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (d *Domain) Evidence(ctx context.Context, id Identity, runID, evidenceID string) (map[string]any, error) {
	var count int64
	if err := d.DB.WithContext(ctx).Raw(`SELECT count(*) FROM futures_runs WHERE id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, runID, id.OwnerID, id.Mode).Scan(&count).Error; err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, ErrNotFound
	}
	var linkCount int64
	if err := d.DB.WithContext(ctx).Raw(`SELECT count(*) FROM futures_evidence_links WHERE run_id=? AND owner_id=? AND mode=? AND record_id=? AND deleted_at IS NULL`, runID, id.OwnerID, id.Mode, evidenceID).Scan(&linkCount).Error; err != nil {
		return nil, err
	}
	if linkCount != 1 {
		return nil, ErrNotFound
	}
	var value struct {
		ID                 string
		Revision           int64
		SupersedesID       *string
		Scope              string
		OwnerID            *int64
		Mode               *string
		ProductID          string
		ContractID         *string
		SourceID           string
		SourceVersion      string
		SourceCluster      string
		SourceType         string
		Metric             string
		Value              string
		Unit               string
		Currency           *string
		CaliberID          string
		PeriodStart        string
		PeriodEnd          string
		TradingDay         *string
		PublishedAt        *time.Time
		PublishedPrecision string
		VersionAvailableAt *time.Time
		FirstObservedAt    time.Time
		RetrievedAt        time.Time
		UsableAt           time.Time
		Quality            datatypes.JSON
		Rights             datatypes.JSON
		Locator            datatypes.JSON
		ContentHash        string
	}
	if err := d.DB.WithContext(ctx).Raw(`SELECT o.id, o.revision, o.supersedes_id, o.scope, o.owner_id, o.mode, o.product_id, o.contract_id,
o.source_id, o.source_version, o.source_cluster, o.source_type, s.metric, coalesce(o.numeric_value::text,o.text_value,'') AS value, o.unit, s.currency,
o.caliber_id, o.period_start::text AS period_start, o.period_end::text AS period_end, o.trading_day::text AS trading_day,
o.published_at, o.published_precision, o.version_available_at, o.first_observed_at, o.retrieved_at, o.usable_at, o.quality, src.rights, o.locator, o.content_hash
FROM futures_observations o
JOIN futures_series s ON s.id=o.series_id
JOIN futures_sources src ON src.source_id=o.source_id
WHERE o.id=? AND o.deleted_at IS NULL`, evidenceID).Scan(&value).Error; err != nil {
		return nil, err
	}
	if value.ID == "" {
		return nil, ErrNotFound
	}
	var quality, rights, locator any
	var currency any
	if value.Currency != nil {
		currency = *value.Currency
	}
	_ = json.Unmarshal(value.Quality, &quality)
	_ = json.Unmarshal(value.Rights, &rights)
	_ = json.Unmarshal(value.Locator, &locator)
	return map[string]any{
		"schema_version": "futures.record.v1", "id": value.ID, "revision": value.Revision, "supersedes_id": value.SupersedesID,
		"scope": value.Scope, "owner_id": value.OwnerID, "mode": value.Mode, "product_id": value.ProductID, "contract_id": value.ContractID,
		"source_id": value.SourceID, "source_version": value.SourceVersion, "source_cluster": value.SourceCluster, "source_type": value.SourceType,
		"metric": value.Metric, "value": value.Value, "unit": value.Unit, "currency": currency, "caliber_id": value.CaliberID,
		"period_start": value.PeriodStart, "period_end": value.PeriodEnd, "trading_day": value.TradingDay,
		"published_at": value.PublishedAt, "published_precision": value.PublishedPrecision, "version_available_at": value.VersionAvailableAt,
		"first_observed_at": value.FirstObservedAt, "retrieved_at": value.RetrievedAt, "usable_at": value.UsableAt,
		"quality": quality, "rights": rights, "locator": locator, "content_hash": value.ContentHash,
	}, nil
}

func (d *Domain) ExportRun(ctx context.Context, id Identity, runID string) (string, error) {
	var run model.Run
	if err := d.DB.WithContext(ctx).Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, runID, id.OwnerID, id.Mode).First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrNotFound
		}
		return "", err
	}
	report, ok := decodeObject(run.Report)
	if !ok {
		return "", ErrInvalidInput
	}
	var rows []struct {
		ID            string
		Value         string
		ExportAllowed bool
	}
	if err := d.DB.WithContext(ctx).Raw(`SELECT o.id, coalesce(o.numeric_value::text,o.text_value,'') AS value,
  (src.status='enabled' AND src.deleted_at IS NULL AND coalesce((src.rights->>'export')::boolean,false)
    AND coalesce((src.rights->>'retain_until')::timestamptz, to_timestamp(0)) >= ?) AS export_allowed
FROM futures_evidence_links l JOIN futures_observations o ON o.id=l.record_id
JOIN futures_sources src ON src.source_id=o.source_id
WHERE l.run_id=? AND l.owner_id=? AND l.mode=? AND l.deleted_at IS NULL`, time.Now().UTC(), runID, id.OwnerID, id.Mode).Scan(&rows).Error; err != nil {
		return "", err
	}
	records := make([]ExportRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, ExportRecord{ID: row.ID, Value: row.Value, ExportAllowed: row.ExportAllowed})
	}
	var calculationRows []struct {
		ID             string
		Value          string
		InputRecordIDs datatypes.JSON
	}
	if err := d.DB.WithContext(ctx).Raw(`SELECT id, coalesce(value::text,'') AS value, input_record_ids FROM futures_calculations WHERE run_id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, runID, id.OwnerID, id.Mode).Scan(&calculationRows).Error; err != nil {
		return "", err
	}
	calculations := make([]ExportCalculation, 0, len(calculationRows))
	for _, row := range calculationRows {
		var ids []string
		_ = json.Unmarshal(row.InputRecordIDs, &ids)
		calculations = append(calculations, ExportCalculation{ID: row.ID, InputRecordIDs: ids, Value: row.Value})
	}
	title := fmt.Sprint(report["question"])
	text := fmt.Sprintf("结论：%v\n覆盖：%v", report["conclusion"], report["coverage_summary"])
	return ExportHTML(ExportInput{Title: title, Text: text, Records: records, Calculations: calculations})
}

func runMap(value model.Run) map[string]any {
	var report any = nil
	if len(value.Report) > 0 {
		_ = json.Unmarshal(value.Report, &report)
	}
	return map[string]any{
		"id": value.ID, "scope": map[string]any{"domain": "futures", "mode": value.Mode, "owner_id": value.OwnerID},
		"draft_id": value.DraftID, "draft_revision": value.DraftRevision, "status": value.Status, "stage": value.Stage,
		"as_of": value.AsOf, "horizon_end": value.HorizonEnd, "report": report, "failure_code": value.FailureCode, "created_at": value.CreatedAt,
	}
}

func claimsMatchSelection(raw datatypes.JSON, selected []map[string]any) bool {
	var parsed []map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return false
	}
	if len(selected) == 0 || len(selected) > len(parsed) {
		return false
	}
	byID := map[string]map[string]any{}
	for _, claim := range parsed {
		byID[fmt.Sprint(claim["id"])] = claim
	}
	for _, claim := range selected {
		want, ok := byID[fmt.Sprint(claim["id"])]
		if !ok || hashJSON(want) != hashJSON(claim) {
			return false
		}
	}
	return true
}

func replayRunIdempotency(tx *gorm.DB, id Identity, key, requestHash string) (*model.Run, error) {
	if key == "" {
		return nil, ErrInvalidInput
	}
	var row model.Idempotency
	err := tx.Where(`owner_id=? AND mode=? AND operation=? AND key=?`, id.OwnerID, id.Mode, "run.create", key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.RequestHash != requestHash {
		return nil, ErrIdempotencyConflict
	}
	var value model.Run
	if err := json.Unmarshal(row.Response, &value); err != nil {
		return nil, err
	}
	if !row.ExpiresAt.After(time.Now().UTC()) {
		return nil, ErrNotFound
	}
	if err := ensureIdempotentObjectVisible(context.Background(), tx, id, value.ID, value); err != nil {
		return nil, err
	}
	return &value, nil
}

func beijingDay(value time.Time) time.Time {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.UTC
	}
	local := value.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location).UTC()
}

func timePtr(value time.Time) *time.Time { return &value }
