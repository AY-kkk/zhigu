package market

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"zhigu/server/service/finance"
)

type Adapter interface {
	ID() string
	List(ctx context.Context, boardKey string) ([]SeedInstrument, error)
	Bars(ctx context.Context, inst SeedInstrument, period, adjust, start, end string, limit int) ([]QuoteBar, error)
	Actions(ctx context.Context, inst SeedInstrument) ([]CorporateAction, error)
}

type EastMoney struct {
	HTTP *HTTP
}

func (e *EastMoney) ID() string { return "eastmoney_push2" }

func (e *EastMoney) getQuote(ctx context.Context, pathQuery string, hosts []string) ([]byte, error) {
	var last error
	for i, host := range hosts {
		raw, err := e.HTTP.Get(ctx, host+pathQuery)
		if err == nil {
			return raw, nil
		}
		last = err
		if i == len(hosts)-1 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	if last == nil {
		return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "行情源不可用")
	}
	return nil, last
}

func secidOf(inst SeedInstrument) (string, error) {
	switch inst.Exchange {
	case "SSE":
		return "1." + inst.Code, nil
	case "SZSE":
		return "0." + inst.Code, nil
	case "BSE":
		return "0." + inst.Code, nil
	case "HKEX":
		return "116." + inst.Code, nil
	default:
		return "", finance.NewError(422, "validation", "INSTRUMENT_UNSUPPORTED", "无 secid 映射")
	}
}

var boardFS = map[string]string{
	"SSE_MAIN": "m:1+t:2", "SSE_STAR": "m:1+t:23", "SSE_ETF": "m:1+t:5",
	"SZSE_MAIN": "m:0+t:6", "SZSE_CHINEXT": "m:0+t:80",
	"BSE":       "m:0+t:81+s:2048",
	"HKEX_MAIN": "m:128+t:3", "HKEX_GEM": "m:128+t:4",
}

// Staging floors abort a publish rather than shipping a truncated page as the full board.
var boardFloors = map[string]int{
	"SSE_MAIN": 1500, "SSE_STAR": 400,
	"SZSE_MAIN": 1400, "SZSE_CHINEXT": 1000,
	"BSE":       200,
	"HKEX_MAIN": 2000, "HKEX_GEM": 200,
}

func (e *EastMoney) List(ctx context.Context, boardKey string) ([]SeedInstrument, error) {
	fs, ok := boardFS[boardKey]
	if !ok {
		return nil, finance.NewError(400, "validation", "INVALID_PARAM", "未知板块")
	}
	var out []SeedInstrument
	var total int
	for page := 1; page <= 80; page++ {
		q := url.Values{}
		q.Set("pn", strconv.Itoa(page))
		q.Set("pz", "100")
		q.Set("po", "1")
		q.Set("np", "1")
		q.Set("fltt", "2")
		q.Set("invt", "2")
		q.Set("fid", "f12")
		q.Set("fs", fs)
		q.Set("fields", "f12,f13,f14")
		raw, err := e.getQuote(ctx, "/api/qt/clist/get?"+q.Encode(), clistHosts)
		if err != nil {
			return nil, err
		}
		env, err := parseClist(raw)
		if err != nil {
			return nil, err
		}
		if page == 1 {
			total = env.Total
			if env.Total == 0 && len(env.Diff) == 0 {
				return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", boardKey+" 目录为空")
			}
		}
		if len(env.Diff) == 0 {
			break
		}
		for _, row := range env.Diff {
			code := strings.TrimSpace(fmt.Sprint(row["f12"]))
			name := strings.TrimSpace(fmt.Sprint(row["f14"]))
			if code == "" || code == "<nil>" {
				continue
			}
			out = append(out, classify(boardKey, code, name))
		}
		if len(env.Diff) < 100 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(80 * time.Millisecond):
		}
	}
	if floor := boardFloors[boardKey]; floor > 0 && len(out) < floor {
		return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE",
			fmt.Sprintf("%s 仅 %d 条，低于完整性下限 %d（源 total=%d）", boardKey, len(out), floor, total))
	}
	return out, nil
}

type clistEnv struct {
	Total int
	Diff  []map[string]any
}

func parseClist(raw []byte) (clistEnv, error) {
	var env struct {
		Data *struct {
			Total int             `json:"total"`
			Diff  json.RawMessage `json:"diff"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return clistEnv{}, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "目录无法解析")
	}
	if env.Data == nil {
		return clistEnv{}, nil
	}
	diff, err := decodeDiff(env.Data.Diff)
	if err != nil {
		return clistEnv{}, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "目录分页无法解析")
	}
	return clistEnv{Total: env.Data.Total, Diff: diff}, nil
}

func decodeDiff(raw json.RawMessage) ([]map[string]any, error) {
	raw = bytesTrim(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if raw[0] == '[' {
		var arr []map[string]any
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, err
		}
		return arr, nil
	}
	var obj map[string]map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, ea := strconv.Atoi(keys[i])
		b, eb := strconv.Atoi(keys[j])
		if ea == nil && eb == nil {
			return a < b
		}
		return keys[i] < keys[j]
	})
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, obj[k])
	}
	return out, nil
}

func bytesTrim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func classify(boardKey, code, name string) SeedInstrument {
	inst := SeedInstrument{Code: code, Name: name, Status: "listed", Lot: 100, AssetType: "stock"}
	switch {
	case strings.HasPrefix(boardKey, "SSE"):
		inst.Exchange, inst.InstrumentID, inst.SecurityID, inst.Currency, inst.Board = "SSE", code+".SH", "sec_"+code, "CNY", "MAIN"
		if boardKey == "SSE_STAR" {
			inst.Board = "STAR"
		}
		if boardKey == "SSE_ETF" {
			inst.AssetType = "fund"
		}
	case strings.HasPrefix(boardKey, "SZSE"):
		inst.Exchange, inst.InstrumentID, inst.SecurityID, inst.Currency, inst.Board = "SZSE", code+".SZ", "sec_"+code, "CNY", "MAIN"
		if boardKey == "SZSE_CHINEXT" {
			inst.Board = "CHINEXT"
		}
	case boardKey == "BSE":
		inst.Exchange, inst.InstrumentID, inst.SecurityID, inst.Currency, inst.Board = "BSE", code+".BJ", "sec_"+code, "CNY", "BSE"
	case strings.HasPrefix(boardKey, "HKEX"):
		inst.Exchange, inst.Currency, inst.Board = "HKEX", "HKD", "MAIN"
		if len(code) == 5 && strings.HasPrefix(code, "8") {
			inst.Currency = "CNY"
		}
		inst.InstrumentID = code + ".HK"
		inst.SecurityID = "sec_" + code
		if boardKey == "HKEX_GEM" {
			inst.Board = "GEM"
		}
		inst.Lot = 0
	}
	if inst.Lot == 0 && inst.Exchange != "HKEX" {
		inst.Lot = 100
	}
	return inst
}

func (e *EastMoney) Bars(ctx context.Context, inst SeedInstrument, period, adjust, start, end string, limit int) ([]QuoteBar, error) {
	sid, err := secidOf(inst)
	if err != nil {
		return nil, err
	}
	switch period {
	case "1d", "1w", "1mo":
	default:
		return nil, finance.NewError(400, "validation", "INVALID_PARAM", "不支持的周期")
	}
	fqt := "0"
	switch adjust {
	case "", "raw":
		fqt = "0"
	case "qfq":
		fqt = "1"
	case "hfq":
		fqt = "2"
	default:
		return nil, finance.NewError(400, "validation", "INVALID_PARAM", "不支持的复权")
	}
	if limit <= 0 {
		limit = 250
	}
	if limit > 2000 {
		limit = 2000
	}
	beg := strings.ReplaceAll(start, "-", "")
	fin := strings.ReplaceAll(end, "-", "")
	if beg == "" {
		beg = time.Now().UTC().AddDate(-8, 0, 0).Format("20060102")
	}
	if fin == "" {
		fin = "20500000"
	}
	q := url.Values{}
	q.Set("secid", sid)
	q.Set("klt", "101")
	q.Set("fqt", fqt)
	q.Set("lmt", strconv.Itoa(limit))
	q.Set("fields1", "f1,f2,f3,f4,f5,f6")
	q.Set("fields2", "f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61")
	q.Set("beg", beg)
	q.Set("end", fin)
	raw, err := e.getQuote(ctx, "/api/qt/stock/kline/get?"+q.Encode(), klineHosts)
	if err != nil {
		return nil, err
	}
	var env struct {
		Data *struct {
			Klines []string `json:"klines"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &env) != nil || env.Data == nil {
		return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "K 线无法解析")
	}
	complete := LastCompleteSession(CalendarID(inst.Exchange), time.Now().UTC())
	out := make([]QuoteBar, 0, len(env.Data.Klines))
	for _, line := range env.Data.Klines {
		p := strings.Split(line, ",")
		if len(p) < 6 {
			continue
		}
		out = append(out, QuoteBar{
			Time: p[0], Open: p[1], Close: p[2], High: p[3], Low: p[4],
			Volume: toShares(p[5], inst.Exchange), IsFinal: p[0] <= complete,
		})
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func toShares(vol, exchange string) string {
	// Frozen 2026-09-21: push2his kline f56 is 手 (lot of 100) for A-share / BSE; shares for HKEX.
	if exchange == "HKEX" {
		return vol
	}
	d, err := decimal.NewFromString(strings.TrimSpace(vol))
	if err != nil {
		return vol
	}
	return d.Mul(decimal.NewFromInt(100)).Truncate(0).String()
}

func (e *EastMoney) Actions(ctx context.Context, inst SeedInstrument) ([]CorporateAction, error) {
	if inst.Exchange == "HKEX" {
		return nil, nil
	}
	q := url.Values{}
	q.Set("pageSize", "50")
	q.Set("pageNumber", "1")
	q.Set("reportName", "RPT_SHAREBONUS_DET")
	q.Set("columns", "ALL")
	q.Set("source", "WEB")
	q.Set("client", "WEB")
	q.Set("filter", fmt.Sprintf(`(SECURITY_CODE="%s")`, inst.Code))
	q.Set("sortColumns", "NOTICE_DATE")
	q.Set("sortTypes", "-1")
	raw, err := e.HTTP.GetHeader(ctx, "https://datacenter-web.eastmoney.com/api/data/v1/get?"+q.Encode(), map[string]string{
		"Referer": "https://data.eastmoney.com/yjfp/",
	})
	if err != nil {
		return nil, err
	}
	var env struct {
		Success bool `json:"success"`
		Result  *struct {
			Data []map[string]any `json:"data"`
		} `json:"result"`
	}
	if json.Unmarshal(raw, &env) != nil || !env.Success || env.Result == nil {
		return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "公司行动无法解析")
	}
	out := make([]CorporateAction, 0, len(env.Result.Data))
	for _, row := range env.Result.Data {
		profile := fmt.Sprint(row["IMPL_PLAN_PROFILE"])
		if strings.Contains(profile, "配股") || strings.Contains(profile, "换股") || strings.Contains(profile, "吸收合并") {
			ex := ymd(fmt.Sprint(row["EX_DIVIDEND_DATE"]))
			out = append(out, CorporateAction{
				Kind: "unsupported", EffectiveAt: ex, EvidenceLevel: "historical_revised",
			})
			continue
		}
		ex := ymd(fmt.Sprint(row["EX_DIVIDEND_DATE"]))
		rec := ymd(fmt.Sprint(row["EQUITY_RECORD_DATE"]))
		if ex == "" {
			continue
		}
		if cash := cashPerShare(row); cash != "" {
			r, e := rec, ex
			avail := e
			out = append(out, CorporateAction{
				Kind: "cash_dividend", EffectiveAt: ex, AvailableAt: &avail,
				RecordDate: ptrIf(r), PayDate: nil, CashAmount: cash,
				EvidenceLevel: "historical_revised",
			})
		}
		if ratio := splitRatio(row, profile); ratio != "" && ratio != "1" {
			avail := ex
			out = append(out, CorporateAction{
				Kind: "split", EffectiveAt: ex, AvailableAt: &avail, Ratio: ratio,
				EvidenceLevel: "historical_revised",
			})
		}
	}
	return out, nil
}

func ptrIf(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ymd(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		return s[:10]
	}
	return ""
}

func cashPerShare(row map[string]any) string {
	raw := strings.TrimSpace(fmt.Sprint(row["PRETAX_BONUS_RMB"]))
	if raw == "" || raw == "<nil>" || strings.EqualFold(raw, "null") {
		return ""
	}
	d, err := decimal.NewFromString(raw)
	if err != nil || !d.GreaterThan(decimal.Zero) {
		return ""
	}
	return d.Div(decimal.NewFromInt(10)).String()
}

var (
	reSong  = regexp.MustCompile(`10送(\d+(?:\.\d+)?)`)
	reZhuan = regexp.MustCompile(`10转(\d+(?:\.\d+)?)`)
)

func splitRatio(row map[string]any, profile string) string {
	add := decimal.Zero
	for _, key := range []string{"BONUS_RATIO", "BONUS_IT_RATIO", "IT_RATIO"} {
		raw := strings.TrimSpace(fmt.Sprint(row[key]))
		if raw == "" || raw == "<nil>" || strings.EqualFold(raw, "null") {
			continue
		}
		if d, err := decimal.NewFromString(raw); err == nil {
			add = add.Add(d)
		}
	}
	if add.IsZero() {
		if m := reSong.FindStringSubmatch(profile); len(m) == 2 {
			add = add.Add(decimal.RequireFromString(m[1]))
		}
		if m := reZhuan.FindStringSubmatch(profile); len(m) == 2 {
			add = add.Add(decimal.RequireFromString(m[1]))
		}
	}
	if add.IsZero() {
		return ""
	}
	return decimal.NewFromInt(1).Add(add.Div(decimal.NewFromInt(10))).String()
}
