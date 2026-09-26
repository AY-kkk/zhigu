package strategy

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/shopspring/decimal"

	"zhigu/server/service/finance"
)

// changeTargets lists the fields an explicit change goal is allowed to touch
// (§12.4.8: 指定局部修改时只修改目标字段及其必要依赖).
type changeTargets struct {
	Position      *string
	StopLoss      *string
	StopLossSet   bool
	TakeProfit    *string
	TakeProfitSet bool
	ThresholdRef  string
	ThresholdDir  string // lt | gt
	ThresholdVal  string
	ThresholdSet  bool
}

func (t changeTargets) empty() bool {
	return t.Position == nil && !t.StopLossSet && !t.TakeProfitSet && !t.ThresholdSet
}

// ModifyEditorState applies ONLY the explicit change goal to the frozen editor
// state; every other trading semantic is preserved byte-semantically.
func ModifyEditorState(base EditorState, text string) (EditorState, changeTargets, []string, error) {
	if strings.TrimSpace(text) == "" {
		return EditorState{}, changeTargets{}, nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "缺少改动目标")
	}
	if forbidden(text) {
		return EditorState{}, changeTargets{}, nil, finance.NewError(422, "validation", "STRATEGY_UNSUPPORTED", "不能使用未来信息、外部请求或任意代码作为交易条件")
	}
	doc := cloneEditorState(base)
	var targets changeTargets
	var assume []string
	if v, ok := parsePositionChange(text); ok {
		doc.Position = Position{Type: "equity_fraction", Value: v}
		targets.Position = &v
		assume = append(assume, "按改动目标将仓位设为 "+v)
	}
	if v, set, ok := parsePctChange(text, "止损"); ok {
		doc.Risk.StopLossPct = v
		targets.StopLoss, targets.StopLossSet = v, set
		if set {
			assume = append(assume, "按改动目标更新止损")
		} else {
			assume = append(assume, "按改动目标停用止损")
		}
	}
	if v, set, ok := parsePctChange(text, "止盈"); ok {
		doc.Risk.TakeProfitPct = v
		targets.TakeProfit, targets.TakeProfitSet = v, set
		if set {
			assume = append(assume, "按改动目标更新止盈")
		} else {
			assume = append(assume, "按改动目标停用止盈")
		}
	}
	if ref, dir, val, ok := parseThresholdChange(text); ok {
		targets.ThresholdRef, targets.ThresholdDir, targets.ThresholdVal, targets.ThresholdSet = ref, dir, val, true
		node := &doc.Entry
		if dir == "gt" {
			node = &doc.Exit
		}
		applyThreshold(node, ref, dir, val)
		assume = append(assume, "按改动目标更新阈值 "+ref)
	}
	if targets.empty() {
		assume = append(assume, "未识别到明确的改动目标，保持原规则不变")
	}
	return doc, targets, assume, nil
}

// ApplyTargets overlays ONLY the targeted fields from the model proposal onto
// the frozen base state, so untouched semantics can never drift (R2 guard).
func ApplyTargets(base, proposal EditorState, t changeTargets) EditorState {
	out := cloneEditorState(base)
	if t.Position != nil {
		if v, ok := parseFraction(proposal.Position.Value); ok {
			out.Position = Position{Type: "equity_fraction", Value: v}
		} else {
			out.Position = Position{Type: "equity_fraction", Value: *t.Position}
		}
	}
	if t.StopLossSet {
		out.Risk.StopLossPct = pickPct(proposal.Risk.StopLossPct, t.StopLoss)
	}
	if t.TakeProfitSet {
		out.Risk.TakeProfitPct = pickPct(proposal.Risk.TakeProfitPct, t.TakeProfit)
	}
	if t.ThresholdSet {
		node := &out.Entry
		propNode := proposal.Entry
		if t.ThresholdDir == "gt" {
			node = &out.Exit
			propNode = proposal.Exit
		}
		if val, ok := thresholdFrom(propNode, t.ThresholdRef, t.ThresholdDir); ok {
			applyThreshold(node, t.ThresholdRef, t.ThresholdDir, val)
		} else {
			applyThreshold(node, t.ThresholdRef, t.ThresholdDir, t.ThresholdVal)
		}
	}
	return out
}

func pickPct(proposal, fallback *string) *string {
	if proposal != nil && strings.TrimSpace(*proposal) != "" {
		return proposal
	}
	return fallback
}

func cloneEditorState(in EditorState) EditorState {
	raw, _ := json.Marshal(in)
	var out EditorState
	_ = json.Unmarshal(raw, &out)
	return out
}

func parsePositionChange(text string) (string, bool) {
	if regexp.MustCompile(`满仓|全仓`).MatchString(text) {
		return "1", true
	}
	if regexp.MustCompile(`半仓|一半|五成`).MatchString(text) {
		return "0.5", true
	}
	if m := regexp.MustCompile(`仓位[^0-9]{0,8}(\d+(?:\.\d+)?)\s*成`).FindStringSubmatch(text); len(m) == 2 {
		d := decimal.RequireFromString(m[1]).Div(decimal.NewFromInt(10))
		return d.String(), true
	}
	if m := regexp.MustCompile(`仓位[^0-9]{0,8}(\d+(?:\.\d+)?)\s*%`).FindStringSubmatch(text); len(m) == 2 {
		d := decimal.RequireFromString(m[1]).Div(decimal.NewFromInt(100))
		return d.String(), true
	}
	if m := regexp.MustCompile(`仓位[^0-9]{0,8}(0?\.\d+|1(?:\.0+)?)`).FindStringSubmatch(text); len(m) == 2 {
		if v, ok := parseFraction(m[1]); ok {
			return v, true
		}
	}
	return "", false
}

func parseFraction(s string) (string, bool) {
	d, err := decimal.NewFromString(strings.TrimSpace(s))
	if err != nil || d.LessThanOrEqual(decimal.Zero) || d.GreaterThan(decimal.NewFromInt(1)) {
		return "", false
	}
	return d.String(), true
}

// parsePctChange: (value, explicitlySet, matched). "取消/不启用" → (nil,true).
func parsePctChange(text, key string) (*string, bool, bool) {
	if regexp.MustCompile(key + `[^0-9]{0,8}(取消|不启用)`).MatchString(text) {
		return nil, true, true
	}
	esc := regexp.QuoteMeta(key)
	if m := regexp.MustCompile(esc + `[^0-9]{0,8}(\d+(?:\.\d+)?)\s*%`).FindStringSubmatch(text); len(m) == 2 {
		v := decimal.RequireFromString(m[1]).Div(decimal.NewFromInt(100))
		s := v.String()
		return &s, true, true
	}
	return nil, false, false
}

func parseThresholdChange(text string) (ref, dir, val string, ok bool) {
	m := regexp.MustCompile(`(?i)(J|K|D|RSI)\s*值?[^0-9]{0,6}(小于|低于|<|大于|高于|>)\s*(\d+(?:\.\d+)?)`).FindStringSubmatch(text)
	if len(m) != 4 {
		return "", "", "", false
	}
	ref = map[string]string{"J": "kdj.j", "K": "kdj.k", "D": "kdj.d", "RSI": "rsi.rsi"}[strings.ToUpper(m[1])]
	switch m[2] {
	case "小于", "低于", "<":
		dir = "lt"
	default:
		dir = "gt"
	}
	return ref, dir, m[3], true
}

// applyThreshold rewrites constant operands of matching comparison leaves in a
// condition tree (editor source form: groups, leaves with repeat, shortcuts).
func applyThreshold(node *json.RawMessage, ref, dir, val string) {
	if node == nil {
		return
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(*node, &obj) != nil || obj == nil {
		return
	}
	for _, key := range []string{"all", "any"} {
		if list, ok := obj[key]; ok {
			var items []json.RawMessage
			_ = json.Unmarshal(list, &items)
			for i := range items {
				applyThreshold(&items[i], ref, dir, val)
			}
			raw, _ := json.Marshal(items)
			obj[key] = raw
			out, _ := json.Marshal(obj)
			*node = out
			return
		}
	}
	if not, ok := obj["not"]; ok {
		applyThreshold(&not, ref, dir, val)
		obj["not"] = not
		out, _ := json.Marshal(obj)
		*node = out
		return
	}
	if _, ok := obj["kind"]; ok {
		return
	}
	op := strings.Trim(string(obj["op"]), `"`)
	matches := (dir == "lt" && (op == "lt" || op == "lte")) || (dir == "gt" && (op == "gt" || op == "gte"))
	if !matches {
		return
	}
	if operandRef(obj["left"]) == ref {
		if isConstantOperand(obj["right"]) {
			obj["right"] = json.RawMessage(`{"constant":"` + val + `"}`)
		}
	}
	if operandRef(obj["right"]) == ref {
		if isConstantOperand(obj["left"]) {
			obj["left"] = json.RawMessage(`{"constant":"` + val + `"}`)
		}
	}
	out, _ := json.Marshal(obj)
	*node = out
}

// thresholdFrom reads a matching leaf's constant from a proposal tree.
func thresholdFrom(node json.RawMessage, ref, dir string) (string, bool) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(node, &obj) != nil || obj == nil {
		return "", false
	}
	for _, key := range []string{"all", "any"} {
		if list, ok := obj[key]; ok {
			var items []json.RawMessage
			_ = json.Unmarshal(list, &items)
			for _, it := range items {
				if v, ok := thresholdFrom(it, ref, dir); ok {
					return v, true
				}
			}
			return "", false
		}
	}
	if not, ok := obj["not"]; ok {
		return thresholdFrom(not, ref, dir)
	}
	op := strings.Trim(string(obj["op"]), `"`)
	matches := (dir == "lt" && (op == "lt" || op == "lte")) || (dir == "gt" && (op == "gt" || op == "gte"))
	if !matches {
		return "", false
	}
	if operandRef(obj["left"]) == ref && isConstantOperand(obj["right"]) {
		var c struct {
			Constant string `json:"constant"`
		}
		_ = json.Unmarshal(obj["right"], &c)
		return c.Constant, true
	}
	if operandRef(obj["right"]) == ref && isConstantOperand(obj["left"]) {
		var c struct {
			Constant string `json:"constant"`
		}
		_ = json.Unmarshal(obj["left"], &c)
		return c.Constant, true
	}
	return "", false
}

func operandRef(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil && obj != nil {
		if ref, ok := obj["ref"].(string); ok {
			return ref
		}
	}
	return ""
}

func isConstantOperand(raw json.RawMessage) bool {
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil && obj != nil {
		_, ok := obj["constant"]
		return ok
	}
	return false
}
