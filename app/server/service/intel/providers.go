package intel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const maxProviderResponseBytes = 8 << 20

type ProviderConfig struct {
	Name         string
	BaseURL      string
	AuthHeader   string
	AuthValue    string
	APIKeyHeader string
	APIKey       string
	Enabled      bool
	Reason       string
}

func providerConfigs() map[string]ProviderConfig {
	ifindURL := strings.TrimSpace(os.Getenv("IFIND_MCP_URL"))
	fuyaoKey := strings.TrimSpace(os.Getenv("FUYAO_API_KEY"))
	out := map[string]ProviderConfig{
		"fixture": {Name: "fixture", Enabled: true},
		"ifind":   {Name: "ifind", BaseURL: ifindURL, AuthHeader: "Authorization", AuthValue: os.Getenv("IFIND_AUTHORIZATION")},
		"cninfo":  {Name: "cninfo", BaseURL: "http://www.cninfo.com.cn/new/hisAnnouncement/query"},
		"fuyao":   {Name: "fuyao", BaseURL: "https://fuyao.aicubes.cn", APIKeyHeader: "X-api-key", APIKey: fuyaoKey},
	}
	ifind := out["ifind"]
	if ifind.BaseURL == "" || ifind.AuthValue == "" {
		ifind.Reason = "missing IFIND_MCP_URL or IFIND_AUTHORIZATION"
	} else {
		ifind.Enabled = true
	}
	out["ifind"] = ifind
	cninfo := out["cninfo"]
	cninfo.Reason = "website endpoint requires production HTTPS/terms verification"
	out["cninfo"] = cninfo
	fuyao := out["fuyao"]
	if fuyao.APIKey == "" {
		fuyao.Reason = "missing FUYAO_API_KEY"
	} else {
		fuyao.Enabled = true
	}
	out["fuyao"] = fuyao
	return out
}

type ProviderRequest struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    []byte
}

type ProviderClient struct {
	Config ProviderConfig
	Allow  map[string]struct{}
	Client *http.Client
}

func NewProviderClient(config ProviderConfig, allowURLs ...string) *ProviderClient {
	allow := make(map[string]struct{}, len(allowURLs))
	for _, raw := range allowURLs {
		if parsed, err := url.Parse(raw); err == nil {
			allow[canonicalOrigin(parsed)] = struct{}{}
		}
	}
	return &ProviderClient{
		Config: config,
		Allow:  allow,
		Client: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return errors.New("provider redirect chain too long")
				}
				if !providerURLAllowed(req.URL, allow) {
					return errors.New("provider redirect target is not allowlisted")
				}
				if len(via) > 0 && via[0].URL.Host != req.URL.Host {
					req.Header.Del("Authorization")
					req.Header.Del("X-api-key")
				}
				return nil
			},
		},
	}
}

func canonicalOrigin(u *url.URL) string {
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return strings.ToLower(u.Scheme + "://" + u.Hostname() + ":" + port)
}

func providerURLAllowed(u *url.URL, allow map[string]struct{}) bool {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return false
	}
	if len(allow) == 0 {
		return false
	}
	if _, ok := allow[canonicalOrigin(u)]; !ok {
		return false
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()) {
		return false
	}
	return true
}

func (p *ProviderClient) Do(ctx context.Context, req ProviderRequest) ([]byte, http.Header, error) {
	if !p.Config.Enabled {
		return nil, nil, newError(422, "PROVIDER_DISABLED", "provider 未启用或未授权")
	}
	parsed, err := url.Parse(req.URL)
	if err != nil || !providerURLAllowed(parsed, p.Allow) {
		return nil, nil, newError(400, "INVALID_PARAM", "provider URL 不在授权范围")
	}
	method := strings.ToUpper(req.Method)
	if method == "" {
		method = http.MethodGet
	}
	request, err := http.NewRequestWithContext(ctx, method, parsed.String(), strings.NewReader(string(req.Body)))
	if err != nil {
		return nil, nil, err
	}
	for k, v := range req.Headers {
		request.Header.Set(k, v)
	}
	if p.Config.AuthHeader != "" && p.Config.AuthValue != "" {
		request.Header.Set(p.Config.AuthHeader, p.Config.AuthValue)
	}
	if p.Config.APIKeyHeader != "" && p.Config.APIKey != "" {
		request.Header.Set(p.Config.APIKeyHeader, p.Config.APIKey)
	}
	response, err := p.Client.Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxProviderResponseBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if len(body) > maxProviderResponseBytes {
		return nil, nil, newError(503, "DATA_UNAVAILABLE", "provider 响应超过8MiB")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, response.Header, fmt.Errorf("provider status %d", response.StatusCode)
	}
	return body, response.Header, nil
}
