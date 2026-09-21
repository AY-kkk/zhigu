package finance

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type ChatCompletionsAdapter struct {
	Client *http.Client
}

type ResponsesAdapter struct {
	Client *http.Client
}

func joinURL(base, suffix string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	suffix = "/" + strings.TrimLeft(suffix, "/")
	return base + suffix
}

func injectScopePrefix(body []byte) []byte {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return body
	}
	inject := func(msgs []any) []any {
		prefix := ResearchScopePrefix
		if len(msgs) > 0 {
			if m, ok := msgs[0].(map[string]any); ok {
				role, _ := m["role"].(string)
				content, _ := m["content"].(string)
				if role == "system" && strings.HasPrefix(content, strings.TrimSpace(strings.Split(ResearchScopePrefix, "\n")[0])) {
					return msgs
				}
				if role == "system" {
					m["content"] = prefix + content
					msgs[0] = m
					return msgs
				}
			}
		}
		sys := map[string]any{"role": "system", "content": prefix}
		return append([]any{sys}, msgs...)
	}
	if msgs, ok := payload["messages"].([]any); ok {
		payload["messages"] = inject(msgs)
	}
	if input, ok := payload["input"].([]any); ok {
		payload["input"] = inject(input)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return raw
}

func stripVendorTools(body []byte) []byte {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return body
	}
	payload["stream"] = false
	if _, ok := payload["model"]; !ok || payload["model"] == "" {
		payload["model"] = ModelAlias()
	}
	raw, _ := json.Marshal(payload)
	return raw
}

func (a *ChatCompletionsAdapter) Forward(baseURL string, apiKey string, body []byte) ([]byte, int, error) {
	body = injectScopePrefix(stripVendorTools(body))
	req, err := http.NewRequest(http.MethodPost, joinURL(baseURL, "/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return nil, 0, NewError(400, "validation", "INVALID_URL", "无效上游地址")
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	cli := a.Client
	if cli == nil {
		cli = http.DefaultClient
	}
	res, err := cli.Do(req)
	if err != nil {
		return nil, 0, NewError(503, "unavailable", "UPSTREAM_UNAVAILABLE", "上游模型不可用")
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return raw, res.StatusCode, nil
}

func (a *ResponsesAdapter) Forward(baseURL string, apiKey string, body []byte) ([]byte, int, error) {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return nil, 0, NewError(400, "validation", "INVALID_JSON", "无法解析 Responses 请求")
	}
	payload["stream"] = false
	payload["background"] = false
	payload["store"] = false
	delete(payload, "previous_response_id")
	delete(payload, "conversation")
	if tools, ok := payload["tools"].([]any); ok {
		filtered := make([]any, 0, len(tools))
		for _, t := range tools {
			m, _ := t.(map[string]any)
			typ, _ := m["type"].(string)
			name, _ := m["name"].(string)
			if typ == "web_search" || typ == "file_search" || typ == "mcp" || typ == "computer" || strings.Contains(strings.ToLower(name), "mcp") {
				return nil, 0, NewError(400, "validation", "TOOL_FORBIDDEN", "禁止内置检索或 MCP 工具")
			}
			if typ == "" || typ == "function" {
				filtered = append(filtered, t)
			}
		}
		payload["tools"] = filtered
	}
	if _, ok := payload["model"]; !ok || payload["model"] == "" {
		payload["model"] = ModelAlias()
	}
	raw, _ := json.Marshal(payload)
	raw = injectScopePrefix(raw)
	req, err := http.NewRequest(http.MethodPost, joinURL(baseURL, "/responses"), bytes.NewReader(raw))
	if err != nil {
		return nil, 0, NewError(400, "validation", "INVALID_URL", "无效上游地址")
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	cli := a.Client
	if cli == nil {
		cli = http.DefaultClient
	}
	res, err := cli.Do(req)
	if err != nil {
		return nil, 0, NewError(503, "unavailable", "UPSTREAM_UNAVAILABLE", "上游模型不可用")
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return out, res.StatusCode, nil
}
