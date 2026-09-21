package finance

import (
	"os"
	"testing"
)

func TestNormalizeInstrumentID(t *testing.T) {
	cases := map[string]string{
		"600519":       "600519.SH",
		"600519.SH":    "600519.SH",
		"300750":       "300750.SZ",
		"000333.SZ":    "000333.SZ",
		"920000":       "920000.BJ",
		"700":          "00700.HK",
		"0700":         "00700.HK",
		"00700.HK":     "00700.HK",
		"700.hk":       "00700.HK",
		InstrumentDemo: InstrumentDemo,
	}
	for in, want := range cases {
		if got := NormalizeInstrumentID(in); got != want {
			t.Fatalf("%s: got %s want %s", in, got, want)
		}
	}
	if id, _, _, ok := aShareListing("200001"); ok {
		t.Fatalf("B share must be excluded, got %s", id)
	}
}

func TestMatchSeedName(t *testing.T) {
	ResetLiveCatalogForTest()
	got := MatchInstrumentsFromText("贵州茅台利润改善，未来一年经营前景值得看好。", 8)
	if len(got) != 1 || got[0].ID != "600519.SH" {
		t.Fatalf("got %+v", got)
	}
}

func TestCatalogSearchOffline(t *testing.T) {
	t.Cleanup(ResetLiveCatalogForTest)
	ReplaceLiveCatalogForTest([]ListedInstrument{
		{ID: "00700.HK", Symbol: "00700", Name: "腾讯控股", Market: MarketHK, Pinyin: "txkg"},
		{ID: "000001.SZ", Symbol: "000001", Name: "平安银行", Market: MarketA, Pinyin: "payh"},
	})
	hits := liveSearch("腾讯", "", 20)
	if len(hits) == 0 || hits[0].ID != "00700.HK" {
		t.Fatalf("search 腾讯: %+v", hits)
	}
	hits = liveSearch("腾讯", MarketA, 20)
	if len(hits) != 0 {
		t.Fatalf("HK name must not appear in A-share filter, got %+v", hits)
	}
	hits = liveSearch("payh", "", 20)
	if len(hits) == 0 || hits[0].ID != "000001.SZ" {
		t.Fatalf("pinyin 平安: %+v", hits)
	}
	matched := MatchInstrumentsFromText("腾讯控股未来一年经营前景值得看好，利润是否改善。", 8)
	if len(matched) != 1 || matched[0].ID != "00700.HK" {
		t.Fatalf("match %+v", matched)
	}
	if !CoveredInstrumentMode("00700.HK", ModeLive) {
		t.Fatal("00700.HK should be covered after catalog replace")
	}
	if CoveredInstrumentMode("00700.HK", ModeFixture) {
		t.Fatal("HK must not be fixture catalog")
	}
}

func TestStatementMetricsFrozen(t *testing.T) {
	if len(FrozenMetrics) != 11 {
		t.Fatalf("want 11 three-statement metrics, got %v", FrozenMetrics)
	}
	for _, name := range []string{MetricRevenue, MetricTotalAssets, MetricOCF, MetricCashEnd} {
		if _, ok := metricDef(name); !ok {
			t.Fatalf("missing %s", name)
		}
	}
}

func TestHKAmountMapping(t *testing.T) {
	rows := []map[string]any{
		{"REPORT_DATE": "2024-12-31 00:00:00", "STD_ITEM_NAME": "营运收入", "STD_ITEM_CODE": "004001999", "AMOUNT": 751766000000},
		{"REPORT_DATE": "2024-12-31 00:00:00", "STD_ITEM_NAME": "股东应占溢利", "STD_ITEM_CODE": "004025002", "AMOUNT": 224842000000},
		{"REPORT_DATE": "2024-12-31 00:00:00", "STD_ITEM_NAME": "总资产", "STD_ITEM_CODE": "004009999", "AMOUNT": 2038986000000},
	}
	rev, ok := hkAmount(rows, "2024-12-31", metricByName[MetricRevenue])
	if !ok || rev != "751766000000" {
		t.Fatalf("revenue %s %v", rev, ok)
	}
	ni, ok := hkAmount(rows, "2024-12-31", metricByName[MetricNetParent])
	if !ok || ni != "224842000000" {
		t.Fatalf("parent %s %v", ni, ok)
	}
	assets, ok := hkAmount(rows, "2024-12-31", metricByName[MetricTotalAssets])
	if !ok || assets != "2038986000000" {
		t.Fatalf("assets %s %v", assets, ok)
	}
}

func TestLiveThreeStatementsAndHK(t *testing.T) {
	if os.Getenv("ZHIGU_STAGE_B_LIVE_DATA") != "1" {
		t.Skip("set ZHIGU_STAGE_B_LIVE_DATA=1 to probe A-share three statements and HK")
	}
	if err := EnsureLiveCatalog(backgroundCatalogCtx()); err != nil {
		t.Fatal(err)
	}
	if _, ok := LookupInstrument("00700.HK"); !ok {
		t.Fatal("catalog missing 00700.HK")
	}
	if _, ok := LookupInstrument("000001.SZ"); !ok {
		t.Fatal("catalog missing 000001.SZ")
	}
	src := NewLiveSource()
	metrics := []string{MetricRevenue, MetricNetParent, MetricOCF, MetricTotalAssets, MetricTotalLiab, MetricEquityParent}
	for _, id := range []string{"600519.SH", "00700.HK"} {
		run := RunSnapshot{ID: "run_live_3s", InstrumentID: id, AsOf: src.now(), Mode: ModeLive}
		fin, n, err := src.Financials(t.Context(), run, metrics, []string{"2024-12-31"})
		if err != nil {
			t.Fatalf("%s financials: %v http=%d", id, err, n)
		}
		if len(fin) == 0 || n == 0 {
			t.Fatalf("%s empty financials http=%d", id, n)
		}
		if got := len(fin[0].Metrics); got != len(metrics) {
			t.Fatalf("%s metrics %d", id, got)
		}
		fil, n2, err := src.Filings(t.Context(), run, "年报", 1)
		if err != nil {
			t.Fatalf("%s filings: %v", id, err)
		}
		if len(fil) == 0 || n2 == 0 {
			t.Fatalf("%s no filings http=%d", id, n2)
		}
		t.Logf("%s financials=%d http_fin=%d filings=%d http_fil=%d unit=%s", id, len(fin), n, len(fil), n2, fin[0].Metrics[0].Unit)
	}
}
