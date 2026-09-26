package finance

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"zhigu/server/httpx"
	"zhigu/server/service/finance"
	"zhigu/server/service/market"
	strategymarket "zhigu/server/service/strategy_market"
	"zhigu/server/testdb"
)

type apiLookup struct{ inst market.InstrumentView }

func (l apiLookup) Instrument(_ context.Context, id string) (market.InstrumentView, error) {
	if id != l.inst.InstrumentID {
		return market.InstrumentView{}, finance.NewError(404, "not_found", "NOT_FOUND", "标的不存在")
	}
	return l.inst, nil
}

type apiEnvelope struct {
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	TraceID string `json:"trace_id"`
}

func setupMarketAPI(t *testing.T) (*gin.Engine, string, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.Start(t)
	db.Exec(`INSERT INTO finance_users (id, username, password_hash, role) VALUES (1001, 'admin', 'x', 'admin')`)
	db.Exec(`INSERT INTO finance_users (id, username, password_hash, role) VALUES (1002, 'user-b', 'x', 'user')`)
	svc := strategymarket.New(db, apiLookup{inst: market.InstrumentView{
		InstrumentID: "600519.SH", Name: "贵州茅台", Exchange: "SSE", Board: "main",
		AssetType: "stock", Currency: "CNY",
	}})
	engine := gin.New()
	RegisterStrategyMarket(engine, svc)
	adminTok, err := httpx.SignToken(1001, "admin", "admin")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	userTok, err := httpx.SignToken(1002, "user-b", "user")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	return engine, adminTok, userTok
}

func doReq(t *testing.T, engine *gin.Engine, method, path, token string, body any, headers map[string]string) (int, apiEnvelope) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		buf.Write(raw)
	}
	req := httptest.NewRequest(method, path, &buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	var env apiEnvelope
	if w.Body.Len() > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &env)
	}
	return w.Code, env
}

func apiVersionContent() strategymarket.VersionContent {
	return strategymarket.VersionContent{
		Name:                "均线趋势策略",
		Summary:             "公开均线规则整理，供学习研究使用",
		Category:            "趋势",
		Tags:                json.RawMessage(`["趋势"]`),
		Markets:             json.RawMessage(`["A"]`),
		SignalPeriod:        "1d",
		Description:         "短均线上穿长均线买入。",
		Hypothesis:          "假定趋势延续。",
		FailureCases:        "震荡市失效。",
		Sources:             json.RawMessage(`[{"title":"公开资料","url":"https://example.com/a","collected_at":"2026-09-20","adaptation":"平台改编"}]`),
		RightsNote:          "仅限学习研究展示",
		EditorSchemaVersion: "strategy.editor.v1",
		RuleTemplate:        json.RawMessage(`{"schema_version":"strategy.market.v1","editor_state":{"name":"验证用策略","instrument_id":null,"signal_period":"1d","price_basis":"raw","indicators":[{"id":"kdj","type":"KDJ","params":{"n":9,"m1":3,"m2":3}}],"entry":{"op":"lt","left":"kdj.j","right":{"constant":"20"}},"exit":{"op":"gt","left":"kdj.j","right":{"constant":"80"}},"position":{"type":"equity_fraction","value":"0.5"},"risk":{"check":"close"},"execution":{"timing":"next_session_open","priority":"exit_first"}},"instrument_binding":{"mode":"select_one","markets":["A"]}}`),
		BacktestDefaults:    json.RawMessage(`{"initial_cash":"100000","currency":"CNY"}`),
	}
}

func TestStrategyMarketAuthAndPermissions(t *testing.T) {
	engine, adminTok, userTok := setupMarketAPI(t)

	code, env := doReq(t, engine, http.MethodGet, "/api/finance/strategy-market/items", "", nil, nil)
	if code != http.StatusUnauthorized || env.Error == nil || env.Error.Code != "UNAUTHENTICATED" {
		t.Fatalf("no token: code=%d env=%+v", code, env)
	}
	// 普通用户写管理端 → 403 FORBIDDEN。
	body := strategymarket.CreateItemReq{Slug: "forbidden", Version: apiVersionContent()}
	code, env = doReq(t, engine, http.MethodPost, "/api/admin/strategy-market/items", userTok, body, map[string]string{"Idempotency-Key": "p1"})
	if code != http.StatusForbidden || env.Error == nil || env.Error.Code != "FORBIDDEN" {
		t.Fatalf("user write admin: code=%d env=%+v", code, env)
	}
	// 普通用户可读市场。
	code, _ = doReq(t, engine, http.MethodGet, "/api/finance/strategy-market/items", userTok, nil, nil)
	if code != http.StatusOK {
		t.Fatalf("user list: code=%d", code)
	}
	_ = adminTok
}

func TestStrategyMarketConsumerFlow(t *testing.T) {
	engine, adminTok, userTok := setupMarketAPI(t)

	// 未知字段严格拒绝。
	code, env := doReq(t, engine, http.MethodPost, "/api/admin/strategy-market/items", adminTok,
		map[string]any{"slug": "x", "version": apiVersionContent(), "extra": 1}, map[string]string{"Idempotency-Key": "u1"})
	if code != http.StatusBadRequest || env.Error == nil || env.Error.Code != "INVALID_JSON" {
		t.Fatalf("unknown field: code=%d env=%+v", code, env)
	}
	// q 超长与非法 limit。
	code, _ = doReq(t, engine, http.MethodGet, "/api/finance/strategy-market/items?q="+strings.Repeat("长", 101), userTok, nil, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("long q: code=%d", code)
	}
	code, _ = doReq(t, engine, http.MethodGet, "/api/finance/strategy-market/items?limit=51", userTok, nil, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("limit: code=%d", code)
	}

	// 管理员上架（创建 → 校验 → 发布）。
	code, env = doReq(t, engine, http.MethodPost, "/api/admin/strategy-market/items", adminTok,
		strategymarket.CreateItemReq{Slug: "trend-1", Version: apiVersionContent()}, map[string]string{"Idempotency-Key": "c1"})
	if code != http.StatusCreated {
		t.Fatalf("create item: code=%d env=%+v", code, env)
	}
	var created struct {
		ItemID    string `json:"item_id"`
		VersionID string `json:"version_id"`
		Revision  int    `json:"revision"`
	}
	_ = json.Unmarshal(env.Data, &created)

	// 草稿对消费者不可见。
	code, _ = doReq(t, engine, http.MethodGet, "/api/finance/strategy-market/items/"+created.ItemID, userTok, nil, nil)
	if code != http.StatusNotFound {
		t.Fatalf("draft detail: code=%d", code)
	}

	code, env = doReq(t, engine, http.MethodPost,
		"/api/admin/strategy-market/items/"+created.ItemID+"/versions/"+created.VersionID+"/validate", adminTok,
		strategymarket.ValidateReq{Revision: 1, ValidationInstrumentID: "600519.SH"}, map[string]string{"Idempotency-Key": "v1"})
	if code != http.StatusCreated {
		t.Fatalf("validate: code=%d env=%+v", code, env)
	}
	code, env = doReq(t, engine, http.MethodPost, "/api/admin/strategy-market/items/"+created.ItemID+"/publish", adminTok,
		strategymarket.PublishReq{Revision: 2, MarketVersionID: created.VersionID, Reason: "内容齐备"}, map[string]string{"Idempotency-Key": "pub1"})
	if code != http.StatusCreated {
		t.Fatalf("publish: code=%d env=%+v", code, env)
	}

	// 消费者列表与详情。
	code, env = doReq(t, engine, http.MethodGet, "/api/finance/strategy-market/items", userTok, nil, nil)
	if code != http.StatusOK {
		t.Fatalf("list: code=%d", code)
	}
	var list struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &list)
	if len(list.Items) != 1 || list.Items[0]["copyable"] != true {
		t.Fatalf("list = %v", list)
	}
	code, _ = doReq(t, engine, http.MethodGet, "/api/finance/strategy-market/items/"+created.ItemID, userTok, nil, nil)
	if code != http.StatusOK {
		t.Fatalf("detail: code=%d", code)
	}

	// 复制：首请求 201，同 key 重试 200 同一草稿。
	copyBody := strategymarket.CopyReq{MarketVersionID: created.VersionID}
	code, env = doReq(t, engine, http.MethodPost, "/api/finance/strategy-market/items/"+created.ItemID+"/copied", userTok, copyBody, nil)
	if code != http.StatusNotFound {
		t.Fatalf("wrong copy path must 404: code=%d", code)
	}
	code, env = doReq(t, engine, http.MethodPost, "/api/finance/strategy-market/items/"+created.ItemID+"/copies", userTok, copyBody, map[string]string{"Idempotency-Key": "copy1"})
	if code != http.StatusCreated {
		t.Fatalf("copy: code=%d env=%+v", code, env)
	}
	var copied struct {
		DraftID  string `json:"draft_id"`
		NextPath string `json:"next_path"`
	}
	_ = json.Unmarshal(env.Data, &copied)
	if copied.DraftID == "" || !strings.Contains(copied.NextPath, copied.DraftID) {
		t.Fatalf("copied = %+v", copied)
	}
	code, env = doReq(t, engine, http.MethodPost, "/api/finance/strategy-market/items/"+created.ItemID+"/copies", userTok, copyBody, map[string]string{"Idempotency-Key": "copy1"})
	if code != http.StatusOK {
		t.Fatalf("copy replay: code=%d", code)
	}
	var replayed struct {
		DraftID string `json:"draft_id"`
	}
	_ = json.Unmarshal(env.Data, &replayed)
	if replayed.DraftID != copied.DraftID {
		t.Fatalf("replay draft %s != %s", replayed.DraftID, copied.DraftID)
	}
}
