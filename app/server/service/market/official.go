package market

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"zhigu/server/service/finance"
)

var officialFloors = map[string]int{
	"SSE": 2000, "SZSE": 2500, "BSE": 200, "HKEX": 2000,
}

var hkEquitySubs = map[string]string{
	"Equity Securities (Main Board)": "MAIN",
	"Equity Securities (GEM)":        "GEM",
	"Depositary Receipts":            "MAIN",
	"Investment Companies":           "MAIN",
}

type OfficialLists struct {
	ByMarket map[string][]SeedInstrument
	Sources  map[string]string
	Errors   map[string]string
	Notes    []string
}

func FetchOfficialLists(ctx context.Context, httpc *HTTP) OfficialLists {
	out := OfficialLists{
		ByMarket: map[string][]SeedInstrument{},
		Sources:  map[string]string{},
		Errors:   map[string]string{},
	}
	if rows, err := fetchSSEOfficial(ctx, httpc); err != nil {
		out.Errors["SSE"] = err.Error()
	} else {
		out.ByMarket["SSE"] = rows
		out.Sources["SSE"] = "sse_gplb_COMMON_SSE_CP_GPJCTPZ_GPLB_GP_L"
	}
	if rows, err := fetchSZSEOfficial(ctx, httpc); err != nil {
		out.Errors["SZSE"] = err.Error()
	} else {
		out.ByMarket["SZSE"] = rows
		out.Sources["SZSE"] = "szse_showreport_xlsx_catalog_1110"
	}
	if rows, err := fetchBSEOfficial(ctx, httpc); err != nil {
		out.Errors["BSE"] = err.Error()
	} else {
		out.ByMarket["BSE"] = rows
		out.Sources["BSE"] = "bse_nqxxCnzq_fcbj2"
		out.Notes = append(out.Notes, "北交所官方现码为 92xxxx；巨潮仍可能保留换码前 8xxxxx（如 835185→920185 贝特瑞）")
	}
	if rows, err := fetchHKEXOfficial(ctx, httpc); err != nil {
		out.Errors["HKEX"] = err.Error()
	} else {
		out.ByMarket["HKEX"] = rows
		out.Sources["HKEX"] = "hkex_list_of_securities_xlsx"
	}
	for mkt, floor := range officialFloors {
		n := len(out.ByMarket[mkt])
		if n == 0 {
			continue
		}
		if n < floor {
			out.Errors[mkt] = "官方名单仅 " + strconv.Itoa(n) + " 条，低于完整性下限 " + strconv.Itoa(floor)
			delete(out.ByMarket, mkt)
			delete(out.Sources, mkt)
		}
	}
	return out
}

func (o OfficialLists) Baseline() map[string][]baselineRow {
	out := map[string][]baselineRow{"SSE": {}, "SZSE": {}, "BSE": {}, "HKEX": {}}
	for mkt, rows := range o.ByMarket {
		for _, row := range rows {
			if row.AssetType != "stock" {
				continue
			}
			out[mkt] = append(out[mkt], baselineRow{ID: row.InstrumentID, Exchange: row.Exchange, Name: row.Name, Code: row.Code})
		}
	}
	return out
}

func MergeOfficialCatalog(cninfo []SeedInstrument, off OfficialLists) []SeedInstrument {
	byID := map[string]int{}
	out := append([]SeedInstrument(nil), cninfo...)
	for i, row := range out {
		byID[row.InstrumentID] = i
	}
	for _, rows := range off.ByMarket {
		for _, row := range rows {
			if i, ok := byID[row.InstrumentID]; ok {
				if out[i].Lot == 0 && row.Lot > 0 {
					out[i].Lot = row.Lot
				}
				if row.Board != "" {
					out[i].Board = row.Board
				}
				if row.Currency != "" {
					out[i].Currency = row.Currency
				}
				if row.NameEN != "" && out[i].NameEN == "" {
					out[i].NameEN = row.NameEN
				}
				continue
			}
			byID[row.InstrumentID] = len(out)
			out = append(out, row)
		}
	}
	return out
}

func fetchSSEOfficial(ctx context.Context, httpc *HTTP) ([]SeedInstrument, error) {
	var out []SeedInstrument
	for _, stockType := range []string{"1", "8"} {
		q := url.Values{}
		q.Set("STOCK_TYPE", stockType)
		q.Set("REG_PROVINCE", "")
		q.Set("CSRC_CODE", "")
		q.Set("STOCK_CODE", "")
		q.Set("sqlId", "COMMON_SSE_CP_GPJCTPZ_GPLB_GP_L")
		q.Set("COMPANY_STATUS", "2,4,5,7,8")
		q.Set("type", "inParams")
		q.Set("isPagination", "true")
		q.Set("pageHelp.cacheSize", "1")
		q.Set("pageHelp.beginPage", "1")
		q.Set("pageHelp.pageSize", "10000")
		q.Set("pageHelp.pageNo", "1")
		q.Set("pageHelp.endPage", "1")
		raw, err := httpc.GetHeader(ctx, "https://query.sse.com.cn/sseQuery/commonQuery.do?"+q.Encode(), map[string]string{
			"Referer": "https://www.sse.com.cn/assortment/stock/list/share/",
		})
		if err != nil {
			return nil, err
		}
		rows, err := parseSSEResult(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	if len(out) == 0 {
		return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "上交所名单为空")
	}
	return dedupeSeeds(out), nil
}

func parseSSEResult(raw []byte) ([]SeedInstrument, error) {
	var env struct {
		Result []map[string]any `json:"result"`
	}
	if json.Unmarshal(raw, &env) != nil {
		return nil, jsonError{"上交所名单无法解析"}
	}
	out := make([]SeedInstrument, 0, len(env.Result))
	for _, row := range env.Result {
		if delist := strings.TrimSpace(fmtString(row["DELIST_DATE"])); delist != "" && delist != "-" && delist != "<nil>" {
			continue
		}
		code := digitsCode(fmtString(row["A_STOCK_CODE"]), 6)
		name := strings.TrimSpace(fmtString(row["SEC_NAME_CN"]))
		if name == "" {
			name = strings.TrimSpace(fmtString(row["COMPANY_ABBR"]))
		}
		if code == "" || name == "" {
			continue
		}
		out = append(out, SeedInstrument{
			InstrumentID: code + ".SH", SecurityID: "sec_" + code, Exchange: "SSE",
			Board: BoardOf("SSE", code), AssetType: "stock", Code: code, Name: name,
			Currency: "CNY", Status: "listed", Lot: 100,
		})
	}
	return out, nil
}

func fetchSZSEOfficial(ctx context.Context, httpc *HTTP) ([]SeedInstrument, error) {
	q := url.Values{}
	q.Set("SHOWTYPE", "xlsx")
	q.Set("CATALOGID", "1110")
	q.Set("TABKEY", "tab1")
	raw, err := httpc.GetHeader(ctx, "https://www.szse.cn/api/report/ShowReport?"+q.Encode(), map[string]string{
		"Referer": "https://www.szse.cn/market/product/stock/list/index.html",
	})
	if err != nil {
		return nil, err
	}
	return parseSZSEGrid(raw)
}

func parseSZSEGrid(raw []byte) ([]SeedInstrument, error) {
	grid, err := readXLSXGrid(raw)
	if err != nil {
		return nil, err
	}
	cols, header := headerCols(grid, "A股代码", "A股简称")
	if header < 0 {
		return nil, jsonError{"深交所 xlsx 缺少 A股代码列"}
	}
	codeCol, nameCol := cols["A股代码"], cols["A股简称"]
	out := make([]SeedInstrument, 0, len(grid)-header-1)
	for _, row := range grid[header+1:] {
		code := digitsCode(cellAt(row, codeCol), 6)
		name := strings.ReplaceAll(cellAt(row, nameCol), " ", "")
		if code == "" || name == "" {
			continue
		}
		out = append(out, SeedInstrument{
			InstrumentID: code + ".SZ", SecurityID: "sec_" + code, Exchange: "SZSE",
			Board: BoardOf("SZSE", code), AssetType: "stock", Code: code, Name: name,
			Currency: "CNY", Status: "listed", Lot: 100,
		})
	}
	if len(out) == 0 {
		return nil, jsonError{"深交所名单为空"}
	}
	return out, nil
}

func fetchBSEOfficial(ctx context.Context, httpc *HTTP) ([]SeedInstrument, error) {
	if _, err := httpc.GetHeader(ctx, "https://www.bse.cn/nq/listedcompany.html", map[string]string{
		"Referer": "https://www.bse.cn/",
	}); err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("page", "0")
	form.Set("typejb", "T")
	form.Set("xxzqdm", "")
	form.Set("sortfield", "xxzqdm")
	form.Set("sorttype", "asc")
	form["xxfcbj[]"] = []string{"2"}
	raw, err := httpc.PostForm(ctx, "https://www.bse.cn/nqxxController/nqxxCnzq.do", form, map[string]string{
		"Referer": "https://www.bse.cn/nq/listedcompany.html",
	})
	if err != nil {
		return nil, err
	}
	page, err := parseBSEPage(raw)
	if err != nil {
		return nil, err
	}
	if page.TotalPages <= 0 || page.TotalPages > 40 {
		return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "北交所分页异常")
	}
	var out []SeedInstrument
	out = append(out, bseSeeds(page.Content)...)
	for p := 1; p < page.TotalPages; p++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
		form.Set("page", strconv.Itoa(p))
		raw, err = httpc.PostForm(ctx, "https://www.bse.cn/nqxxController/nqxxCnzq.do", form, map[string]string{
			"Referer": "https://www.bse.cn/nq/listedcompany.html",
		})
		if err != nil {
			return nil, err
		}
		next, perr := parseBSEPage(raw)
		if perr != nil {
			return nil, perr
		}
		out = append(out, bseSeeds(next.Content)...)
	}
	if len(out) == 0 {
		return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "北交所名单为空")
	}
	return dedupeSeeds(out), nil
}

type bsePage struct {
	Content       []map[string]any `json:"content"`
	TotalPages    int              `json:"totalPages"`
	TotalElements int              `json:"totalElements"`
	Msg           string           `json:"msg"`
}

func parseBSEPage(raw []byte) (bsePage, error) {
	s := strings.TrimSpace(string(raw))
	i := strings.Index(s, "[")
	if i < 0 {
		return bsePage{}, jsonError{"北交所名单无法解析"}
	}
	s = strings.TrimSpace(s[i:])
	if strings.HasSuffix(s, ")") {
		s = strings.TrimSuffix(s, ")")
	}
	var pages []bsePage
	if json.Unmarshal([]byte(s), &pages) != nil || len(pages) == 0 {
		return bsePage{}, jsonError{"北交所名单无法解析"}
	}
	if pages[0].Msg != "" {
		return bsePage{}, jsonError{pages[0].Msg}
	}
	return pages[0], nil
}

func bseSeeds(rows []map[string]any) []SeedInstrument {
	out := make([]SeedInstrument, 0, len(rows))
	for _, row := range rows {
		code := digitsCode(fmtString(row["xxzqdm"]), 6)
		name := strings.TrimSpace(fmtString(row["xxzqjc"]))
		if code == "" || name == "" {
			continue
		}
		out = append(out, SeedInstrument{
			InstrumentID: code + ".BJ", SecurityID: "sec_" + code, Exchange: "BSE",
			Board: "BSE", AssetType: "stock", Code: code, Name: name,
			Currency: "CNY", Status: "listed", Lot: 100,
		})
	}
	return out
}

func fetchHKEXOfficial(ctx context.Context, httpc *HTTP) ([]SeedInstrument, error) {
	raw, err := httpc.GetHeader(ctx, "https://www.hkex.com.hk/eng/services/trading/securities/securitieslists/ListOfSecurities.xlsx", map[string]string{
		"Referer": "https://www.hkex.com.hk/",
	})
	if err != nil {
		return nil, err
	}
	return parseHKEXGrid(raw)
}

func parseHKEXGrid(raw []byte) ([]SeedInstrument, error) {
	grid, err := readXLSXGrid(raw)
	if err != nil {
		return nil, err
	}
	cols, header := headerCols(grid, "Stock Code", "Name of Securities", "Category", "Sub-Category")
	if header < 0 {
		return nil, jsonError{"港交所 xlsx 缺少 Stock Code 列"}
	}
	lotCol, curCol := -1, -1
	if header < len(grid) {
		for i, v := range grid[header] {
			switch strings.TrimSpace(strings.ReplaceAll(v, "\n", " ")) {
			case "Board Lot":
				lotCol = i
			case "Trading Currency":
				curCol = i
			}
		}
	}
	out := make([]SeedInstrument, 0, 3000)
	for _, row := range grid[header+1:] {
		if cellAt(row, cols["Category"]) != "Equity" {
			continue
		}
		sub := cellAt(row, cols["Sub-Category"])
		board, ok := hkEquitySubs[sub]
		if !ok {
			continue
		}
		code := digitsCode(cellAt(row, cols["Stock Code"]), 5)
		name := cellAt(row, cols["Name of Securities"])
		if code == "" || name == "" {
			continue
		}
		ccy := "HKD"
		if cur := strings.ToUpper(cellAt(row, curCol)); cur == "CNY" || cur == "RMB" {
			ccy = "CNY"
		} else if cur == "USD" {
			ccy = "USD"
		} else if len(code) == 5 && strings.HasPrefix(code, "8") {
			ccy = "CNY"
		}
		lot := 0
		if lotCol >= 0 {
			lot = atoiSafe(strings.ReplaceAll(cellAt(row, lotCol), ",", ""))
		}
		out = append(out, SeedInstrument{
			InstrumentID: code + ".HK", SecurityID: "sec_" + code, Exchange: "HKEX",
			Board: board, AssetType: "stock", Code: code, Name: name, NameEN: name,
			Currency: ccy, Status: "listed", Lot: lot,
		})
	}
	if len(out) == 0 {
		return nil, jsonError{"港交所股票名单为空"}
	}
	return out, nil
}

var codeDigits = regexp.MustCompile(`\d+`)

func digitsCode(raw string, width int) string {
	s := strings.TrimSpace(strings.Split(raw, ".")[0])
	s = strings.ReplaceAll(s, ",", "")
	m := codeDigits.FindString(s)
	if m == "" {
		return ""
	}
	if len(m) > width {
		m = m[len(m)-width:]
	}
	for len(m) < width {
		m = "0" + m
	}
	return m
}

func fmtString(v any) string {
	if v == nil {
		return ""
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" || s == "-" || s == "<nil>" {
		return ""
	}
	return s
}
