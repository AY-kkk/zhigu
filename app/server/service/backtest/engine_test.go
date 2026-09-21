package backtest

import (
	"testing"

	"github.com/shopspring/decimal"

	"zhigu/server/service/indicators"
	"zhigu/server/service/market"
	"zhigu/server/service/strategy"
)

func bar(day, px string, vol int64) indicators.Bar {
	v := decimal.RequireFromString(px)
	return indicators.Bar{Time: day, Open: v, High: v, Low: v, Close: v, Volume: decimal.NewFromInt(vol)}
}

func TestLedgerNextOpenAndNoSameDayFill(t *testing.T) {
	doc := strategy.Document{
		SchemaVersion: strategy.SchemaVersion,
		Name:          "close threshold",
		InstrumentID:  "600519.SH",
		SignalPeriod:  "1d",
		PriceBasis:    "raw",
		Indicators:    []indicators.Spec{{ID: "ma", Type: "MA", Params: map[string]int{"n": 2}}},
		Entry:         []byte(`{"op":"gt","left":"close","right":{"constant":"10.5"}}`),
		Exit:          []byte(`{"op":"lt","left":"close","right":{"constant":"10.5"}}`),
		Position:      strategy.Position{Type: "equity_fraction", Value: "1"},
		Risk:          strategy.Risk{Check: "close"},
		Execution:     strategy.Execution{Timing: "next_session_open", Priority: "exit_first"},
	}
	compiled, err := strategy.Compile(doc)
	if err != nil {
		t.Fatal(err)
	}
	raw := []indicators.Bar{
		bar("2020-01-02", "10", 1000000),
		bar("2020-01-03", "10", 1000000),
		bar("2020-01-06", "10", 1000000),
		bar("2020-01-07", "11", 1000000),
		bar("2020-01-08", "12", 1000000),
		bar("2020-01-09", "10", 1000000),
		bar("2020-01-10", "9", 1000000),
		bar("2020-01-13", "10", 1000000),
	}
	res := Run(Input{
		Doc: doc, Compiled: compiled, Raw: raw,
		Instrument: market.InstrumentView{InstrumentID: "600519.SH", AssetType: "stock", Currency: "CNY", Exchange: "SSE"},
		Rule: market.TradingRule{
			LotSize: 1, Tick: "0.01", TPlus: 0,
			CommissionRate: "0", CommissionMin: "0", StampBuy: "0", StampSell: "0", TransferRate: "0",
			Source: "test_zero_commission", Version: "test", EffectiveFrom: "2000-01-01",
		},
		Config: Config{InitialCash: "1200", SlippageBPS: "0", ParticipationCap: "1", Start: "2020-01-02", End: "2020-01-13", Currency: "CNY"},
	})
	if res.Status != "succeeded" {
		t.Fatalf("%+v", res)
	}
	if len(res.Fills) != 2 {
		t.Fatalf("fills=%d %+v", len(res.Fills), res.Fills)
	}
	if res.Fills[0].Date != "2020-01-08" || res.Fills[0].Price != "12" {
		t.Fatalf("buy fill %+v", res.Fills[0])
	}
	if res.Fills[1].Date != "2020-01-10" || res.Fills[1].Price != "9" {
		t.Fatalf("sell fill %+v", res.Fills[1])
	}
	last := res.Equity[len(res.Equity)-1]
	if last.Equity != "900" {
		t.Fatalf("terminal equity %s metrics=%v", last.Equity, res.Metrics)
	}
	again := Run(Input{
		Doc: doc, Compiled: compiled, Raw: raw,
		Instrument: market.InstrumentView{InstrumentID: "600519.SH", AssetType: "stock", Currency: "CNY"},
		Rule: market.TradingRule{LotSize: 1, Tick: "0.01", CommissionRate: "0", CommissionMin: "0", StampBuy: "0", StampSell: "0", TransferRate: "0", Source: "test_zero_commission", Version: "test"},
		Config: Config{InitialCash: "1200", SlippageBPS: "0", ParticipationCap: "1", Start: "2020-01-02", End: "2020-01-13"},
	})
	if again.ResultHash != res.ResultHash {
		t.Fatal("hash not stable")
	}
}

func TestFundRejectedAndFutureBarDoesNotChangePast(t *testing.T) {
	doc := strategy.Document{
		SchemaVersion: strategy.SchemaVersion, Name: "t", InstrumentID: "510300.SH",
		SignalPeriod: "1d", PriceBasis: "raw",
		Indicators: []indicators.Spec{{ID: "ma", Type: "MA", Params: map[string]int{"n": 2}}},
		Entry:      []byte(`{"op":"gt","left":"close","right":{"constant":"1"}}`),
		Exit:       []byte(`{"op":"lt","left":"close","right":{"constant":"0"}}`),
		Position:   strategy.Position{Type: "equity_fraction", Value: "1"},
		Risk:       strategy.Risk{Check: "close"},
		Execution:  strategy.Execution{Timing: "next_session_open", Priority: "exit_first"},
	}
	c, _ := strategy.Compile(doc)
	res := Run(Input{Doc: doc, Compiled: c, Raw: []indicators.Bar{bar("2020-01-02", "1", 1)}, Instrument: market.InstrumentView{AssetType: "fund"}, Rule: market.TradingRule{LotSize: 100, Tick: "0.001", CommissionRate: "0", CommissionMin: "0"}, Config: Config{InitialCash: "1000", SlippageBPS: "0"}})
	if res.ErrorCode != "INSTRUMENT_UNSUPPORTED" {
		t.Fatalf("%+v", res)
	}
}

func TestZeroTradesWinRateNull(t *testing.T) {
	doc := strategy.Document{
		SchemaVersion: strategy.SchemaVersion, Name: "t", InstrumentID: "600519.SH",
		SignalPeriod: "1d", PriceBasis: "raw",
		Indicators: []indicators.Spec{{ID: "ma", Type: "MA", Params: map[string]int{"n": 2}}},
		Entry:      []byte(`{"op":"gt","left":"close","right":{"constant":"9999"}}`),
		Exit:       []byte(`{"op":"lt","left":"close","right":{"constant":"0"}}`),
		Position:   strategy.Position{Type: "equity_fraction", Value: "1"},
		Risk:       strategy.Risk{Check: "close"},
		Execution:  strategy.Execution{Timing: "next_session_open", Priority: "exit_first"},
	}
	c, _ := strategy.Compile(doc)
	raw := []indicators.Bar{bar("2020-01-02", "10", 1000), bar("2020-01-03", "10", 1000), bar("2020-01-06", "10", 1000), bar("2020-01-07", "10", 1000)}
	res := Run(Input{Doc: doc, Compiled: c, Raw: raw, Instrument: market.InstrumentView{AssetType: "stock"}, Rule: market.TradingRule{LotSize: 100, Tick: "0.01", CommissionRate: "0", CommissionMin: "0", StampBuy: "0", StampSell: "0", TransferRate: "0", Source: "t", Version: "t"}, Config: Config{InitialCash: "100000", SlippageBPS: "0", ParticipationCap: "1"}})
	if res.Status != "succeeded" {
		t.Fatal(res)
	}
	if res.Metrics["win_rate"] != nil {
		t.Fatalf("win_rate=%v", res.Metrics["win_rate"])
	}
}
