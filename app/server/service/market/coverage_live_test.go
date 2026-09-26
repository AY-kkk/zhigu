package market

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestLiveCoverageAudit(t *testing.T) {
	if os.Getenv("ZHIGU_STRATEGY_LIVE_DATA") != "1" {
		t.Skip("set ZHIGU_STRATEGY_LIVE_DATA=1 to run live catalog/kline coverage audit")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	httpClient := NewHTTP(25 * time.Second)
	em := &EastMoney{HTTP: httpClient}

	cninfo, err := ListCninfoCatalog(ctx, httpClient)
	if err != nil {
		t.Fatal(err)
	}
	off := FetchOfficialLists(ctx, httpClient)
	merged := MergeOfficialCatalog(cninfo, off)
	byEx := map[string]int{}
	seen := map[string]bool{}
	for _, row := range merged {
		if row.AssetType != "stock" {
			continue
		}
		byEx[row.Exchange]++
		seen[row.InstrumentID] = true
	}
	if byEx["SSE"] == 0 || byEx["SZSE"] == 0 || byEx["BSE"] == 0 || byEx["HKEX"] == 0 {
		t.Fatalf("catalog missing a market: %+v", byEx)
	}
	if !seen["600519.SH"] || !seen["300750.SZ"] || !seen["00700.HK"] {
		t.Fatalf("smoke names missing")
	}

	rep := CoverageReport{
		AsOf:     time.Now().UTC().Format(time.RFC3339),
		SourceID: "cninfo_list+exchange_official+eastmoney_kline", SourceContract: SourceContractVersion,
		OperationalCounts: byEx, OfficialSources: off.Sources, OfficialErrors: off.Errors,
		Notes: []string{
			"运营目录 = 巨潮上市名单 ∪ 交易所官方名单。覆盖率分母为官方名单，不是巨潮。",
			"claim_100 仅表示运营目录包含全部官方股票柜台，不是全市场 K 线、公司行动或回测签收。",
			"K 线按 push2his → 72.push2his → 80.push2his 回退。",
		},
		KlineSamples: map[string]any{},
	}
	rep.Notes = append(rep.Notes, off.Notes...)
	for mkt, msg := range off.Errors {
		rep.Notes = append(rep.Notes, "official "+mkt+": "+msg)
	}

	cmp := CompareCoverage(merged, off.Baseline())
	rep.BaselineCounts = cmp.BaselineCounts
	rep.Matched = cmp.Matched
	rep.Coverage = cmp.Coverage
	rep.MissingSamples = cmp.MissingSamples
	rep.Claim100 = cmp.Claim100 && len(off.Errors) == 0 &&
		len(off.ByMarket["SSE"]) > 0 && len(off.ByMarket["SZSE"]) > 0 &&
		len(off.ByMarket["BSE"]) > 0 && len(off.ByMarket["HKEX"]) > 0
	if !rep.Claim100 {
		rep.Notes = append(rep.Notes, "未声称 100%：官方分母缺失或运营目录未覆盖全部官方代码")
	}

	cnCmp := CompareCoverage(cninfo, off.Baseline())
	for _, mkt := range []string{"SSE", "SZSE", "BSE", "HKEX"} {
		if cnCmp.BaselineCounts[mkt] == 0 {
			continue
		}
		if cnCmp.Matched[mkt] != cnCmp.BaselineCounts[mkt] {
			rep.Notes = append(rep.Notes, "巨潮相对官方 "+mkt+" 匹配 "+
				strconv.Itoa(cnCmp.Matched[mkt])+"/"+strconv.Itoa(cnCmp.BaselineCounts[mkt])+"，缺口由官方名单并入运营目录")
		}
	}

	samples := []SeedInstrument{
		{Exchange: "SSE", Code: "600519", InstrumentID: "600519.SH"},
		{Exchange: "SZSE", Code: "300750", InstrumentID: "300750.SZ"},
		{Exchange: "HKEX", Code: "00700", InstrumentID: "00700.HK"},
	}
	if seen["920185.BJ"] {
		samples = append(samples, SeedInstrument{Exchange: "BSE", Code: "920185", InstrumentID: "920185.BJ"})
	} else if seen["920000.BJ"] {
		samples = append(samples, SeedInstrument{Exchange: "BSE", Code: "920000", InstrumentID: "920000.BJ"})
	} else {
		samples = append(samples, SeedInstrument{Exchange: "BSE", Code: "835185", InstrumentID: "835185.BJ"})
	}
	for _, inst := range samples {
		bars, berr := em.Bars(ctx, inst, "1d", "raw", "", "", 5)
		if berr != nil || len(bars) == 0 {
			if inst.Exchange == "BSE" {
				rep.Notes = append(rep.Notes, "kline "+inst.InstrumentID+" unavailable")
				continue
			}
			t.Fatalf("kline %s: %v n=%d", inst.InstrumentID, berr, len(bars))
		}
		rep.KlineSamples[inst.InstrumentID] = map[string]any{"n": len(bars), "last": bars[len(bars)-1].Time, "close": bars[len(bars)-1].Close, "volume": bars[len(bars)-1].Volume}
	}
	if seen["80700.HK"] {
		rep.Notes = append(rep.Notes, "80700.HK present")
	} else {
		rep.Notes = append(rep.Notes, "80700.HK missing after official merge")
	}

	if dir := os.Getenv("ZHIGU_STRATEGY_COVERAGE_OUT"); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
		raw, _ := json.MarshalIndent(rep, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, "coverage.json"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
		contract, _ := json.MarshalIndent(SourceContract(), "", "  ")
		if err := os.WriteFile(filepath.Join(dir, "source-contract.json"), contract, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
