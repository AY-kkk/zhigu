package strategy

import (
	"encoding/json"
	"testing"
)

func TestCompileExample(t *testing.T) {
	raw := []byte(`{
  "schema_version": "strategy.v1",
  "name": "腾讯 MACD 与 KDJ 日线策略",
  "instrument_id": "00700.HK",
  "signal_period": "1d",
  "price_basis": "causal_qfq",
  "indicators": [
    {"id": "macd", "type": "MACD", "params": {"fast": 12, "slow": 26, "signal": 9}},
    {"id": "kdj", "type": "KDJ", "params": {"n": 9, "m1": 3, "m2": 3}}
  ],
  "entry": {"all": [
    {"op": "crosses_above", "left": "macd.dif", "right": "macd.dea"},
    {"op": "lt", "left": "kdj.k", "right": {"constant": "30"}}
  ]},
  "exit": {"op": "crosses_below", "left": "macd.dif", "right": "macd.dea"},
  "position": {"type": "equity_fraction", "value": "0.5"},
  "risk": {"check": "close", "stop_loss_pct": "0.05", "take_profit_pct": null, "max_holding_bars": null},
  "execution": {"timing": "next_session_open", "priority": "exit_first"}
}`)
	doc, err := ParseDSL(raw)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Compile(doc)
	if err != nil {
		t.Fatal(err)
	}
	if c.WarmupBars < 26 {
		t.Fatalf("warmup %d", c.WarmupBars)
	}
}

func TestRejectUnknownFieldAndNegLag(t *testing.T) {
	_, err := ParseDSL([]byte(`{"schema_version":"strategy.v1","name":"x","instrument_id":"a","python":"os.system"}`))
	if err == nil {
		t.Fatal("unknown field")
	}
	doc := mustDoc(t)
	doc.Entry = json.RawMessage(`{"op":"gt","left":"close","right":{"constant":"1"},"lag":-1}`)
	if _, err := Compile(doc); err == nil {
		t.Fatal("neg lag")
	}
}

func TestWeeklyUnsupported(t *testing.T) {
	doc := mustDoc(t)
	doc.SignalPeriod = "1w"
	if _, err := Compile(doc); err == nil {
		t.Fatal("weekly")
	}
}

func TestGenerateTencentPhrase(t *testing.T) {
	out := GenerateFromText(GenerateInput{
		Text:         "用腾讯做日线策略：MACD 金叉且 KDJ 的 K 小于 30 时半仓买入，MACD 死叉卖出；收盘亏损达到 5% 也卖出。用 10 万港币回测 2022 年至 2025 年。",
		InstrumentID: "00700.HK",
	})
	if out.Status != "ready" || out.Document == nil {
		t.Fatalf("%+v", out)
	}
	if out.Document.Position.Value != "0.5" || out.Document.Risk.StopLossPct == nil || *out.Document.Risk.StopLossPct != "0.05" {
		t.Fatalf("pos/stop %+v", out.Document)
	}
	if _, err := Compile(*out.Document); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateClarificationAndForbidden(t *testing.T) {
	q := GenerateFromText(GenerateInput{Text: "金叉买入", InstrumentID: "600519.SH"})
	if q.Status != "needs_clarification" {
		t.Fatalf("%+v", q)
	}
	f := GenerateFromText(GenerateInput{Text: "未来三天会涨才买", InstrumentID: "600519.SH"})
	if f.Status != "unsupported" {
		t.Fatalf("%+v", f)
	}
}

func mustDoc(t *testing.T) Document {
	t.Helper()
	raw := []byte(`{
	  "schema_version":"strategy.v1","name":"t","instrument_id":"600519.SH",
	  "signal_period":"1d","price_basis":"raw",
	  "indicators":[{"id":"ma","type":"MA","params":{"n":5}}],
	  "entry":{"op":"gt","left":"close","right":{"constant":"1"}},
	  "exit":{"op":"lt","left":"close","right":{"constant":"0"}},
	  "position":{"type":"equity_fraction","value":"1"},
	  "risk":{"check":"close","stop_loss_pct":null,"take_profit_pct":null,"max_holding_bars":null},
	  "execution":{"timing":"next_session_open","priority":"exit_first"}
	}`)
	doc, err := ParseDSL(raw)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}
