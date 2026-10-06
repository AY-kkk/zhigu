package futures

import (
	"context"
	"fmt"
	"testing"
	"time"
	model "zhigu/server/model/futures"

	"gorm.io/datatypes"
)

func TestIngestObservationsFeedsWorkbenchAndIsIdempotent(t *testing.T) {
	db := repoTestDB(t)
	now := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)
	domain := NewDomain(db, "")
	domain.Now = func() time.Time { return now }
	if err := db.Exec(`INSERT INTO futures_operations(version,mode,reason,allowed_user_ids,updated_at) VALUES(1,'live','offline-test','[1]'::jsonb,?)`, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO futures_products(id,product_id,name,exchange,template_version,admission,created_at) VALUES('p-ingest','SHFE.CU','铜','SHFE','v1','admitted',?) ON CONFLICT (product_id) DO NOTHING`, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO futures_sources(id,source_id,version,publisher,url,status,credential_ref,rights,metrics,schedule_version,adapter_version,health,manifest,created_at)
VALUES('src-ingest','src-ingest',1,'offline','https://example.invalid','enabled','none','{"display":true,"model_use":true,"export":true,"retain_until":"2030-01-01T00:00:00Z"}'::jsonb,'["exchange_inventory_26w"]'::jsonb,'cal-v1','adapter-v1','healthy','{}'::jsonb,?)`, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO futures_series(id,product_id,contract_id,metric,unit,currency,caliber,frequency,calendar_version,caliber_hash,created_at)
VALUES('series-ingest','SHFE.CU',NULL,'exchange_inventory_26w','tonne',NULL,'standard','daily','cal-v1','hash-ingest',?)`, now).Error; err != nil {
		t.Fatal(err)
	}

	input := ObservationInput{
		SchemaVersion: "futures.record.v1", ID: "obs-ingest", Revision: 1, Scope: "public",
		ProductID: "SHFE.CU", SourceID: "src-ingest", SourceVersion: "1", SourceCluster: "cluster-ingest",
		SourceType: "industry_data", Metric: "exchange_inventory_26w", Value: ptrString("100"), Unit: "tonne", CaliberID: "standard",
		PeriodStart: "2026-10-01", PeriodEnd: "2026-10-01", TradingDay: ptrString("2026-10-01"),
		PublishedPrecision: "date", FirstObservedAt: now, RetrievedAt: now, UsableAt: now,
		Quality:      datatypes.JSON(`{"acquisition":"available","freshness":"current","verification":"source_recorded"}`),
		SourceRights: datatypes.JSON(`{"display":true,"model_use":true,"export":true}`),
		Locator:      datatypes.JSON(`{"source_url":"https://example.invalid","page":null,"row_key":"obs-ingest","quote":null}`),
		ContentHash:  "hash-obs-ingest",
	}
	result, err := domain.IngestObservations(context.Background(), Identity{OwnerID: 1, Mode: "live"}, []ObservationInput{input})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.InsertedIDs) != 1 || result.InsertedIDs[0] != input.ID {
		t.Fatalf("insert result=%+v", result)
	}
	replay, err := domain.IngestObservations(context.Background(), Identity{OwnerID: 1, Mode: "live"}, []ObservationInput{input})
	if err != nil {
		t.Fatal(err)
	}
	if len(replay.InsertedIDs) != 0 {
		t.Fatalf("idempotent replay inserted=%v", replay.InsertedIDs)
	}

	workbench, err := domain.Workbench(context.Background(), Identity{OwnerID: 1, Mode: "live"}, "SHFE.CU", "", 60, "settlement")
	if err != nil {
		t.Fatal(err)
	}
	records, _ := workbench["records"].([]any)
	if len(records) != 1 {
		t.Fatalf("workbench records=%#v", workbench["records"])
	}
	missing, _ := workbench["missing_metrics"].([]string)
	for _, metric := range missing {
		if metric == "exchange_inventory_26w" {
			t.Fatalf("ingested metric still missing: %#v", workbench)
		}
	}
}

func ptrString(value string) *string { return &value }

func TestConditionRecordsUsesLatestAvailableWindow(t *testing.T) {
	db := repoTestDB(t)
	now := time.Now().UTC()
	if err := db.Exec(`INSERT INTO futures_sources(id,source_id,version,publisher,url,status,credential_ref,rights,metrics,schedule_version,adapter_version,health,manifest)
 VALUES('window-source','window-source',1,'offline','https://example.invalid','enabled','none','{"model_use":true,"retain_until":"2099-01-01T00:00:00Z"}','[]','v1','v1','healthy','{}')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO futures_series(id,product_id,metric,unit,caliber,frequency,calendar_version,caliber_hash)
 VALUES('window-series','SHFE.CU','inventory','tonne','standard','irregular','v1','window')`).Error; err != nil {
		t.Fatal(err)
	}
	for day := 1; day <= 7; day++ {
		usable := now.Add(-time.Hour)
		if day == 7 {
			usable = now.Add(time.Hour)
		}
		period := fmt.Sprintf("2026-09-%02d", day)
		if err := db.Exec(`INSERT INTO futures_observations(id,scope,source_id,natural_key,source_version,content_hash,series_id,product_id,source_cluster,caliber_id,period_start,period_end,revision,numeric_value,unit,published_precision,first_observed_at,retrieved_at,usable_at,quality,source_type,locator)
  VALUES(?,'public','window-source',?,1,'hash','window-series','SHFE.CU','cluster','standard',?,?::date,1,?,'tonne','date',?,?,?,'{}','industry_data','{}')`, fmt.Sprintf("window-%d", day), period, period, period, day, usable, usable, usable).Error; err != nil {
			t.Fatal(err)
		}
	}
	records, ids, _, err := conditionRecords(context.Background(), db, model.Hypothesis{OwnerID: 1, Mode: "live", ProductID: "SHFE.CU"}, CheckCondition{Kind: ConditionMetricCompare, SeriesID: "window-series"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 || records[0].Period != "2026-09-04" || records[2].Period != "2026-09-06" {
		t.Fatalf("want latest available window (4,5,6), got ids=%v records=%+v", ids, records)
	}
}
