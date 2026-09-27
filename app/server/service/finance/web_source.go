package finance

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxWebArticleBytes = 5 * 1024 * 1024

type WebFetchResult struct {
	FinalURL    string
	StatusCode  int
	ContentType string
	Body        []byte
	FetchedAt   time.Time
}

type WebFetcher interface {
	Fetch(ctx context.Context, rawURL string) (WebFetchResult, error)
}

type PublicWebFetcher struct{}

func validatePublicWebURL(raw string) (*url.URL, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, NewError(400, "validation", "INVALID_URL", "链接不能为空")
	}
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Hostname() == "" {
		return nil, NewError(400, "validation", "INVALID_URL", "链接格式无效")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" && scheme != "http" {
		return nil, NewError(400, "validation", "INVALID_URL_SCHEME", "仅支持 http/https 链接")
	}
	if parsed.User != nil {
		return nil, NewError(400, "validation", "URL_CREDENTIALS_FORBIDDEN", "链接不得包含用户名或密码")
	}
	if parsed.Port() != "" && parsed.Port() != "80" && parsed.Port() != "443" {
		return nil, NewError(400, "validation", "URL_PORT_FORBIDDEN", "仅允许标准 80/443 端口")
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return nil, NewError(400, "validation", "SSRF_BLOCKED", "禁止本地地址")
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return nil, NewError(400, "validation", "URL_DNS_FAILED", "无法解析链接域名")
	}
	for _, ip := range ips {
		if blockedWebIP(ip) {
			return nil, NewError(400, "validation", "SSRF_BLOCKED", "禁止内网或本地地址")
		}
	}
	return parsed, nil
}

func blockedWebIP(ip net.IP) bool {
	return ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

func publicWebClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 15 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, portText, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			if net.ParseIP(host) != nil {
				ip := net.ParseIP(host)
				if blockedWebIP(ip) {
					return nil, NewError(400, "validation", "SSRF_BLOCKED", "禁止内网或本地地址")
				}
				return dialer.DialContext(ctx, network, address)
			}
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil || len(ips) == 0 {
				return nil, NewError(400, "validation", "URL_DNS_FAILED", "无法解析链接域名")
			}
			for _, ip := range ips {
				if blockedWebIP(ip) {
					continue
				}
				port, _ := strconv.Atoi(portText)
				if port != 80 && port != 443 {
					return nil, NewError(400, "validation", "URL_PORT_FORBIDDEN", "仅允许标准 80/443 端口")
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), portText))
			}
			return nil, NewError(400, "validation", "SSRF_BLOCKED", "禁止内网或本地地址")
		},
	}
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 4 {
				return NewError(400, "validation", "URL_REDIRECT_LIMIT", "链接重定向次数过多")
			}
			_, err := validatePublicWebURL(req.URL.String())
			return err
		},
	}
}

func (PublicWebFetcher) Fetch(ctx context.Context, rawURL string) (WebFetchResult, error) {
	parsed, err := validatePublicWebURL(rawURL)
	if err != nil {
		return WebFetchResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return WebFetchResult{}, NewError(400, "validation", "INVALID_URL", "链接格式无效")
	}
	req.Header.Set("User-Agent", ProductUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9")
	res, err := publicWebClient().Do(req)
	if err != nil {
		return WebFetchResult{}, NewError(502, "unavailable", "WEB_FETCH_FAILED", "网页读取失败")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 400 {
		return WebFetchResult{}, NewError(502, "unavailable", "WEB_FETCH_FAILED", fmt.Sprintf("网页返回 HTTP %d", res.StatusCode))
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(res.Header.Get("Content-Type"), ";")[0]))
	if contentType != "text/html" && contentType != "application/xhtml+xml" && contentType != "text/plain" {
		return WebFetchResult{}, NewError(415, "validation", "UNSUPPORTED_WEB_CONTENT", "链接内容不是可读 HTML 或纯文本")
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxWebArticleBytes+1))
	if err != nil {
		return WebFetchResult{}, NewError(502, "unavailable", "WEB_FETCH_FAILED", "网页正文读取失败")
	}
	if len(raw) > maxWebArticleBytes {
		return WebFetchResult{}, NewError(413, "validation", "WEB_ARTICLE_TOO_LARGE", "网页正文不能超过 5 MB")
	}
	finalURL := res.Request.URL.String()
	if _, err := validatePublicWebURL(finalURL); err != nil {
		return WebFetchResult{}, err
	}
	return WebFetchResult{FinalURL: finalURL, StatusCode: res.StatusCode, ContentType: contentType, Body: raw, FetchedAt: time.Now().UTC()}, nil
}
