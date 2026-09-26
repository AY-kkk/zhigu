package intel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"zhigu/server/service/finance"
	"zhigu/server/testdb"
)

func extractionFixtureOutput(sourceRevision, text string) map[string]any {
	return map[string]any{
		"schema_version":     "intel.extraction.v1",
		"source_revision_id": sourceRevision,
		"entities":           []any{map[string]any{"text": "DEMO.A", "kind": "subject", "code": "DEMO.A"}},
		"claims": []any{map[string]any{
			"claim_key": "__core__", "field": "deal_exists", "text": text,
			"modality": "reported", "grade": "rumor", "stance": "support",
			"quote":  map[string]any{"start": 0, "end": len([]rune(text)), "text": text},
			"params": []any{},
		}},
		"candidate_events": []any{},
	}
}

func TestBuildModelRequestChatAndResponsesAreSchemaLocked(t *testing.T) {
	req := ExtractionRequest{SourceRevisionID: "rev_1", Text: "网传A正在洽谈收购B。"}
	for _, protocol := range []string{"openai_chat_completions", "openai_responses"} {
		raw, err := BuildModelRequest(protocol, "model-x", req)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "model-x" {
			t.Fatalf("protocol %s body=%#v", protocol, body)
		}
		if protocol == "openai_chat_completions" {
			format, _ := body["response_format"].(map[string]any)
			schema, _ := format["json_schema"].(map[string]any)
			if schema["name"] != intelExtractionName || schema["strict"] != true {
				t.Fatalf("chat format=%#v", format)
			}
		} else {
			if body["background"] != false || body["store"] != false || body["stream"] != false {
				t.Fatalf("responses constraints=%#v", body)
			}
			if tools, ok := body["tools"].([]any); !ok || len(tools) != 0 {
				t.Fatalf("responses tools=%#v", body["tools"])
			}
			text, _ := body["text"].(map[string]any)
			format, _ := text["format"].(map[string]any)
			if format["name"] != intelExtractionName || format["strict"] != true {
				t.Fatalf("responses format=%#v", text)
			}
		}
	}
}

func TestParseAndValidateModelResponses(t *testing.T) {
	sourceText := "公司正在推进收购B。"
	output := extractionFixtureOutput("rev_1", sourceText)
	rawOutput, _ := json.Marshal(output)
	chatRaw, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"message": map[string]any{"content": string(rawOutput)}}},
		"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30},
	})
	chatOutput, chatUsage, err := parseModelResponse("openai_chat_completions", chatRaw)
	if err != nil || chatOutput["source_revision_id"] != "rev_1" || chatUsage["total_tokens"] != float64(30) {
		t.Fatalf("chat output=%#v usage=%#v err=%v", chatOutput, chatUsage, err)
	}
	responsesRaw, _ := json.Marshal(map[string]any{
		"output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": string(rawOutput)}}}},
		"usage":  map[string]any{"input_tokens": 10, "output_tokens": 20, "total_tokens": 30},
	})
	responsesOutput, _, err := parseModelResponse("openai_responses", responsesRaw)
	if err != nil || responsesOutput["schema_version"] != "intel.extraction.v1" {
		t.Fatalf("responses output=%#v err=%v", responsesOutput, err)
	}
	req := ExtractionRequest{SourceRevisionID: "rev_1", Text: sourceText}
	if err := validateExtractionOutput(req, sourceText, output); err != nil {
		t.Fatal(err)
	}
	bad := extractionFixtureOutput("rev_other", sourceText)
	if err := validateExtractionOutput(req, sourceText, bad); err == nil || ErrorCode(err) != "INVALID_CITATION" {
		t.Fatalf("bad source err=%v", err)
	}
	badQuote := extractionFixtureOutput("rev_1", sourceText)
	claims := badQuote["claims"].([]any)
	claim := claims[0].(map[string]any)
	claim["quote"].(map[string]any)["end"] = 2
	if err := validateExtractionOutput(req, sourceText, badQuote); err == nil || ErrorCode(err) != "INVALID_CITATION" {
		t.Fatalf("bad quote err=%v", err)
	}
}

func TestModelExtractorRejectsOverlongInputAndReturnsTypedResult(t *testing.T) {
	frozen := FrozenModelConfig{
		ConfigID: "cfg_1", ConfigDigest: "digest", Protocol: "openai_chat_completions",
		Model: "model-x", BaseURL: "https://api.example.com/v1", PromptVersion: intelPromptVersion,
	}
	extractor := NewModelExtractorV2(frozen)
	text := strings.Repeat("长", intelInputTokenLimit*4+1)
	if _, err := extractor.Extract(context.Background(), ExtractionRequest{SourceRevisionID: "rev", Text: text}); err == nil || ErrorCode(err) != "REVIEW_REQUIRED" {
		t.Fatalf("overlong err=%v", err)
	}
	sourceText := "公司正在推进收购B。"
	output := extractionFixtureOutput("rev_1", sourceText)
	rawOutput, _ := json.Marshal(output)
	extractor.Call = func(_ context.Context, cfg FrozenModelConfig, body []byte) ([]byte, error) {
		if cfg.ConfigID != "cfg_1" || !strings.Contains(string(body), intelExtractionName) {
			t.Fatalf("cfg=%#v body=%s", cfg, body)
		}
		raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(rawOutput)}}}})
		return raw, nil
	}
	result, err := extractor.Extract(context.Background(), ExtractionRequest{SourceRevisionID: "rev_1", Text: sourceText})
	if err != nil {
		t.Fatal(err)
	}
	if result.ConfigDigest != "digest" || result.SchemaVersion != "intel.extraction.v1" {
		t.Fatalf("result=%#v", result)
	}
}

func TestFreezeActiveModelUsesConfigServiceAndKeepsDigest(t *testing.T) {
	db := testdb.Start(t)
	configs := finance.NewConfigService(db)
	public := map[string]any{
		"base_url": "https://api.example.com/v1", "protocol": "openai_responses", "model": "model-x",
	}
	saved, err := configs.SaveModel(context.Background(), public, "secret-key")
	if err != nil {
		t.Fatal(err)
	}
	if err := configs.MarkTested(context.Background(), saved.ID, saved.ConfigDigest); err != nil {
		t.Fatal(err)
	}
	if err := configs.Activate(context.Background(), saved.ID); err != nil {
		t.Fatal(err)
	}
	frozen, err := FreezeActiveModel(context.Background(), configs)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.ConfigID != saved.ID || frozen.ConfigDigest != saved.ConfigDigest || frozen.Protocol != "openai_responses" || frozen.APIKey != "secret-key" {
		t.Fatalf("frozen=%#v", frozen)
	}
}
