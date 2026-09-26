package market

import "testing"

func TestDayKey(t *testing.T) {
	if DayKey("2026-09-21T00:00:00Z") != "2026-09-21" || DayKey("2026-09-21") != "2026-09-21" {
		t.Fatal(DayKey("2026-09-21T00:00:00Z"))
	}
}

func TestParsePush2QuoteAShare(t *testing.T) {
	raw := []byte(`{"rc":0,"data":{"f43":125257,"f44":125995,"f45":125080,"f46":125900,"f47":25017,"f57":"600519","f58":"贵州茅台","f60":125712,"f86":1789978297,"f152":2,"f169":-455,"f170":-36}}`)
	got, err := parsePush2Quote(raw, SeedInstrument{InstrumentID: "600519.SH", Exchange: "SSE", Code: "600519", Name: "贵州茅台"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Last != "1252.57" || got.Volume != "2501700" || got.Change != "-4.55" {
		t.Fatalf("%+v", got)
	}
}

func TestParseUlistQuotes(t *testing.T) {
	raw := []byte(`{"rc":0,"data":{"total":2,"diff":[{"f2":1252.57,"f3":-0.36,"f4":-4.55,"f12":"600519","f13":1,"f14":"贵州茅台"},{"f2":430.0,"f3":2.63,"f4":11,"f12":"00700","f13":116,"f14":"腾讯控股"}]}}`)
	got, err := parseUlistQuotes(raw, map[string]SeedInstrument{
		"600519.SH": {InstrumentID: "600519.SH", Exchange: "SSE", Code: "600519"},
		"00700.HK":  {InstrumentID: "00700.HK", Exchange: "HKEX", Code: "00700"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["600519.SH"].Last != "1252.57" || got["00700.HK"].Last != "430" {
		t.Fatalf("%+v", got)
	}
}

func TestParseUlistQuotesObjectDiffAndHKMarket128(t *testing.T) {
	raw := []byte(`{"rc":0,"data":{"diff":{"0":{"f2":11.73,"f3":0.5,"f4":0.06,"f12":"000001","f13":0,"f14":"平安银行"},"1":{"f2":67.55,"f3":1.2,"f4":0.8,"f12":"1","f13":128,"f14":"长和"}}}}`)
	got, err := parseUlistQuotes(raw, map[string]SeedInstrument{
		"000001.SZ": {InstrumentID: "000001.SZ", Exchange: "SZSE", Code: "000001"},
		"00001.HK":  {InstrumentID: "00001.HK", Exchange: "HKEX", Code: "00001"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["000001.SZ"].Last != "11.73" || got["00001.HK"].Last != "67.55" {
		t.Fatalf("%+v", got)
	}
}

func TestParseUlistQuotesSZAndBSEMarketZero(t *testing.T) {
	raw := []byte(`{"rc":0,"data":{"diff":[{"f2":4.12,"f3":1.2,"f4":0.05,"f12":"000001","f13":0,"f14":"平安银行"},{"f2":10.5,"f3":-1,"f4":-0.11,"f12":"920185","f13":0,"f14":"北交所样例"}]}}`)
	got, err := parseUlistQuotes(raw, map[string]SeedInstrument{
		"000001.SZ": {InstrumentID: "000001.SZ", Exchange: "SZSE", Code: "000001"},
		"920185.BJ": {InstrumentID: "920185.BJ", Exchange: "BSE", Code: "920185"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["000001.SZ"].Last != "4.12" || got["920185.BJ"].Last != "10.5" {
		t.Fatalf("%+v", got)
	}
}

func TestParsePush2QuoteHK(t *testing.T) {
	raw := []byte(`{"rc":0,"data":{"f43":430000,"f44":434800,"f45":423000,"f46":424600,"f47":19669333,"f57":"00700","f58":"腾讯控股","f60":419000,"f86":1789978114,"f152":2,"f169":11000,"f170":263}}`)
	got, err := parsePush2Quote(raw, SeedInstrument{InstrumentID: "00700.HK", Exchange: "HKEX", Code: "00700", Name: "腾讯控股"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Last != "430" || got.Volume != "19669333" || got.Change != "11" {
		t.Fatalf("%+v", got)
	}
}

func TestApplyLiveQuotesKeepsGaps(t *testing.T) {
	items := []InstrumentView{{InstrumentID: "000001.SZ"}, {InstrumentID: "000022.SZ"}, {InstrumentID: "000043.SZ"}}
	missing := applyLiveQuotes(items, map[string]QuoteSnapshot{
		"000001.SZ": {Last: "11.73", Change: "0.06", ChangePct: "0.5"},
		"000022.SZ": {Last: "16.46", MarketTime: "2018-12-20", Quality: map[string]any{"freshness_status": "daily_close"}},
	})
	if items[0].Last != "11.73" || items[0].QuoteBasis != "last" {
		t.Fatalf("live %+v", items[0])
	}
	if items[1].QuoteBasis != "daily_close" || items[1].QuoteAsOf != "2018-12-20" {
		t.Fatalf("close %+v", items[1])
	}
	if len(missing) != 1 || missing[0] != "000043.SZ" {
		t.Fatalf("%v", missing)
	}
}

func TestPreferInstrumentPicksShorterAShare(t *testing.T) {
	items := []InstrumentView{
		{InstrumentID: "01036.HK", Name: "万科海外", Exchange: "HKEX", AssetType: "stock"},
		{InstrumentID: "02202.HK", Name: "万科企业", Exchange: "HKEX", AssetType: "stock"},
		{InstrumentID: "000002.SZ", Name: "万科A", Exchange: "SZSE", AssetType: "stock"},
	}
	if got := PreferInstrument("万科", "万科金叉买入", items); got != "000002.SZ" {
		t.Fatal(got)
	}
	if got := PreferInstrument("万科", "港股万科金叉买入", items); got != "01036.HK" {
		t.Fatal(got)
	}
}

func TestStaleQuoteDate(t *testing.T) {
	if day, ok := staleQuoteDate("2020-05-27", "2026-09-21", 15); !ok || day != "2020-05-27" {
		t.Fatalf("%s %v", day, ok)
	}
	if _, ok := staleQuoteDate("2026-09-18", "2026-09-22", 15); ok {
		t.Fatal("recent bar should stay live")
	}
}
