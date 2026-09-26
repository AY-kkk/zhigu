package intel

import (
	"context"
	"errors"
	"os"
)

type ExtractionRequest struct {
	SourceRevisionID string
	Text             string
	SchemaVersion    string
}

type ExtractionResult struct {
	SchemaVersion string
	ConfigID      string
	ConfigDigest  string
	PromptVersion string
	Output        map[string]any
	Usage         map[string]any
}

type ModelExtractor struct {
	ConfigID      string
	ConfigDigest  string
	Protocol      string
	Model         string
	PromptVersion string
	Enabled       bool
}

func NewModelExtractor() *ModelExtractor {
	configID := os.Getenv("ZHIGU_INTEL_MODEL_CONFIG_ID")
	digest := os.Getenv("ZHIGU_INTEL_MODEL_CONFIG_DIGEST")
	protocol := os.Getenv("ZHIGU_INTEL_MODEL_PROTOCOL")
	model := os.Getenv("ZHIGU_INTEL_MODEL")
	enabled := configID != "" && digest != "" && (protocol == "openai_chat_completions" || protocol == "openai_responses") && model != ""
	return &ModelExtractor{ConfigID: configID, ConfigDigest: digest, Protocol: protocol, Model: model, PromptVersion: "intel-extraction-v1", Enabled: enabled}
}

// Extract is deliberately fail-closed. Fixture extraction is manual/golden data;
// a live extractor must freeze config and model before any call and must never
// fabricate output when credentials or config are absent.
func (m *ModelExtractor) Extract(ctx context.Context, req ExtractionRequest) (ExtractionResult, error) {
	if !m.Enabled {
		return ExtractionResult{}, newError(503, "MODEL_UNAVAILABLE", "模型配置未启用")
	}
	return ExtractionResult{}, errors.New("live model extraction requires provider-specific authenticated adapter")
}
