package finance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const eastMoneyFinancialURL = "https://datacenter.eastmoney.com/securities/api/data/v1/get"

type emEnvelope struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Result  struct {
		Data []map[string]any `json:"data"`
	} `json:"result"`
}

func fetchEastMoneySheet(ctx context.Context, httpc *CountedHTTP, reportName, securityCode string) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("reportName", reportName)
	q.Set("columns", "ALL")
	q.Set("filter", fmt.Sprintf(`(SECURITY_CODE="%s")`, securityCode))
	q.Set("pageNumber", "1")
	q.Set("pageSize", "20")
	q.Set("sortTypes", "-1")
	q.Set("sortColumns", "REPORT_DATE")
	q.Set("source", "HSF10")
	q.Set("client", "PC")
	body, _, err := httpc.Get(ctx, eastMoneyFinancialURL+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var env emEnvelope
	if err := json.Unmarshal(body, &env); err != nil || !env.Success {
		msg := env.Message
		if msg == "" {
			msg = "东方财富财务接口无法解析"
		}
		return nil, NewError(503, "unavailable", "DATA_UNAVAILABLE", msg)
	}
	return env.Result.Data, nil
}

func annualRow(rows []map[string]any, periodEnd string) map[string]any {
	want := periodEnd
	if len(want) == 4 {
		want = want + "-12-31"
	}
	for _, row := range rows {
		if strVal(row["REPORT_TYPE"]) != "年报" {
			continue
		}
		got := strings.TrimSpace(strVal(row["REPORT_DATE"]))
		if strings.HasPrefix(got, want) {
			return row
		}
	}
	return nil
}

func strVal(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.8f", t), "0"), ".")
	case json.Number:
		return t.String()
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
}

func decimalYuan(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	raw := strings.TrimSpace(strVal(v))
	if raw == "" || raw == "<nil>" {
		return "", false
	}
	d, err := decimal.NewFromString(raw)
	if err != nil {
		return "", false
	}
	return d.String(), true
}

func parseEMDate(v any) (time.Time, bool) {
	s := strings.TrimSpace(strVal(v))
	if s == "" {
		return time.Time{}, false
	}
	if i := strings.IndexByte(s, ' '); i > 0 {
		s = s[:i]
	}
	day, err := time.ParseInLocation("2006-01-02", s, time.FixedZone("CST", 8*3600))
	if err != nil {
		return time.Time{}, false
	}
	return day, true
}

func conservativeDayTimes(dayCST time.Time) (published, available time.Time) {
	// published_at encodes the calendar day at 00:00Z for v1 compatibility, not an exact clock.
	published = time.Date(dayCST.Year(), dayCST.Month(), dayCST.Day(), 0, 0, 0, 0, time.UTC)
	next := dayCST.AddDate(0, 0, 1)
	available = time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, dayCST.Location()).UTC()
	return published, available
}

type hkReportMeta struct {
	Currency string
	GAAP     string
	Dates    []string
	ByPeriod map[string]map[string]any
}

func fetchHKReportMeta(ctx context.Context, httpc *CountedHTTP, symbol string) (hkReportMeta, error) {
	q := url.Values{}
	q.Set("reportName", hkSummary)
	q.Set("columns", "SECUCODE,SECURITY_CODE,SECURITY_NAME_ABBR,START_DATE,REPORT_DATE,FISCAL_YEAR,CURRENCY,ACCOUNT_STANDARD,REPORT_TYPE")
	q.Set("filter", fmt.Sprintf(`(SECUCODE="%s.HK")`, symbol))
	q.Set("source", "F10")
	q.Set("client", "PC")
	body, _, err := httpc.Get(ctx, eastMoneyFinancialURL+"?"+q.Encode())
	if err != nil {
		return hkReportMeta{}, err
	}
	var env emEnvelope
	if err := json.Unmarshal(body, &env); err != nil || !env.Success || len(env.Result.Data) == 0 {
		return hkReportMeta{}, NewError(503, "unavailable", "DATA_UNAVAILABLE", "港股财务摘要无法解析")
	}
	rawList := env.Result.Data[0]["REPORT_LIST"]
	meta := hkReportMeta{ByPeriod: map[string]map[string]any{}}
	for _, row := range asObjectSlice(rawList) {
		if strVal(row["REPORT_TYPE"]) != "年报" {
			continue
		}
		got := strVal(row["REPORT_DATE"])
		if got == "" {
			continue
		}
		meta.Dates = append(meta.Dates, got)
		key := got
		if len(key) >= 10 {
			key = key[:10]
		}
		meta.ByPeriod[key] = row
	}
	if len(meta.Dates) == 0 {
		return hkReportMeta{}, NewError(503, "unavailable", "DATA_UNAVAILABLE", "缺少已披露港股年报")
	}
	return meta, nil
}

func fetchHKSheet(ctx context.Context, httpc *CountedHTTP, reportName, symbol string, dates []string) ([]map[string]any, error) {
	quoted := make([]string, 0, len(dates))
	for _, d := range dates {
		quoted = append(quoted, "'"+d+"'")
	}
	cols := "SECUCODE,SECURITY_CODE,SECURITY_NAME_ABBR,ORG_CODE,REPORT_DATE,DATE_TYPE_CODE,FISCAL_YEAR,STD_ITEM_CODE,STD_ITEM_NAME,AMOUNT"
	if reportName == hkIncome || reportName == hkCash {
		cols = "SECUCODE,SECURITY_CODE,SECURITY_NAME_ABBR,ORG_CODE,REPORT_DATE,DATE_TYPE_CODE,FISCAL_YEAR,START_DATE,STD_ITEM_CODE,STD_ITEM_NAME,AMOUNT"
	}
	q := url.Values{}
	q.Set("reportName", reportName)
	q.Set("columns", cols)
	q.Set("filter", fmt.Sprintf(`(SECUCODE="%s.HK")(REPORT_DATE in (%s))`, symbol, strings.Join(quoted, ",")))
	q.Set("pageNumber", "1")
	q.Set("pageSize", "500")
	q.Set("sortTypes", "-1,1")
	q.Set("sortColumns", "REPORT_DATE,STD_ITEM_CODE")
	q.Set("source", "F10")
	q.Set("client", "PC")
	body, _, err := httpc.Get(ctx, eastMoneyFinancialURL+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var env emEnvelope
	if err := json.Unmarshal(body, &env); err != nil || !env.Success {
		msg := env.Message
		if msg == "" {
			msg = "东方财富港股报表无法解析"
		}
		return nil, NewError(503, "unavailable", "DATA_UNAVAILABLE", msg)
	}
	return env.Result.Data, nil
}

func matchHKAnnualDates(meta hkReportMeta, periods []string) ([]string, error) {
	var out []string
	for _, period := range periods {
		want := normalizePeriodEnd(period)
		found := ""
		if row, ok := meta.ByPeriod[want]; ok {
			found = strVal(row["REPORT_DATE"])
		} else {
			for _, d := range meta.Dates {
				if strings.HasPrefix(d, want) {
					found = d
					break
				}
			}
		}
		if found == "" {
			return nil, NewError(503, "unavailable", "DATA_UNAVAILABLE", "缺少已披露年度报表："+want)
		}
		out = append(out, found)
	}
	return out, nil
}

func hkAmount(rows []map[string]any, periodEnd string, def MetricDef) (string, bool) {
	want := normalizePeriodEnd(periodEnd)
	var hit []map[string]any
	for _, row := range rows {
		if strings.HasPrefix(strVal(row["REPORT_DATE"]), want) {
			hit = append(hit, row)
		}
	}
	for _, name := range def.HKNames {
		for _, row := range hit {
			if strVal(row["STD_ITEM_NAME"]) == name {
				return decimalYuan(row["AMOUNT"])
			}
		}
	}
	for _, code := range def.HKCodes {
		for _, row := range hit {
			if strVal(row["STD_ITEM_CODE"]) == code {
				return decimalYuan(row["AMOUNT"])
			}
		}
	}
	return "", false
}

func mapHKCurrency(raw string) string {
	s := strings.TrimSpace(raw)
	switch {
	case s == "" || strings.Contains(s, "人民币") || strings.EqualFold(s, "CNY") || strings.EqualFold(s, "RMB"):
		if s == "" {
			return "HKD"
		}
		return "CNY"
	case strings.Contains(s, "港") || strings.EqualFold(s, "HKD"):
		return "HKD"
	case strings.Contains(s, "美元") || strings.EqualFold(s, "USD"):
		return "USD"
	default:
		return s
	}
}

func asObjectSlice(v any) []map[string]any {
	switch t := v.(type) {
	case []map[string]any:
		return t
	case []any:
		out := make([]map[string]any, 0, len(t))
		for _, item := range t {
			row, ok := item.(map[string]any)
			if ok {
				out = append(out, row)
			}
		}
		return out
	default:
		return nil
	}
}

func emSourceURL(inst ListedInstrument) string {
	if inst.Market == MarketHK {
		return "https://emweb.securities.eastmoney.com/PC_HKF10/NewFinancialAnalysis/Index?type=web&code=" + inst.Symbol
	}
	prefix := "SZ"
	switch {
	case strings.HasSuffix(inst.ID, ".SH"):
		prefix = "SH"
	case strings.HasSuffix(inst.ID, ".BJ"):
		prefix = "BJ"
	}
	return "https://emweb.securities.eastmoney.com/PC_HSF10/NewFinanceAnalysis/Index?type=web&code=" + prefix + inst.Symbol
}
