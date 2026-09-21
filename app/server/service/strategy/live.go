package strategy

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"zhigu/server/service/finance"
)

//go:embed prompt.md
var livePrompt string

type ModelCall func(ctx context.Context, protocol, baseURL, apiKey, model string, body []byte) ([]byte, error)

type LiveInput struct {
	Text         string
	InstrumentID string
	Protocol     string
	BaseURL      string
	APIKey       string
	Model        string
	ConfigID     string
	Call         ModelCall
}

func precheckGenerate(in GenerateInput) *GenerateResult {
	text := strings.TrimSpace(in.Text)
	if text == "" {
		r := failGen("STRATEGY_INVALID", "请输入策略描述")
		return &r
	}
	if utf8.RuneCountInString(text) > MaxInputRunes {
		r := failGen("STRATEGY_INVALID", "描述超过 4000 字")
		return &r
	}
	if forbidden(text) {
		return &GenerateResult{
			Status: "unsupported", ErrorCode: "STRATEGY_UNSUPPORTED",
			ErrorMessage:  "不能使用未来信息、外部请求或任意代码作为交易条件",
			PromptVersion: PromptVersion, Source: "heuristic",
		}
	}
	if strings.TrimSpace(in.InstrumentID) == "" {
		return &GenerateResult{
			Status:        "needs_clarification",
			Questions:     []string{"请先选择要交易的股票柜台。"},
			PromptVersion: PromptVersion, Source: "heuristic",
		}
	}
	return nil
}

func GenerateLive(ctx context.Context, in LiveInput) GenerateResult {
	if r := precheckGenerate(GenerateInput{Text: in.Text, InstrumentID: in.InstrumentID}); r != nil {
		return *r
	}
	if strings.TrimSpace(in.APIKey) == "" || strings.TrimSpace(in.BaseURL) == "" {
		return GenerateResult{Status: "failed", ErrorCode: "MODEL_UNAVAILABLE", ErrorMessage: "没有可用的策略模型配置，未做付费 live", PromptVersion: PromptVersion, Source: "none"}
	}
	proto, err := finance.NormalizeProtocol(in.Protocol)
	if err != nil {
		return GenerateResult{Status: "failed", ErrorCode: "MODEL_UNAVAILABLE", ErrorMessage: err.Error(), PromptVersion: PromptVersion, Source: "none"}
	}
	call := in.Call
	if call == nil {
		call = defaultModelCall
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
	}
	user := "instrument_id=" + in.InstrumentID + "\n用户描述：\n" + in.Text
	body := chatBody(proto, in.Model, livePrompt, user)
	raw, err := call(ctx, proto, in.BaseURL, in.APIKey, in.Model, body)
	if err != nil {
		return GenerateResult{Status: "failed", ErrorCode: "MODEL_UNAVAILABLE", ErrorMessage: err.Error(), PromptVersion: PromptVersion, Source: proto}
	}
	out := parseLive(raw, in.InstrumentID)
	out.Source = proto
	out.PromptVersion = PromptVersion
	if out.Status == "ready" && out.Document != nil {
		if _, err := Compile(*out.Document); err == nil {
			return out
		}
	}
	if ctx.Err() != nil {
		return failLive(proto, "STRATEGY_INVALID", "模型输出无法通过编译")
	}
	fix := chatBody(proto, in.Model, livePrompt, user+"\n上次输出无法通过 schema/编译，请只输出纠正后的 JSON。上次：\n"+extractJSON(raw))
	raw2, err := call(ctx, proto, in.BaseURL, in.APIKey, in.Model, fix)
	if err != nil {
		return GenerateResult{Status: "failed", ErrorCode: "MODEL_UNAVAILABLE", ErrorMessage: err.Error(), PromptVersion: PromptVersion, Source: proto}
	}
	out = parseLive(raw2, in.InstrumentID)
	out.Source = proto
	out.PromptVersion = PromptVersion
	if out.Status == "ready" && out.Document != nil {
		if _, err := Compile(*out.Document); err != nil {
			ae := finance.AsAppError(err)
			return failLive(proto, ae.Code, ae.Message)
		}
	}
	return out
}

func failLive(src, code, msg string) GenerateResult {
	return GenerateResult{Status: "failed", ErrorCode: code, ErrorMessage: msg, PromptVersion: PromptVersion, Source: src}
}

func parseLive(raw []byte, instrumentID string) GenerateResult {
	js := extractJSON(raw)
	if js == "" {
		return failLive("model", "STRATEGY_INVALID", "模型未返回 JSON")
	}
	var env struct {
		Status       string          `json:"status"`
		Document     json.RawMessage `json:"document"`
		Assumptions  []string        `json:"assumptions"`
		Questions    []string        `json:"questions"`
		ErrorCode    string          `json:"error_code"`
		ErrorMessage string          `json:"error_message"`
	}
	if err := json.Unmarshal([]byte(js), &env); err != nil || (env.Status == "" && len(env.Document) == 0) {
		doc, err := ParseDSL([]byte(js))
		if err != nil {
			return failLive("model", "STRATEGY_INVALID", "无法解析策略 JSON")
		}
		if doc.InstrumentID == "" {
			doc.InstrumentID = instrumentID
		}
		return GenerateResult{Status: "ready", Document: &doc, PromptVersion: PromptVersion, Source: "model"}
	}
	status := env.Status
	if status == "" {
		status = "ready"
	}
	if status == "needs_clarification" || status == "unsupported" || status == "failed" {
		return GenerateResult{Status: status, Assumptions: env.Assumptions, Questions: env.Questions, ErrorCode: env.ErrorCode, ErrorMessage: env.ErrorMessage, PromptVersion: PromptVersion, Source: "model"}
	}
	if len(env.Document) == 0 || string(env.Document) == "null" {
		doc, err := ParseDSL([]byte(js))
		if err != nil {
			return failLive("model", "STRATEGY_INVALID", "缺少 document")
		}
		if doc.InstrumentID == "" {
			doc.InstrumentID = instrumentID
		}
		return GenerateResult{Status: "ready", Document: &doc, PromptVersion: PromptVersion, Source: "model"}
	}
	doc, err := ParseDSL(env.Document)
	if err != nil {
		return failLive("model", "STRATEGY_INVALID", "document 无法通过 schema")
	}
	if doc.InstrumentID == "" {
		doc.InstrumentID = instrumentID
	}
	if instrumentID != "" && doc.InstrumentID != instrumentID {
		return GenerateResult{Status: "failed", ErrorCode: "STRATEGY_INVALID", ErrorMessage: "不能替换用户已选定的标的", PromptVersion: PromptVersion, Source: "model"}
	}
	return GenerateResult{Status: "ready", Document: &doc, Assumptions: env.Assumptions, PromptVersion: PromptVersion, Source: "model"}
}

func extractJSON(raw []byte) string {
	s := string(raw)
	var payload map[string]any
	if json.Unmarshal(raw, &payload) == nil {
		if _, ok := payload["document"]; ok {
			return string(raw)
		}
		if _, ok := payload["schema_version"]; ok {
			return string(raw)
		}
		if c, ok := payload["choices"].([]any); ok && len(c) > 0 {
			if m, ok := c[0].(map[string]any); ok {
				if msg, ok := m["message"].(map[string]any); ok {
					if t, ok := msg["content"].(string); ok {
						s = t
					}
				}
			}
		}
		if out, ok := payload["output"].([]any); ok {
			for _, item := range out {
				m, _ := item.(map[string]any)
				if content, ok := m["content"].([]any); ok {
					for _, part := range content {
						pm, _ := part.(map[string]any)
						if t, ok := pm["text"].(string); ok {
							s = t
						}
					}
				}
			}
		}
	}
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			return s[i : j+1]
		}
	}
	return ""
}

func chatBody(protocol, model, sys, user string) []byte {
	if model == "" {
		model = "finance-research"
	}
	if protocol == finance.ProtocolResponses {
		raw, _ := json.Marshal(map[string]any{
			"model": model,
			"input": []map[string]string{
				{"role": "system", "content": sys},
				{"role": "user", "content": user},
			},
		})
		return raw
	}
	raw, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": sys},
			{"role": "user", "content": user},
		},
	})
	return raw
}

func defaultModelCall(ctx context.Context, protocol, baseURL, apiKey, _ string, body []byte) ([]byte, error) {
	if err := finance.ValidateUpstreamURL(baseURL); err != nil {
		return nil, err
	}
	path := "/chat/completions"
	if protocol == finance.ProtocolResponses {
		path = "/responses"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, finance.NewError(400, "validation", "INVALID_URL", "无效上游地址")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	cli := &http.Client{Timeout: 40 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return finance.NewError(400, "validation", "SSRF_BLOCKED", "禁止重定向")
	}}
	res, err := cli.Do(req)
	if err != nil {
		return nil, finance.NewError(503, "unavailable", "MODEL_UNAVAILABLE", "上游模型不可用")
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if res.StatusCode >= 300 {
		return nil, finance.NewError(503, "unavailable", "MODEL_UNAVAILABLE", "上游 HTTP "+res.Status)
	}
	return raw, nil
}
