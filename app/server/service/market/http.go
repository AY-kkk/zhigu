package market

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"zhigu/server/service/finance"
)

var quoteHosts = map[string]struct{}{
	"push2.eastmoney.com":          {},
	"push2his.eastmoney.com":       {},
	"72.push2.eastmoney.com":       {},
	"72.push2his.eastmoney.com":    {},
	"80.push2.eastmoney.com":       {},
	"80.push2his.eastmoney.com":    {},
	"82.push2delay.eastmoney.com":  {},
	"datacenter.eastmoney.com":     {},
	"datacenter-web.eastmoney.com": {},
	"www.cninfo.com.cn":            {},
	"static.cninfo.com.cn":         {},
	"finance.sina.com.cn":          {},
	"quotes.sina.cn":               {},
	"www.szse.cn":                  {},
	"query.sse.com.cn":             {},
	"www.bse.cn":                   {},
	"www.hkex.com.hk":              {},
}

var klineHosts = []string{
	"https://push2his.eastmoney.com",
	"https://72.push2his.eastmoney.com",
	"https://80.push2his.eastmoney.com",
}

var clistHosts = []string{
	"https://push2.eastmoney.com",
	"https://72.push2.eastmoney.com",
	"https://80.push2.eastmoney.com",
}

type HTTP struct {
	client *http.Client
}

func NewHTTP(timeout time.Duration) *HTTP {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.MaxConnsPerHost = 2
	jar, _ := cookiejar.New(nil)
	return &HTTP{client: &http.Client{
		Timeout:   timeout,
		Transport: allowTrip{base},
		Jar:       jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return finance.NewError(400, "validation", "SSRF_BLOCKED", "重定向过多")
			}
			return allowQuoteURL(req.URL)
		},
	}}
}

type allowTrip struct{ base http.RoundTripper }

func (t allowTrip) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := allowQuoteURL(req.URL); err != nil {
		return nil, err
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", finance.ProductUserAgent)
	}
	host := strings.ToLower(req.URL.Hostname())
	if req.Header.Get("Referer") == "" && strings.Contains(host, "eastmoney.com") {
		req.Header.Set("Referer", "https://quote.eastmoney.com/")
	}
	return t.base.RoundTrip(req)
}

func allowQuoteURL(u *url.URL) error {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") {
		return finance.NewError(400, "validation", "SSRF_BLOCKED", "禁止的行情地址")
	}
	host := strings.ToLower(u.Hostname())
	if _, ok := quoteHosts[host]; !ok {
		return finance.NewError(400, "validation", "SSRF_BLOCKED", "主机不在行情允许名单")
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
		return finance.NewError(400, "validation", "SSRF_BLOCKED", "禁止内网地址")
	}
	return nil
}

func (h *HTTP) Get(ctx context.Context, raw string) ([]byte, error) {
	return h.GetHeader(ctx, raw, nil)
}

func (h *HTTP) GetHeader(ctx context.Context, raw string, extra map[string]string) ([]byte, error) {
	return h.do(ctx, http.MethodGet, raw, nil, extra)
}

func (h *HTTP) PostForm(ctx context.Context, raw string, form url.Values, extra map[string]string) ([]byte, error) {
	body := strings.NewReader(form.Encode())
	if extra == nil {
		extra = map[string]string{}
	}
	if extra["Content-Type"] == "" {
		extra["Content-Type"] = "application/x-www-form-urlencoded"
	}
	return h.do(ctx, http.MethodPost, raw, body, extra)
}

func (h *HTTP) do(ctx context.Context, method, raw string, body io.Reader, extra map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, raw, body)
	if err != nil {
		return nil, finance.NewError(400, "validation", "INVALID_URL", "无效地址")
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	res, err := h.client.Do(req)
	if err != nil {
		return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "行情源不可用")
	}
	defer res.Body.Close()
	got, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "读取行情失败")
	}
	if res.StatusCode >= 300 {
		return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", fmt.Sprintf("上游 HTTP %d", res.StatusCode))
	}
	return got, nil
}

func SourceContract() map[string]any {
	return map[string]any{
		"source_id":               "cninfo_list+exchange_official+eastmoney_kline",
		"source_contract_version": SourceContractVersion,
		"hosts": []string{
			"www.cninfo.com.cn", "query.sse.com.cn", "www.szse.cn", "www.bse.cn", "www.hkex.com.hk",
			"push2.eastmoney.com", "72.push2.eastmoney.com",
			"push2his.eastmoney.com", "72.push2his.eastmoney.com", "80.push2his.eastmoney.com",
			"datacenter-web.eastmoney.com",
		},
		"catalog": map[string]any{
			"operational": "cninfo listed names merged with exchange official lists",
			"official": map[string]string{
				"SSE":  "https://query.sse.com.cn/sseQuery/commonQuery.do sqlId=COMMON_SSE_CP_GPJCTPZ_GPLB_GP_L STOCK_TYPE=1,8 COMPANY_STATUS=2,4,5,7,8",
				"SZSE": "https://www.szse.cn/api/report/ShowReport SHOWTYPE=xlsx CATALOGID=1110 TABKEY=tab1",
				"BSE":  "POST https://www.bse.cn/nqxxController/nqxxCnzq.do typejb=T xxfcbj[]=2 after GET listedcompany.html cookie",
				"HKEX": "https://www.hkex.com.hk/eng/services/trading/securities/securitieslists/ListOfSecurities.xlsx Category=Equity Main/GEM/DR/Investment Companies",
			},
		},
		"list": map[string]any{
			"path":   "/api/qt/clist/get",
			"hosts":  clistHosts,
			"fields": "f12 code, f13 market, f14 name",
			"fs": map[string]string{
				"SSE_MAIN": "m:1+t:2", "SSE_STAR": "m:1+t:23",
				"SZSE_MAIN": "m:0+t:6", "SZSE_CHINEXT": "m:0+t:80",
				"BSE":       "m:0+t:81+s:2048",
				"HKEX_MAIN": "m:128+t:3", "HKEX_GEM": "m:128+t:4",
				"SSE_ETF": "m:1+t:5",
			},
		},
		"kline": map[string]any{
			"path":  "/api/qt/stock/kline/get",
			"hosts": klineHosts,
			"secid": map[string]string{"SSE": "1.code", "SZSE": "0.code", "BSE": "0.code", "HKEX": "116.code"},
			"klt":   map[string]int{"1d": 101, "1w": 102, "1mo": 103},
			"fqt":   map[string]int{"raw": 0, "qfq": 1, "hfq": 2},
			"note":  "BSE secid uses eastmoney market 0 explicitly, not copied from SZSE board rules. Hosts tried in order on disconnect.",
		},
		"timezone":    "exchange local calendar date",
		"intraday":    "not in this contract; delay_seconds unknown",
		"verified_at": "contract-frozen-2026-09-21",
	}
}
