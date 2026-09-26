package strategy

// 受控 Chat／Responses 断言（R2-H2/R3）：modify 请求携带冻结原编辑态，
// explain 只认模型输出；两协议行为一致。

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"zhigu/server/service/finance"
	"zhigu/server/service/indicators"
)

const liveDocJSON = `{"schema_version":"strategy.v2","name":"x","instrument_id":"600519.SH","signal_period":"1d","price_basis":"raw","indicators":[],"entry":{"op":"gt","left":"close","right":{"constant":"0"}},"exit":{"op":"lt","left":"close","right":{"constant":"0"}},"position":{"type":"equity_fraction","value":"0.5"},"risk":{"check":"close"},"execution":{"timing":"next_session_open","priority":"exit_first"}}`

func userContent(t *testing.T, body []byte) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if msgs, ok := m["messages"].([]any); ok && len(msgs) > 1 {
		u, _ := msgs[1].(map[string]any)
		s, _ := u["content"].(string)
		return s
	}
	if input, ok := m["input"].([]any); ok && len(input) > 1 {
		u, _ := input[1].(map[string]any)
		s, _ := u["content"].(string)
		return s
	}
	t.Fatalf("no user content in request: %s", body)
	return ""
}

func TestLiveModifyRequestCarriesBaseRules(t *testing.T) {
	inst := "600519.SH"
	base := EditorState{
		Name: "保留原策略", InstrumentID: &inst, SignalPeriod: "1d", PriceBasis: "raw",
		Indicators: []indicators.Spec{{ID: "kdj", Type: "KDJ", Params: map[string]int{"n": 9, "m1": 3, "m2": 3}}},
		Entry:      json.RawMessage(`{"op":"lt","left":"kdj.j","right":{"constant":"17"}}`),
		Exit:       json.RawMessage(`{"op":"gt","left":"kdj.j","right":{"constant":"83"}}`),
		Position:   Position{Type: "equity_fraction", Value: "0.8"},
		Risk:       Risk{Check: "close"},
		Execution:  Execution{Timing: "next_session_open", Priority: "exit_first"},
	}
	rawBase, _ := json.Marshal(base)
	for _, proto := range []string{"openai_chat_completions", "openai_responses"} {
		var sent string
		call := func(_ context.Context, _, _, _ string, _ string, body []byte) ([]byte, error) {
			sent = userContent(t, body)
			return []byte(`{"status":"ready","document":` + liveDocJSON + `}`), nil
		}
		res := GenerateLive(context.Background(), LiveInput{
			Text: "只把仓位改为半仓，其他规则不变", InstrumentID: "600519.SH",
			Protocol: proto, BaseURL: "https://api.example.com", APIKey: "k", Model: "m",
			Call: call, Mode: "modify", BaseRules: string(rawBase),
		})
		if res.Status != "ready" {
			t.Fatalf("[%s] modify live failed: %+v", proto, res)
		}
		// 请求必须携带冻结的当前编辑态与改动目标（R2-H2）。
		for _, want := range []string{"当前规则(JSON)", `"constant":"17"`, "kdj.j", "只把仓位改为半仓"} {
			if !strings.Contains(sent, want) {
				t.Fatalf("[%s] modify request missing %q:\n%s", proto, want, sent)
			}
		}
	}
}

func TestExplainLiveUsesModelBothProtocols(t *testing.T) {
	for _, proto := range []string{"openai_chat_completions", "openai_responses"} {
		var sent string
		call := func(_ context.Context, _, _, _ string, _ string, body []byte) ([]byte, error) {
			sent = userContent(t, body)
			if proto == "openai_responses" {
				return []byte(`{"output":[{"content":[{"text":"这条策略在趋势形成时买入。"}]}]}`), nil
			}
			return []byte(`{"choices":[{"message":{"content":"这条策略在趋势形成时买入。"}}]}`), nil
		}
		text, err := ExplainLive(context.Background(), LiveInput{
			Protocol: proto, BaseURL: "https://api.example.com", APIKey: "k", Model: "m", Call: call,
		}, `{"schema_version":"strategy.v2"}`)
		if err != nil || text != "这条策略在趋势形成时买入。" {
			t.Fatalf("[%s] explain = %q err=%v", proto, text, err)
		}
		if !strings.Contains(sent, "当前规则(JSON)") {
			t.Fatalf("[%s] explain request missing rules:\n%s", proto, sent)
		}
	}
}

func TestExplainLiveWithoutConfigUnavailable(t *testing.T) {
	_, err := ExplainLive(context.Background(), LiveInput{}, "{}")
	if err == nil || finance.ErrorCode(err) != "MODEL_UNAVAILABLE" {
		t.Fatalf("want MODEL_UNAVAILABLE, got %v", err)
	}
}
