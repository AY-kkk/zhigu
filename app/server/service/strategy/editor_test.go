package strategy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"zhigu/server/service/finance"
	"zhigu/server/service/indicators"
)

func baseEditorState(entry, exit string) EditorState {
	inst := "00700.HK"
	return EditorState{
		Name:         "测试策略",
		InstrumentID: &inst,
		SignalPeriod: "1d",
		PriceBasis:   "raw",
		Indicators:   []indicators.Spec{{ID: "kdj", Type: "KDJ", Params: map[string]int{"n": 9, "m1": 3, "m2": 3}}},
		Entry:        json.RawMessage(entry),
		Exit:         json.RawMessage(exit),
		Position:     Position{Type: "equity_fraction", Value: "0.5"},
		Risk:         Risk{Check: "close"},
		Execution:    Execution{Timing: "next_session_open", Priority: "exit_first"},
	}
}

func mustCompileEditor(t *testing.T, state EditorState) EditorResult {
	t.Helper()
	res, err := CompileEditor(state, "")
	if err != nil {
		t.Fatalf("CompileEditor: %v", err)
	}
	if res.Status != "ready" {
		t.Fatalf("status = %s missing = %v", res.Status, res.Missing)
	}
	return res
}

func condJSON(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("cond json: %v", err)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func TestEditorRepeatExpansion(t *testing.T) {
	// 「J 小于 20 连续 3 天」：repeat 按滞后偏移展开为 all，内部引用 lag 叠加。
	state := baseEditorState(
		`{"op":"lt","left":{"ref":"kdj.j","lag":1},"right":{"constant":"20"},"repeat":3}`,
		`{"op":"gt","left":"kdj.d","right":{"constant":"80"}}`)
	res := mustCompileEditor(t, state)
	want := `{"all":[{"left":{"lag":1,"ref":"kdj.j"},"op":"lt","right":{"constant":"20"}},{"left":{"lag":2,"ref":"kdj.j"},"op":"lt","right":{"constant":"20"}},{"left":{"lag":3,"ref":"kdj.j"},"op":"lt","right":{"constant":"20"}}]}`
	if got := condJSON(t, res.Document.Entry); got != want {
		t.Fatalf("entry = %s\nwant = %s", got, want)
	}
	if len(res.Continuity) != 0 {
		t.Fatalf("unexpected continuity checks: %v", res.Continuity)
	}
}

func TestEditorVolumeIncreaseExpansion(t *testing.T) {
	// PRD §3.5 示例：J＜20 且连续放量 3 天（放量编译为 3 组量比较 + 可信连续性检查）。
	state := baseEditorState(
		`{"all":[{"op":"lt","left":"kdj.j","right":{"constant":"20"}},{"kind":"volume_increase","days":3}]}`,
		`{"op":"gt","left":"kdj.j","right":{"constant":"80"}}`)
	res := mustCompileEditor(t, state)
	got := condJSON(t, res.Document.Entry)
	wantPrefix := `{"all":[{"left":"kdj.j","op":"lt","right":{"constant":"20"}},{"all":[{"left":"volume","op":"gt","right":{"lag":1,"ref":"volume"}},{"left":{"lag":1,"ref":"volume"},"op":"gt","right":{"lag":2,"ref":"volume"}},{"left":{"lag":2,"ref":"volume"},"op":"gt","right":{"lag":3,"ref":"volume"}}]}]}`
	if got != wantPrefix {
		t.Fatalf("entry = %s\nwant = %s", got, wantPrefix)
	}
	if len(res.Continuity) != 1 || res.Continuity[0] != (ContinuityCheck{Kind: "consecutive_rising_volume", Ref: "volume", Days: 3, BaseLag: 0, Path: "entry/all[1]"}) {
		t.Fatalf("continuity = %+v", res.Continuity)
	}
	// 附录 §4.2：预热覆盖最大 lag 与交叉前一根 —— 量窗口最大 lag 3 → 至少 4 根比较 bar。
	if res.Compiled.WarmupBars < 4 {
		t.Fatalf("warmup = %d, want >= 4", res.Compiled.WarmupBars)
	}
}

func TestEditorNeedsClarification(t *testing.T) {
	state := baseEditorState(`{"op":"lt","left":"kdj.j","right":null}`, `{"op":"gt","left":"kdj.j","right":{"constant":"80"}}`)
	state.InstrumentID = nil
	res, err := CompileEditor(state, "")
	if err != nil {
		t.Fatalf("CompileEditor: %v", err)
	}
	if res.Status != "needs_clarification" {
		t.Fatalf("status = %s", res.Status)
	}
	if res.Document != nil || res.Compiled != nil {
		t.Fatal("needs_clarification 不得产出可执行 DSL/compiled")
	}
	joined := strings.Join(res.Missing, ",")
	if !strings.Contains(joined, "instrument_id") || !strings.Contains(joined, "entry") {
		t.Fatalf("missing = %v", res.Missing)
	}
}

func TestEditorIllegalInputRejected(t *testing.T) {
	cases := []struct {
		name  string
		entry string
	}{
		{"negative lag", `{"op":"lt","left":{"ref":"kdj.j","lag":-1},"right":{"constant":"20"}}`},
		{"lag over 250", `{"op":"lt","left":{"ref":"kdj.j","lag":251},"right":{"constant":"20"}}`},
		{"unknown leaf field", `{"op":"lt","left":"kdj.j","right":{"constant":"20"},"expr":"1+1"}`},
		{"mixed group keys", `{"all":[{"op":"lt","left":"kdj.j","right":{"constant":"20"}}],"op":"gt"}`},
		{"operand unknown field", `{"op":"lt","left":{"ref":"kdj.j","foo":1},"right":{"constant":"20"}}`},
		{"constant not decimal", `{"op":"lt","left":"kdj.j","right":{"constant":"abc"}}`},
		{"repeat out of range", `{"op":"lt","left":"kdj.j","right":{"constant":"20"},"repeat":21}}`},
		{"repeat expands past lag limit", `{"op":"lt","left":{"ref":"kdj.j","lag":245},"right":{"constant":"20"},"repeat":10}`},
		{"bad shortcut days", `{"kind":"volume_increase","days":0}`},
		{"unknown shortcut", `{"kind":"price_jump","days":3}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := baseEditorState(tc.entry, `{"op":"gt","left":"kdj.j","right":{"constant":"80"}}`)
			if _, err := CompileEditor(state, ""); err == nil {
				t.Fatal("want error")
			} else if finance.ErrorCode(err) != "STRATEGY_INVALID" && finance.ErrorCode(err) != "STRATEGY_UNSUPPORTED" {
				t.Fatalf("error code = %s", finance.ErrorCode(err))
			}
		})
	}
}

func TestEditorJValueNotClamped(t *testing.T) {
	// J=-5 是合法阈值，J>100 同样合法（反例15）。
	state := baseEditorState(
		`{"op":"lt","left":"kdj.j","right":{"constant":"-5"}}`,
		`{"op":"gt","left":"kdj.j","right":{"constant":"120"}}`)
	res := mustCompileEditor(t, state)
	if res.Document == nil {
		t.Fatal("no document")
	}
}

func TestEditorVolumeWindowEvaluation(t *testing.T) {
	state := baseEditorState(
		`{"kind":"volume_increase","days":3}`,
		`{"op":"gt","left":"kdj.j","right":{"constant":"80"}}`)
	res := mustCompileEditor(t, state)

	mk := func(vols []string) []indicators.Bar {
		bars := make([]indicators.Bar, len(vols))
		for i, v := range vols {
			bars[i] = indicators.Bar{
				Time: "2026-09-0" + string(rune('1'+i)),
				Open: decimal.NewFromInt(1), High: decimal.NewFromInt(2),
				Low: decimal.NewFromInt(1), Close: decimal.NewFromInt(1),
				Volume: decimal.RequireFromString(v),
			}
		}
		return bars
	}

	okCase := []string{"100", "120", "150", "180"}
	bars := mk(okCase)
	if v, ready := EvalCondWithChecks(res.Document.Entry, res.Compiled.Continuity, bars, nil, 3); !v || !ready {
		t.Fatalf("放量成立却得到 v=%v ready=%v", v, ready)
	}
	// 反例15：任一日量相等或中途缩量不成立。
	if v, ready := EvalCondWithChecks(res.Document.Entry, res.Compiled.Continuity, mk([]string{"100", "120", "120", "180"}), nil, 3); v || !ready {
		t.Fatalf("量相等应不成立，得到 v=%v ready=%v", v, ready)
	}
	if v, ready := EvalCondWithChecks(res.Document.Entry, res.Compiled.Continuity, mk([]string{"100", "120", "110", "180"}), nil, 3); v || !ready {
		t.Fatalf("中途缩量应不成立，得到 v=%v ready=%v", v, ready)
	}
	// 反例16：零成交日中断连续性。
	if v, ready := EvalCondWithChecks(res.Document.Entry, res.Compiled.Continuity, mk([]string{"100", "0", "150", "180"}), nil, 3); v || !ready {
		t.Fatalf("零成交应不成立，得到 v=%v ready=%v", v, ready)
	}
	// 反例16：只有 3 根量 bar / 未预热 → 数据不足。
	if v, ready := EvalCondWithChecks(res.Document.Entry, res.Compiled.Continuity, mk([]string{"100", "120", "150"}), nil, 2); v || ready {
		t.Fatalf("未预热应数据不足，得到 v=%v ready=%v", v, ready)
	}
	_ = bars
}

func TestMarketTemplateParsing(t *testing.T) {
	tplJSON := `{"schema_version":"strategy.market.v1","editor_state":{"name":"x","signal_period":"1d","price_basis":"raw","indicators":[],"entry":{},"exit":{},"position":{"type":"equity_fraction","value":"0.5"},"risk":{"check":"close"},"execution":{"timing":"next_session_open","priority":"exit_first"}},"instrument_binding":{"mode":"select_one","markets":["A","HK"]}}`
	tpl, err := ParseMarketTemplate([]byte(tplJSON))
	if err != nil {
		t.Fatalf("ParseMarketTemplate: %v", err)
	}
	if tpl.SchemaVersion != MarketSchemaVersion {
		t.Fatalf("schema = %s", tpl.SchemaVersion)
	}
	bad := strings.Replace(tplJSON, `"mode":"select_one"`, `"mode":"fixed"`, 1)
	if _, err := ParseMarketTemplate([]byte(bad)); err == nil {
		t.Fatal("want binding mode error")
	}
}
