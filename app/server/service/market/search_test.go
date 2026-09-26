package market

import (
	"context"
	"testing"
	"time"

	"zhigu/server/testdb"
)

func TestSearchFixtureUniverse(t *testing.T) {
	t.Setenv("ZHIGU_MARKET_MODE", "fixture")
	db := testdb.Start(t)
	svc := NewService(db)
	ctx := context.Background()
	if err := svc.EnsureFixture(ctx); err != nil {
		t.Fatal(err)
	}
	hit, err := svc.Search(ctx, "茅台", "", "", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(hit.Items) == 0 || hit.Items[0].InstrumentID != "600519.SH" {
		t.Fatalf("%+v", hit.Items)
	}
	hk, err := svc.Search(ctx, "700", "", "", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	foundHK := false
	for _, it := range hk.Items {
		if it.InstrumentID == "00700.HK" && it.Currency == "HKD" {
			foundHK = true
		}
		if it.InstrumentID == "80700.HK" && it.Currency != "CNY" {
			t.Fatalf("RMB counter marked %s", it.Currency)
		}
	}
	if !foundHK {
		t.Fatalf("hk %+v", hk.Items)
	}
	fund, err := svc.Search(ctx, "510300.SH", "", "", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(fund.Items) == 0 || fund.Items[0].AssetType != "fund" {
		t.Fatalf("510300 must be fund, got %+v", fund.Items)
	}
	bj, err := svc.Search(ctx, "835185", "", "", "", 20)
	if err != nil || len(bj.Items) == 0 || bj.Items[0].Exchange != "BSE" {
		t.Fatalf("bj %+v %v", bj.Items, err)
	}
	ohlc, err := svc.GetOHLCV(ctx, "00700.HK", "1d", "raw", "", "", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(ohlc.Bars) == 0 || ohlc.Currency != "HKD" {
		t.Fatalf("%+v", ohlc)
	}
	if !ohlc.HasMore {
		t.Fatal("limit 20 of synthetic history should set has_more")
	}
	page1, err := svc.Search(ctx, "", "", "", "", 2)
	if err != nil || len(page1.Items) != 2 || page1.NextCursor == nil {
		t.Fatalf("page1 %+v %v", page1, err)
	}
	page2, err := svc.Search(ctx, "", "", "", *page1.NextCursor, 2)
	if err != nil || len(page2.Items) == 0 {
		t.Fatalf("page2 %+v %v", page2, err)
	}
	if page1.Items[0].InstrumentID == page2.Items[0].InstrumentID {
		t.Fatalf("cursor did not advance %s", page1.Items[0].InstrumentID)
	}
}

func TestDailyCloseQuotesAboveEight(t *testing.T) {
	t.Setenv("ZHIGU_MARKET_MODE", "fixture")
	db := testdb.Start(t)
	svc := NewService(db)
	ctx := context.Background()
	if err := svc.EnsureFixture(ctx); err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, row := range FixtureUniverse() {
		ids = append(ids, row.InstrumentID)
	}
	ids = append(ids, "missing.SH", "missing.HK")
	if len(ids) <= 8 {
		t.Fatalf("need more than 8 ids, got %d", len(ids))
	}
	got := svc.dailyCloseQuotes(ctx, ids)
	if len(got) != len(FixtureUniverse()) {
		t.Fatalf("got %d want %d", len(got), len(FixtureUniverse()))
	}
	if got["600519.SH"].Last == "" {
		t.Fatal("empty close")
	}
}

func TestLastCompleteSessionAfterClose(t *testing.T) {
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC) // 18:00 CST Friday
	got := LastCompleteSession("CN", now)
	if got != "2026-09-18" {
		t.Fatalf("got %s", got)
	}
	morning := time.Date(2026, 9, 18, 2, 0, 0, 0, time.UTC) // 10:00 CST
	got = LastCompleteSession("CN", morning)
	if got != "2026-09-17" {
		t.Fatalf("open session should use previous complete day, got %s", got)
	}
}

func TestCompareCoverageDoesNotClaim100OnGap(t *testing.T) {
	op := []SeedInstrument{{InstrumentID: "600519.SH", Exchange: "SSE", AssetType: "stock"}}
	base := map[string][]baselineRow{
		"SSE":  {{ID: "600519.SH"}, {ID: "601398.SH"}},
		"SZSE": {},
		"BSE":  {},
		"HKEX": {},
	}
	rep := CompareCoverage(op, base)
	if rep.Claim100 || rep.Coverage["SSE"] != 0.5 {
		t.Fatalf("%+v", rep)
	}
}
