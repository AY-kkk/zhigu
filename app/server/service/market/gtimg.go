package market

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"zhigu/server/service/finance"
)

func gtimgSymbol(inst SeedInstrument) (string, error) {
	switch inst.Exchange {
	case "SSE":
		return "sh" + inst.Code, nil
	case "SZSE":
		return "sz" + inst.Code, nil
	case "BSE":
		return "bj" + inst.Code, nil
	case "HKEX":
		return "hk" + inst.Code, nil
	default:
		return "", finance.NewError(422, "validation", "INSTRUMENT_UNSUPPORTED", "无 gtimg 代码映射")
	}
}

func (e *EastMoney) barsFromGtimg(ctx context.Context, inst SeedInstrument, adjust, start, end string, limit int) ([]QuoteBar, error) {
	sym, err := gtimgSymbol(inst)
	if err != nil {
		return nil, err
	}
	adj := ""
	switch adjust {
	case "", "raw":
	case "qfq":
		adj = "qfq"
	case "hfq":
		adj = "hfq"
	default:
		return nil, finance.NewError(400, "validation", "INVALID_PARAM", "不支持的复权")
	}
	cap := 640
	path := "/appstock/app/fqkline/get"
	if inst.Exchange == "HKEX" {
		cap = 320
		path = "/appstock/app/hkfqkline/get"
		if adj == "" {
			adj = "qfq"
		}
	}
	if limit <= 0 {
		limit = 250
	}
	if limit > cap {
		limit = cap
	}
	rawURL := "https://web.ifzq.gtimg.cn" + path + "?param=" + fmt.Sprintf("%s,day,,,%d,%s", sym, limit, adj)
	raw, err := e.HTTP.GetHeader(ctx, rawURL, map[string]string{
		"User-Agent": "Mozilla/5.0",
		"Referer":    "https://gu.qq.com/",
	})
	if err != nil {
		return nil, err
	}
	return parseGtimgBars(raw, inst, adj, start, end, limit)
}

func parseGtimgBars(raw []byte, inst SeedInstrument, adj, start, end string, limit int) ([]QuoteBar, error) {
	var env struct {
		Code int                        `json:"code"`
		Data map[string]json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &env) != nil || env.Code != 0 || env.Data == nil {
		return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "K 线无法解析")
	}
	sym, err := gtimgSymbol(inst)
	if err != nil {
		return nil, err
	}
	blockRaw, ok := env.Data[sym]
	if !ok {
		return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "K 线无法解析")
	}
	var block map[string]json.RawMessage
	if json.Unmarshal(blockRaw, &block) != nil {
		return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "K 线无法解析")
	}
	keys := []string{"day"}
	switch adj {
	case "qfq":
		keys = []string{"qfqday", "day"}
	case "hfq":
		keys = []string{"hfqday", "day"}
	}
	var rows [][]any
	for _, k := range keys {
		if json.Unmarshal(block[k], &rows) == nil && len(rows) > 0 {
			break
		}
		rows = nil
	}
	if len(rows) == 0 {
		return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "K 线无法解析")
	}
	complete := LastCompleteSession(CalendarID(inst.Exchange), time.Now().UTC())
	out := make([]QuoteBar, 0, len(rows))
	for _, row := range rows {
		if len(row) < 6 {
			continue
		}
		day := cellString(row[0])
		if start != "" && day < start {
			continue
		}
		if end != "" && day > end {
			continue
		}
		out = append(out, QuoteBar{
			Time: day, Open: cellString(row[1]), Close: cellString(row[2]),
			High: cellString(row[3]), Low: cellString(row[4]),
			Volume: toShares(cellString(row[5]), inst.Exchange), IsFinal: day <= complete,
		})
	}
	if len(out) == 0 {
		return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "K 线无法解析")
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func parseGtimgQuotes(raw []byte, byID map[string]SeedInstrument) (map[string]QuoteSnapshot, error) {
	bySym := map[string]SeedInstrument{}
	for _, inst := range byID {
		sym, err := gtimgSymbol(inst)
		if err != nil {
			continue
		}
		bySym[sym] = inst
	}
	obs := time.Now().UTC().Format(time.RFC3339)
	out := map[string]QuoteSnapshot{}
	for _, rec := range strings.Split(string(raw), ";") {
		rec = strings.TrimSpace(rec)
		if !strings.HasPrefix(rec, "v_") {
			continue
		}
		eq := strings.Index(rec, `="`)
		if eq < 3 {
			continue
		}
		sym := strings.TrimPrefix(rec[:eq], "v_")
		inst, ok := bySym[sym]
		if !ok {
			continue
		}
		body := strings.TrimSuffix(rec[eq+2:], `"`)
		fields := strings.Split(body, "~")
		if len(fields) < 4 {
			continue
		}
		last := strings.TrimSpace(fields[3])
		if last == "" || last == "0" || last == "0.00" || last == "0.000" {
			continue
		}
		chg, pct := "", ""
		if len(fields) > 4 {
			if lastD, err1 := decimal.NewFromString(last); err1 == nil {
				if prevD, err2 := decimal.NewFromString(strings.TrimSpace(fields[4])); err2 == nil && !prevD.IsZero() {
					delta := lastD.Sub(prevD)
					chg = delta.String()
					pct = delta.Div(prevD).Mul(decimal.NewFromInt(100)).Round(2).String()
				}
			}
		}
		out[inst.InstrumentID] = QuoteSnapshot{
			InstrumentID: inst.InstrumentID, Name: inst.Name, Exchange: inst.Exchange,
			Last: last, Change: chg, ChangePct: pct,
			ObservedAt: obs, IsFinal: false, SourceID: "gtimg_qt",
			Quality: map[string]any{"status": "unverified", "freshness_status": "delay_unknown", "warnings": []string{"最新价延迟未知，不是交易所实时"}},
		}
	}
	if len(out) == 0 {
		return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "最新价无法解析")
	}
	return out, nil
}

func cellString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
