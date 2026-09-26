package indicators

import (
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

const EngineVersion = "indicators.v1"

func init() {
	decimal.DivisionPrecision = 34
}

type Bar struct {
	Time   string
	Open   decimal.Decimal
	High   decimal.Decimal
	Low    decimal.Decimal
	Close  decimal.Decimal
	Volume decimal.Decimal
}

type Spec struct {
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	Params map[string]int `json:"params"`
}

type Series struct {
	ID               string
	Type             string
	Params           map[string]int
	Fields           map[string][]*decimal.Decimal
	Ready            []bool
	Warmup           int
	CalculationStart string
	EngineVersion    string
}

type Descriptor struct {
	Type           string            `json:"type"`
	Pane           string            `json:"pane"`
	Params         map[string]int    `json:"default_params"`
	ParamRange     map[string][2]int `json:"param_range"`
	Outputs        []string          `json:"outputs"`
	Warmup         string            `json:"warmup_rule"`
	FormulaVersion string            `json:"formula_version"`
	StrategyOK     bool              `json:"strategy_capable"`
}

func Registry() []Descriptor {
	r := []Descriptor{
		{Type: "MA", Pane: "price", Params: map[string]int{"n": 20}, ParamRange: map[string][2]int{"n": {2, 500}}, Outputs: []string{"ma"}, Warmup: "N", FormulaVersion: EngineVersion, StrategyOK: true},
		{Type: "EMA", Pane: "price", Params: map[string]int{"n": 12}, ParamRange: map[string][2]int{"n": {2, 500}}, Outputs: []string{"ema"}, Warmup: "N", FormulaVersion: EngineVersion, StrategyOK: true},
		{Type: "BOLL", Pane: "price", Params: map[string]int{"n": 20, "k": 2}, ParamRange: map[string][2]int{"n": {2, 500}, "k": {1, 10}}, Outputs: []string{"mid", "upper", "lower"}, Warmup: "N", FormulaVersion: EngineVersion, StrategyOK: true},
		{Type: "VOL", Pane: "volume", Params: map[string]int{"ma5": 5, "ma10": 10}, ParamRange: map[string][2]int{"ma5": {2, 500}, "ma10": {2, 500}}, Outputs: []string{"volume", "ma5", "ma10"}, Warmup: "max(ma)", FormulaVersion: EngineVersion, StrategyOK: true},
		{Type: "MACD", Pane: "oscillator", Params: map[string]int{"fast": 12, "slow": 26, "signal": 9}, ParamRange: map[string][2]int{"fast": {2, 500}, "slow": {2, 500}, "signal": {2, 500}}, Outputs: []string{"dif", "dea", "hist"}, Warmup: "slow+signal-1", FormulaVersion: EngineVersion, StrategyOK: true},
		{Type: "KDJ", Pane: "oscillator", Params: map[string]int{"n": 9, "m1": 3, "m2": 3}, ParamRange: map[string][2]int{"n": {2, 500}, "m1": {2, 500}, "m2": {2, 500}}, Outputs: []string{"k", "d", "j"}, Warmup: "N", FormulaVersion: EngineVersion, StrategyOK: true},
		{Type: "RSI", Pane: "oscillator", Params: map[string]int{"n": 12}, ParamRange: map[string][2]int{"n": {2, 500}}, Outputs: []string{"rsi"}, Warmup: "N+1", FormulaVersion: EngineVersion, StrategyOK: true},
		{Type: "WR", Pane: "oscillator", Params: map[string]int{"n": 10}, ParamRange: map[string][2]int{"n": {2, 500}}, Outputs: []string{"wr"}, Warmup: "N", FormulaVersion: EngineVersion, StrategyOK: true},
		{Type: "BIAS", Pane: "oscillator", Params: map[string]int{"n": 6}, ParamRange: map[string][2]int{"n": {2, 500}}, Outputs: []string{"bias"}, Warmup: "N", FormulaVersion: EngineVersion, StrategyOK: true},
		{Type: "CCI", Pane: "oscillator", Params: map[string]int{"n": 14}, ParamRange: map[string][2]int{"n": {2, 500}}, Outputs: []string{"cci"}, Warmup: "N", FormulaVersion: EngineVersion, StrategyOK: true},
		{Type: "ATR", Pane: "oscillator", Params: map[string]int{"n": 14}, ParamRange: map[string][2]int{"n": {2, 500}}, Outputs: []string{"atr"}, Warmup: "N", FormulaVersion: EngineVersion, StrategyOK: true},
		{Type: "OBV", Pane: "oscillator", Params: map[string]int{}, ParamRange: map[string][2]int{}, Outputs: []string{"obv"}, Warmup: "2", FormulaVersion: EngineVersion, StrategyOK: true},
	}
	sort.Slice(r, func(i, j int) bool { return r[i].Type < r[j].Type })
	return r
}

func Lookup(typ string) (Descriptor, bool) {
	t := strings.ToUpper(strings.TrimSpace(typ))
	for _, d := range Registry() {
		if d.Type == t {
			return d, true
		}
	}
	return Descriptor{}, false
}

func NormalizeSpec(s Spec) (Spec, int, error) {
	d, ok := Lookup(s.Type)
	if !ok {
		return Spec{}, 0, fmt.Errorf("unknown indicator %s", s.Type)
	}
	s.Type = d.Type
	if s.Params == nil {
		s.Params = map[string]int{}
	}
	out := map[string]int{}
	for k, def := range d.Params {
		v, exists := s.Params[k]
		if !exists {
			v = def
		}
		if rng, ok := d.ParamRange[k]; ok {
			if v < rng[0] || v > rng[1] {
				return Spec{}, 0, fmt.Errorf("indicator %s param %s out of range", s.Type, k)
			}
		}
		out[k] = v
	}
	s.Params = out
	if s.ID == "" {
		s.ID = strings.ToLower(s.Type)
	}
	return s, warmupOf(s), nil
}

func warmupOf(s Spec) int {
	switch s.Type {
	case "MA", "EMA", "BOLL", "WR", "BIAS", "CCI", "ATR", "KDJ":
		return s.Params["n"]
	case "RSI":
		return s.Params["n"] + 1
	case "OBV":
		return 2
	case "VOL":
		w := s.Params["ma5"]
		if s.Params["ma10"] > w {
			w = s.Params["ma10"]
		}
		return w
	case "MACD":
		return s.Params["slow"] + s.Params["signal"] - 1
	default:
		return 1
	}
}

func Compute(bars []Bar, spec Spec) (Series, error) {
	norm, warmup, err := NormalizeSpec(spec)
	if err != nil {
		return Series{}, err
	}
	n := len(bars)
	out := Series{
		ID: norm.ID, Type: norm.Type, Params: norm.Params,
		Fields: map[string][]*decimal.Decimal{}, Ready: make([]bool, n),
		Warmup: warmup, EngineVersion: EngineVersion,
	}
	if n > 0 {
		start := warmup - 1
		if start < 0 {
			start = 0
		}
		if start >= n {
			out.CalculationStart = ""
		} else {
			out.CalculationStart = bars[start].Time
		}
	}
	switch norm.Type {
	case "MA":
		out.Fields["ma"] = smaClose(bars, norm.Params["n"], out.Ready)
	case "EMA":
		out.Fields["ema"] = emaClose(bars, norm.Params["n"], out.Ready)
	case "BOLL":
		mid, upper, lower := boll(bars, norm.Params["n"], norm.Params["k"], out.Ready)
		out.Fields["mid"], out.Fields["upper"], out.Fields["lower"] = mid, upper, lower
	case "VOL":
		vol, ma5, ma10 := volSeries(bars, norm.Params["ma5"], norm.Params["ma10"], out.Ready)
		out.Fields["volume"], out.Fields["ma5"], out.Fields["ma10"] = vol, ma5, ma10
	case "MACD":
		dif, dea, hist := macd(bars, norm.Params["fast"], norm.Params["slow"], norm.Params["signal"], out.Ready)
		out.Fields["dif"], out.Fields["dea"], out.Fields["hist"] = dif, dea, hist
	case "KDJ":
		k, d, j := kdj(bars, norm.Params["n"], norm.Params["m1"], norm.Params["m2"], out.Ready)
		out.Fields["k"], out.Fields["d"], out.Fields["j"] = k, d, j
	case "RSI":
		out.Fields["rsi"] = rsi(bars, norm.Params["n"], out.Ready)
	case "WR":
		out.Fields["wr"] = wr(bars, norm.Params["n"], out.Ready)
	case "BIAS":
		out.Fields["bias"] = bias(bars, norm.Params["n"], out.Ready)
	case "CCI":
		out.Fields["cci"] = cci(bars, norm.Params["n"], out.Ready)
	case "ATR":
		out.Fields["atr"] = atr(bars, norm.Params["n"], out.Ready)
	case "OBV":
		out.Fields["obv"] = obv(bars, out.Ready)
	}
	return out, nil
}

func ComputeMany(bars []Bar, specs []Spec) ([]Series, error) {
	out := make([]Series, 0, len(specs))
	for _, s := range specs {
		row, err := Compute(bars, s)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

func Strings(points []*decimal.Decimal) []*string {
	out := make([]*string, len(points))
	for i, p := range points {
		if p == nil {
			continue
		}
		s := p.String()
		out[i] = &s
	}
	return out
}
