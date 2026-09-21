package strategy

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/shopspring/decimal"

	"zhigu/server/service/finance"
	"zhigu/server/service/indicators"
)

const MaxInputRunes = 4000

type GenerateInput struct {
	Text         string
	InstrumentID string
}

type GenerateResult struct {
	Status        string    `json:"status"`
	Document      *Document `json:"document,omitempty"`
	Assumptions   []string  `json:"assumptions"`
	Questions     []string  `json:"questions,omitempty"`
	ErrorCode     string    `json:"error_code,omitempty"`
	ErrorMessage  string    `json:"error_message,omitempty"`
	PromptVersion string    `json:"prompt_version"`
	Source        string    `json:"source"`
}

func GenerateFromText(in GenerateInput) GenerateResult {
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return failGen("STRATEGY_INVALID", "请输入策略描述")
	}
	if utf8.RuneCountInString(text) > MaxInputRunes {
		return failGen("STRATEGY_INVALID", "描述超过 4000 字")
	}
	if forbidden(text) {
		return GenerateResult{
			Status: "unsupported", ErrorCode: "STRATEGY_UNSUPPORTED",
			ErrorMessage:  "不能使用未来信息、外部请求或任意代码作为交易条件",
			PromptVersion: PromptVersion, Source: "heuristic",
		}
	}
	inst := strings.TrimSpace(in.InstrumentID)
	if inst == "" {
		inst = InferInstrumentID(text)
	}
	if inst == "" {
		return GenerateResult{
			Status:        "needs_clarification",
			Questions:     []string{"请先选择要交易的股票，或在描述里写上代码/名称，例如 茅台金叉买入。"},
			PromptVersion: PromptVersion, Source: "heuristic",
		}
	}
	name := strings.TrimSpace(inst + " 日线策略")
	if strings.Contains(text, "腾讯") {
		name = "腾讯 MACD 与 KDJ 日线策略"
	}

	specs, assume := specsFrom(text)
	entry := entryFrom(text, specs)
	exit := exitFrom(text, specs)
	pos := "1"
	if strings.Contains(text, "半仓") || strings.Contains(text, "一半") {
		pos = "0.5"
	} else {
		assume = append(assume, "未指定仓位，默认满仓 1.0")
	}
	var stop *string
	if m := regexp.MustCompile(`亏损(?:达到|超过)?\s*(\d+(?:\.\d+)?)\s*%`).FindStringSubmatch(text); len(m) == 2 {
		v := scalePct(m[1])
		stop = &v
	} else if strings.Contains(text, "止损") && !regexp.MustCompile(`\d`).MatchString(text) {
		return GenerateResult{
			Status:        "needs_clarification",
			Questions:     []string{"止损需要明确的百分比，例如收盘亏损 5%。"},
			PromptVersion: PromptVersion, Source: "heuristic",
		}
	}
	doc := Document{
		SchemaVersion: SchemaVersion,
		Name:          name,
		InstrumentID:  inst,
		SignalPeriod:  "1d",
		PriceBasis:    "causal_qfq",
		Indicators:    specs,
		Entry:         entry,
		Exit:          exit,
		Position:      Position{Type: "equity_fraction", Value: pos},
		Risk:          Risk{Check: "close", StopLossPct: stop},
		Execution:     Execution{Timing: "next_session_open", Priority: "exit_first"},
	}
	assume = append(assume, "价格基准默认 causal_qfq", "信号于次一交易日开盘模拟成交")
	if _, err := Compile(doc); err != nil {
		ae := finance.AsAppError(err)
		return failGen(ae.Code, ae.Message)
	}
	return GenerateResult{Status: "ready", Document: &doc, Assumptions: assume, PromptVersion: PromptVersion, Source: "heuristic"}
}

func NameQuery(text string) string {
	stripped := regexp.MustCompile(`(?i)金叉|死叉|上穿|下穿|买入|卖出|开仓|平仓|满仓|半仓|止损|超买|超卖|低位|高位|均线|策略|回测|日线|周线|月线|macd|kdj|rsi|ema|boll|atr|cci|bias|obv|\bma\b|[0-9.％%]+`).ReplaceAllString(text, " ")
	return strings.Join(strings.Fields(stripped), "")
}

func InferInstrumentID(text string) string {
	if m := regexp.MustCompile(`(?i)\b(\d{6}\.(?:SH|SZ|BJ)|\d{5}\.HK)\b`).FindString(text); m != "" {
		return strings.ToUpper(m)
	}
	aliases := []struct{ k, v string }{
		{"贵州茅台", "600519.SH"},
		{"茅台", "600519.SH"},
		{"宁德时代", "300750.SZ"},
		{"宁德", "300750.SZ"},
		{"美的", "000333.SZ"},
		{"腾讯控股", "00700.HK"},
		{"腾讯", "00700.HK"},
	}
	for _, a := range aliases {
		if strings.Contains(text, a.k) {
			return a.v
		}
	}
	if m := regexp.MustCompile(`(?:^|[^\d.])(\d{6})(?:[^\d.]|$)`).FindStringSubmatch(text); len(m) == 2 {
		code := m[1]
		switch {
		case strings.HasPrefix(code, "6"):
			return code + ".SH"
		case strings.HasPrefix(code, "4"), strings.HasPrefix(code, "8"), strings.HasPrefix(code, "9"):
			return code + ".BJ"
		default:
			return code + ".SZ"
		}
	}
	if (strings.Contains(text, "港股") || strings.Contains(strings.ToUpper(text), "HK")) && regexp.MustCompile(`(?:^|[^\d.])(\d{5})(?:[^\d.]|$)`).MatchString(text) {
		code := regexp.MustCompile(`(?:^|[^\d.])(\d{5})(?:[^\d.]|$)`).FindStringSubmatch(text)[1]
		return code + ".HK"
	}
	return ""
}

func specsFrom(text string) ([]indicators.Spec, []string) {
	up := strings.ToUpper(text)
	var specs []indicators.Spec
	var assume []string
	wantMA := strings.Contains(text, "均线") || (strings.Contains(up, "MA") && !strings.Contains(up, "MACD"))
	wantMACD := strings.Contains(up, "MACD")
	wantKDJ := strings.Contains(up, "KDJ") || strings.Contains(text, "超卖") || strings.Contains(text, "超买")
	wantRSI := strings.Contains(up, "RSI")
	if (strings.Contains(text, "金叉") || strings.Contains(text, "死叉") || strings.Contains(text, "上穿") || strings.Contains(text, "下穿")) && !wantMA && !wantMACD && !wantKDJ {
		wantMACD = true
		assume = append(assume, "未指明金叉指标，默认 MACD 12/26/9")
	}
	if wantMA {
		specs = append(specs,
			indicators.Spec{ID: "ma5", Type: "MA", Params: map[string]int{"n": 5}},
			indicators.Spec{ID: "ma20", Type: "MA", Params: map[string]int{"n": 20}},
		)
		assume = append(assume, "均线未指定周期，使用 MA5 与 MA20")
	}
	if wantMACD {
		specs = append(specs, indicators.Spec{ID: "macd", Type: "MACD", Params: map[string]int{"fast": 12, "slow": 26, "signal": 9}})
		assume = append(assume, "MACD 参数未指定，使用 12/26/9")
	}
	if wantKDJ {
		specs = append(specs, indicators.Spec{ID: "kdj", Type: "KDJ", Params: map[string]int{"n": 9, "m1": 3, "m2": 3}})
		assume = append(assume, "KDJ 参数未指定，使用 9/3/3")
	}
	if wantRSI {
		specs = append(specs, indicators.Spec{ID: "rsi", Type: "RSI", Params: map[string]int{"n": 12}})
		assume = append(assume, "RSI 参数未指定，使用 12")
	}
	if len(specs) == 0 {
		specs = []indicators.Spec{
			{ID: "macd", Type: "MACD", Params: map[string]int{"fast": 12, "slow": 26, "signal": 9}},
			{ID: "kdj", Type: "KDJ", Params: map[string]int{"n": 9, "m1": 3, "m2": 3}},
		}
		assume = append(assume, "未指明指标，默认 MACD 12/26/9 与 KDJ 9/3/3")
	}
	return specs, assume
}

func scalePct(v string) string {
	d := decimal.RequireFromString(v)
	return d.Div(decimal.NewFromInt(100)).String()
}

func hasType(specs []indicators.Spec, typ string) bool {
	for _, s := range specs {
		if s.Type == typ {
			return true
		}
	}
	return false
}

func entryFrom(text string, specs []indicators.Spec) json.RawMessage {
	parts := []map[string]any{}
	up := strings.ToUpper(text)
	if hasType(specs, "MA") && (strings.Contains(text, "金叉") || strings.Contains(text, "上穿") || strings.Contains(text, "均线")) {
		parts = append(parts, map[string]any{"op": "crosses_above", "left": "ma5.ma", "right": "ma20.ma"})
	}
	if hasType(specs, "MACD") && (strings.Contains(text, "金叉") || strings.Contains(text, "上穿") || strings.Contains(up, "MACD")) {
		parts = append(parts, map[string]any{"op": "crosses_above", "left": "macd.dif", "right": "macd.dea"})
	}
	if hasType(specs, "KDJ") {
		thr := "20"
		if m := regexp.MustCompile(`K[^0-9]{0,6}(?:小于|低于|<)\s*(\d+)`).FindStringSubmatch(text); len(m) == 2 {
			thr = m[1]
		} else if strings.Contains(text, "超卖") || strings.Contains(text, "低位") {
			thr = "20"
		}
		if strings.Contains(up, "KDJ") || strings.Contains(text, "超卖") || strings.Contains(text, "低位") || regexp.MustCompile(`K[^0-9]{0,6}(?:小于|低于|<)`).MatchString(text) {
			parts = append(parts, map[string]any{"op": "lt", "left": "kdj.k", "right": map[string]string{"constant": thr}})
		}
	}
	if hasType(specs, "RSI") {
		if m := regexp.MustCompile(`RSI[^0-9]{0,6}(?:小于|低于|<)\s*(\d+)`).FindStringSubmatch(up); len(m) == 2 {
			parts = append(parts, map[string]any{"op": "lt", "left": "rsi.rsi", "right": map[string]string{"constant": m[1]}})
		}
	}
	if len(parts) == 0 && hasType(specs, "MACD") {
		parts = append(parts, map[string]any{"op": "crosses_above", "left": "macd.dif", "right": "macd.dea"})
	}
	if len(parts) == 0 {
		parts = append(parts, map[string]any{"op": "gt", "left": "close", "right": map[string]string{"constant": "0"}})
	}
	if len(parts) == 1 {
		b, _ := json.Marshal(parts[0])
		return b
	}
	b, _ := json.Marshal(map[string]any{"all": parts})
	return b
}

func exitFrom(text string, specs []indicators.Spec) json.RawMessage {
	if hasType(specs, "MA") && (strings.Contains(text, "死叉") || strings.Contains(text, "下穿") || strings.Contains(text, "均线")) {
		b, _ := json.Marshal(map[string]any{"op": "crosses_below", "left": "ma5.ma", "right": "ma20.ma"})
		return b
	}
	if hasType(specs, "MACD") {
		b, _ := json.Marshal(map[string]any{"op": "crosses_below", "left": "macd.dif", "right": "macd.dea"})
		return b
	}
	b, _ := json.Marshal(map[string]any{"op": "lt", "left": "close", "right": map[string]string{"constant": "0"}})
	return b
}

func forbidden(text string) bool {
	keys := []string{"未来三天", "未来会涨", "读取文件", "发送请求", "http://", "https://", "eval(", "os.system", "import os", "exec("}
	for _, k := range keys {
		if strings.Contains(strings.ToLower(text), strings.ToLower(k)) {
			return true
		}
	}
	return false
}

func failGen(code, msg string) GenerateResult {
	return GenerateResult{Status: "failed", ErrorCode: code, ErrorMessage: msg, PromptVersion: PromptVersion, Source: "heuristic"}
}
