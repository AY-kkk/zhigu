package strategy_market

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"gorm.io/gorm"

	"zhigu/server/service/finance"
	"zhigu/server/service/market"
	"zhigu/server/testdb"
)

const (
	adminUID uint = 1001
	otherUID uint = 1002
)

type stubLookup struct{ inst market.InstrumentView }

func (s stubLookup) Instrument(_ context.Context, id string) (market.InstrumentView, error) {
	if id != s.inst.InstrumentID {
		return market.InstrumentView{}, finance.NewError(404, "not_found", "NOT_FOUND", "标的不存在")
	}
	return s.inst, nil
}

func setupMarket(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	db := testdb.Start(t)
	db.Exec(`INSERT INTO finance_users (id, username, password_hash, role) VALUES (1001, 'admin', 'x', 'admin')`)
	db.Exec(`INSERT INTO finance_users (id, username, password_hash, role) VALUES (1002, 'user-b', 'x', 'user')`)
	svc := New(db, stubLookup{inst: market.InstrumentView{
		InstrumentID: "600519.SH", Name: "贵州茅台", Exchange: "SSE", Board: "main",
		AssetType: "stock", Currency: "CNY",
	}})
	return svc, db
}

const ruleTemplateJSON = `{"schema_version":"strategy.market.v1","editor_state":{"name":"验证用策略","instrument_id":null,"signal_period":"1d","price_basis":"raw","indicators":[{"id":"kdj","type":"KDJ","params":{"n":9,"m1":3,"m2":3}}],"entry":{"op":"lt","left":"kdj.j","right":{"constant":"20"}},"exit":{"op":"gt","left":"kdj.j","right":{"constant":"80"}},"position":{"type":"equity_fraction","value":"0.5"},"risk":{"check":"close"},"execution":{"timing":"next_session_open","priority":"exit_first"}},"instrument_binding":{"mode":"select_one","markets":["A"]}}`

func validContent() VersionContent {
	return VersionContent{
		Name:                "均线趋势策略",
		Summary:             "公开均线规则整理，供学习研究使用",
		Category:            "趋势",
		Tags:                json.RawMessage(`["趋势","日线"]`),
		Markets:             json.RawMessage(`["A"]`),
		SignalPeriod:        "1d",
		Description:         "短均线上穿长均线买入。",
		Hypothesis:          "假定趋势延续。",
		FailureCases:        "震荡市失效。",
		Sources:             json.RawMessage(`[{"title":"公开资料","url":"https://example.com/a","collected_at":"2026-09-20","adaptation":"平台改编"}]`),
		RightsNote:          "仅限学习研究展示",
		EditorSchemaVersion: "strategy.editor.v1",
		RuleTemplate:        json.RawMessage(ruleTemplateJSON),
		BacktestDefaults:    json.RawMessage(`{"initial_cash":"100000","currency":"CNY"}`),
	}
}

func mustCreateItem(t *testing.T, s *Service, uid uint, key string, content VersionContent) map[string]any {
	t.Helper()
	out, _, err := s.CreateItem(context.Background(), uid, key, CreateItemReq{Slug: "t-" + key, Version: content})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	return out
}

func mustValidate(t *testing.T, s *Service, uid uint, key, itemID, versionID string, revision int) map[string]any {
	t.Helper()
	out, _, err := s.Validate(context.Background(), uid, key, itemID, versionID, ValidateReq{Revision: revision, ValidationInstrumentID: "600519.SH"})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return out
}

func mustPublish(t *testing.T, s *Service, uid uint, key, itemID, versionID string, revision int) map[string]any {
	t.Helper()
	out, _, err := s.Publish(context.Background(), uid, key, itemID, PublishReq{Revision: revision, MarketVersionID: versionID, Reason: "内容齐备"})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	return out
}

// publishFixture walks create → validate → publish and returns item/version ids.
func publishFixture(t *testing.T, s *Service, uid uint, tag string) (itemID, versionID string, revision int) {
	t.Helper()
	created := mustCreateItem(t, s, uid, "k-create-"+tag, validContent())
	itemID, _ = created["item_id"].(string)
	versionID, _ = created["version_id"].(string)
	v := mustValidate(t, s, uid, "k-validate-"+tag, itemID, versionID, 1)
	st, _ := v["validation_status"].(string)
	if st != ValidationPassed {
		t.Fatalf("validation_status = %s report = %v", st, v["validation_report"])
	}
	p := mustPublish(t, s, uid, "k-publish-"+tag, itemID, versionID, 2)
	rev, _ := p["revision"].(int)
	return itemID, versionID, rev
}

func errorCode(err error) string {
	if err == nil {
		return ""
	}
	return finance.ErrorCode(err)
}

func TestPublicationLifecycle(t *testing.T) {
	s, db := setupMarket(t)
	ctx := context.Background()

	created := mustCreateItem(t, s, adminUID, "k1", validContent())
	itemID := created["item_id"].(string)
	versionID := created["version_id"].(string)

	// draft 条目不出现在市场列表，详情 404（未发布404）。
	list, err := s.List(ctx, ListQuery{})
	if err != nil || len(list["items"].([]MarketCard)) != 0 {
		t.Fatalf("draft leaked to list: %v %v", list, err)
	}
	if _, err := s.Detail(ctx, itemID); err == nil || finance.HTTPStatus(err) != 404 {
		t.Fatalf("draft detail want 404, got %v", err)
	}

	v := mustValidate(t, s, adminUID, "k2", itemID, versionID, 1)
	if v["validation_status"] != ValidationPassed {
		t.Fatalf("validate failed: %v", v)
	}
	rep := v["validation_report"].(json.RawMessage)
	if !strings.Contains(string(rep), "dsl_hash") || !strings.Contains(string(rep), "600519.SH") {
		t.Fatalf("report = %s", rep)
	}

	mustPublish(t, s, adminUID, "k3", itemID, versionID, 2)
	list, _ = s.List(ctx, ListQuery{})
	items := list["items"].([]MarketCard)
	if len(items) != 1 || !items[0].Copyable || items[0].BacktestStatus != BacktestNotTested {
		t.Fatalf("items = %+v", items)
	}
	detail, err := s.Detail(ctx, itemID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if detail["market_version_id"] != versionID || detail["copyable"] != true {
		t.Fatalf("detail = %v", detail)
	}

	// 新版本不可变：旧版本行不变，发布切换指针。
	content2 := validContent()
	content2.Name = "均线趋势策略 v2"
	created2, _, err := s.CreateVersion(ctx, adminUID, "k4", itemID, CreateVersionReq{Revision: 3, Version: content2})
	if err != nil {
		t.Fatalf("CreateVersion: %v", err)
	}
	version2, _ := created2["version_id"].(string)
	var oldRow map[string]any
	db.Raw(`SELECT name, content_hash FROM finance_strategy_market_versions WHERE id = ?`, versionID).Scan(&oldRow)
	if oldRow["name"] != "均线趋势策略" {
		t.Fatalf("old version mutated: %v", oldRow)
	}
	mustValidate(t, s, adminUID, "k5", itemID, version2, 4)
	mustPublish(t, s, adminUID, "k6", itemID, version2, 5)
	detail, _ = s.Detail(ctx, itemID)
	if detail["market_version_id"] != version2 {
		t.Fatalf("pointer not switched: %v", detail["market_version_id"])
	}

	// 下架：列表清空、详情 410。
	if _, _, err := s.Withdraw(ctx, adminUID, "k7", itemID, WithdrawReq{Revision: 6, Reason: "整理中"}); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}
	list, _ = s.List(ctx, ListQuery{})
	if len(list["items"].([]MarketCard)) != 0 {
		t.Fatalf("withdrawn item still listed: %v", list)
	}
	if _, err := s.Detail(ctx, itemID); err == nil || finance.HTTPStatus(err) != 410 {
		t.Fatalf("withdrawn detail want 410, got %v", err)
	}

	// 审计为 append-only 且覆盖全部动作。
	var actions []string
	db.Raw(`SELECT action FROM finance_strategy_market_audit WHERE item_id = ? ORDER BY created_at, id`, itemID).Scan(&actions)
	want := []string{"create_version", "validate", "publish", "create_version", "validate", "publish", "withdraw"}
	if strings.Join(actions, ",") != strings.Join(want, ",") {
		t.Fatalf("audit actions = %v", actions)
	}
}

func TestPublicationNegativeCases(t *testing.T) {
	s, db := setupMarket(t)
	ctx := context.Background()

	bad := validContent()
	bad.Name = strings.Repeat("长", 81)
	if _, _, err := s.CreateItem(ctx, adminUID, "n1", CreateItemReq{Slug: "n1", Version: bad}); errorCode(err) != "STRATEGY_INVALID" {
		t.Fatalf("want name length rejection, got %v", err)
	}
	badSrc := validContent()
	badSrc.Sources = json.RawMessage(`[{"title":"","collected_at":"2026-09-20","adaptation":"原版"}]`)
	if _, _, err := s.CreateItem(ctx, adminUID, "n2", CreateItemReq{Slug: "n2", Version: badSrc}); errorCode(err) != "STRATEGY_INVALID" {
		t.Fatalf("want sources rejection, got %v", err)
	}
	badURL := validContent()
	badURL.Sources = json.RawMessage(`[{"title":"t","url":"javascript:alert(1)","collected_at":"2026-09-20","adaptation":"原版"}]`)
	if _, _, err := s.CreateItem(ctx, adminUID, "n3", CreateItemReq{Slug: "n3", Version: badURL}); errorCode(err) != "STRATEGY_INVALID" {
		t.Fatalf("want url scheme rejection, got %v", err)
	}

	created := mustCreateItem(t, s, adminUID, "n4", validContent())
	itemID := created["item_id"].(string)
	versionID := created["item_id"].(string) // wrong id on purpose
	if _, _, err := s.Publish(ctx, adminUID, "n5", itemID, PublishReq{Revision: 1, MarketVersionID: versionID, Reason: "x"}); errorCode(err) != "NOT_FOUND" {
		t.Fatalf("want version NOT_FOUND, got %v", err)
	}
	versionID, _ = created["version_id"].(string)
	// 未校验不得发布。
	if _, _, err := s.Publish(ctx, adminUID, "n6", itemID, PublishReq{Revision: 1, MarketVersionID: versionID, Reason: "x"}); errorCode(err) != "MARKET_NOT_COPYABLE" {
		t.Fatalf("want MARKET_NOT_COPYABLE, got %v", err)
	}
	// revision CAS。
	if _, _, err := s.Validate(ctx, adminUID, "n7", itemID, versionID, ValidateReq{Revision: 99, ValidationInstrumentID: "600519.SH"}); errorCode(err) != "REVISION_CONFLICT" {
		t.Fatalf("want REVISION_CONFLICT, got %v", err)
	}
	// 篡改内容后发布必须被内容 hash 拦截。
	mustValidate(t, s, adminUID, "n8", itemID, versionID, 1)
	db.Exec(`UPDATE finance_strategy_market_versions SET summary = 'tampered' WHERE id = ?`, versionID)
	if _, _, err := s.Publish(ctx, adminUID, "n9", itemID, PublishReq{Revision: 2, MarketVersionID: versionID, Reason: "x"}); errorCode(err) != "MARKET_VERSION_CHANGED" {
		t.Fatalf("want MARKET_VERSION_CHANGED, got %v", err)
	}
	// 相同幂等键不同请求体 → 409。
	if _, _, err := s.CreateItem(ctx, adminUID, "n4", CreateItemReq{Slug: "different", Version: validContent()}); errorCode(err) != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("want IDEMPOTENCY_CONFLICT, got %v", err)
	}
}

func TestValidateRecordsFailures(t *testing.T) {
	s, _ := setupMarket(t)
	ctx := context.Background()

	created := mustCreateItem(t, s, adminUID, "v1", validContent())
	itemID := created["item_id"].(string)
	versionID := created["version_id"].(string)
	// 验证标的不在目录 → 请求级拒绝。
	if _, _, err := s.Validate(ctx, adminUID, "v2", itemID, versionID, ValidateReq{Revision: 1, ValidationInstrumentID: "000001.XX"}); errorCode(err) != "STRATEGY_INVALID" {
		t.Fatalf("want STRATEGY_INVALID, got %v", err)
	}
	// 非 1d 信号：内容级失败被记录而不是发布。
	content := validContent()
	content.SignalPeriod = "1w"
	created2 := mustCreateItem(t, s, adminUID, "v3", content)
	v := mustValidate(t, s, adminUID, "v4", created2["item_id"].(string), created2["version_id"].(string), 1)
	if v["validation_status"] != ValidationFailed {
		t.Fatalf("want failed report, got %v", v)
	}
}
