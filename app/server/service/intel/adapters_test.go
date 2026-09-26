package intel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonHTTPResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"req-fake"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}
}

func TestFuyaoSearchNormalizesContract(t *testing.T) {
	t.Setenv("FUYAO_API_KEY", "test-key")
	client := NewFuyaoClient()
	client.Client.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/meta/tickers/search" || req.URL.Query().Get("asset_type") != "a-share" {
			t.Fatalf("unexpected request %s", req.URL)
		}
		if req.Header.Get("X-api-key") == "" {
			t.Fatal("missing X-api-key")
		}
		return jsonHTTPResponse(req, 200, `{"code":0,"message":"ok","request_id":"r1","data":{"item":[{"thscode":"000001.SZ","name":"演示公司A","exchange":"SZSE"}],"source_updated_at":"2026-09-26T00:00:00Z"}}`), nil
	})}
	out, err := client.SearchTickers(context.Background(), "演示", 20)
	if err != nil {
		t.Fatal(err)
	}
	if out.RequestID != "r1" || len(out.Items) != 1 || out.Items[0].Code != "000001.SZ" {
		t.Fatalf("out=%#v", out)
	}
}

func TestCNInfoPaginatesAndAllowsEmptyResult(t *testing.T) {
	client := &CNInfoClient{Client: NewProviderClient(ProviderConfig{Name: "cninfo", Enabled: true}, "http://www.cninfo.com.cn/new/hisAnnouncement/query")}
	page := 0
	client.Client.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		page++
		if page == 1 {
			return jsonHTTPResponse(req, 200, `{"totalAnnouncement":1,"announcements":[{"secCode":"000001","secName":"演示公司A","orgId":"gssz0000001","announcementId":"a1","announcementTitle":"公告一","announcementTime":1783296000000,"adjunctUrl":"/finalpage/2026-09-26/a1.PDF","adjunctType":"PDF"}]}`), nil
		}
		return jsonHTTPResponse(req, 200, `{"totalAnnouncement":1,"announcements":[]}`), nil
	})}
	out, err := client.SearchAnnouncements(context.Background(), CNInfoQuery{Symbol: "000001", OrgID: "gssz0000001", Column: "szse", Plate: "sz", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].ProviderDocID != "a1" || out[0].Precision != "day" || !strings.Contains(out[0].URL, "static.cninfo.com.cn") {
		t.Fatalf("out=%#v", out)
	}
}

func TestIFindMCPFreezesToolsAndSchemaWithoutGuessing(t *testing.T) {
	t.Setenv("IFIND_MCP_URL", "https://api-mcp.51ifind.com:8643/ds-mcp-servers/hexin-ifind-ds-news-mcp")
	t.Setenv("IFIND_AUTHORIZATION", "account-exported-auth")
	client := NewIFindMCPClient()
	var methods []string
	client.Client.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(req.Body)
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		method, _ := payload["method"].(string)
		methods = append(methods, method)
		switch method {
		case "initialize":
			return jsonHTTPResponse(req, 200, `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26"}}`), nil
		case "notifications/initialized":
			return jsonHTTPResponse(req, 202, `{}`), nil
		case "tools/list":
			return jsonHTTPResponse(req, 200, `{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"actual_news_search","description":"actual tool","inputSchema":{"type":"object"}}]}}`), nil
		default:
			t.Fatalf("unexpected method %s", method)
			return nil, nil
		}
	})}
	capability, err := client.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(capability.Tools) != 1 || capability.Tools[0].Name != "actual_news_search" || capability.Tools[0].SchemaHash == "" {
		t.Fatalf("capability=%#v", capability)
	}
	if strings.Join(methods, ",") != "initialize,notifications/initialized,tools/list" {
		t.Fatalf("methods=%v", methods)
	}
}

func TestProviderClientRejectsSameOriginDifferentPath(t *testing.T) {
	config := ProviderConfig{Name: "test", BaseURL: "https://example.com/allowed", Enabled: true}
	client := NewProviderClient(config, "https://example.com/allowed")
	client.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatal("request must be rejected before transport")
		return nil, nil
	})}
	_, _, err := client.Do(context.Background(), ProviderRequest{URL: "https://example.com/other"})
	if err == nil || ErrorCode(err) != "INVALID_PARAM" {
		t.Fatalf("err=%v", err)
	}
}
