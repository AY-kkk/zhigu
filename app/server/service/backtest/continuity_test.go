package backtest

// 正式回归集（reviews/stage-b-2026-09-24 反例的等价用例，R1/R7-H4）：
// 连续放量检查必须进入实际回测；检查按节点作用域生效。

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"

	"zhigu/server/service/indicators"
	"zhigu/server/service/market"
	"zhigu/server/service/strategy"
)

func continuityBars(vols []int64) []indicators.Bar {
	dates := []string{"2020-01-02", "2020-01-03", "2020-01-06", "2020-01-07", "2020-01-08", "2020-01-09", "2020-01-10", "2020-01-13"}
	bars := make([]indicators.Bar, 0, len(vols))
	for i, v := range vols {
		p := decimal.NewFromInt(10)
		bars = append(bars, indicators.Bar{Time: dates[i], Open: p, High: p, Low: p, Close: p, Volume: decimal.NewFromInt(v)})
	}
	return bars
}

func continuityInput(t *testing.T, entry, exit string, bars []indicators.Bar) Input {
	t.Helper()
	inst := "600519.SH"
	r, err := strategy.CompileEditor(strategy.EditorState{
		Name: "continuity", InstrumentID: &inst, SignalPeriod: "1d", PriceBasis: "raw",
		Indicators: []indicators.Spec{{ID: "ma", Type: "MA", Params: map[string]int{"n": 2}}},
		Entry:      json.RawMessage(entry), Exit: json.RawMessage(exit),
		Position:  strategy.Position{Type: "equity_fraction", Value: "1"},
		Risk:      strategy.Risk{Check: "close"},
		Execution: strategy.Execution{Timing: "next_session_open", Priority: "exit_first"},
	}, "")
	if err != nil || r.Status != "ready" {
		t.Fatalf("compile %v %+v", err, r)
	}
	return Input{
		Doc: *r.Document, Compiled: *r.Compiled, Raw: bars,
		Instrument: market.InstrumentView{InstrumentID: inst, AssetType: "stock", Currency: "CNY", Exchange: "SSE"},
		Rule: market.TradingRule{LotSize: 1, Tick: "0.01", TPlus: 0, CommissionRate: "0", CommissionMin: "0",
			StampBuy: "0", StampSell: "0", TransferRate: "0", Source: "test_zero_commission", Version: "test", EffectiveFrom: "2000-01-01"},
		Config: Config{InitialCash: "1200", SlippageBPS: "0", ParticipationCap: "1",
			Start: bars[0].Time, End: bars[len(bars)-1].Time, Currency: "CNY"},
	}
}

// TestBacktestRejectsZeroVolumeContinuity：零成交日打断连续放量，实际回测不得买入（R1）。
func TestBacktestRejectsZeroVolumeContinuity(t *testing.T) {
	bars := continuityBars([]int64{0, 0, 0, 0, 100000, 200000, 300000, 400000})
	in := continuityInput(t, `{"kind":"volume_increase","days":3}`, `{"op":"lt","left":"close","right":{"constant":"0"}}`, bars)
	checked, ready := strategy.EvalCondWithChecks(in.Doc.Entry, in.Compiled.Continuity, bars, nil, 6)
	if checked || !ready {
		t.Fatalf("guard baseline invalid: %v %v", checked, ready)
	}
	res := Run(in)
	if res.Status != "succeeded" {
		t.Fatalf("engine setup: %+v", res)
	}
	if len(res.Fills) > 0 {
		t.Fatalf("zero-volume window must not buy; fills=%+v", res.Fills)
	}
}

// TestBacktestNormalVolumeWindowFills：正常放量窗口照常成交（防误杀，R1-H4）。
func TestBacktestNormalVolumeWindowFills(t *testing.T) {
	bars := continuityBars([]int64{10000, 20000, 30000, 40000, 50000, 60000, 70000, 80000})
	in := continuityInput(t, `{"kind":"volume_increase","days":3}`, `{"op":"lt","left":"close","right":{"constant":"0"}}`, bars)
	res := Run(in)
	if res.Status != "succeeded" {
		t.Fatalf("engine setup: %+v", res)
	}
	buys := 0
	for _, f := range res.Fills {
		buys++
		_ = f
	}
	if buys == 0 {
		t.Fatalf("normal rising window must buy; signals=%+v", res.Signals)
	}
}

// TestBacktestMissingWindowInsufficient：窗口所需交易日不足时不发信号（R1-H4）。
func TestBacktestMissingWindowInsufficient(t *testing.T) {
	bars := continuityBars([]int64{10000, 20000, 30000, 40000, 50000, 60000, 70000, 80000})
	in := continuityInput(t, `{"kind":"volume_increase","days":3}`, `{"op":"lt","left":"close","right":{"constant":"0"}}`, bars)
	// 预热结束前（窗口未预热）不得产生 entry 信号。
	for _, s := range resSignals(t, in) {
		if s.Kind == "entry" && s.Date <= bars[2].Time {
			t.Fatalf("entry fired before window warmed: %+v", s)
		}
	}
	// 数据不足以支撑窗口时求值必须返回 not-ready。
	if _, ready := strategy.EvalCondWithChecks(in.Doc.Entry, in.Compiled.Continuity, bars, nil, 2); ready {
		t.Fatal("unwarmed window must be data-insufficient")
	}
}

func resSignals(t *testing.T, in Input) []Signal {
	t.Helper()
	res := Run(in)
	if res.Status != "succeeded" {
		t.Fatalf("engine setup: %+v", res)
	}
	return res.Signals
}

// TestContinuityScopedToEntryNode：entry 的窗口检查不得误杀 exit 或其他分支（R1-H1）。
func TestContinuityScopedToEntryNode(t *testing.T) {
	bars := continuityBars([]int64{0, 0, 0, 0, 100000, 200000, 300000, 400000})
	in := continuityInput(t,
		`{"all":[{"kind":"volume_increase","days":3},{"op":"gt","left":"close","right":{"constant":"5"}}]}`,
		`{"op":"gt","left":"close","right":{"constant":"5"}}`,
		bars)
	entryChecks := strategy.ChecksUnder(in.Compiled.Continuity, "entry")
	exitChecks := strategy.ChecksUnder(in.Compiled.Continuity, "exit")
	if len(exitChecks) != 0 {
		t.Fatalf("exit must not carry entry checks: %+v", exitChecks)
	}
	// entry 在零量窗口日被自己的检查拦下。
	if v, _ := strategy.EvalCondWithChecks(in.Doc.Entry, entryChecks, bars, nil, 6); v {
		t.Fatal("entry check must block its own node")
	}
	// exit 无检查，同日正常求值（close=10 > 5）。
	if v, ready := strategy.EvalCondWithChecks(in.Doc.Exit, exitChecks, bars, nil, 6); !v || !ready {
		t.Fatalf("exit wrongly gated by entry check: v=%v ready=%v", v, ready)
	}
}
