package indicators

import (
	"testing"

	"github.com/shopspring/decimal"
)

func barsFromCloses(cs ...string) []Bar {
	out := make([]Bar, len(cs))
	for i, c := range cs {
		v := decimal.RequireFromString(c)
		out[i] = Bar{
			Time:   "2020-01-0" + string(rune('1'+i)),
			Open:   v, High: v, Low: v, Close: v,
			Volume: decimal.NewFromInt(int64((i + 1) * 100)),
		}
	}
	return out
}

func ohlc(h, l, c string, vol int64) Bar {
	return Bar{
		High: decimal.RequireFromString(h),
		Low:  decimal.RequireFromString(l),
		Open: decimal.RequireFromString(c),
		Close: decimal.RequireFromString(c),
		Volume: decimal.NewFromInt(vol),
	}
}

func mustDec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestSMAGold(t *testing.T) {
	s, err := Compute(barsFromCloses("1", "2", "3", "4", "5"), Spec{Type: "MA", Params: map[string]int{"n": 3}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Fields["ma"][1] != nil || !s.Ready[2] {
		t.Fatalf("warmup: %+v ready=%v", s.Fields["ma"][1], s.Ready)
	}
	if s.Fields["ma"][2].String() != "2" || s.Fields["ma"][3].String() != "3" || s.Fields["ma"][4].String() != "4" {
		t.Fatalf("sma got %s %s %s", s.Fields["ma"][2], s.Fields["ma"][3], s.Fields["ma"][4])
	}
}

func TestEMAGold(t *testing.T) {
	s, err := Compute(barsFromCloses("1", "2", "3", "4"), Spec{Type: "EMA", Params: map[string]int{"n": 3}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Ready[1] || !s.Ready[2] {
		t.Fatalf("ema ready %v", s.Ready)
	}
	if s.Fields["ema"][2].String() != "2.25" {
		t.Fatalf("ema[2]=%s", s.Fields["ema"][2])
	}
	if s.Fields["ema"][3].String() != "3.125" {
		t.Fatalf("ema[3]=%s", s.Fields["ema"][3])
	}
}

func TestBOLLPopulationStd(t *testing.T) {
	s, err := Compute(barsFromCloses("1", "2", "3"), Spec{Type: "BOLL", Params: map[string]int{"n": 3, "k": 2}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Fields["mid"][2].String() != "2" {
		t.Fatalf("mid=%s", s.Fields["mid"][2])
	}
	sd := sqrt(mustDec("2").Div(mustDec("3")))
	wantU := mustDec("2").Add(mustDec("2").Mul(sd)).RoundBank(34)
	if !s.Fields["upper"][2].Equal(wantU) {
		t.Fatalf("upper=%s want %s", s.Fields["upper"][2], wantU)
	}
}

func TestKDJGoldAndFlatWindow(t *testing.T) {
	bars := []Bar{
		ohlc("10", "8", "9", 100),
		ohlc("11", "9", "10", 100),
		ohlc("12", "9", "11", 100),
	}
	bars[0].Time, bars[1].Time, bars[2].Time = "d1", "d2", "d3"
	s, err := Compute(bars, Spec{Type: "KDJ", Params: map[string]int{"n": 3, "m1": 3, "m2": 3}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Fields["k"][2].Round(2).String() != "58.33" ||
		s.Fields["d"][2].Round(2).String() != "52.78" ||
		s.Fields["j"][2].Round(2).String() != "69.44" {
		t.Fatalf("kdj k=%s d=%s j=%s", s.Fields["k"][2], s.Fields["d"][2], s.Fields["j"][2])
	}
	flat := []Bar{ohlc("10", "10", "10", 1), ohlc("10", "10", "10", 1), ohlc("10", "10", "10", 1)}
	f, _ := Compute(flat, Spec{Type: "KDJ", Params: map[string]int{"n": 3, "m1": 3, "m2": 3}})
	if f.Fields["k"][2].Round(6).String() != "50" {
		t.Fatalf("flat k=%s", f.Fields["k"][2])
	}
}

func TestMACDHistTimesTwo(t *testing.T) {
	closes := make([]string, 40)
	for i := range closes {
		closes[i] = decimal.NewFromInt(int64(100 + i)).String()
	}
	s, err := Compute(barsFromCloses(closes...), Spec{Type: "MACD", Params: map[string]int{"fast": 3, "slow": 5, "signal": 2}})
	if err != nil {
		t.Fatal(err)
	}
	last := len(closes) - 1
	if !s.Ready[last] || s.Fields["dif"][last] == nil || s.Fields["dea"][last] == nil {
		t.Fatal("macd not ready")
	}
	want := mustDec("2").Mul(s.Fields["dif"][last].Sub(*s.Fields["dea"][last])).RoundBank(34)
	if !s.Fields["hist"][last].Equal(want) {
		t.Fatalf("hist=%s want %s", s.Fields["hist"][last], want)
	}
	if s.Warmup != 5+2-1 {
		t.Fatalf("warmup %d", s.Warmup)
	}
}

func TestRSIWilderAndZeros(t *testing.T) {
	s, err := Compute(barsFromCloses("10", "11", "12", "11"), Spec{Type: "RSI", Params: map[string]int{"n": 2}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Fields["rsi"][2].String() != "100" {
		t.Fatalf("rsi all-up=%s", s.Fields["rsi"][2])
	}
	if s.Fields["rsi"][3].String() != "50" {
		t.Fatalf("rsi mixed=%s", s.Fields["rsi"][3])
	}
	flat, _ := Compute(barsFromCloses("10", "10", "10"), Spec{Type: "RSI", Params: map[string]int{"n": 2}})
	if flat.Fields["rsi"][2].String() != "50" {
		t.Fatalf("rsi flat=%s", flat.Fields["rsi"][2])
	}
}

func TestWRBIASCCIROBVATR(t *testing.T) {
	bars := []Bar{ohlc("10", "8", "9", 100), ohlc("11", "9", "10", 200), ohlc("12", "9", "11", 150)}
	w, _ := Compute(bars, Spec{Type: "WR", Params: map[string]int{"n": 3}})
	if w.Fields["wr"][2].String() != "25" {
		t.Fatalf("wr=%s", w.Fields["wr"][2])
	}
	b, _ := Compute(barsFromCloses("1", "2", "3"), Spec{Type: "BIAS", Params: map[string]int{"n": 3}})
	if b.Fields["bias"][2].String() != "50" {
		t.Fatalf("bias=%s", b.Fields["bias"][2])
	}
	zeroSMA := barsFromCloses("0", "0", "0")
	bz, _ := Compute(zeroSMA, Spec{Type: "BIAS", Params: map[string]int{"n": 3}})
	if bz.Fields["bias"][2] != nil {
		t.Fatal("bias divide-by-zero should be null")
	}
	c, _ := Compute(bars, Spec{Type: "CCI", Params: map[string]int{"n": 3}})
	if c.Fields["cci"][2] == nil || !c.Ready[2] {
		t.Fatal("cci")
	}
	flat := []Bar{ohlc("10", "10", "10", 1), ohlc("10", "10", "10", 1), ohlc("10", "10", "10", 1)}
	cf, _ := Compute(flat, Spec{Type: "CCI", Params: map[string]int{"n": 3}})
	if cf.Fields["cci"][2].String() != "0" {
		t.Fatalf("cci flat=%s", cf.Fields["cci"][2])
	}
	o, _ := Compute(barsFromCloses("10", "11", "10"), Spec{Type: "OBV", Params: map[string]int{}})
	if o.Fields["obv"][0].String() != "0" {
		t.Fatalf("obv0=%s", o.Fields["obv"][0])
	}
	if o.Fields["obv"][1].String() != "200" { // volume (i+1)*100 -> bar1 vol=200
		t.Fatalf("obv1=%s", o.Fields["obv"][1])
	}
	if o.Fields["obv"][2].String() != "-100" { // 200-300
		t.Fatalf("obv2=%s", o.Fields["obv"][2])
	}
	a, _ := Compute(bars, Spec{Type: "ATR", Params: map[string]int{"n": 2}})
	if a.Fields["atr"][1] == nil || !a.Ready[1] {
		t.Fatal("atr warmup")
	}
}

func TestCrossBoundary(t *testing.T) {
	left := []*decimal.Decimal{ptr(mustDec("1")), ptr(mustDec("3"))}
	right := []*decimal.Decimal{ptr(mustDec("2")), ptr(mustDec("2"))}
	ready := []bool{true, true}
	if !CrossesAbove(left, right, ready, ready, 1) {
		t.Fatal("expected cross above")
	}
	if CrossesAbove(left, right, ready, ready, 0) {
		t.Fatal("no cross at 0")
	}
	equalThenUp := []*decimal.Decimal{ptr(mustDec("2")), ptr(mustDec("3"))}
	if !CrossesAbove(equalThenUp, right, ready, ready, 1) {
		t.Fatal("equal then up is a golden cross")
	}
}

func TestParamRangeRejected(t *testing.T) {
	_, err := Compute(barsFromCloses("1", "2"), Spec{Type: "MA", Params: map[string]int{"n": 1}})
	if err == nil {
		t.Fatal("expected range error")
	}
}

func TestAnchorDoesNotReseed(t *testing.T) {
	bars := barsFromCloses("1", "2", "3", "4", "5", "6")
	full, _ := Compute(bars, Spec{Type: "EMA", Params: map[string]int{"n": 3}})
	prefix, _ := Compute(bars[:5], Spec{Type: "EMA", Params: map[string]int{"n": 3}})
	if !full.Fields["ema"][4].Equal(*prefix.Fields["ema"][4]) {
		t.Fatalf("prefix ema drifted %s vs %s", prefix.Fields["ema"][4], full.Fields["ema"][4])
	}
}
