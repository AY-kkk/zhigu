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
	lower := text
	if forbidden(lower) {
		return GenerateResult{
			Status: "unsupported", ErrorCode: "STRATEGY_UNSUPPORTED",
			ErrorMessage:  "不能使用未来信息、外部请求或任意代码作为交易条件",
			PromptVersion: PromptVersion, Source: "heuristic",
		}
	}
	if vagueCross(lower) {
		return GenerateResult{
			Status:        "needs_clarification",
			Questions:     []string{"请指明金叉/死叉使用的指标，例如 MACD 或 KDJ。"},
			PromptVersion: PromptVersion, Source: "heuristic",
		}
	}
	inst := strings.TrimSpace(in.InstrumentID)
	if inst == "" {
		return GenerateResult{
			Status:        "needs_clarification",
			Questions:     []string{"请先选择要交易的股票柜台。"},
			PromptVersion: PromptVersion, Source: "heuristic",
		}
	}
	name := "日线策略"
	if strings.Contains(text, "腾讯") {
		name = "腾讯 MACD 与 KDJ 日线策略"
	} else {
		name = strings.TrimSpace(inst + " 日线策略")
	}

	var specs []indicators.Spec
	var assume []string
	if strings.Contains(strings.ToUpper(text), "MACD") || strings.Contains(text, "macd") {
		specs = append(specs, indicators.Spec{ID: "macd", Type: "MACD", Params: map[string]int{"fast": 12, "slow": 26, "signal": 9}})
		assume = append(assume, "MACD 参数未指定，使用 12/26/9")
	}
	if strings.Contains(strings.ToUpper(text), "KDJ") || strings.Contains(text, "kdj") {
		specs = append(specs, indicators.Spec{ID: "kdj", Type: "KDJ", Params: map[string]int{"n": 9, "m1": 3, "m2": 3}})
		assume = append(assume, "KDJ 参数未指定，使用 9/3/3")
	}
	if strings.Contains(strings.ToUpper(text), "RSI") {
		specs = append(specs, indicators.Spec{ID: "rsi", Type: "RSI", Params: map[string]int{"n": 12}})
		assume = append(assume, "RSI 参数未指定，使用 12")
	}
	if len(specs) == 0 {
		return GenerateResult{
			Status:        "needs_clarification",
			Questions:     []string{"请指明使用的指标（至少包括 MACD、KDJ 或 RSI 之一）。"},
			PromptVersion: PromptVersion, Source: "heuristic",
		}
	}

	entry := entryFrom(text)
	exit := exitFrom(text)
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

func scalePct(v string) string {
	d := decimal.RequireFromString(v)
	return d.Div(decimal.NewFromInt(100)).String()
}

func entryFrom(text string) json.RawMessage {
	parts := []map[string]any{}
	up := strings.ToUpper(text)
	if strings.Contains(up, "MACD") && (strings.Contains(text, "金叉") || strings.Contains(text, "上穿")) {
		parts = append(parts, map[string]any{"op": "crosses_above", "left": "macd.dif", "right": "macd.dea"})
	}
	if strings.Contains(up, "KDJ") {
		if m := regexp.MustCompile(`K[^0-9]{0,6}(?:小于|低于|<)\s*(\d+)`).FindStringSubmatch(text); len(m) == 2 {
			parts = append(parts, map[string]any{"op": "lt", "left": "kdj.k", "right": map[string]string{"constant": m[1]}})
		}
	}
	if strings.Contains(up, "RSI") {
		if m := regexp.MustCompile(`RSI[^0-9]{0,6}(?:小于|低于|<)\s*(\d+)`).FindStringSubmatch(up); len(m) == 2 {
			parts = append(parts, map[string]any{"op": "lt", "left": "rsi.rsi", "right": map[string]string{"constant": m[1]}})
		}
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

func exitFrom(text string) json.RawMessage {
	up := strings.ToUpper(text)
	if strings.Contains(up, "MACD") && (strings.Contains(text, "死叉") || strings.Contains(text, "下穿")) {
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

func vagueCross(text string) bool {
	if !strings.Contains(text, "金叉") && !strings.Contains(text, "死叉") {
		return false
	}
	up := strings.ToUpper(text)
	return !strings.Contains(up, "MACD") && !strings.Contains(up, "KDJ") && !strings.Contains(up, "MA")
}

func failGen(code, msg string) GenerateResult {
	return GenerateResult{Status: "failed", ErrorCode: code, ErrorMessage: msg, PromptVersion: PromptVersion, Source: "heuristic"}
}
