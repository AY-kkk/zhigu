package market

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestDigitsCode(t *testing.T) {
	if got := digitsCode("1", 6); got != "000001" {
		t.Fatalf("got %s", got)
	}
	if got := digitsCode("00700.HK", 5); got != "00700" {
		t.Fatalf("got %s", got)
	}
	if got := digitsCode("80700", 5); got != "80700" {
		t.Fatalf("got %s", got)
	}
}

func TestParseSSEResultSkipsDelisted(t *testing.T) {
	raw := []byte(`{"result":[
		{"A_STOCK_CODE":"600000","SEC_NAME_CN":"浦发银行","DELIST_DATE":"-"},
		{"A_STOCK_CODE":"688001","COMPANY_ABBR":"华兴源创"},
		{"A_STOCK_CODE":"600001","SEC_NAME_CN":"已退市","DELIST_DATE":"2020-01-01"}
	]}`)
	rows, err := parseSSEResult(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].InstrumentID != "600000.SH" || rows[1].Board != "STAR" {
		t.Fatalf("%+v", rows)
	}
}

func TestParseBSEPage(t *testing.T) {
	raw := []byte(`null([{"content":[{"xxzqdm":"920185","xxzqjc":"贝特瑞"}],"totalPages":18,"totalElements":344}])`)
	page, err := parseBSEPage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalPages != 18 || len(page.Content) != 1 {
		t.Fatalf("%+v", page)
	}
	got := bseSeeds(page.Content)
	if len(got) != 1 || got[0].InstrumentID != "920185.BJ" {
		t.Fatalf("%+v", got)
	}
}

func TestMergeOfficialAddsHKLotAndRMBCounter(t *testing.T) {
	cn := []SeedInstrument{
		{InstrumentID: "00700.HK", Exchange: "HKEX", AssetType: "stock", Code: "00700", Name: "腾讯控股", Currency: "HKD", Lot: 0, Board: "MAIN"},
		{InstrumentID: "835185.BJ", Exchange: "BSE", AssetType: "stock", Code: "835185", Name: "贝特瑞", Currency: "CNY", Lot: 100, Board: "BSE"},
	}
	off := OfficialLists{ByMarket: map[string][]SeedInstrument{
		"HKEX": {
			{InstrumentID: "00700.HK", Exchange: "HKEX", AssetType: "stock", Code: "00700", Name: "TENCENT", Currency: "HKD", Lot: 100, Board: "MAIN"},
			{InstrumentID: "80700.HK", Exchange: "HKEX", AssetType: "stock", Code: "80700", Name: "TENCENT-R", Currency: "CNY", Lot: 100, Board: "MAIN"},
		},
		"BSE": {
			{InstrumentID: "920185.BJ", Exchange: "BSE", AssetType: "stock", Code: "920185", Name: "贝特瑞", Currency: "CNY", Lot: 100, Board: "BSE"},
		},
	}}
	got := MergeOfficialCatalog(cn, off)
	byID := map[string]SeedInstrument{}
	for _, row := range got {
		byID[row.InstrumentID] = row
	}
	if byID["00700.HK"].Lot != 100 {
		t.Fatalf("lot %+v", byID["00700.HK"])
	}
	if byID["80700.HK"].Currency != "CNY" {
		t.Fatalf("rmb %+v", byID["80700.HK"])
	}
	if _, ok := byID["835185.BJ"]; !ok {
		t.Fatal("old BSE code dropped")
	}
	if _, ok := byID["920185.BJ"]; !ok {
		t.Fatal("new BSE code missing")
	}
}

func TestParseSZSEAndHKEXXLSX(t *testing.T) {
	sz := miniInlineXLSX([][]string{
		{"板块", "公司全称", "英文名称", "注册地址", "A股代码", "A股简称"},
		{"主板", "平安银行股份有限公司", "PAB", "深圳", "000001", "平安银行"},
		{"创业板", "宁德时代新能源科技股份有限公司", "CATL", "宁德", "300750", "宁德时代"},
	})
	rows, err := parseSZSEGrid(sz)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].InstrumentID != "000001.SZ" || rows[1].Board != "CHINEXT" {
		t.Fatalf("%+v", rows)
	}

	hk := miniInlineXLSX([][]string{
		{"List of Securities"},
		{"Updated as at 22/09/2026"},
		{"Stock Code", "Name of Securities", "Category", "Sub-Category", "Board Lot", "ISIN", "x", "x", "x", "x", "x", "x", "x", "x", "x", "x", "Trading Currency"},
		{"00700", "TENCENT", "Equity", "Equity Securities (Main Board)", "100", "", "", "", "", "", "", "", "", "", "", "", "HKD"},
		{"08001", "GEMCO", "Equity", "Equity Securities (GEM)", "2000", "", "", "", "", "", "", "", "", "", "", "", "HKD"},
		{"80700", "TENCENT-R", "Equity", "Equity Securities (Main Board)", "100", "", "", "", "", "", "", "", "", "", "", "", "CNY"},
		{"02800", "TRACKER FUND", "Exchange Traded Products", "Exchange Traded Funds", "100", "", "", "", "", "", "", "", "", "", "", "", "HKD"},
	})
	hkRows, err := parseHKEXGrid(hk)
	if err != nil {
		t.Fatal(err)
	}
	if len(hkRows) != 3 {
		t.Fatalf("want 3 equity rows got %+v", hkRows)
	}
	byID := map[string]SeedInstrument{}
	for _, row := range hkRows {
		byID[row.InstrumentID] = row
	}
	if byID["08001.HK"].Board != "GEM" || byID["80700.HK"].Currency != "CNY" || byID["00700.HK"].Lot != 100 {
		t.Fatalf("%+v", byID)
	}
}

func TestAllowOfficialAndKlineHosts(t *testing.T) {
	for _, host := range []string{"www.szse.cn", "query.sse.com.cn", "www.bse.cn", "www.hkex.com.hk", "72.push2his.eastmoney.com", "web.ifzq.gtimg.cn"} {
		if _, ok := quoteHosts[host]; !ok {
			t.Fatalf("missing host %s", host)
		}
	}
	if len(klineHosts) < 2 || !strings.Contains(klineHosts[1], "72.push2his") {
		t.Fatalf("%v", klineHosts)
	}
}

func miniInlineXLSX(rows [][]string) []byte {
	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	body.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for i, row := range rows {
		body.WriteString(`<row r="` + itoaRow(i+1) + `">`)
		for c, v := range row {
			if v == "" {
				continue
			}
			ref := colLetter(c) + itoaRow(i+1)
			body.WriteString(`<c r="` + ref + `" t="inlineStr"><is><t>` + xmlEscape(v) + `</t></is></c>`)
		}
		body.WriteString(`</row>`)
	}
	body.WriteString(`</sheetData></worksheet>`)
	buf := new(bytes.Buffer)
	z := zip.NewWriter(buf)
	w, _ := z.Create("xl/worksheets/sheet1.xml")
	_, _ = w.Write([]byte(body.String()))
	_ = z.Close()
	return buf.Bytes()
}

func itoaRow(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func colLetter(i int) string {
	n := i + 1
	var s string
	for n > 0 {
		n--
		s = string(rune('A'+n%26)) + s
		n /= 26
	}
	return s
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
