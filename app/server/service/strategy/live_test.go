package strategy

import (
	"context"
	"encoding/json"
	"testing"
)

func TestGenerateLiveUsesFakeUpstream(t *testing.T) {
	doc := `{
	  "schema_version":"strategy.v1","name":"t","instrument_id":"00700.HK",
	  "signal_period":"1d","price_basis":"causal_qfq",
	  "indicators":[{"id":"macd","type":"MACD","params":{"fast":12,"slow":26,"signal":9}}],
	  "entry":{"op":"crosses_above","left":"macd.dif","right":"macd.dea"},
	  "exit":{"op":"crosses_below","left":"macd.dif","right":"macd.dea"},
	  "position":{"type":"equity_fraction","value":"0.5"},
	  "risk":{"check":"close","stop_loss_pct":"0.05","take_profit_pct":null,"max_holding_bars":null},
	  "execution":{"timing":"next_session_open","priority":"exit_first"}
	}`
	calls := 0
	out := GenerateLive(context.Background(), LiveInput{
		Text: "MACD金叉买入死叉卖出，半仓，收盘亏损5%卖出", InstrumentID: "00700.HK",
		Protocol: "openai_chat_completions", BaseURL: "https://example.invalid/v1", APIKey: "k", Model: "m",
		Call: func(ctx context.Context, protocol, baseURL, apiKey, model string, body []byte) ([]byte, error) {
			calls++
			env, _ := json.Marshal(map[string]any{
				"choices": []any{map[string]any{"message": map[string]any{"content": `{"status":"ready","document":` + doc + `,"assumptions":["live"]}`}}},
			})
			return env, nil
		},
	})
	if out.Status != "ready" || out.Document == nil || out.Document.Position.Value != "0.5" {
		t.Fatalf("%+v", out)
	}
	if out.Source != "openai_chat_completions" || calls != 1 {
		t.Fatalf("source %s calls %d", out.Source, calls)
	}
}

func TestGenerateLiveForbiddenDoesNotCallModel(t *testing.T) {
	calls := 0
	out := GenerateLive(context.Background(), LiveInput{
		Text: "未来三天会涨才买", InstrumentID: "600519.SH",
		Protocol: "openai_chat_completions", BaseURL: "https://example.invalid/v1", APIKey: "k",
		Call: func(ctx context.Context, protocol, baseURL, apiKey, model string, body []byte) ([]byte, error) {
			calls++
			return nil, nil
		},
	})
	if out.Status != "unsupported" || calls != 0 {
		t.Fatalf("%+v calls %d", out, calls)
	}
}

func TestGenerateLiveRetriesInvalidJSON(t *testing.T) {
	doc := `{
	  "schema_version":"strategy.v1","name":"t","instrument_id":"600519.SH",
	  "signal_period":"1d","price_basis":"raw",
	  "indicators":[{"id":"ma","type":"MA","params":{"n":5}}],
	  "entry":{"op":"gt","left":"close","right":{"constant":"1"}},
	  "exit":{"op":"lt","left":"close","right":{"constant":"0"}},
	  "position":{"type":"equity_fraction","value":"1"},
	  "risk":{"check":"close","stop_loss_pct":null,"take_profit_pct":null,"max_holding_bars":null},
	  "execution":{"timing":"next_session_open","priority":"exit_first"}
	}`
	calls := 0
	out := GenerateLive(context.Background(), LiveInput{
		Text: "收盘价大于1买入小于0卖出", InstrumentID: "600519.SH",
		Protocol: "openai_chat_completions", BaseURL: "https://example.invalid/v1", APIKey: "k",
		Call: func(ctx context.Context, protocol, baseURL, apiKey, model string, body []byte) ([]byte, error) {
			calls++
			if calls == 1 {
				return []byte(`{"choices":[{"message":{"content":"not json"}}]}`), nil
			}
			raw, _ := json.Marshal(map[string]any{
				"choices": []any{map[string]any{"message": map[string]any{"content": doc}}},
			})
			return raw, nil
		},
	})
	if out.Status != "ready" || calls != 2 {
		t.Fatalf("%+v calls %d", out, calls)
	}
}
