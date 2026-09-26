package finance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var dataHosts = map[string]struct{}{
	"datacenter.eastmoney.com":       {},
	"emweb.securities.eastmoney.com": {},
	"www.cninfo.com.cn":              {},
	"static.cninfo.com.cn":           {},
	"money.finance.sina.com.cn":      {},
}

type countedRoundTripper struct {
	base    http.RoundTripper
	mu      sync.Mutex
	count   int
	limit   int
	targets []string
}

func (t *countedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := allowDataURL(req.URL); err != nil {
		return nil, err
	}
	t.mu.Lock()
	if t.count >= t.limit {
		t.mu.Unlock()
		return nil, NewError(503, "unavailable", "DATA_UNAVAILABLE", "超过冻结 HTTP 上限")
	}
	t.count++
	t.targets = append(t.targets, req.Method+" "+req.URL.String())
	t.mu.Unlock()
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", ProductUserAgent)
	}
	return t.base.RoundTrip(req)
}

type CountedHTTP struct {
	client *http.Client
	trip   *countedRoundTripper
}

func NewCountedHTTP(limit int, timeout time.Duration) *CountedHTTP {
	if limit <= 0 {
		limit = 1
	}
	base := http.DefaultTransport.(*http.Transport).Clone()
	trip := &countedRoundTripper{base: base, limit: limit}
	return &CountedHTTP{
		trip: trip,
		client: &http.Client{
			Timeout:   timeout,
			Transport: trip,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("too many redirects")
				}
				return allowDataURL(req.URL)
			},
		},
	}
}

func (c *CountedHTTP) Count() int {
	c.trip.mu.Lock()
	defer c.trip.mu.Unlock()
	return c.trip.count
}

func (c *CountedHTTP) Targets() []string {
	c.trip.mu.Lock()
	defer c.trip.mu.Unlock()
	out := make([]string, len(c.trip.targets))
	copy(out, c.trip.targets)
	return out
}

func (c *CountedHTTP) Do(req *http.Request) (*http.Response, error) {
	return c.client.Do(req)
}

func (c *CountedHTTP) Get(ctx context.Context, raw string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, "", NewError(400, "validation", "INVALID_URL", "无效地址")
	}
	res, err := c.Do(req)
	if err != nil {
		return nil, "", wrapDataErr(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, "", NewError(503, "unavailable", "DATA_UNAVAILABLE", "读取响应失败")
	}
	if res.StatusCode >= 300 {
		return nil, "", NewError(503, "unavailable", "DATA_UNAVAILABLE", fmt.Sprintf("上游 HTTP %d", res.StatusCode))
	}
	ct := res.Header.Get("Content-Type")
	return body, ct, nil
}

func (c *CountedHTTP) PostForm(ctx context.Context, raw string, form url.Values, extra http.Header) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, raw, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, NewError(400, "validation", "INVALID_URL", "无效地址")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", ProductUserAgent)
	for k, vs := range extra {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	res, err := c.Do(req)
	if err != nil {
		return nil, wrapDataErr(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, NewError(503, "unavailable", "DATA_UNAVAILABLE", "读取响应失败")
	}
	if res.StatusCode >= 300 {
		return nil, NewError(503, "unavailable", "DATA_UNAVAILABLE", fmt.Sprintf("上游 HTTP %d", res.StatusCode))
	}
	return body, nil
}

func allowDataURL(u *url.URL) error {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") {
		return NewError(400, "validation", "SSRF_BLOCKED", "禁止的数据地址")
	}
	host := strings.ToLower(u.Hostname())
	if _, ok := dataHosts[host]; !ok {
		return NewError(400, "validation", "SSRF_BLOCKED", "主机不在数据允许名单")
	}
	if ip := net.ParseIP(host); ip != nil && blockedIP(ip) {
		return NewError(400, "validation", "SSRF_BLOCKED", "禁止内网地址")
	}
	return nil
}

func wrapDataErr(err error) error {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae
	}
	return NewError(503, "unavailable", "DATA_UNAVAILABLE", "数据源不可用")
}

func SourceRules() map[string]any {
	hosts := make([]string, 0, len(dataHosts))
	for h := range dataHosts {
		hosts = append(hosts, h)
	}
	return map[string]any{
		"allowed_hosts": hosts,
		"user_agent":    ProductUserAgent,
		"body_rule":     "filings require title+time+url plus extracted digital PDF text with locator",
		"date_rule":     "NOTICE_DATE or announcementTime as Beijing calendar date; date_precision=day; available_at=next CST midnight",
		"period_rule":   "annual REPORT_TYPE=年报 only; period end YYYY-12-31",
		"unit_rule":     "A-share East Money amounts are CNY yuan; HK amounts use disclosed currency; scale_factor=1",
		"http_limits": map[string]int{
			"financial": FinancialHTTPLimit,
			"filing":    FilingHTTPLimit,
		},
		"notes": []string{
			"These HTTP endpoints are public website interfaces, not licensed redistributable open data.",
			"They change, must be rate-limited, and are not claimed as commercially redistributable.",
			"Sina finance JSON is a frozen backup source version, not a runtime failover.",
			"Live catalog is cninfo szse_stock.json (A shares including BJ) plus hke_stock.json (HK).",
			"HK three statements use East Money HKF10 item rows; A shares use HSF10 columnar sheets.",
		},
	}
}
