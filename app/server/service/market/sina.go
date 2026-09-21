package market

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"zhigu/server/service/finance"
)

func klineEnough(inst SeedInstrument, bars []QuoteBar) bool {
	if len(bars) == 0 {
		return false
	}
	if inst.Exchange == "HKEX" {
		return true
	}
	return len(bars) >= 20
}

func sinaSymbol(inst SeedInstrument) (string, error) {
	switch inst.Exchange {
	case "SSE":
		return "sh" + inst.Code, nil
	case "SZSE":
		return "sz" + inst.Code, nil
	case "BSE":
		return "bj" + inst.Code, nil
	default:
		return "", finance.NewError(422, "validation", "INSTRUMENT_UNSUPPORTED", "新浪日线不含该市场")
	}
}

func (e *EastMoney) barsFromSina(ctx context.Context, inst SeedInstrument, start, end string, limit int) ([]QuoteBar, error) {
	sym, err := sinaSymbol(inst)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 250
	}
	if limit > 1023 {
		limit = 1023
	}
	rawURL := fmt.Sprintf("https://quotes.sina.cn/cn/api/json_v2.php/CN_MarketDataService.getKLineData?symbol=%s&scale=240&ma=no&datalen=%d", sym, limit)
	raw, err := e.HTTP.GetHeader(ctx, rawURL, map[string]string{
		"User-Agent": "Mozilla/5.0",
		"Referer":    "https://finance.sina.com.cn/",
	})
	if err != nil {
		return nil, err
	}
	return parseSinaBars(raw, inst, start, end, limit)
}

func parseSinaBars(raw []byte, inst SeedInstrument, start, end string, limit int) ([]QuoteBar, error) {
	var rows []struct {
		Day    string `json:"day"`
		Open   string `json:"open"`
		High   string `json:"high"`
		Low    string `json:"low"`
		Close  string `json:"close"`
		Volume string `json:"volume"`
	}
	if json.Unmarshal(raw, &rows) != nil || len(rows) == 0 {
		return nil, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "K 线无法解析")
	}
	complete := LastCompleteSession(CalendarID(inst.Exchange), time.Now().UTC())
	out := make([]QuoteBar, 0, len(rows))
	for _, r := range rows {
		if start != "" && r.Day < start {
			continue
		}
		if end != "" && r.Day > end {
			continue
		}
		out = append(out, QuoteBar{
			Time: r.Day, Open: r.Open, High: r.High, Low: r.Low, Close: r.Close,
			Volume: r.Volume, IsFinal: r.Day <= complete,
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
