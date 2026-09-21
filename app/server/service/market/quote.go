package market

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"zhigu/server/service/finance"
)

type QuoteSnapshot struct {
	InstrumentID string         `json:"instrument_id"`
	Name         string         `json:"name"`
	Exchange     string         `json:"exchange"`
	Last         string         `json:"last"`
	Open         string         `json:"open"`
	High         string         `json:"high"`
	Low          string         `json:"low"`
	PrevClose    string         `json:"prev_close"`
	Change       string         `json:"change"`
	ChangePct    string         `json:"change_pct"`
	Volume       string         `json:"volume"`
	ObservedAt   string         `json:"observed_at"`
	MarketTime   string         `json:"market_time,omitempty"`
	IsFinal      bool           `json:"is_final"`
	SourceID     string         `json:"source_id"`
	Quality      map[string]any `json:"quality"`
}

func (e *EastMoney) LastQuotes(ctx context.Context, insts []SeedInstrument) (map[string]QuoteSnapshot, error) {
	out := map[string]QuoteSnapshot{}
	if len(insts) == 0 {
		return out, nil
	}
	const chunk = 20
	var lastErr error
	for i := 0; i < len(insts); i += chunk {
		end := i + chunk
		if end > len(insts) {
			end = len(insts)
		}
		part, err := e.lastQuotesOnce(ctx, insts[i:end])
		if err != nil {
			lastErr = err
			break
		}
		for k, v := range part {
			out[k] = v
		}
	}
	missing := make([]SeedInstrument, 0, len(insts))
	for _, inst := range insts {
		if _, ok := out[inst.InstrumentID]; !ok {
			missing = append(missing, inst)
		}
	}
	if len(missing) > 0 {
		gt, err := e.lastQuotesGtimg(ctx, missing)
		if err != nil {
			if len(out) == 0 {
				if lastErr != nil {
					return nil, lastErr
				}
				return nil, err
			}
		} else {
			for k, v := range gt {
				out[k] = v
			}
		}
	}
	if len(out) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return out, nil
}

func (e *EastMoney) lastQuotesOnce(ctx context.Context, insts []SeedInstrument) (map[string]QuoteSnapshot, error) {
	secids := make([]string, 0, len(insts))
	byID := map[string]SeedInstrument{}
	for _, inst := range insts {
		sid, err := secidOf(inst)
		if err != nil {
			continue
		}
		secids = append(secids, sid)
		byID[inst.InstrumentID] = inst
	}
	if len(secids) == 0 {
		return nil, finance.NewError(422, "validation", "INSTRUMENT_UNSUPPORTED", "无 secid 映射")
	}
	q := url.Values{}
	q.Set("fltt", "2")
	q.Set("secids", strings.Join(secids, ","))
	q.Set("fields", "f2,f3,f4,f12,f13,f14")
	raw, err := e.getQuote(ctx, "/api/qt/ulist.np/get?"+q.Encode(), clistHosts)
	if err != nil {
		return nil, err
	}
	return parseUlistQuotes(raw, byID)
}

func parseUlistQuotes(raw []byte, byID map[string]SeedInstrument) (map[string]QuoteSnapshot, error) {
	var env struct {
		Data *struct {
			Diff json.RawMessage `json:"diff"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &env) != nil || env.Data == nil {
		return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "最新价无法解析")
	}
	rows, err := decodeDiff(env.Data.Diff)
	if err != nil {
		return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "最新价无法解析")
	}
	obs := time.Now().UTC().Format(time.RFC3339)
	out := map[string]QuoteSnapshot{}
	for _, row := range rows {
		code := strings.TrimSpace(asString(row["f12"]))
		mkt := marketCode(row["f13"])
		id := ulistInstrumentID(code, mkt)
		if id == "" {
			continue
		}
		inst, ok := byID[id]
		if !ok {
			for _, cand := range byID {
				if cand.Code == code || cand.InstrumentID == id {
					inst, id, ok = cand, cand.InstrumentID, true
					break
				}
			}
		}
		if !ok {
			continue
		}
		last := strings.TrimSpace(asString(row["f2"]))
		if last == "" || last == "0" || last == "-" {
			continue
		}
		out[inst.InstrumentID] = QuoteSnapshot{
			InstrumentID: inst.InstrumentID, Name: strings.TrimSpace(asString(row["f14"])), Exchange: inst.Exchange,
			Last: last, Change: strings.TrimSpace(asString(row["f4"])), ChangePct: strings.TrimSpace(asString(row["f3"])),
			ObservedAt: obs, IsFinal: false, SourceID: "eastmoney_push2",
			Quality: map[string]any{"status": "unverified", "freshness_status": "delay_unknown"},
		}
	}
	return out, nil
}

func (e *EastMoney) lastQuotesGtimg(ctx context.Context, insts []SeedInstrument) (map[string]QuoteSnapshot, error) {
	out := map[string]QuoteSnapshot{}
	const chunk = 40
	var lastErr error
	for i := 0; i < len(insts); i += chunk {
		end := i + chunk
		if end > len(insts) {
			end = len(insts)
		}
		part, err := e.lastQuotesGtimgOnce(ctx, insts[i:end])
		if err != nil {
			lastErr = err
			continue
		}
		for k, v := range part {
			out[k] = v
		}
	}
	if len(out) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "最新价不可用")
	}
	return out, nil
}

func (e *EastMoney) lastQuotesGtimgOnce(ctx context.Context, insts []SeedInstrument) (map[string]QuoteSnapshot, error) {
	syms := make([]string, 0, len(insts))
	byID := map[string]SeedInstrument{}
	for _, inst := range insts {
		sym, err := gtimgSymbol(inst)
		if err != nil {
			continue
		}
		syms = append(syms, sym)
		byID[inst.InstrumentID] = inst
	}
	if len(syms) == 0 {
		return nil, finance.NewError(422, "validation", "INSTRUMENT_UNSUPPORTED", "无 gtimg 代码映射")
	}
	raw, err := e.HTTP.GetHeader(ctx, "https://qt.gtimg.cn/q="+strings.Join(syms, ","), map[string]string{
		"User-Agent": "Mozilla/5.0",
		"Referer":    "https://gu.qq.com/",
	})
	if err != nil {
		return nil, err
	}
	return parseGtimgQuotes(raw, byID)
}

func ulistInstrumentID(code string, market int) string {
	switch market {
	case 1:
		return code + ".SH"
	case 116, 128:
		for len(code) < 5 {
			code = "0" + code
		}
		return code + ".HK"
	case 0:
		if strings.HasPrefix(code, "92") || (strings.HasPrefix(code, "8") && len(code) == 6) {
			return code + ".BJ"
		}
		return code + ".SZ"
	default:
		return ""
	}
}

func (e *EastMoney) LastQuote(ctx context.Context, inst SeedInstrument) (QuoteSnapshot, error) {
	if q, err := e.lastQuoteStockGet(ctx, inst); err == nil {
		return q, nil
	}
	got, err := e.LastQuotes(ctx, []SeedInstrument{inst})
	if err == nil {
		if snap, ok := got[inst.InstrumentID]; ok && snap.Last != "" {
			return snap, nil
		}
	}
	return e.lastQuoteStockGet(ctx, inst)
}

func (e *EastMoney) lastQuoteStockGet(ctx context.Context, inst SeedInstrument) (QuoteSnapshot, error) {
	sid, err := secidOf(inst)
	if err != nil {
		return QuoteSnapshot{}, err
	}
	q := url.Values{}
	q.Set("secid", sid)
	q.Set("fields", "f43,f44,f45,f46,f47,f57,f58,f60,f86,f152,f169,f170")
	raw, err := e.getQuote(ctx, "/api/qt/stock/get?"+q.Encode(), clistHosts)
	if err != nil {
		return QuoteSnapshot{}, err
	}
	return parsePush2Quote(raw, inst)
}

func parsePush2Quote(raw []byte, inst SeedInstrument) (QuoteSnapshot, error) {
	var env struct {
		Data map[string]any `json:"data"`
	}
	if json.Unmarshal(raw, &env) != nil || env.Data == nil {
		return QuoteSnapshot{}, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "最新价无法解析")
	}
	dec := intFromAny(env.Data["f152"], 2)
	pricePlaces := dec
	if inst.Exchange == "HKEX" {
		pricePlaces = 3
	}
	last := emScaled(env.Data["f43"], pricePlaces)
	if last == "" || last == "0" {
		return QuoteSnapshot{}, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "最新价为空")
	}
	obs := time.Now().UTC().Format(time.RFC3339)
	if ts := int64FromAny(env.Data["f86"]); ts > 0 {
		obs = time.Unix(ts, 0).UTC().Format(time.RFC3339)
	}
	name := strings.TrimSpace(asString(env.Data["f58"]))
	if name == "" {
		name = inst.Name
	}
	return QuoteSnapshot{
		InstrumentID: inst.InstrumentID, Name: name, Exchange: inst.Exchange,
		Last: last, Open: emScaled(env.Data["f46"], pricePlaces), High: emScaled(env.Data["f44"], pricePlaces),
		Low: emScaled(env.Data["f45"], pricePlaces), PrevClose: emScaled(env.Data["f60"], pricePlaces),
		Change: emScaled(env.Data["f169"], pricePlaces), ChangePct: emScaled(env.Data["f170"], dec),
		Volume: toShares(asString(env.Data["f47"]), inst.Exchange), ObservedAt: obs, MarketTime: obs,
		IsFinal: false, SourceID: "eastmoney_push2",
		Quality: map[string]any{"status": "unverified", "freshness_status": "delay_unknown", "warnings": []string{"最新价延迟未知，不是交易所实时"}},
	}, nil
}

func emScaled(v any, places int) string {
	if places < 0 {
		places = 0
	}
	d, err := decimal.NewFromString(strings.TrimSpace(asString(v)))
	if err != nil {
		return ""
	}
	den := decimal.NewFromInt(10).Pow(decimal.NewFromInt(int64(places)))
	return d.Div(den).String()
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	case int:
		return strconv.Itoa(t)
	default:
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(cellString(v), ""), ""))
	}
}

func marketCode(v any) int {
	s := strings.TrimSpace(asString(v))
	if s == "" {
		return -1
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		f, err2 := strconv.ParseFloat(s, 64)
		if err2 != nil {
			return -1
		}
		return int(f)
	}
	return n
}

func intFromAny(v any, fallback int) int {
	s := strings.TrimSpace(asString(v))
	n, err := strconv.Atoi(s)
	if err != nil {
		f, err2 := strconv.ParseFloat(s, 64)
		if err2 != nil {
			return fallback
		}
		return int(f)
	}
	if n <= 0 {
		return fallback
	}
	return n
}

func int64FromAny(v any) int64 {
	s := strings.TrimSpace(asString(v))
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		f, err2 := strconv.ParseFloat(s, 64)
		if err2 != nil {
			return 0
		}
		return int64(f)
	}
	return n
}
