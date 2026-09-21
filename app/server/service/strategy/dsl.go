package strategy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"zhigu/server/service/finance"
	"zhigu/server/service/indicators"
)

const (
	SchemaVersion   = "strategy.v1"
	CompilerVersion = "strategy.compile.v1"
	PromptVersion   = "strategy.prompt.v1"
	MaxIndicators   = 20
	MaxCondNodes    = 100
	MaxCondDepth    = 8
	MaxLag          = 250
)

type Document struct {
	SchemaVersion string            `json:"schema_version"`
	Name          string            `json:"name"`
	InstrumentID  string            `json:"instrument_id"`
	SignalPeriod  string            `json:"signal_period"`
	PriceBasis    string            `json:"price_basis"`
	Indicators    []indicators.Spec `json:"indicators"`
	Entry         json.RawMessage   `json:"entry"`
	Exit          json.RawMessage   `json:"exit"`
	Position      Position          `json:"position"`
	Risk          Risk              `json:"risk"`
	Execution     Execution         `json:"execution"`
}

type Position struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type Risk struct {
	Check          string  `json:"check"`
	StopLossPct    *string `json:"stop_loss_pct"`
	TakeProfitPct  *string `json:"take_profit_pct"`
	MaxHoldingBars *int    `json:"max_holding_bars"`
}

type Execution struct {
	Timing   string `json:"timing"`
	Priority string `json:"priority"`
}

type Compiled struct {
	Normalized      Document `json:"normalized_rules"`
	Assumptions     []string `json:"assumptions"`
	RequiredData    []string `json:"required_data"`
	WarmupBars      int      `json:"warmup_bars"`
	CapabilityErrs  []string `json:"capability_errors"`
	CompilerVersion string   `json:"compiler_version"`
}

func ParseDSL(raw []byte) (Document, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var doc Document
	if err := dec.Decode(&doc); err != nil {
		return Document{}, finance.NewError(422, "validation", "STRATEGY_INVALID", "策略 JSON 无法解析或含未知字段")
	}
	return doc, nil
}

func Compile(doc Document) (Compiled, error) {
	var caps []string
	var assume []string
	if doc.SchemaVersion != SchemaVersion {
		return Compiled{}, finance.NewError(422, "validation", "STRATEGY_INVALID", "schema_version 必须为 strategy.v1")
	}
	if strings.TrimSpace(doc.Name) == "" || strings.TrimSpace(doc.InstrumentID) == "" {
		return Compiled{}, finance.NewError(422, "validation", "STRATEGY_INVALID", "缺少名称或标的")
	}
	if doc.SignalPeriod != "1d" {
		caps = append(caps, "当前仅支持日线信号，周月信号尚未开放")
		return compiledErr(doc, assume, caps), finance.NewError(422, "validation", "STRATEGY_UNSUPPORTED", "当前仅支持日线信号")
	}
	switch doc.PriceBasis {
	case "":
		doc.PriceBasis = "causal_qfq"
		assume = append(assume, "未指定价格基准，默认 causal_qfq")
	case "raw", "causal_qfq":
	default:
		return Compiled{}, finance.NewError(422, "validation", "STRATEGY_INVALID", "price_basis 仅支持 raw 或 causal_qfq")
	}
	if len(doc.Indicators) == 0 || len(doc.Indicators) > MaxIndicators {
		return Compiled{}, finance.NewError(422, "validation", "STRATEGY_INVALID", "指标数量须为 1～20")
	}
	ids := map[string]indicators.Spec{}
	warmup := 0
	for i, spec := range doc.Indicators {
		norm, w, err := indicators.NormalizeSpec(spec)
		if err != nil {
			return Compiled{}, finance.NewError(422, "validation", "STRATEGY_INVALID", err.Error())
		}
		if _, dup := ids[norm.ID]; dup {
			return Compiled{}, finance.NewError(422, "validation", "STRATEGY_INVALID", "指标 id 重复")
		}
		ids[norm.ID] = norm
		doc.Indicators[i] = norm
		if w > warmup {
			warmup = w
		}
	}
	warmup++ // extra bar for cross
	nodes := 0
	if err := walkCond(doc.Entry, ids, 0, &nodes); err != nil {
		return Compiled{}, err
	}
	if err := walkCond(doc.Exit, ids, 0, &nodes); err != nil {
		return Compiled{}, err
	}
	if nodes > MaxCondNodes {
		return Compiled{}, finance.NewError(422, "validation", "STRATEGY_INVALID", "条件节点超过 100")
	}
	if doc.Position.Type != "equity_fraction" {
		return Compiled{}, finance.NewError(422, "validation", "STRATEGY_INVALID", "仓位类型仅支持 equity_fraction")
	}
	frac, err := decimal.NewFromString(strings.TrimSpace(doc.Position.Value))
	if err != nil || frac.LessThanOrEqual(decimal.Zero) || frac.GreaterThan(decimal.NewFromInt(1)) {
		return Compiled{}, finance.NewError(422, "validation", "STRATEGY_INVALID", "仓位须为 0～1 的十进制字符串")
	}
	if doc.Risk.Check != "close" {
		return Compiled{}, finance.NewError(422, "validation", "STRATEGY_UNSUPPORTED", "风险检查仅支持收盘")
	}
	if err := pctOrNil(doc.Risk.StopLossPct, "stop_loss_pct"); err != nil {
		return Compiled{}, err
	}
	if err := pctOrNil(doc.Risk.TakeProfitPct, "take_profit_pct"); err != nil {
		return Compiled{}, err
	}
	if doc.Risk.MaxHoldingBars != nil && *doc.Risk.MaxHoldingBars < 1 {
		return Compiled{}, finance.NewError(422, "validation", "STRATEGY_INVALID", "max_holding_bars 须为正整数")
	}
	if doc.Execution.Timing != "next_session_open" || doc.Execution.Priority != "exit_first" {
		return Compiled{}, finance.NewError(422, "validation", "STRATEGY_UNSUPPORTED", "执行仅支持 next_session_open / exit_first")
	}
	req := []string{"ohlcv.1d.raw", "trading_calendar", "fee_schedule", "lot_tick_rules"}
	if doc.PriceBasis == "causal_qfq" {
		req = append(req, "corporate_actions.causal")
	}
	return Compiled{
		Normalized: doc, Assumptions: assume, RequiredData: req,
		WarmupBars: warmup, CapabilityErrs: caps, CompilerVersion: CompilerVersion,
	}, nil
}

func compiledErr(doc Document, assume, caps []string) Compiled {
	return Compiled{Normalized: doc, Assumptions: assume, CapabilityErrs: caps, CompilerVersion: CompilerVersion}
}

func pctOrNil(v *string, name string) error {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil
	}
	d, err := decimal.NewFromString(strings.TrimSpace(*v))
	if err != nil || d.LessThanOrEqual(decimal.Zero) || d.GreaterThan(decimal.NewFromInt(1)) {
		return finance.NewError(422, "validation", "STRATEGY_INVALID", name+" 须为 0～1 的比例")
	}
	return nil
}

func walkCond(raw json.RawMessage, ids map[string]indicators.Spec, depth int, nodes *int) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return finance.NewError(422, "validation", "STRATEGY_INVALID", "缺少入场或退出条件")
	}
	if depth > MaxCondDepth {
		return finance.NewError(422, "validation", "STRATEGY_INVALID", "条件嵌套超过 8 层")
	}
	*nodes++
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return finance.NewError(422, "validation", "STRATEGY_INVALID", "条件不是对象")
	}
	if all, ok := obj["all"]; ok {
		return walkList(all, ids, depth, nodes)
	}
	if any, ok := obj["any"]; ok {
		return walkList(any, ids, depth, nodes)
	}
	if not, ok := obj["not"]; ok {
		return walkCond(not, ids, depth+1, nodes)
	}
	op := strings.Trim(string(obj["op"]), `"`)
	switch op {
	case "gt", "gte", "lt", "lte", "eq", "crosses_above", "crosses_below":
	default:
		return finance.NewError(422, "validation", "STRATEGY_INVALID", "不支持的比较运算")
	}
	if err := walkOperand(obj["left"], ids); err != nil {
		return err
	}
	if err := walkOperand(obj["right"], ids); err != nil {
		return err
	}
	if lagRaw, ok := obj["lag"]; ok {
		var lag int
		if err := json.Unmarshal(lagRaw, &lag); err != nil || lag < 0 || lag > MaxLag {
			return finance.NewError(422, "validation", "STRATEGY_INVALID", "lag 仅允许 0～250")
		}
	}
	if _, ok := obj["expr"]; ok {
		return finance.NewError(422, "validation", "STRATEGY_INVALID", "禁止可执行表达式")
	}
	return nil
}

func walkList(raw json.RawMessage, ids map[string]indicators.Spec, depth int, nodes *int) error {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || len(items) == 0 {
		return finance.NewError(422, "validation", "STRATEGY_INVALID", "all/any 必须是非空数组")
	}
	for _, it := range items {
		if err := walkCond(it, ids, depth+1, nodes); err != nil {
			return err
		}
	}
	return nil
}

func walkOperand(raw json.RawMessage, ids map[string]indicators.Spec) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return finance.NewError(422, "validation", "STRATEGY_INVALID", "缺少操作数")
	}
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil && obj != nil {
		if c, ok := obj["constant"]; ok {
			s, _ := c.(string)
			if _, err := decimal.NewFromString(s); err != nil {
				return finance.NewError(422, "validation", "STRATEGY_INVALID", "常数必须是十进制字符串")
			}
			return nil
		}
		return finance.NewError(422, "validation", "STRATEGY_INVALID", "操作数对象仅允许 constant")
	}
	var ref string
	if err := json.Unmarshal(raw, &ref); err != nil {
		return finance.NewError(422, "validation", "STRATEGY_INVALID", "操作数无法解析")
	}
	return resolveRef(ref, ids)
}

func resolveRef(ref string, ids map[string]indicators.Spec) error {
	switch ref {
	case "open", "high", "low", "close", "volume":
		return nil
	}
	id, field, ok := strings.Cut(ref, ".")
	if !ok {
		return finance.NewError(422, "validation", "STRATEGY_INVALID", fmt.Sprintf("无法解析引用 %s", ref))
	}
	spec, exists := ids[id]
	if !exists {
		return finance.NewError(422, "validation", "STRATEGY_INVALID", "引用了未声明的指标")
	}
	d, _ := indicators.Lookup(spec.Type)
	for _, out := range d.Outputs {
		if out == field {
			return nil
		}
	}
	return finance.NewError(422, "validation", "STRATEGY_INVALID", "指标输出字段不存在")
}
