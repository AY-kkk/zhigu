package intel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"zhigu/server/service/finance"
)

const (
	intelPromptVersion    = "intel-extraction-v1"
	intelExtractionName   = "intel_extraction_v1"
	intelInputTokenLimit  = 16000
	intelOutputTokenLimit = 4000
	maxModelResponseBytes = 2 << 20
)

type FrozenModelConfig struct {
	ConfigID      string
	ConfigDigest  string
	Protocol      string
	Model         string
	BaseURL       string
	APIKey        string
	PromptVersion string
}

type ModelCall func(ctx context.Context, cfg FrozenModelConfig, body []byte) ([]byte, error)

type ModelExtractorV2 struct {
	Frozen  FrozenModelConfig
	Client  *http.Client
	Call    ModelCall
	Enabled bool
}

func FreezeActiveModel(ctx context.Context, configs *finance.ConfigService) (FrozenModelConfig, error) {
	if configs == nil {
		return FrozenModelConfig{}, newError(503, "MODEL_UNAVAILABLE", "模型配置服务不可用")
	}
	row, err := configs.Active(ctx, "model")
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return FrozenModelConfig{}, newError(503, "MODEL_UNAVAILABLE", "没有启用的模型配置")
		}
		return FrozenModelConfig{}, err
	}
	var public map[string]any
	if err := json.Unmarshal(row.PublicConfig, &public); err != nil {
		return FrozenModelConfig{}, newError(503, "MODEL_UNAVAILABLE", "模型配置无法解析")
	}
	baseURL, _ := public["base_url"].(string)
	protocol, _ := public["protocol"].(string)
	model, _ := public["model"].(string)
	if protocol == "" {
		protocol = "openai_chat_completions"
	}
	if protocol != "openai_chat_completions" && protocol != "openai_responses" {
		return FrozenModelConfig{}, newError(409, "MODEL_PROTOCOL_MISMATCH", "冻结协议不受支持")
	}
	if model == "" {
		return FrozenModelConfig{}, newError(503, "MODEL_UNAVAILABLE", "模型配置缺少 model")
	}
	if err := finance.ValidateUpstreamURL(baseURL); err != nil {
		return FrozenModelConfig{}, err
	}
	apiKey := ""
	if len(row.SecretCiphertext) > 0 {
		apiKey, err = finance.DecryptSecret(row.SecretCiphertext)
		if err != nil {
			return FrozenModelConfig{}, newError(503, "MODEL_UNAVAILABLE", "模型密钥不可解密")
		}
	}
	return FrozenModelConfig{
		ConfigID: row.ID, ConfigDigest: row.ConfigDigest, Protocol: protocol,
		Model: model, BaseURL: strings.TrimRight(baseURL, "/"), APIKey: apiKey, PromptVersion: intelPromptVersion,
	}, nil
}

func NewModelExtractorV2(frozen FrozenModelConfig) *ModelExtractorV2 {
	enabled := frozen.ConfigID != "" && frozen.ConfigDigest != "" && frozen.Model != "" && frozen.BaseURL != "" &&
		(frozen.Protocol == "openai_chat_completions" || frozen.Protocol == "openai_responses")
	return &ModelExtractorV2{
		Frozen: frozen, Enabled: enabled,
		Client: &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return newError(400, "INVALID_PARAM", "模型请求禁止重定向")
		}},
	}
}

func extractionSchema() map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"schema_version", "source_revision_id", "entities", "claims", "candidate_events"},
		"properties": map[string]any{
			"schema_version":     map[string]any{"const": "intel.extraction.v1"},
			"source_revision_id": map[string]any{"type": "string", "minLength": 1},
			"entities":           map[string]any{"type": "array"},
			"claims":             map[string]any{"type": "array"},
			"candidate_events":   map[string]any{"type": "array"},
		},
	}
}

func BuildModelRequest(protocol, model string, req ExtractionRequest) ([]byte, error) {
	if protocol != "openai_chat_completions" && protocol != "openai_responses" {
		return nil, newError(409, "MODEL_PROTOCOL_MISMATCH", "协议不受支持")
	}
	estimated := (utf8.RuneCountInString(req.Text) + 3) / 4
	if estimated > intelInputTokenLimit {
		return nil, newError(422, "REVIEW_REQUIRED", "材料超过16000 token，转人工复核")
	}
	system := "你是投资事件抽取器。只抽取实体、命题、模态、参数、引用和候选事件；不得决定事件归并、状态、支持度或投资结论。所有 quote 必须逐字来自输入并绑定 source_revision_id。只返回符合 schema 的 JSON。"
	raw, err := json.Marshal(map[string]any{"schema_version": "intel.extraction.v1", "source_revision_id": req.SourceRevisionID, "text": req.Text})
	if err != nil {
		return nil, err
	}
	user := string(raw)
	if protocol == "openai_responses" {
		return json.Marshal(map[string]any{
			"model": model, "background": false, "store": false, "stream": false,
			"previous_response_id": nil, "conversation": nil, "tools": []any{},
			"max_output_tokens": intelOutputTokenLimit,
			"text":              map[string]any{"format": map[string]any{"type": "json_schema", "name": intelExtractionName, "strict": true, "schema": extractionSchema()}},
			"input":             []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}},
		})
	}
	return json.Marshal(map[string]any{
		"model": model, "stream": false, "temperature": 0, "max_tokens": intelOutputTokenLimit,
		"tool_choice": "none", "tools": []any{},
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": intelExtractionName, "strict": true, "schema": extractionSchema()}},
		"messages":        []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}},
	})
}

func parseModelResponse(protocol string, raw []byte) (map[string]any, map[string]any, error) {
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, nil, newError(503, "MODEL_UNAVAILABLE", "模型响应不是 JSON")
	}
	var text string
	if protocol == "openai_responses" {
		if value, ok := envelope["output_text"].(string); ok {
			text = value
		}
		if text == "" {
			if output, ok := envelope["output"].([]any); ok {
				var builder strings.Builder
				for _, rawItem := range output {
					item, _ := rawItem.(map[string]any)
					if item["type"] != "message" {
						continue
					}
					content, _ := item["content"].([]any)
					for _, rawPart := range content {
						part, _ := rawPart.(map[string]any)
						if value, ok := part["text"].(string); ok {
							builder.WriteString(value)
						}
					}
				}
				text = builder.String()
			}
		}
	} else {
		choices, _ := envelope["choices"].([]any)
		if len(choices) > 0 {
			choice, _ := choices[0].(map[string]any)
			message, _ := choice["message"].(map[string]any)
			switch content := message["content"].(type) {
			case string:
				text = content
			case []any:
				var builder strings.Builder
				for _, rawPart := range content {
					part, _ := rawPart.(map[string]any)
					if value, ok := part["text"].(string); ok {
						builder.WriteString(value)
					}
				}
				text = builder.String()
			}
		}
	}
	text = strings.TrimSpace(text)
	var output map[string]any
	if err := json.Unmarshal([]byte(text), &output); err != nil {
		return nil, nil, newError(503, "MODEL_UNAVAILABLE", "模型未返回合法抽取 JSON")
	}
	usage := map[string]any{}
	if rawUsage, ok := envelope["usage"].(map[string]any); ok {
		usage = rawUsage
	}
	return output, usage, nil
}

func validateExtractionOutput(req ExtractionRequest, sourceText string, output map[string]any) error {
	if output["schema_version"] != "intel.extraction.v1" {
		return newError(503, "MODEL_UNAVAILABLE", "抽取 schema_version 不合法")
	}
	if output["source_revision_id"] != req.SourceRevisionID {
		return newError(422, "INVALID_CITATION", "source_revision_id 错配")
	}
	for _, key := range []string{"entities", "claims", "candidate_events"} {
		if _, ok := output[key].([]any); !ok {
			return newError(503, "MODEL_UNAVAILABLE", "抽取结果缺少 "+key+" 数组")
		}
	}
	claims, _ := output["claims"].([]any)
	runes := []rune(sourceText)
	for _, rawClaim := range claims {
		claim, _ := rawClaim.(map[string]any)
		if claim == nil {
			return newError(503, "MODEL_UNAVAILABLE", "claim 不是对象")
		}
		for _, key := range []string{"claim_key", "field", "text", "modality", "grade", "stance", "quote", "params"} {
			if _, ok := claim[key]; !ok {
				return newError(503, "MODEL_UNAVAILABLE", "claim 缺少 "+key)
			}
		}
		quote, _ := claim["quote"].(map[string]any)
		start, startOK := jsonNumberInt(quote["start"])
		end, endOK := jsonNumberInt(quote["end"])
		quoteText, textOK := quote["text"].(string)
		if !startOK || !endOK || !textOK || start < 0 || end < start || end > len(runes) {
			return newError(422, "INVALID_CITATION", "quote 区间不合法")
		}
		if string(runes[start:end]) != quoteText {
			return newError(422, "INVALID_CITATION", "quote 与原文 offset 不一致")
		}
	}
	return nil
}

func (m *ModelExtractorV2) Extract(ctx context.Context, req ExtractionRequest) (ExtractionResult, error) {
	if !m.Enabled {
		return ExtractionResult{}, newError(503, "MODEL_UNAVAILABLE", "模型配置未启用")
	}
	if req.SourceRevisionID == "" || strings.TrimSpace(req.Text) == "" {
		return ExtractionResult{}, newError(400, "INVALID_PARAM", "source_revision_id/text 不能为空")
	}
	body, err := BuildModelRequest(m.Frozen.Protocol, m.Frozen.Model, req)
	if err != nil {
		return ExtractionResult{}, err
	}
	var raw []byte
	if m.Call != nil {
		raw, err = m.Call(ctx, m.Frozen, body)
	} else {
		raw, err = m.doHTTP(ctx, body)
	}
	if err != nil {
		return ExtractionResult{}, err
	}
	output, usage, err := parseModelResponse(m.Frozen.Protocol, raw)
	if err != nil {
		return ExtractionResult{}, err
	}
	if err := validateExtractionOutput(req, req.Text, output); err != nil {
		return ExtractionResult{}, err
	}
	encoded, _ := json.Marshal(output)
	if (utf8.RuneCountInString(string(encoded))+3)/4 > intelOutputTokenLimit {
		return ExtractionResult{}, newError(422, "REVIEW_REQUIRED", "抽取结果超过4000 token，转人工复核")
	}
	return ExtractionResult{
		SchemaVersion: "intel.extraction.v1", ConfigID: m.Frozen.ConfigID,
		ConfigDigest: m.Frozen.ConfigDigest, PromptVersion: m.Frozen.PromptVersion,
		Output: output, Usage: usage,
	}, nil
}

func (m *ModelExtractorV2) doHTTP(ctx context.Context, body []byte) ([]byte, error) {
	path := "/chat/completions"
	if m.Frozen.Protocol == "openai_responses" {
		path = "/responses"
	}
	base := strings.TrimRight(m.Frozen.BaseURL, "/")
	if err := finance.ValidateUpstreamURL(base); err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	if m.Frozen.APIKey != "" {
		request.Header.Set("Authorization", "Bearer "+m.Frozen.APIKey)
	}
	client := m.Client
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, newError(503, "MODEL_UNAVAILABLE", "模型上游不可用")
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxModelResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxModelResponseBytes {
		return nil, newError(503, "MODEL_UNAVAILABLE", "模型响应超过2MiB")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, newError(503, "MODEL_UNAVAILABLE", fmt.Sprintf("模型 HTTP %d", response.StatusCode))
	}
	return raw, nil
}

func envModelConfig() FrozenModelConfig {
	protocol := strings.TrimSpace(os.Getenv("ZHIGU_INTEL_MODEL_PROTOCOL"))
	if protocol == "" {
		protocol = "openai_chat_completions"
	}
	return FrozenModelConfig{
		ConfigID:      strings.TrimSpace(os.Getenv("ZHIGU_INTEL_MODEL_CONFIG_ID")),
		ConfigDigest:  strings.TrimSpace(os.Getenv("ZHIGU_INTEL_MODEL_CONFIG_DIGEST")),
		Protocol:      protocol,
		Model:         strings.TrimSpace(os.Getenv("ZHIGU_INTEL_MODEL")),
		BaseURL:       strings.TrimRight(strings.TrimSpace(os.Getenv("ZHIGU_INTEL_MODEL_BASE_URL")), "/"),
		APIKey:        os.Getenv("ZHIGU_INTEL_MODEL_API_KEY"),
		PromptVersion: intelPromptVersion,
	}
}

func jsonNumberInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}
