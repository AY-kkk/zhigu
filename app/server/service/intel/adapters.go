package intel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type NormalizedSource struct {
	Provider        string     `json:"provider"`
	ProviderDocID   string     `json:"provider_doc_id"`
	URL             string     `json:"url"`
	Publisher       string     `json:"publisher"`
	Title           string     `json:"title"`
	SourceType      string     `json:"source_type"`
	TextOrExcerpt   string     `json:"text_or_excerpt"`
	DisclosedAt     *time.Time `json:"disclosed_at"`
	SourceUpdatedAt *time.Time `json:"source_updated_at"`
	Precision       string     `json:"precision"`
	Rights          string     `json:"rights"`
	RequestID       string     `json:"request_id"`
}

type Ticker struct {
	Code            string     `json:"code"`
	Name            string     `json:"name"`
	Exchange        string     `json:"exchange"`
	SourceUpdatedAt *time.Time `json:"source_updated_at,omitempty"`
}

type TickerSearchResult struct {
	RequestID string   `json:"request_id"`
	Items     []Ticker `json:"items"`
}

type FuyaoClient struct{ Client *ProviderClient }

func NewFuyaoClient() *FuyaoClient {
	config := providerConfigs()["fuyao"]
	endpoint := "https://fuyao.aicubes.cn/api/meta/tickers/search"
	config.BaseURL = endpoint
	return &FuyaoClient{Client: NewProviderClient(config, endpoint)}
}

func (f *FuyaoClient) SearchTickers(ctx context.Context, query string, limit int) (TickerSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" || len([]rune(query)) > 50 {
		return TickerSearchResult{}, newError(400, "INVALID_PARAM", "q 必须为 1–50 字")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 20 {
		return TickerSearchResult{}, newError(400, "INVALID_PARAM", "limit 最大 20")
	}
	endpoint := fmt.Sprintf("https://fuyao.aicubes.cn/api/meta/tickers/search?q=%s&asset_type=a-share&limit=%d", url.QueryEscape(query), limit)
	body, headers, err := f.Client.Do(ctx, ProviderRequest{Method: "GET", URL: endpoint})
	if err != nil {
		return TickerSearchResult{}, err
	}
	var payload struct {
		Code      int    `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
		Data      struct {
			Item          []json.RawMessage `json:"item"`
			SourceUpdated *time.Time        `json:"source_updated_at"`
			UpdatedAt     *time.Time        `json:"updated_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return TickerSearchResult{}, newError(503, "DATA_UNAVAILABLE", "扶摇响应无法解析")
	}
	if payload.Code != 0 {
		return TickerSearchResult{}, newError(503, "DATA_UNAVAILABLE", "扶摇业务响应失败")
	}
	requestID := payload.RequestID
	if requestID == "" {
		requestID = headers.Get("X-Request-Id")
	}
	items := make([]Ticker, 0, len(payload.Data.Item))
	for _, raw := range payload.Data.Item {
		var row struct {
			THSCode  string `json:"thscode"`
			Code     string `json:"code"`
			Name     string `json:"name"`
			Exchange string `json:"exchange"`
		}
		if err := json.Unmarshal(raw, &row); err != nil {
			continue
		}
		code := strings.TrimSpace(row.THSCode)
		if code == "" {
			code = strings.TrimSpace(row.Code)
		}
		if code == "" || strings.TrimSpace(row.Name) == "" {
			continue
		}
		items = append(items, Ticker{Code: code, Name: row.Name, Exchange: row.Exchange, SourceUpdatedAt: payload.Data.SourceUpdated})
	}
	return TickerSearchResult{RequestID: requestID, Items: items}, nil
}

type CNInfoQuery struct {
	Symbol    string
	OrgID     string
	Column    string
	Plate     string
	SearchKey string
	StartDate string
	EndDate   string
	PageNum   int
	PageSize  int
	Limit     int
}

type CNInfoClient struct{ Client *ProviderClient }

func NewCNInfoClient() *CNInfoClient {
	config := providerConfigs()["cninfo"]
	endpoint := "http://www.cninfo.com.cn/new/hisAnnouncement/query"
	config.BaseURL = endpoint
	return &CNInfoClient{Client: NewProviderClient(config, endpoint)}
}

func (c *CNInfoClient) SearchAnnouncements(ctx context.Context, query CNInfoQuery) ([]NormalizedSource, error) {
	if strings.TrimSpace(query.Symbol) == "" || strings.TrimSpace(query.OrgID) == "" {
		return nil, newError(400, "INVALID_PARAM", "symbol/orgId 不能为空")
	}
	if query.PageNum <= 0 {
		query.PageNum = 1
	}
	if query.PageSize <= 0 || query.PageSize > 100 {
		query.PageSize = 100
	}
	if query.Limit <= 0 {
		query.Limit = query.PageSize
	}
	if query.Limit > 1000 {
		query.Limit = 1000
	}
	seDate := ""
	if query.StartDate != "" || query.EndDate != "" {
		if query.StartDate == "" || query.EndDate == "" {
			return nil, newError(400, "INVALID_PARAM", "起止日期必须同时提供")
		}
		seDate = query.StartDate + "~" + query.EndDate
	}
	out := make([]NormalizedSource, 0, query.Limit)
	for len(out) < query.Limit {
		form := url.Values{}
		form.Set("pageNum", strconv.Itoa(query.PageNum))
		form.Set("pageSize", strconv.Itoa(query.PageSize))
		form.Set("column", query.Column)
		form.Set("plate", query.Plate)
		form.Set("tabName", "fulltext")
		form.Set("stock", query.Symbol+","+query.OrgID)
		form.Set("searchkey", query.SearchKey)
		form.Set("seDate", seDate)
		raw, headers, err := c.Client.Do(ctx, ProviderRequest{
			Method: "POST",
			URL:    "http://www.cninfo.com.cn/new/hisAnnouncement/query",
			Headers: map[string]string{
				"Content-Type":     "application/x-www-form-urlencoded",
				"Referer":          "http://www.cninfo.com.cn/new/disclosure/stock",
				"Origin":           "http://www.cninfo.com.cn",
				"X-Requested-With": "XMLHttpRequest",
			},
			Body: []byte(form.Encode()),
		})
		if err != nil {
			return nil, err
		}
		var payload struct {
			Total         int `json:"totalAnnouncement"`
			Announcements []struct {
				SecCode           string `json:"secCode"`
				SecName           string `json:"secName"`
				OrgID             string `json:"orgId"`
				AnnouncementID    string `json:"announcementId"`
				AnnouncementTitle string `json:"announcementTitle"`
				AnnouncementTime  int64  `json:"announcementTime"`
				AdjunctURL        string `json:"adjunctUrl"`
				AdjunctType       string `json:"adjunctType"`
			} `json:"announcements"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, newError(503, "DATA_UNAVAILABLE", "巨潮响应无法解析")
		}
		if len(payload.Announcements) == 0 {
			break
		}
		requestID := headers.Get("X-Request-Id")
		for _, row := range payload.Announcements {
			documentURL := row.AdjunctURL
			if documentURL != "" && strings.HasPrefix(documentURL, "/") {
				documentURL = "http://static.cninfo.com.cn/" + strings.TrimPrefix(documentURL, "/")
			}
			var disclosed *time.Time
			precision := "unknown"
			if row.AnnouncementTime > 0 {
				t := time.UnixMilli(row.AnnouncementTime).In(time.FixedZone("CST", 8*3600))
				day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
				disclosed = &day
				precision = "day"
			}
			out = append(out, NormalizedSource{
				Provider: "cninfo", ProviderDocID: row.AnnouncementID, URL: documentURL,
				Publisher: row.SecName, Title: strings.TrimSpace(row.AnnouncementTitle),
				SourceType: "announcement", TextOrExcerpt: "", DisclosedAt: disclosed,
				Precision: precision, Rights: "link-and-summary", RequestID: requestID,
			})
			if len(out) >= query.Limit {
				break
			}
		}
		query.PageNum++
		if (query.PageNum-1)*query.PageSize >= payload.Total {
			break
		}
	}
	return out, nil
}

type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
	SchemaHash  string          `json:"schema_hash"`
}

type MCPCapability struct {
	Provider     string    `json:"provider"`
	Endpoint     string    `json:"endpoint"`
	Protocol     string    `json:"protocol"`
	Transport    string    `json:"transport"`
	Tools        []MCPTool `json:"tools"`
	AllowedTypes []string  `json:"allowed_types"`
	VerifiedAt   time.Time `json:"verified_at"`
	RequestID    string    `json:"request_id"`
}

type IFindMCPClient struct {
	Client   *ProviderClient
	Endpoint string
	nextID   int
}

func NewIFindMCPClient() *IFindMCPClient {
	config := providerConfigs()["ifind"]
	endpoint := strings.TrimSpace(os.Getenv("IFIND_MCP_URL"))
	config.BaseURL = endpoint
	return &IFindMCPClient{Client: NewProviderClient(config, endpoint), Endpoint: endpoint}
}

func (m *IFindMCPClient) rpc(ctx context.Context, method string, params any, notification bool) (json.RawMessage, string, error) {
	payload := map[string]any{"jsonrpc": "2.0", "method": method}
	id := 0
	if !notification {
		m.nextID++
		id = m.nextID
		payload["id"] = id
	}
	if params != nil {
		payload["params"] = params
	}
	rawBody, _ := json.Marshal(payload)
	body, headers, err := m.Client.Do(ctx, ProviderRequest{
		Method: m.EndpointMethod(), URL: m.Endpoint,
		Headers: map[string]string{"Content-Type": "application/json", "Accept": "application/json, text/event-stream"},
		Body:    rawBody,
	})
	if err != nil {
		return nil, "", err
	}
	requestID := headers.Get("X-Request-Id")
	if notification {
		return nil, requestID, nil
	}
	result, err := decodeMCPResponse(body, id)
	return result, requestID, err
}

func (m *IFindMCPClient) EndpointMethod() string { return "POST" }

func (m *IFindMCPClient) Discover(ctx context.Context) (MCPCapability, error) {
	if _, requestID, err := m.rpc(ctx, "initialize", map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "zhigu-intel", "version": "1.0"},
	}, false); err != nil {
		return MCPCapability{}, err
	} else {
		_ = requestID
	}
	if _, _, err := m.rpc(ctx, "notifications/initialized", nil, true); err != nil {
		return MCPCapability{}, err
	}
	rawTools, requestID, err := m.rpc(ctx, "tools/list", nil, false)
	if err != nil {
		return MCPCapability{}, err
	}
	var payload struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(rawTools, &payload); err != nil {
		return MCPCapability{}, newError(503, "DATA_UNAVAILABLE", "MCP tools/list 无法解析")
	}
	tools := make([]MCPTool, 0, len(payload.Tools))
	for _, tool := range payload.Tools {
		tools = append(tools, MCPTool{Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema, SchemaHash: hashJSON(tool.InputSchema)})
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return MCPCapability{
		Provider: "ifind", Endpoint: m.Endpoint, Protocol: "mcp-jsonrpc-2.0", Transport: "streamable-http",
		Tools: tools, AllowedTypes: []string{}, VerifiedAt: time.Now().UTC(), RequestID: requestID,
	}, nil
}

func (m *IFindMCPClient) CallTool(ctx context.Context, name string, arguments any) (json.RawMessage, string, error) {
	if strings.TrimSpace(name) == "" {
		return nil, "", newError(400, "INVALID_PARAM", "工具名不能为空")
	}
	raw, requestID, err := m.rpc(ctx, "tools/call", map[string]any{"name": name, "arguments": arguments}, false)
	return raw, requestID, err
}

func decodeMCPResponse(body []byte, id int) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "event:") || strings.HasPrefix(trimmed, "data:") {
		for _, line := range strings.Split(trimmed, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			var envelope struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if json.Unmarshal([]byte(data), &envelope) == nil && envelope.ID == id {
				if len(envelope.Error) > 0 {
					return nil, newError(503, "DATA_UNAVAILABLE", "MCP 返回错误")
				}
				return envelope.Result, nil
			}
		}
		return nil, newError(503, "DATA_UNAVAILABLE", "SSE 响应缺少匹配 request id")
	}
	var envelope struct {
		ID     int             `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, newError(503, "DATA_UNAVAILABLE", "MCP 响应无法解析")
	}
	if envelope.ID != id {
		return nil, newError(503, "DATA_UNAVAILABLE", "MCP response id 不匹配")
	}
	if len(envelope.Error) > 0 {
		return nil, newError(503, "DATA_UNAVAILABLE", "MCP 返回错误")
	}
	return envelope.Result, nil
}

func hashJSON(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
