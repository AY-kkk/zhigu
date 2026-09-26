package strategy

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"zhigu/server/service/indicators"
)

func EvalCond(raw json.RawMessage, bars []indicators.Bar, series []indicators.Series, i int) (bool, bool) {
	return newEnv(bars, series).cond(raw, i)
}

type evalEnv struct {
	bars     []indicators.Bar
	series   map[string]indicators.Series
	priceOHL map[string][]*decimal.Decimal
	always   []bool
	shifts   map[string]shiftedSeries
	checks   []ContinuityCheck
}

func newEnv(bars []indicators.Bar, series []indicators.Series) *evalEnv {
	e := &evalEnv{
		bars:   bars,
		series: map[string]indicators.Series{},
		shifts: map[string]shiftedSeries{},
		priceOHL: map[string][]*decimal.Decimal{
			"open":   make([]*decimal.Decimal, len(bars)),
			"high":   make([]*decimal.Decimal, len(bars)),
			"low":    make([]*decimal.Decimal, len(bars)),
			"close":  make([]*decimal.Decimal, len(bars)),
			"volume": make([]*decimal.Decimal, len(bars)),
		},
		always: make([]bool, len(bars)),
	}
	for i, b := range bars {
		o, h, l, c, v := b.Open, b.High, b.Low, b.Close, b.Volume
		e.priceOHL["open"][i], e.priceOHL["high"][i], e.priceOHL["low"][i] = &o, &h, &l
		e.priceOHL["close"][i], e.priceOHL["volume"][i] = &c, &v
		e.always[i] = true
	}
	for _, s := range series {
		e.series[s.ID] = s
	}
	return e
}

func (e *evalEnv) cond(raw json.RawMessage, i int) (bool, bool) {
	return e.condAt(raw, i, "", false)
}

// condAt evaluates a node at the given expanded-tree path. With checked=true,
// continuity checks bound to that path gate the node itself, so an entry window
// never affects exit or sibling OR branches.
func (e *evalEnv) condAt(raw json.RawMessage, i int, path string, checked bool) (bool, bool) {
	if checked {
		for _, c := range e.checks {
			if relCheckPath(c.Path) == path {
				ok, ready := checkContinuity(c, e.bars, i)
				if !ready {
					return false, false
				}
				if !ok {
					return false, true
				}
			}
		}
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return false, false
	}
	if all, ok := obj["all"]; ok {
		var items []json.RawMessage
		_ = json.Unmarshal(all, &items)
		okAll := true
		ready := true
		for k, it := range items {
			v, r := e.condAt(it, i, joinPath(path, "all["+strconv.Itoa(k)+"]"), checked)
			ready = ready && r
			okAll = okAll && v
		}
		return okAll && ready, ready
	}
	if any, ok := obj["any"]; ok {
		var items []json.RawMessage
		_ = json.Unmarshal(any, &items)
		okAny := false
		ready := true
		for k, it := range items {
			v, r := e.condAt(it, i, joinPath(path, "any["+strconv.Itoa(k)+"]"), checked)
			ready = ready && r
			okAny = okAny || v
		}
		return okAny && ready, ready
	}
	if not, ok := obj["not"]; ok {
		v, r := e.condAt(not, i, joinPath(path, "not"), checked)
		return (!v) && r, r
	}
	op := strings.Trim(string(obj["op"]), `"`)
	lag := 0
	if lagRaw, ok := obj["lag"]; ok {
		_ = json.Unmarshal(lagRaw, &lag)
	}
	idx := i - lag
	if idx < 0 {
		return false, false
	}
	lv, lr, lready := e.operand(obj["left"], idx)
	rv, rr, rready := e.operand(obj["right"], idx)
	ready := lr && rr && lready && rready
	if !ready {
		return false, false
	}
	switch op {
	case "gt":
		return lv.GreaterThan(*rv), true
	case "gte":
		return lv.GreaterThanOrEqual(*rv), true
	case "lt":
		return lv.LessThan(*rv), true
	case "lte":
		return lv.LessThanOrEqual(*rv), true
	case "eq":
		return lv.Equal(*rv), true
	case "crosses_above":
		return e.cross(obj["left"], obj["right"], idx, true), true
	case "crosses_below":
		return e.cross(obj["left"], obj["right"], idx, false), true
	}
	return false, false
}

func (e *evalEnv) cross(left, right json.RawMessage, i int, above bool) bool {
	ls, lready, okL := e.seriesOf(left)
	rs, rready, okR := e.seriesOf(right)
	if !okL || !okR {
		return false
	}
	if above {
		return indicators.CrossesAbove(ls, rs, lready, rready, i)
	}
	return indicators.CrossesBelow(ls, rs, lready, rready, i)
}

func (e *evalEnv) operand(raw json.RawMessage, i int) (*decimal.Decimal, bool, bool) {
	vals, ready, ok := e.seriesOf(raw)
	if !ok || i < 0 || i >= len(vals) || vals[i] == nil {
		return nil, false, false
	}
	r := true
	if ready != nil && i < len(ready) {
		r = ready[i]
	}
	return vals[i], true, r
}

func (e *evalEnv) seriesOf(raw json.RawMessage) ([]*decimal.Decimal, []bool, bool) {
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil && obj != nil {
		if c, ok := obj["constant"]; ok {
			s, _ := c.(string)
			d := decimal.RequireFromString(s)
			vals := make([]*decimal.Decimal, len(e.bars))
			for i := range vals {
				cp := d
				vals[i] = &cp
			}
			return vals, e.always, true
		}
		if ref, ok := obj["ref"].(string); ok && ref != "" {
			lag := 0
			if lr, ok := obj["lag"]; ok {
				switch v := lr.(type) {
				case float64:
					lag = int(v)
				case int:
					lag = v
				}
			}
			vals, ready, ok := e.seriesOfRef(ref)
			if !ok {
				return nil, nil, false
			}
			if lag <= 0 {
				return vals, ready, true
			}
			return e.shifted(ref, lag, vals, ready)
		}
		return nil, nil, false
	}
	var ref string
	if json.Unmarshal(raw, &ref) != nil {
		return nil, nil, false
	}
	return e.seriesOfRef(ref)
}

func (e *evalEnv) seriesOfRef(ref string) ([]*decimal.Decimal, []bool, bool) {
	if vals, ok := e.priceOHL[ref]; ok {
		return vals, e.always, true
	}
	id, field, ok := strings.Cut(ref, ".")
	if !ok {
		return nil, nil, false
	}
	s, exists := e.series[id]
	if !exists {
		return nil, nil, false
	}
	return s.Fields[field], s.Ready, true
}

// shifted returns the series viewed lag bars back: out[k] = in[k-lag].
// Views are cached per (ref, lag) so evaluation stays linear in bars.
func (e *evalEnv) shifted(ref string, lag int, vals []*decimal.Decimal, ready []bool) ([]*decimal.Decimal, []bool, bool) {
	key := ref + "|" + strconv.Itoa(lag)
	if v, ok := e.shifts[key]; ok {
		return v.vals, v.ready, true
	}
	n := len(e.bars)
	out := make([]*decimal.Decimal, n)
	outReady := make([]bool, n)
	for k := 0; k < n; k++ {
		j := k - lag
		if j >= 0 && j < len(vals) {
			out[k] = vals[j]
			if ready != nil && j < len(ready) {
				outReady[k] = ready[j]
			}
		}
	}
	e.shifts[key] = shiftedSeries{vals: out, ready: outReady}
	return out, outReady, true
}

type shiftedSeries struct {
	vals  []*decimal.Decimal
	ready []bool
}
