package strategy_market

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"gorm.io/gorm"

	model "zhigu/server/model/strategy"
)

func draftCount(t *testing.T, db *gorm.DB, itemID, versionID string) int64 {
	t.Helper()
	var n int64
	db.Model(&model.Draft{}).Where("origin_market_item_id = ? AND origin_market_version_id = ?", itemID, versionID).Count(&n)
	return n
}

func TestCopyHappyAndIdempotent(t *testing.T) {
	s, db := setupMarket(t)
	ctx := context.Background()
	itemID, versionID, rev := publishFixture(t, s, adminUID, "c1")

	out, replayed, err := s.Copy(ctx, adminUID, "copy-key-1", itemID, CopyReq{MarketVersionID: versionID})
	if err != nil || replayed {
		t.Fatalf("Copy: %v replayed=%v", err, replayed)
	}
	draftID, _ := out["draft_id"].(string)
	if draftID == "" || out["next_path"] != "/app/strategies?draft_id="+draftID {
		t.Fatalf("copy response = %v", out)
	}
	origin := out["origin"].(map[string]any)
	if origin["market_item_id"] != itemID || origin["market_version_id"] != versionID || origin["version_no"] != 1 {
		t.Fatalf("origin = %v", origin)
	}
	// 绝不自动替用户选择验证样本标的。
	var d model.Draft
	if err := db.Where("id = ?", draftID).Take(&d).Error; err != nil {
		t.Fatalf("draft missing: %v", err)
	}
	if d.InstrumentID != nil {
		t.Fatalf("draft auto-picked instrument %s", *d.InstrumentID)
	}
	if d.Status != "needs_clarification" {
		t.Fatalf("status = %s", d.Status)
	}
	var fs map[string]string
	_ = json.Unmarshal(d.FieldSources, &fs)
	if fs["entry"] != "market_default" || fs["instrument_id"] != "market_default" {
		t.Fatalf("field_sources = %v", fs)
	}
	var cfg map[string]any
	_ = json.Unmarshal(d.BacktestConfigDraft, &cfg)
	if cfg["initial_cash"] != "100000" || cfg["currency"] != "CNY" {
		t.Fatalf("config draft = %v", cfg)
	}

	// 同 key 同体 → 重放同一草稿，不产生第二份副本。
	out2, replayed, err := s.Copy(ctx, adminUID, "copy-key-1", itemID, CopyReq{MarketVersionID: versionID})
	if err != nil || !replayed || out2["draft_id"] != draftID {
		t.Fatalf("replay: %v replayed=%v out=%v", err, replayed, out2)
	}
	if n := draftCount(t, db, itemID, versionID); n != 1 {
		t.Fatalf("draft rows = %d, want 1", n)
	}
	// 同 key 异体 → 409。
	if _, _, err := s.Copy(ctx, adminUID, "copy-key-1", itemID, CopyReq{MarketVersionID: versionID, Overrides: &CopyOverrides{InitialCash: "200000"}}); errorCode(err) != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("want IDEMPOTENCY_CONFLICT, got %v", err)
	}
	_ = rev
}

func TestCopyConcurrencyOneDraft(t *testing.T) {
	s, db := setupMarket(t)
	ctx := context.Background()
	itemID, versionID, _ := publishFixture(t, s, adminUID, "c2")

	var wg sync.WaitGroup
	ids := make(chan string, 4)
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, _, err := s.Copy(ctx, adminUID, "copy-race", itemID, CopyReq{MarketVersionID: versionID})
			if err != nil {
				errs <- err
				return
			}
			ids <- out["draft_id"].(string)
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent copy: %v", err)
	}
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		} else if id != first {
			t.Fatalf("concurrent copies created %s and %s", first, id)
		}
	}
	if n := draftCount(t, db, itemID, versionID); n != 1 {
		t.Fatalf("draft rows = %d, want 1", n)
	}
}

func TestCopyGuards(t *testing.T) {
	s, db := setupMarket(t)
	ctx := context.Background()
	itemID, versionID, rev := publishFixture(t, s, adminUID, "c3")

	// 发布新版本后，旧版本请求 → MARKET_VERSION_CHANGED。
	created2, _, err := s.CreateVersion(ctx, adminUID, "c3-v2", itemID, CreateVersionReq{Revision: rev, Version: validContent()})
	if err != nil {
		t.Fatalf("CreateVersion: %v", err)
	}
	newVersion, _ := created2["version_id"].(string)
	mustValidate(t, s, adminUID, "c3-v2val", itemID, newVersion, rev+1)
	mustPublish(t, s, adminUID, "c3-v2pub", itemID, newVersion, rev+2)
	if _, _, err := s.Copy(ctx, adminUID, "c3-old", itemID, CopyReq{MarketVersionID: versionID}); errorCode(err) != "MARKET_VERSION_CHANGED" {
		t.Fatalf("want MARKET_VERSION_CHANGED, got %v", err)
	}
	current := itemIDVersion(t, db, itemID)

	// 校验被撤销（失败态）→ 不可复制。
	db.Exec(`UPDATE finance_strategy_market_versions SET validation_status = 'failed' WHERE id = ?`, current)
	if _, _, err := s.Copy(ctx, adminUID, "c3-nocopy", itemID, CopyReq{MarketVersionID: current}); errorCode(err) != "MARKET_NOT_COPYABLE" {
		t.Fatalf("want MARKET_NOT_COPYABLE, got %v", err)
	}
	db.Exec(`UPDATE finance_strategy_market_versions SET validation_status = 'passed' WHERE id = ?`, current)

	// 覆盖参数校验。
	if _, _, err := s.Copy(ctx, adminUID, "c3-cur", itemID, CopyReq{MarketVersionID: current, Overrides: &CopyOverrides{InstrumentID: "600519.SH", Currency: "HKD"}}); errorCode(err) != "STRATEGY_INVALID" {
		t.Fatalf("want currency mismatch rejection, got %v", err)
	}
	if _, _, err := s.Copy(ctx, adminUID, "c3-cash", itemID, CopyReq{MarketVersionID: current, Overrides: &CopyOverrides{InitialCash: "-1"}}); errorCode(err) != "STRATEGY_INVALID" {
		t.Fatalf("want initial_cash rejection, got %v", err)
	}
	// 指定标的 → 草稿就绪且来源标注 user。
	out, _, err := s.Copy(ctx, adminUID, "c3-inst", itemID, CopyReq{MarketVersionID: current, Overrides: &CopyOverrides{InstrumentID: "600519.SH", InitialCash: "200000", Currency: "CNY"}})
	if err != nil {
		t.Fatalf("Copy with override: %v", err)
	}
	if out["status"] != "ready" {
		t.Fatalf("status = %v", out["status"])
	}
	var d model.Draft
	db.Where("id = ?", out["draft_id"]).Take(&d)
	if d.InstrumentID == nil || *d.InstrumentID != "600519.SH" || len(d.DSL) == 0 {
		t.Fatalf("draft not ready: %+v", d)
	}
	var fs map[string]string
	_ = json.Unmarshal(d.FieldSources, &fs)
	if fs["instrument_id"] != "user" || fs["backtest_config.initial_cash"] != "user" {
		t.Fatalf("field_sources = %v", fs)
	}

	// 下架后：停止新复制，已有副本保留；同 key 重放仍返回原副本。
	itemID2, versionID2, rev2 := publishFixture(t, s, adminUID, "c4")
	outKeep, _, err := s.Copy(ctx, adminUID, "c4-keep", itemID2, CopyReq{MarketVersionID: versionID2})
	if err != nil {
		t.Fatalf("Copy before withdraw: %v", err)
	}
	if _, _, err := s.Withdraw(ctx, adminUID, "c4-wd", itemID2, WithdrawReq{Revision: rev2, Reason: "整理中"}); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}
	if _, _, err := s.Copy(ctx, adminUID, "c4-new", itemID2, CopyReq{MarketVersionID: versionID2}); errorCode(err) != "MARKET_ITEM_WITHDRAWN" {
		t.Fatalf("want MARKET_ITEM_WITHDRAWN, got %v", err)
	}
	replay, replayed, err := s.Copy(ctx, adminUID, "c4-keep", itemID2, CopyReq{MarketVersionID: versionID2})
	if err != nil || !replayed || replay["draft_id"] != outKeep["draft_id"] {
		t.Fatalf("replay after withdraw: %v replayed=%v", err, replayed)
	}
	// 删除副本后同 key 重试 → 410 COPY_TARGET_GONE，且不新建草稿。
	db.Exec(`DELETE FROM finance_strategy_drafts WHERE id = ?`, outKeep["draft_id"])
	if _, _, err := s.Copy(ctx, adminUID, "c4-keep", itemID2, CopyReq{MarketVersionID: versionID2}); errorCode(err) != "COPY_TARGET_GONE" {
		t.Fatalf("want COPY_TARGET_GONE, got %v", err)
	}
}

// itemIDVersion returns the item's current version id.
func itemIDVersion(t *testing.T, db *gorm.DB, itemID string) string {
	t.Helper()
	var item model.MarketItem
	if err := db.Where("id = ?", itemID).Take(&item).Error; err != nil {
		t.Fatalf("item: %v", err)
	}
	if item.CurrentVersionID == nil {
		t.Fatalf("item %s has no current version", itemID)
	}
	return *item.CurrentVersionID
}
