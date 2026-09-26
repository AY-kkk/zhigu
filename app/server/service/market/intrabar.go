package market

import "time"

// finalCompleteDate 返回最后一根已完成 bar 的日期。盘中未完成 bar 不能充当最新完整日，
// 否则快照在收盘后仍被判定新鲜，K 线停在盘中旧值不再更新。
func finalCompleteDate(bars []QuoteBar) *string {
	for i := len(bars) - 1; i >= 0; i-- {
		if bars[i].IsFinal {
			d := bars[i].Time
			return &d
		}
	}
	return nil
}

// intrabarDay 计算报价对应的交易日。无法解析、非交易日或晚于今天时返回空：
// 休市日与异常未来时间戳不补造 bar。
func intrabarDay(q QuoteSnapshot, calendarID string, now time.Time) string {
	cst := time.FixedZone("CST", 8*3600)
	day := DayKey(q.MarketTime)
	if day == "" {
		if t, err := time.Parse(time.RFC3339, q.ObservedAt); err == nil {
			day = t.In(cst).Format("2006-01-02")
		}
	}
	if day == "" || !IsTradingDay(calendarID, day) {
		return ""
	}
	if today := now.In(cst).Format("2006-01-02"); day > today {
		return ""
	}
	return day
}

// applyIntrabar 把延迟行情快照合成为当日未完成 bar。仅用于展示层合成，
// 不写入不可变快照；已定格的 bar 不覆盖，指标仍只取 is_final=true 的序列。
func applyIntrabar(bars []QuoteBar, q QuoteSnapshot, calendarID string, now time.Time) []QuoteBar {
	if q.Last == "" {
		return bars
	}
	day := intrabarDay(q, calendarID, now)
	if day == "" {
		return bars
	}
	out := make([]QuoteBar, 0, len(bars)+1)
	out = append(out, bars...)
	if len(out) == 0 {
		return append(out, intrabarFromQuote(day, q))
	}
	last := out[len(out)-1]
	lastDay := DayKey(last.Time)
	if day < lastDay {
		return out
	}
	if day > lastDay {
		return append(out, intrabarFromQuote(day, q))
	}
	if last.IsFinal {
		return out
	}
	if q.Open != "" {
		last.Open = q.Open
	}
	if q.High != "" {
		last.High = q.High
	}
	if q.Low != "" {
		last.Low = q.Low
	}
	if q.Volume != "" {
		last.Volume = q.Volume
	}
	last.Close = q.Last
	last.IsFinal = false
	out[len(out)-1] = last
	return out
}

func intrabarFromQuote(day string, q QuoteSnapshot) QuoteBar {
	b := QuoteBar{Time: day, Open: q.Open, High: q.High, Low: q.Low, Close: q.Last, Volume: q.Volume, IsFinal: false}
	if b.Open == "" {
		b.Open = q.Last
	}
	if b.High == "" {
		b.High = q.Last
	}
	if b.Low == "" {
		b.Low = q.Last
	}
	if b.Volume == "" {
		b.Volume = "0"
	}
	return b
}
