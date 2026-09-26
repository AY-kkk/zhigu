package intel

import (
	"context"
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

// ModelExtractor remains the fail-closed environment-configured wrapper used by
// fixtures and deployment checks. Production jobs should freeze ConfigService.Active
// with FreezeActiveModel and use ModelExtractorV2.
type ModelExtractor struct {
	ConfigID      string
	ConfigDigest  string
	Protocol      string
	Model         string
	PromptVersion string
	Enabled       bool
	v2            *ModelExtractorV2
}

func NewModelExtractor() *ModelExtractor {
	frozen := envModelConfig()
	v2 := NewModelExtractorV2(frozen)
	return &ModelExtractor{
		ConfigID: frozen.ConfigID, ConfigDigest: frozen.ConfigDigest,
		Protocol: frozen.Protocol, Model: frozen.Model, PromptVersion: frozen.PromptVersion,
		Enabled: v2.Enabled, v2: v2,
	}
}

func (m *ModelExtractor) Extract(ctx context.Context, req ExtractionRequest) (ExtractionResult, error) {
	if m.v2 == nil {
		return ExtractionResult{}, newError(503, "MODEL_UNAVAILABLE", "模型配置未启用")
	}
	return m.v2.Extract(ctx, req)
}
