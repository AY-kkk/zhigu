package finance

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestGrowthRatePercentHalfUp(t *testing.T) {
	v, formula, err := Calculate("growth_rate", decimal.NewFromInt(120), decimal.NewFromInt(100))
	if err != nil {
		t.Fatal(err)
	}
	if v.StringFixed(2) != "20.00" {
		t.Fatalf("want 20.00 got %s", v.StringFixed(2))
	}
	if !strings.Contains(formula, "×100") {
		t.Fatalf("formula %s", formula)
	}
}

func TestGrowthRateNonPositivePrevious(t *testing.T) {
	_, _, err := Calculate("growth_rate", decimal.NewFromInt(10), decimal.NewFromInt(-1))
	if ErrorCode(err) != "INSUFFICIENT_DENOMINATOR" {
		t.Fatalf("got %v", err)
	}
}

func TestUnknownsPublishGate(t *testing.T) {
	if ValidateUnknowns([]string{"待核实"}) == nil {
		t.Fatal("bare 待核实 must fail")
	}
	if ValidateUnknowns([]string{"无法取数"}) == nil {
		t.Fatal("prefix without explanation must fail")
	}
	got := NormalizeUnknowns([]string{"缺少年报正文"})
	if len(got) != 1 || !ValidUnknown(got[0]) {
		t.Fatalf("normalize %v", got)
	}
	if err := ValidateUnknowns([]string{"无法取数：东方财富未返回 2024 年报营业收入。"}); err != nil {
		t.Fatal(err)
	}
}

func TestJudgeOutlookInsufficient(t *testing.T) {
	item := ClaimItem{ClaimID: "c1", Text: "利润改善所以未来一年股价会上涨", ClaimType: "inference"}
	got := JudgeClaim(item, []Argument{{ClaimType: "fact", Text: "利润增长", EvidenceIDs: []string{"ev_1"}}}, nil)
	if got.Verdict != "insufficient" {
		t.Fatalf("outlook must be insufficient, got %+v", got)
	}
}

func TestSynthesizerPromptHasThreeChecks(t *testing.T) {
	p := SynthesizerPrompt()
	for _, n := range []string{"利润质量", "经营现金流", "一次性损益", "研究口径（服务端冻结）", "A股或港股", "现金流量表"} {
		if !strings.Contains(p, n) {
			t.Fatalf("missing %s", n)
		}
	}
}

func TestProtocolNormalize(t *testing.T) {
	chat, err := NormalizeProtocol("openai-chat-completions")
	if err != nil || chat != ProtocolChatCompletions {
		t.Fatalf("legacy alias: %s %v", chat, err)
	}
	if _, err := NormalizeProtocol("other"); err == nil {
		t.Fatal("unknown protocol must fail")
	}
}

func TestCatalogFrozenThree(t *testing.T) {
	ids := LiveInstrumentIDs()
	if len(ids) != 3 {
		t.Fatalf("want 3, got %v", ids)
	}
	for _, id := range []string{"600519.SH", "300750.SZ", "000333.SZ"} {
		if !CoveredInstrumentMode(id, ModeLive) {
			t.Fatalf("%s not covered", id)
		}
	}
	if CoveredInstrumentMode(InstrumentDemo, ModeLive) {
		t.Fatal("DEMO must not be live catalog")
	}
}

func TestLiveDataSampleGated(t *testing.T) {
	if os.Getenv("ZHIGU_STAGE_B_LIVE_DATA") != "1" {
		t.Skip("set ZHIGU_STAGE_B_LIVE_DATA=1 to probe East Money and Cninfo")
	}
	src := NewLiveSource()
	for _, id := range LiveInstrumentIDs() {
		run := RunSnapshot{ID: "run_live", InstrumentID: id, AsOf: src.now(), Mode: ModeLive}
		fin, n, err := src.Financials(context.Background(), run, FrozenMetrics, []string{"2024-12-31", "2023-12-31"})
		if err != nil {
			t.Fatalf("%s financials: %v", id, err)
		}
		if len(fin) == 0 || n == 0 {
			t.Fatalf("%s no financial records http=%d", id, n)
		}
		fil, n2, err := src.Filings(context.Background(), run, "年度报告", 1)
		if err != nil {
			t.Fatalf("%s filings: %v", id, err)
		}
		if len(fil) == 0 || n2 == 0 {
			t.Fatalf("%s no filings http=%d", id, n2)
		}
		t.Logf("%s financials=%d http_fin=%d filings=%d http_fil=%d", id, len(fin), n, len(fil), n2)
	}
}

func TestConnectorEgressAllowlist(t *testing.T) {
	httpc := NewCountedHTTP(1, time.Second)
	ctx := context.Background()
	_, _, err := httpc.Get(ctx, "http://127.0.0.1/")
	if ErrorCode(err) != "SSRF_BLOCKED" {
		t.Fatalf("loopback: %v", err)
	}
	_, _, err = httpc.Get(ctx, "https://example.com/")
	if ErrorCode(err) != "SSRF_BLOCKED" {
		t.Fatalf("unknown host: %v", err)
	}
	_, _, err = httpc.Get(ctx, "file:///etc/hosts")
	if ErrorCode(err) != "SSRF_BLOCKED" {
		t.Fatalf("file scheme: %v", err)
	}
	if FinancialHTTPLimit != 6 || FilingHTTPLimit != 6 {
		t.Fatalf("http limits fin=%d fil=%d", FinancialHTTPLimit, FilingHTTPLimit)
	}
}

func TestStageBDecimalAndMetricBasis(t *testing.T) {
	TestGrowthRatePercentHalfUp(t)
	TestGrowthRateNonPositivePrevious(t)
	basis := defaultBasis("10000", "CNY")
	if basis.ScaleFactor != "1" || basis.OriginalValue != "10000" || basis.OriginalUnit != "CNY" {
		t.Fatalf("yuan stays yuan: %+v", basis)
	}
	if err := comparableMetrics("growth_rate",
		Metric{Metric: "revenue", Unit: "CNY", ValueType: "actual"},
		Metric{Metric: "revenue", Unit: "USD", ValueType: "actual"},
	); err == nil {
		t.Fatal("mixed currency must be rejected")
	}
	if err := comparableMetrics("growth_rate",
		Metric{Metric: "revenue", Unit: "CNY", ValueType: "actual"},
		Metric{Metric: "revenue", Unit: "CNY", ValueType: "estimate"},
	); err == nil {
		t.Fatal("estimate must not compare as actual")
	}
}

func TestStageBConnectorEgressContract(t *testing.T) {
	TestConnectorEgressAllowlist(t)
}

func TestStageBLiveFailClosedAndOwnership(t *testing.T) {
	if CoveredInstrumentMode(InstrumentDemo, ModeLive) {
		t.Fatal("DEMO must not be live")
	}
	t.Setenv("ZHIGU_DATA_MODE", ModeLive)
	t.Cleanup(ResetLiveCatalogForTest)
	ReplaceLiveCatalogForTest([]ListedInstrument{
		{ID: "00700.HK", Symbol: "00700", Name: "腾讯控股", Market: MarketHK, OrgID: "gshk0000700", Column: "hke", Plate: "hke"},
	})
	svc := setup(t)
	out, err := svc.ParseClaim(ctxUser(1001), ParseInput{
		Text: "演示公司的收入增长能否支持未来一年股价上涨？这是一条超过二十个字的测试观点。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.InstrumentID != nil {
		t.Fatalf("live must not default DEMO, got %s", *out.InstrumentID)
	}
	demo := InstrumentDemo
	if _, err := svc.PatchClaim(ctxUser(1001), out.DraftID, PatchDraftInput{Revision: out.Revision, InstrumentID: &demo}); err == nil || ErrorCode(err) != "UNSUPPORTED_INSTRUMENT" {
		t.Fatalf("DEMO patch in live: %v", err)
	}
	hk := "00700.HK"
	patched, err := svc.PatchClaim(ctxUser(1001), out.DraftID, PatchDraftInput{Revision: out.Revision, InstrumentID: &hk})
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateResearch(ctxUser(1001), "fail-closed-1", createReqFrom(t, patched))
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.GetResearch(ctxUser(1001), created.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if view.InstrumentID != "00700.HK" || view.Mode != ModeLive || view.InstrumentID == InstrumentDemo {
		t.Fatalf("live run must stay on catalog name, got %+v", view)
	}
	if _, err := svc.GetResearch(ctxUser(1002), created.RunID); !isClass(err, "not_found") || ErrorCode(err) != "RUN_NOT_FOUND" {
		t.Fatalf("cross-owner: %v", err)
	}
	t.Setenv("ZHIGU_RESEARCH_MODE", "")
	t.Setenv("ZHIGU_RESEARCH_URL", "")
	if _, err := NewHTTPResearchClient(); err == nil {
		t.Fatal("live research client must fail without ZHIGU_RESEARCH_URL")
	}
}

func TestChatAndResponsesAdapters(t *testing.T) {
	var chatBody, respBody []byte
	chatSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("chat path %s", r.URL.Path)
		}
		chatBody, _ = ioReadAll(r)
		_, _ = w.Write([]byte(`{"id":"chatcmpl_x","choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer chatSrv.Close()
	respSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("responses path %s", r.URL.Path)
		}
		respBody, _ = ioReadAll(r)
		_, _ = w.Write([]byte(`{"id":"resp_x","status":"completed","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer respSrv.Close()
	raw := []byte(`{"model":"finance-research","messages":[{"role":"user","content":"hello"}]}`)
	out, code, err := (&ChatCompletionsAdapter{Client: chatSrv.Client()}).Forward(chatSrv.URL, "k", raw)
	if err != nil || code != 200 || !bytes.Contains(out, []byte("chatcmpl")) {
		t.Fatalf("chat adapter %d %v %s", code, err, out)
	}
	if !bytes.Contains(chatBody, []byte("研究口径（服务端冻结）")) {
		t.Fatalf("scope prefix missing: %s", chatBody)
	}
	raw2 := []byte(`{"model":"finance-research","input":[{"role":"user","content":"hello"}],"tools":[{"type":"function","name":"get_financials"}]}`)
	out, code, err = (&ResponsesAdapter{Client: respSrv.Client()}).Forward(respSrv.URL, "k", raw2)
	if err != nil || code != 200 || !bytes.Contains(out, []byte("resp_")) {
		t.Fatalf("responses adapter %d %v %s", code, err, out)
	}
	if !bytes.Contains(respBody, []byte(`"store":false`)) || bytes.Contains(respBody, []byte("previous_response_id")) {
		t.Fatalf("responses constraints missing: %s", respBody)
	}
	if !bytes.Contains(respBody, []byte("研究口径（服务端冻结）")) {
		t.Fatalf("responses scope prefix missing: %s", respBody)
	}
}

func ioReadAll(r *http.Request) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r.Body)
	return buf.Bytes(), err
}
