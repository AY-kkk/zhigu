package market

import (
	"context"
	"encoding/json"
	"strings"
)

type CoverageReport struct {
	AsOf              string              `json:"as_of"`
	SourceID          string              `json:"source_id"`
	SourceContract    string              `json:"source_contract_version"`
	OperationalCounts map[string]int      `json:"operational_counts"`
	BaselineCounts    map[string]int      `json:"baseline_counts"`
	Matched           map[string]int      `json:"matched"`
	Coverage          map[string]float64  `json:"coverage"`
	MissingSamples    map[string][]string `json:"missing_samples"`
	OfficialSources   map[string]string   `json:"official_sources,omitempty"`
	OfficialErrors    map[string]string   `json:"official_errors,omitempty"`
	Notes             []string            `json:"notes"`
	Claim100          bool                `json:"claim_100"`
	KlineSamples      map[string]any      `json:"kline_samples"`
}

type baselineRow struct {
	ID, Exchange, Name, Code string
}

type cninfoFile struct {
	StockList []cninfoRow `json:"stockList"`
}

type cninfoRow struct {
	Code     string `json:"code"`
	Category string `json:"category"`
	Name     string `json:"zwjc"`
}

func FetchCninfoBaseline(ctx context.Context, httpc *HTTP) (map[string][]baselineRow, error) {
	out := map[string][]baselineRow{"SSE": {}, "SZSE": {}, "BSE": {}, "HKEX": {}}
	aRaw, err := httpc.Get(ctx, "http://www.cninfo.com.cn/new/data/szse_stock.json")
	if err != nil {
		return nil, err
	}
	var aFile cninfoFile
	if json.Unmarshal(aRaw, &aFile) != nil {
		return nil, jsonError{"A股基准名单无法解析"}
	}
	for _, row := range aFile.StockList {
		code := strings.TrimSpace(row.Code)
		name := strings.TrimSpace(row.Name)
		if code == "" || name == "" || row.Category == "B股" {
			continue
		}
		switch {
		case strings.HasPrefix(code, "6"):
			out["SSE"] = append(out["SSE"], baselineRow{ID: code + ".SH", Exchange: "SSE", Name: name, Code: code})
		case strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3"):
			out["SZSE"] = append(out["SZSE"], baselineRow{ID: code + ".SZ", Exchange: "SZSE", Name: name, Code: code})
		case strings.HasPrefix(code, "92") || (len(code) == 6 && strings.HasPrefix(code, "8")):
			out["BSE"] = append(out["BSE"], baselineRow{ID: code + ".BJ", Exchange: "BSE", Name: name, Code: code})
		}
	}
	hkRaw, err := httpc.Get(ctx, "http://www.cninfo.com.cn/new/data/hke_stock.json")
	if err != nil {
		return out, err
	}
	var hkFile cninfoFile
	if json.Unmarshal(hkRaw, &hkFile) != nil {
		return out, nil
	}
	for _, row := range hkFile.StockList {
		code := strings.TrimSpace(row.Code)
		name := strings.TrimSpace(row.Name)
		if code == "" || name == "" || (row.Category != "" && row.Category != "港股") {
			continue
		}
		for len(code) < 5 {
			code = "0" + code
		}
		out["HKEX"] = append(out["HKEX"], baselineRow{ID: code + ".HK", Exchange: "HKEX", Name: name, Code: code})
	}
	return out, nil
}

func CompareCoverage(op []SeedInstrument, base map[string][]baselineRow) CoverageReport {
	rep := CoverageReport{
		SourceID: "eastmoney_push2", SourceContract: SourceContractVersion,
		OperationalCounts: map[string]int{}, BaselineCounts: map[string]int{},
		Matched: map[string]int{}, Coverage: map[string]float64{}, MissingSamples: map[string][]string{},
		Notes: []string{
			"基准由调用方提供。Claim100 仅在沪深北港分母均非空且逐项匹配时为 true。",
			"4 开头代码视为新三板，不计入北交所分母。",
		},
	}
	opIDs := map[string]SeedInstrument{}
	for _, row := range op {
		if row.AssetType != "stock" {
			rep.OperationalCounts["non_stock"]++
			continue
		}
		opIDs[row.InstrumentID] = row
		rep.OperationalCounts[row.Exchange]++
	}
	allMatch := true
	for _, mkt := range []string{"SSE", "SZSE", "BSE", "HKEX"} {
		rows := base[mkt]
		rep.BaselineCounts[mkt] = len(rows)
		hit := 0
		var miss []string
		for _, b := range rows {
			if _, ok := opIDs[b.ID]; ok {
				hit++
			} else if len(miss) < 15 {
				miss = append(miss, b.ID+" "+b.Name)
			}
		}
		rep.Matched[mkt] = hit
		if len(rows) == 0 {
			rep.Coverage[mkt] = 0
			allMatch = false
		} else {
			rep.Coverage[mkt] = float64(hit) / float64(len(rows))
			if hit != len(rows) {
				allMatch = false
			}
		}
		rep.MissingSamples[mkt] = miss
	}
	rep.Claim100 = allMatch
	return rep
}

type jsonError struct{ s string }

func (e jsonError) Error() string { return e.s }
