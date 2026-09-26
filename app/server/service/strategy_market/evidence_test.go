package strategy_market

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	model "zhigu/server/model/strategy"
	"zhigu/server/service/strategy"
)

// evidenceFixture publishes a market version and attaches one succeeded
// verification backtest owned by adminUID, bound to the validated DSL/instrument.
func evidenceFixture(t *testing.T, s *Service, db *gorm.DB) (itemID, versionID, runID string, rev int) {
	t.Helper()
	itemID, versionID, rev = publishFixture(t, s, adminUID, "e1")

	var tpl strategy.MarketTemplate
	if err := json.Unmarshal([]byte(ruleTemplateJSON), &tpl); err != nil {
		t.Fatalf("template: %v", err)
	}
	res, err := strategy.CompileEditor(tpl.EditorState, "600519.SH")
	if err != nil || res.Status != "ready" {
		t.Fatalf("compile: %v %v", err, res.Status)
	}
	docRaw, _ := json.Marshal(res.Document)

	now := nowUTC()
	if err := db.Create(&model.Strategy{ID: "st_e1", OwnerID: adminUID, Name: "验证策略", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("strategy: %v", err)
	}
	if err := db.Create(&model.Version{
		ID: "stv_e1", StrategyID: "st_e1", OwnerID: adminUID, Revision: 1, Name: "验证策略",
		DSL: datatypes.JSON(docRaw), DSLHash: "h", Compiled: datatypes.JSON([]byte("{}")),
		CompilerVersion: strategy.CompilerVersionV2, CreatedAt: now,
	}).Error; err != nil {
		t.Fatalf("version: %v", err)
	}
	resultHash := "result-hash-1"
	runID = "bt_e1"
	cfg := map[string]any{"instrument_id": "600519.SH", "initial_cash": "100000", "currency": "CNY", "private_note": "PRIVATE_INPUT"}
	cfgRaw, _ := json.Marshal(cfg)
	manRaw, _ := json.Marshal(map[string]any{"data_snapshot_id": "snap-1"})
	if err := db.Create(&model.BacktestRun{
		ID: runID, OwnerID: adminUID, StrategyID: "st_e1", StrategyVersionID: "stv_e1",
		Status: "succeeded", Progress: datatypes.JSON([]byte("{}")),
		Config: datatypes.JSON(cfgRaw), ConfigHash: "ch", Manifest: datatypes.JSON(manRaw),
		ResultSummary: datatypes.JSON([]byte(`{"total_return":"0.12"}`)), ResultHash: &resultHash,
		IdempotencyKey: "ik-e1", RequestHash: "rh-e1", ExecutionEpoch: 1, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("run: %v", err)
	}
	if err := db.Create(&model.Result{
		RunID: runID, Metrics: datatypes.JSON([]byte(`{"total_return":"0.12"}`)),
		Signals: datatypes.JSON([]byte("[]")), Assumptions: datatypes.JSON([]byte(`["历史模拟，不代表真实成交"]`)),
		Quality: datatypes.JSON([]byte("{}")), ResultHash: resultHash, CreatedAt: now,
	}).Error; err != nil {
		t.Fatalf("result: %v", err)
	}
	for i, day := range []string{"2026-09-01", "2026-09-02", "2026-09-03"} {
		if err := db.Create(&model.EquityRow{
			RunID: runID, TradeDate: day, Cash: "50000", PositionQty: "100",
			MarketValue: "50000", Equity: "10000" + string(rune('0'+i)),
		}).Error; err != nil {
			t.Fatalf("equity: %v", err)
		}
	}
	if err := db.Create(&model.Order{ID: "ord_e1", RunID: runID, Side: "buy", Status: "filled", Qty: "100", CreatedAt: now, Payload: datatypes.JSON([]byte("{}"))}).Error; err != nil {
		t.Fatalf("order: %v", err)
	}
	if err := db.Create(&model.Fill{ID: "fill_e1", RunID: runID, OrderID: "ord_e1", FillDate: "2026-09-01", Qty: "100", Price: "100", Fees: datatypes.JSON([]byte(`[{"kind":"commission","amount":"5"}]`)), CashDelta: "-10005", Payload: datatypes.JSON([]byte("{}")), CreatedAt: now}).Error; err != nil {
		t.Fatalf("fill: %v", err)
	}
	return itemID, versionID, runID, rev
}

func TestEvidenceImportReviewFlow(t *testing.T) {
	s, db := setupMarket(t)
	ctx := context.Background()
	itemID, versionID, runID, rev := evidenceFixture(t, s, db)

	out, _, err := s.ImportEvidence(ctx, adminUID, "ev-imp", itemID, versionID, EvidenceImportReq{Revision: rev, SourceRunID: runID, RightsNote: "已获展示授权"})
	if err != nil {
		t.Fatalf("ImportEvidence: %v", err)
	}
	evidenceID, _ := out["evidence_id"].(string)
	if out["status"] != EvidencePending {
		t.Fatalf("status = %v", out["status"])
	}

	// 未批准前公开不可读；pending 不算回测证据。
	if _, err := s.EvidenceSection(ctx, itemID, evidenceID, "overview", "", 0); err == nil || errorCode(err) != "NOT_FOUND" {
		t.Fatalf("pending evidence must not be public, got %v", err)
	}
	list, _ := s.List(ctx, ListQuery{})
	if list["items"].([]MarketCard)[0].BacktestStatus != BacktestNotTested {
		t.Fatalf("pending evidence counted as backtest evidence")
	}

	if _, _, err := s.ReviewEvidence(ctx, adminUID, "ev-app", itemID, evidenceID, EvidenceReviewReq{Revision: rev + 1, Decision: "approve", Reason: "核对通过"}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	overview, err := s.EvidenceSection(ctx, itemID, evidenceID, "overview", "", 0)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	raw, _ := json.Marshal(overview)
	if strings.Contains(string(raw), runID) || strings.Contains(string(raw), "PRIVATE_INPUT") || strings.Contains(string(raw), "private_note") {
		t.Fatalf("private fields leaked: %s", raw)
	}
	if overview["result_hash"] != "result-hash-1" {
		t.Fatalf("overview = %v", overview)
	}
	list, _ = s.List(ctx, ListQuery{})
	if list["items"].([]MarketCard)[0].BacktestStatus != BacktestHasEvidence {
		t.Fatalf("approved evidence not reflected")
	}

	// 曲线与成交分页。
	eq, err := s.EvidenceSection(ctx, itemID, evidenceID, "equity", "", 2)
	if err != nil {
		t.Fatalf("equity: %v", err)
	}
	items := eq["items"].([]json.RawMessage)
	if len(items) != 2 || eq["next_cursor"] == nil {
		t.Fatalf("equity page = %v", eq)
	}
	eq2, err := s.EvidenceSection(ctx, itemID, evidenceID, "equity", *eq["next_cursor"].(*string), 2)
	if err != nil || len(eq2["items"].([]json.RawMessage)) != 1 {
		t.Fatalf("equity page2 = %v err=%v", eq2, err)
	}
	tr, err := s.EvidenceSection(ctx, itemID, evidenceID, "trades", "", 100)
	if err != nil || len(tr["items"].([]json.RawMessage)) != 1 {
		t.Fatalf("trades = %v err=%v", tr, err)
	}

	// 撤销后公开查询不得返回原内容。
	if _, _, err := s.ReviewEvidence(ctx, adminUID, "ev-rev", itemID, evidenceID, EvidenceReviewReq{Revision: rev + 2, Decision: "revoke", Reason: "授权撤回"}); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := s.EvidenceSection(ctx, itemID, evidenceID, "overview", "", 0); err == nil || errorCode(err) != "NOT_FOUND" {
		t.Fatalf("revoked evidence must not be public, got %v", err)
	}
}

func TestEvidenceGuards(t *testing.T) {
	s, db := setupMarket(t)
	ctx := context.Background()
	itemID, versionID, runID, rev := evidenceFixture(t, s, db)

	// 他人的回测不可导入（统一 404）。
	db.Exec(`UPDATE finance_backtest_runs SET owner_id = ? WHERE id = ?`, otherUID, runID)
	if _, _, err := s.ImportEvidence(ctx, adminUID, "g1", itemID, versionID, EvidenceImportReq{Revision: rev, SourceRunID: runID, RightsNote: "x"}); errorCode(err) != "NOT_FOUND" {
		t.Fatalf("want NOT_FOUND for others' run, got %v", err)
	}
	db.Exec(`UPDATE finance_backtest_runs SET owner_id = ? WHERE id = ?`, adminUID, runID)

	// 非 succeeded 运行不可导入。
	db.Exec(`UPDATE finance_backtest_runs SET status = 'failed' WHERE id = ?`, runID)
	if _, _, err := s.ImportEvidence(ctx, adminUID, "g2", itemID, versionID, EvidenceImportReq{Revision: rev, SourceRunID: runID, RightsNote: "x"}); errorCode(err) != "NOT_FOUND" {
		t.Fatalf("want NOT_FOUND for non-succeeded run, got %v", err)
	}
	db.Exec(`UPDATE finance_backtest_runs SET status = 'succeeded' WHERE id = ?`, runID)

	// 标的不一致 → 拒绝。
	db.Exec(`UPDATE finance_backtest_runs SET config = ? WHERE id = ?`, datatypes.JSON([]byte(`{"instrument_id":"300750.SZ"}`)), runID)
	if _, _, err := s.ImportEvidence(ctx, adminUID, "g3", itemID, versionID, EvidenceImportReq{Revision: rev, SourceRunID: runID, RightsNote: "x"}); errorCode(err) != "STRATEGY_INVALID" {
		t.Fatalf("want STRATEGY_INVALID instrument mismatch, got %v", err)
	}
	db.Exec(`UPDATE finance_backtest_runs SET config = ? WHERE id = ?`, datatypes.JSON([]byte(`{"instrument_id":"600519.SH"}`)), runID)

	// DSL 不一致 → 拒绝。
	db.Exec(`UPDATE finance_strategy_versions SET dsl = ? WHERE id = 'stv_e1'`, datatypes.JSON([]byte(`{"schema_version":"strategy.v2","name":"other"}`)))
	if _, _, err := s.ImportEvidence(ctx, adminUID, "g4", itemID, versionID, EvidenceImportReq{Revision: rev, SourceRunID: runID, RightsNote: "x"}); errorCode(err) != "STRATEGY_INVALID" {
		t.Fatalf("want STRATEGY_INVALID dsl mismatch, got %v", err)
	}

	// 批准缺披露权限记录的证据 → 拒绝（无证据不补收益）。
	evID := "sme_manual"
	now := nowUTC()
	if err := db.Create(&model.MarketEvidence{
		ID: evID, ItemID: itemID, MarketVersionID: versionID, Status: EvidencePending,
		ValidationInstrumentID: "600519.SH", BoundDSLHash: "h",
		Config: datatypes.JSON([]byte("{}")), Manifest: datatypes.JSON([]byte("{}")),
		Metrics: datatypes.JSON([]byte("{}")), Equity: datatypes.JSON([]byte("[]")),
		Trades: datatypes.JSON([]byte("[]")), Limitations: datatypes.JSON([]byte("[]")),
		ResultHash: "rh", EvidenceHash: "eh", CreatedBy: adminUID, CreatedAt: now,
	}).Error; err != nil {
		t.Fatalf("manual evidence: %v", err)
	}
	if _, _, err := s.ReviewEvidence(ctx, adminUID, "g5", itemID, evID, EvidenceReviewReq{Revision: rev, Decision: "approve", Reason: "x"}); errorCode(err) != "STRATEGY_INVALID" {
		t.Fatalf("want STRATEGY_INVALID missing rights note, got %v", err)
	}
	// 非法决策与重复批准。
	if _, _, err := s.ReviewEvidence(ctx, adminUID, "g6", itemID, evID, EvidenceReviewReq{Revision: rev, Decision: "maybe", Reason: "x"}); errorCode(err) != "INVALID_PARAM" {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
	if _, _, err := s.ReviewEvidence(ctx, adminUID, "g7", itemID, evID, EvidenceReviewReq{Revision: 999, Decision: "reject", Reason: "x"}); errorCode(err) != "REVISION_CONFLICT" {
		t.Fatalf("want REVISION_CONFLICT, got %v", err)
	}
}
