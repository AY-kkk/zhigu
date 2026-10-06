package futures

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ObservationInput struct {
	SchemaVersion      string         `json:"schema_version"`
	ID                 string         `json:"id"`
	Revision           int            `json:"revision"`
	SupersedesID       *string        `json:"supersedes_id"`
	Scope              string         `json:"scope"`
	OwnerID            *uint          `json:"owner_id"`
	Mode               *string        `json:"mode"`
	ProductID          string         `json:"product_id"`
	ContractID         *string        `json:"contract_id"`
	SourceID           string         `json:"source_id"`
	SourceVersion      string         `json:"source_version"`
	SourceCluster      string         `json:"source_cluster"`
	SourceType         string         `json:"source_type"`
	Metric             string         `json:"metric"`
	Value              *string        `json:"value"`
	Unit               string         `json:"unit"`
	Currency           *string        `json:"currency"`
	CaliberID          string         `json:"caliber_id"`
	PeriodStart        string         `json:"period_start"`
	PeriodEnd          string         `json:"period_end"`
	TradingDay         *string        `json:"trading_day"`
	PublishedAt        *time.Time     `json:"published_at"`
	PublishedPrecision string         `json:"published_precision"`
	VersionAvailableAt *time.Time     `json:"version_available_at"`
	FirstObservedAt    time.Time      `json:"first_observed_at"`
	RetrievedAt        time.Time      `json:"retrieved_at"`
	UsableAt           time.Time      `json:"usable_at"`
	Quality            datatypes.JSON `json:"quality"`
	SourceRights       datatypes.JSON `json:"rights"`
	Locator            datatypes.JSON `json:"locator"`
	ContentHash        string         `json:"content_hash"`
}

type ObservationIngestResult struct {
	InsertedIDs []string `json:"inserted_ids"`
	Evaluated   int      `json:"evaluated_conditions"`
}

func (d *Domain) IngestObservations(ctx context.Context, id Identity, inputs []ObservationInput) (ObservationIngestResult, error) {
	if len(inputs) == 0 || len(inputs) > 500 {
		return ObservationIngestResult{}, ErrInvalidInput
	}
	if err := d.AuthorizeExecution(ctx, id); err != nil {
		return ObservationIngestResult{}, err
	}
	result := ObservationIngestResult{InsertedIDs: make([]string, 0, len(inputs))}
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, input := range inputs {
			recordID, inserted, err := ingestObservation(tx, id, input)
			if err != nil {
				return err
			}
			if inserted {
				result.InsertedIDs = append(result.InsertedIDs, recordID)
			}
		}
		return nil
	})
	if err != nil {
		return ObservationIngestResult{}, err
	}
	evaluated, err := EvaluateDueConditions(ctx, d.DB, d.Now().UTC(), 500)
	result.Evaluated = evaluated
	return result, err
}

func ingestObservation(tx *gorm.DB, id Identity, input ObservationInput) (string, bool, error) {
	if input.SchemaVersion != "futures.record.v1" || input.ID == "" || input.ContentHash == "" || input.Revision < 1 ||
		input.ProductID == "" || input.SourceID == "" || input.SourceVersion == "" || input.SourceCluster == "" ||
		input.SourceType == "" || input.Metric == "" || input.Unit == "" || input.CaliberID == "" {
		return "", false, ErrInvalidInput
	}
	if !oneOf(input.Scope, "public", "private") || !oneOf(input.SourceType, "official_release", "market_data", "industry_data", "user_document", "calculation") ||
		!oneOf(input.PublishedPrecision, "timestamp", "date", "unknown") {
		return "", false, ErrInvalidInput
	}
	if input.Scope == "public" && (input.OwnerID != nil || input.Mode != nil) {
		return "", false, ErrInvalidInput
	}
	if input.Scope == "private" && (input.OwnerID == nil || *input.OwnerID != id.OwnerID || input.Mode == nil || *input.Mode != id.Mode) {
		return "", false, ErrInvalidInput
	}
	if input.FirstObservedAt.IsZero() || input.RetrievedAt.IsZero() || input.UsableAt.IsZero() {
		return "", false, ErrInvalidInput
	}
	if input.UsableAt.Before(input.FirstObservedAt) || input.RetrievedAt.Before(input.FirstObservedAt) {
		return "", false, ErrInvalidInput
	}
	if _, err := time.Parse("2006-01-02", input.PeriodStart); err != nil {
		return "", false, ErrInvalidInput
	}
	if _, err := time.Parse("2006-01-02", input.PeriodEnd); err != nil {
		return "", false, ErrInvalidInput
	}
	if input.TradingDay != nil {
		if _, err := time.Parse("2006-01-02", *input.TradingDay); err != nil {
			return "", false, ErrInvalidInput
		}
	}
	if input.Value != nil {
		if _, err := decimal.NewFromString(*input.Value); err != nil {
			return "", false, ErrInvalidInput
		}
	}
	var quality struct {
		Acquisition  string `json:"acquisition"`
		Freshness    string `json:"freshness"`
		Verification string `json:"verification"`
	}
	if err := json.Unmarshal(input.Quality, &quality); err != nil {
		return "", false, ErrInvalidInput
	}
	if !oneOf(quality.Acquisition, "available", "missing", "fetch_failed", "not_entitled") ||
		!oneOf(quality.Freshness, "current", "delayed", "stale", "unknown") ||
		!oneOf(quality.Verification, "source_recorded", "cross_checked", "conflicted", "retracted") {
		return "", false, ErrInvalidInput
	}
	if quality.Acquisition != "available" && input.Value != nil {
		return "", false, ErrInvalidInput
	}

	var source struct {
		Version int64
		Metrics datatypes.JSON
		Rights  datatypes.JSON
	}
	err := tx.Raw(`SELECT version,metrics,rights FROM futures_sources WHERE source_id=? AND status='enabled' AND deleted_at IS NULL`, input.SourceID).Scan(&source).Error
	if err != nil {
		return "", false, err
	}
	if source.Version == 0 || fmt.Sprint(source.Version) != input.SourceVersion {
		return "", false, ErrForbidden
	}
	var sourceRights struct {
		ModelUse bool `json:"model_use"`
		Display  bool `json:"display"`
	}
	if err := json.Unmarshal(source.Rights, &sourceRights); err != nil || !sourceRights.ModelUse || !sourceRights.Display {
		return "", false, ErrForbidden
	}
	var metrics []string
	if err := json.Unmarshal(source.Metrics, &metrics); err != nil {
		return "", false, ErrInvalidInput
	}
	if !containsString(metrics, input.Metric) {
		return "", false, ErrForbidden
	}

	var series struct {
		ProductID, Metric, Unit, Caliber string
		Currency                         *string
	}
	err = tx.Raw(`SELECT product_id,metric,unit,caliber,currency FROM futures_series
WHERE product_id=? AND metric=? AND caliber=? AND contract_id IS NOT DISTINCT FROM ? AND deleted_at IS NULL`,
		input.ProductID, input.Metric, input.CaliberID, input.ContractID).Scan(&series).Error
	if err != nil {
		return "", false, err
	}
	if series.ProductID == "" || series.Unit != input.Unit || (series.Currency == nil) != (input.Currency == nil) ||
		(series.Currency != nil && input.Currency != nil && *series.Currency != *input.Currency) {
		return "", false, ErrInvalidInput
	}
	var contractCount int64
	if input.ContractID != nil {
		if err := tx.Raw(`SELECT count(*) FROM futures_contracts WHERE id=? AND product_id=? AND kind='actual' AND deleted_at IS NULL`, *input.ContractID, input.ProductID).Scan(&contractCount).Error; err != nil {
			return "", false, err
		}
		if contractCount != 1 {
			return "", false, ErrForbidden
		}
	}
	var seriesID string
	if err := tx.Raw(`SELECT id FROM futures_series WHERE product_id=? AND metric=? AND caliber=? AND contract_id IS NOT DISTINCT FROM ? AND deleted_at IS NULL`,
		input.ProductID, input.Metric, input.CaliberID, input.ContractID).Scan(&seriesID).Error; err != nil {
		return "", false, err
	}
	if seriesID == "" {
		return "", false, ErrNotFound
	}

	supersedesID := input.SupersedesID
	if supersedesID == nil && input.Revision > 1 {
		var prior string
		if err := tx.Raw(`SELECT id FROM futures_observations WHERE series_id=? AND period_start=?::date AND revision<? AND deleted_at IS NULL ORDER BY revision DESC LIMIT 1`,
			seriesID, input.PeriodStart, input.Revision).Scan(&prior).Error; err != nil {
			return "", false, err
		}
		if prior == "" {
			return "", false, ErrRevisionConflict
		}
		supersedesID = &prior
	}
	if supersedesID != nil {
		var priorRevision int
		if err := tx.Raw(`SELECT revision FROM futures_observations WHERE id=? AND series_id=? AND period_start=?::date AND deleted_at IS NULL`,
			*supersedesID, seriesID, input.PeriodStart).Scan(&priorRevision).Error; err != nil {
			return "", false, err
		}
		if priorRevision != input.Revision-1 {
			return "", false, ErrRevisionConflict
		}
	}
	var existing struct {
		ID, Scope, ContentHash string
		OwnerID                *uint
	}
	err = tx.Raw(`SELECT id,scope,content_hash,owner_id FROM futures_observations WHERE id=?`, input.ID).Scan(&existing).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return "", false, err
	}
	if existing.ID != "" {
		sameOwner := (existing.Scope == "public" && input.Scope == "public" && existing.OwnerID == nil) ||
			(existing.Scope == "private" && input.Scope == "private" && existing.OwnerID != nil && input.OwnerID != nil && *existing.OwnerID == *input.OwnerID)
		if !sameOwner || existing.ContentHash != input.ContentHash {
			return "", false, ErrForbidden
		}
		return input.ID, false, nil
	}
	naturalKey := hashJSON(map[string]any{
		"series_id": seriesID, "product_id": input.ProductID, "contract_id": input.ContractID,
		"period_start": input.PeriodStart, "period_end": input.PeriodEnd, "caliber_id": input.CaliberID, "source_cluster": input.SourceCluster,
	})
	var numericValue any
	if input.Value != nil {
		numericValue = decimal.RequireFromString(*input.Value)
	}
	var ownerID any
	if input.OwnerID != nil {
		ownerID = *input.OwnerID
	}
	var mode any
	if input.Mode != nil {
		mode = *input.Mode
	}
	res := tx.Exec(`INSERT INTO futures_observations(
id,scope,owner_id,mode,source_id,natural_key,source_version,content_hash,series_id,product_id,contract_id,source_cluster,caliber_id,
period_start,period_end,trading_day,revision,supersedes_id,numeric_value,text_value,unit,published_at,published_precision,version_available_at,
first_observed_at,retrieved_at,usable_at,quality,source_type,locator,created_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?::date,?::date,?::date,?,?,?,NULL,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT DO NOTHING`,
		input.ID, input.Scope, ownerID, mode, input.SourceID, naturalKey, input.SourceVersion, input.ContentHash, seriesID, input.ProductID,
		input.ContractID, input.SourceCluster, input.CaliberID, input.PeriodStart, input.PeriodEnd, input.TradingDay, input.Revision,
		supersedesID, numericValue, input.Unit, input.PublishedAt, input.PublishedPrecision, input.VersionAvailableAt, input.FirstObservedAt,
		input.RetrievedAt, input.UsableAt, input.Quality, input.SourceType, input.Locator, time.Now().UTC())
	if res.Error != nil {
		return "", false, res.Error
	}
	return input.ID, res.RowsAffected == 1, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
