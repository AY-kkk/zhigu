package futures

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	model "zhigu/server/model/futures"
)

func reviewRegressionDraft(t *testing.T, db *gorm.DB) model.Draft {
	t.Helper()
	now := time.Now().UTC()
	parsed := 1
	value := model.Draft{
		ID: "review_regression_draft", OwnerID: 1, Mode: "live", Revision: 1,
		Input:      model.DraftInput{ProductID: "SHFE.CU", HorizonDays: 14, Text: "这是一条仅用于回归验证的合成研究输入，不用于真实交易"},
		ParseState: "succeeded", ParsedRevision: &parsed,
		Claims:    datatypes.JSON(`[{"id":"claim_regression","kind":"hypothesis","text":"synthetic-only","locator":null}]`),
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&value).Error; err != nil {
		t.Fatal(err)
	}
	return value
}

func TestReviewRegressionCreateRunPersistsCompleteVersions(t *testing.T) {
	db := repoTestDB(t)
	draft := reviewRegressionDraft(t, db)
	domain := NewDomain(db, "").WithMode("live")
	claims := []map[string]any{{"id": "claim_regression", "kind": "hypothesis", "text": "synthetic-only", "locator": nil}}
	run, err := domain.CreateRun(context.Background(), Identity{OwnerID: 1, Mode: "live"}, "regression-run-create", RunCreateInput{
		DraftID: draft.ID, ExpectedRevision: 1, Claims: claims, AcceptLimits: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if run["id"] == "" {
		t.Fatal("run was not created")
	}
	var stored model.Run
	if err := db.First(&stored, "id=?", run["id"]).Error; err != nil {
		t.Fatal(err)
	}
	var versions map[string]any
	if err := json.Unmarshal(stored.Versions, &versions); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"template", "formula", "policy", "model_config", "source_manifest"} {
		if versions[key] == nil {
			t.Fatalf("missing frozen version field %q in %#v", key, versions)
		}
	}
	var snapshot map[string]any
	if err := json.Unmarshal(stored.TaskSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	meta, _ := snapshot["version_meta"].(map[string]any)
	for _, key := range []string{"draft_revision", "input_hash", "claims_hash"} {
		if meta[key] == nil {
			t.Fatalf("missing frozen task metadata %q in %#v", key, snapshot)
		}
	}
}

func TestReviewRegressionEvidenceUsesJoinedSeriesAndSourceRights(t *testing.T) {
	db := repoTestDB(t)
	draft := reviewRegressionDraft(t, db)
	now := time.Now().UTC()
	run := model.Run{ID: "review_regression_run", OwnerID: 1, Mode: "live", DraftID: draft.ID, DraftRevision: 1,
		Status: "succeeded", Stage: "complete", AsOf: now, HorizonEnd: now.Add(time.Hour), IdempotencyKey: "evidence", RequestHash: "evidence",
		ClaimsSnapshot: datatypes.JSON(`[]`), Versions: datatypes.JSON(`{"draft_revision":1}`), TaskSnapshot: datatypes.JSON(`{}`), CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO futures_products(id,product_id,name,exchange,template_version,admission,created_at) VALUES('p','SHFE.CU','铜','SHFE','v1','admitted',?) ON CONFLICT (product_id) DO NOTHING`, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO futures_sources(id,source_id,version,publisher,url,status,credential_ref,rights,metrics,schedule_version,adapter_version,health,manifest,created_at)
VALUES('src','src',1,'offline','https://example.invalid','enabled','none','{"export":true}'::jsonb,'["inventory"]'::jsonb,'cal-v1','adapter-v1','healthy','{}'::jsonb,?)`, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO futures_series(id,product_id,contract_id,metric,unit,currency,caliber,frequency,calendar_version,caliber_hash,created_at)
VALUES('series','SHFE.CU',NULL,'inventory','tonne','CNY','standard','daily','cal-v1','hash',?)`, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO futures_observations(id,scope,owner_id,mode,source_id,natural_key,source_version,content_hash,series_id,product_id,contract_id,source_cluster,caliber_id,period_start,period_end,trading_day,revision,numeric_value,unit,published_at,published_precision,version_available_at,first_observed_at,retrieved_at,usable_at,quality,source_type,locator,created_at)
VALUES('obs','public',NULL,NULL,'src','natural',1,'content','series','SHFE.CU',NULL,'cluster','standard',?,?::date,?::date,1,987654321,'tonne',?,'date',?,?,?,?,'{}'::jsonb,'industry_data','{}'::jsonb,?)`, now.Format("2006-01-02"), now.Format("2006-01-02"), now.Format("2006-01-02"), now, now, now, now, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO futures_evidence_links(id,owner_id,mode,run_id,record_id,created_at) VALUES('link',1,'live',?, 'obs',?)`, run.ID, now).Error; err != nil {
		t.Fatal(err)
	}
	domain := NewDomain(db, "").WithMode("live")
	evidence, err := domain.Evidence(context.Background(), Identity{OwnerID: 1, Mode: "live"}, run.ID, "obs")
	if err != nil {
		t.Fatal(err)
	}
	if evidence["metric"] != "inventory" || evidence["currency"] != "CNY" {
		t.Fatalf("joined evidence fields missing: %#v", evidence)
	}
	if _, err := domain.ExportRun(context.Background(), Identity{OwnerID: 1, Mode: "live"}, run.ID); err != nil {
		t.Fatal(err)
	}
}

func TestReviewRegressionCalculationRoundTripPreservesUnit(t *testing.T) {
	db := repoTestDB(t)
	draft := reviewRegressionDraft(t, db)
	now := time.Now().UTC()
	run := model.Run{ID: "review_regression_calc_run", OwnerID: 1, Mode: "live", DraftID: draft.ID, DraftRevision: 1,
		Status: "running", Stage: "evidence", AsOf: now, HorizonEnd: now.Add(time.Hour), IdempotencyKey: "calc", RequestHash: "calc",
		ClaimsSnapshot: datatypes.JSON(`[]`), Versions: datatypes.JSON(`{"draft_revision":1}`), TaskSnapshot: datatypes.JSON(`{}`), CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO futures_manifests(id,owner_id,mode,product_id,as_of,manifest_hash,versions,created_at) VALUES('manifest-calc',1,'live','SHFE.CU',?,?,'{}'::jsonb,?)`, now, "hash", now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO futures_products(id,product_id,name,exchange,template_version,admission,created_at) VALUES('p-calc','SHFE.CU','铜','SHFE','v1','admitted',?) ON CONFLICT (product_id) DO NOTHING`, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO futures_sources(id,source_id,version,publisher,url,status,credential_ref,rights,metrics,schedule_version,adapter_version,health,manifest,created_at) VALUES('src-calc','src-calc',1,'offline','https://example.invalid','enabled','none','{"model_use":true,"export":true,"retain_until":"2030-01-01T00:00:00Z"}'::jsonb,'[]'::jsonb,'cal-v1','adapter-v1','healthy','{}'::jsonb,?)`, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO futures_series(id,product_id,contract_id,metric,unit,currency,caliber,frequency,calendar_version,caliber_hash,created_at) VALUES('series-calc','SHFE.CU',NULL,'inventory','CNY/tonne','CNY','standard','daily','cal-v1','hash-calc',?)`, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO futures_contracts(id,product_id,kind,last_trading_at,price_precision,price_unit,multiplier,calendar_version,source_version,created_at) VALUES('contract-calc','SHFE.CU','actual',?,2,'CNY/tonne',1,'cal-v1','1',?)`, now.AddDate(1, 0, 0), now).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		id, natural string
		value       int64
	}{{"spot", "spot", 500}, {"futures", "futures", 0}} {
		if err := db.Exec(`INSERT INTO futures_observations(id,scope,owner_id,mode,source_id,natural_key,source_version,content_hash,series_id,product_id,contract_id,source_cluster,caliber_id,period_start,period_end,trading_day,revision,numeric_value,unit,published_at,published_precision,version_available_at,first_observed_at,retrieved_at,usable_at,quality,source_type,locator,created_at)
VALUES(?,'public',NULL,NULL,'src-calc',?,1,?,'series-calc','SHFE.CU',NULL,'cluster','standard',?,?::date,?::date,1,?,'CNY/tonne',?,'date',?,?,?,?,'{}'::jsonb,'market_data','{}'::jsonb,?)`, row.id, row.natural, row.id, now.Format("2006-01-02"), now.Format("2006-01-02"), now.Format("2006-01-02"), decimal.NewFromInt(row.value), now, now, now, now, now, now).Error; err != nil {
			t.Fatal(err)
		}
		if row.id == "futures" {
			if err := db.Exec(`UPDATE futures_observations SET contract_id='contract-calc' WHERE id='futures'`).Error; err != nil {
				t.Fatal(err)
			}
		}
		if err := db.Exec(`INSERT INTO futures_manifest_records(manifest_id,record_id,position) VALUES('manifest-calc',?,?)`, row.id, row.id == "futures").Error; err != nil {
			t.Fatal(err)
		}
	}
	domain := NewDomain(db, "").WithMode("live")
	result, err := domain.ExecuteInternalTool(context.Background(), InternalToolCall{
		GrantSpec: GrantSpec{Domain: "futures", OwnerID: 1, Mode: "live", RunID: run.ID, TaskID: "task", Generation: 1, ManifestID: "manifest-calc", Stage: "support", ArgsHash: "hash"},
		Name:      "futures_calculate", Arguments: map[string]any{"formula": "basis", "inputs": []any{map[string]any{"record_id": "spot", "role": "spot"}, map[string]any{"record_id": "futures", "role": "futures"}}, "as_of": now.Format(time.RFC3339)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result["value"] != "500" || result["unit"] != "CNY/tonne" {
		t.Fatalf("calculation round trip mismatch: %#v", result)
	}
}

func TestReviewRegressionHistoryFKDoesNotBlockCurrentRevision(t *testing.T) {
	db := repoTestDB(t)
	draft := reviewRegressionDraft(t, db)
	if err := db.Exec(`INSERT INTO futures_draft_versions(draft_id,owner_id,mode,revision,input,claims,created_at) VALUES(?,1,'live',1,'{}'::jsonb,'[]'::jsonb,?)`, draft.ID, time.Now().UTC()).Error; err != nil {
		t.Fatal(err)
	}
	res := db.Exec(`UPDATE futures_drafts SET revision=2 WHERE id=?`, draft.ID)
	if res.Error != nil {
		t.Fatal(res.Error)
	}
	if res.RowsAffected != 1 {
		t.Fatal("current draft revision was not updated")
	}
}

func TestReviewRegressionIdempotentCreateDoesNotReplayDeletedBody(t *testing.T) {
	db := repoTestDB(t)
	ctx := context.Background()
	domain := NewDomain(db, "").WithMode("live")
	identity := Identity{OwnerID: 1, Mode: "live"}
	now := time.Now().UTC()
	if err := db.Exec(`INSERT INTO futures_products(id,product_id,name,exchange,template_version,admission,created_at) VALUES('p-del','SHFE.CU','铜','SHFE','v1','admitted',?) ON CONFLICT (product_id) DO NOTHING`, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO futures_admission(product_id,version,status,evidence_manifest_id,reason,created_at) VALUES('SHFE.CU',1,'admitted','offline','test',?)`, now).Error; err != nil {
		t.Fatal(err)
	}
	input := model.DraftInput{ProductID: "SHFE.CU", HorizonDays: 14, Text: "这是一条仅用于验证删除后幂等重放不返回正文的合成文本"}
	first, err := domain.CreateDraft(ctx, identity, "regression-delete-replay", input)
	if err != nil {
		t.Fatal(err)
	}
	id := first["id"].(string)
	if _, err := domain.DeleteDraft(ctx, identity, id, "regression-delete"); err != nil {
		t.Fatal(err)
	}
	_, err = domain.CreateDraft(ctx, identity, "regression-delete-replay", input)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted draft body was replayed or wrong error: %v", err)
	}
}
