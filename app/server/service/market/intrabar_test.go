package market

import (
	"testing"
	"time"
)

// 采样时刻取 2026-09-24 14:00 CST（交易日盘中）。
var intrabarNow = mustTime("2026-09-24T06:00:00Z")

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// 盘中未完成 bar 不得充当最新完整日，否则快照在收盘后仍被判定新鲜，K 线永远停在盘中旧值。
func TestFinalCompleteDateSkipsUnfinalBar(t *testing.T) {
	bars := []QuoteBar{
		{Time: "2026-09-22", IsFinal: true},
		{Time: "2026-09-23", IsFinal: true},
		{Time: "2026-09-24", IsFinal: false},
	}
	got := finalCompleteDate(bars)
	if got == nil || *got != "2026-09-23" {
		t.Fatalf("finalCompleteDate = %v, want 2026-09-23", got)
	}
	if all := finalCompleteDate([]QuoteBar{{Time: "2026-09-24", IsFinal: false}}); all != nil {
		t.Fatalf("only unfinal bars should yield nil, got %v", *all)
	}
}

func TestApplyIntrabarReplacesUnfinalBarSameDay(t *testing.T) {
	bars := []QuoteBar{
		{Time: "2026-09-23", Open: "10", High: "11", Low: "9", Close: "10.5", Volume: "100", IsFinal: true},
		{Time: "2026-09-24", Open: "10.5", High: "10.8", Low: "10.4", Close: "10.6", Volume: "50", IsFinal: false},
	}
	q := QuoteSnapshot{MarketTime: "2026-09-24T02:00:00Z", Last: "10.9", Open: "10.5", High: "11.2", Low: "10.3", Volume: "80"}
	out := applyIntrabar(bars, q, "CN", intrabarNow)
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	last := out[1]
	if last.Close != "10.9" || last.High != "11.2" || last.Low != "10.3" || last.Volume != "80" {
		t.Fatalf("intraday bar not updated: %+v", last)
	}
	if last.IsFinal {
		t.Fatalf("overlay bar must stay unfinal: %+v", last)
	}
	if out[0].Close != "10.5" {
		t.Fatalf("history bar mutated: %+v", out[0])
	}
}

func TestApplyIntrabarKeepsFinalBar(t *testing.T) {
	bars := []QuoteBar{{Time: "2026-09-24", Open: "10", High: "11", Low: "9", Close: "11", Volume: "100", IsFinal: true}}
	q := QuoteSnapshot{MarketTime: "2026-09-24T08:00:00Z", Last: "99"}
	out := applyIntrabar(bars, q, "CN", intrabarNow)
	if out[0].Close != "11" {
		t.Fatalf("final bar must not be overwritten: %+v", out[0])
	}
}

func TestApplyIntrabarAppendsNewSession(t *testing.T) {
	bars := []QuoteBar{{Time: "2026-09-23", Open: "10", High: "11", Low: "9", Close: "10.5", Volume: "100", IsFinal: true}}
	q := QuoteSnapshot{MarketTime: "2026-09-24T02:00:00Z", Last: "10.7", Open: "10.6", High: "10.9", Low: "10.5", Volume: "30"}
	out := applyIntrabar(bars, q, "CN", intrabarNow)
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	last := out[1]
	if last.Time != "2026-09-24" || last.Close != "10.7" || last.IsFinal {
		t.Fatalf("new session bar wrong: %+v", last)
	}
}

func TestApplyIntrabarIgnoresOlderQuoteAndEmpty(t *testing.T) {
	bars := []QuoteBar{{Time: "2026-09-24", Open: "10", High: "11", Low: "9", Close: "11", Volume: "100", IsFinal: false}}
	if out := applyIntrabar(bars, QuoteSnapshot{MarketTime: "2026-09-22T02:00:00Z", Last: "5"}, "CN", intrabarNow); out[0].Close != "11" {
		t.Fatalf("older quote must be ignored: %+v", out[0])
	}
	if out := applyIntrabar(bars, QuoteSnapshot{MarketTime: "2026-09-24T02:00:00Z"}, "CN", intrabarNow); out[0].Close != "11" {
		t.Fatalf("empty last must be ignored: %+v", out[0])
	}
	if out := applyIntrabar(bars, QuoteSnapshot{Last: "7"}, "CN", intrabarNow); out[0].Close != "11" {
		t.Fatalf("quote without any time must be ignored: %+v", out[0])
	}
}

// 兜底报价（gtimg）只有 observed_at，没有 market_time，也要能合入当日 bar。
func TestApplyIntrabarFallsBackToObservedAt(t *testing.T) {
	bars := []QuoteBar{{Time: "2026-09-24", Open: "10", High: "11", Low: "9", Close: "10.6", Volume: "100", IsFinal: false}}
	q := QuoteSnapshot{ObservedAt: "2026-09-24T02:30:00Z", Last: "10.8"}
	out := applyIntrabar(bars, q, "CN", intrabarNow)
	if out[0].Close != "10.8" {
		t.Fatalf("observed_at fallback not applied: %+v", out[0])
	}
}

// 休市日与未来时间戳不编造 bar。
func TestApplyIntrabarRejectsHolidayAndFutureDay(t *testing.T) {
	bars := []QuoteBar{{Time: "2026-09-24", Open: "10", High: "11", Low: "9", Close: "11", Volume: "100", IsFinal: true}}
	holiday := QuoteSnapshot{ObservedAt: "2026-09-25T02:00:00Z", Last: "12"}
	if out := applyIntrabar(bars, holiday, "CN", mustTime("2026-09-25T06:00:00Z")); len(out) != 1 {
		t.Fatalf("holiday quote must not create a bar: %+v", out)
	}
	future := QuoteSnapshot{MarketTime: "2026-09-28T02:00:00Z", Last: "12"}
	if out := applyIntrabar(bars, future, "CN", intrabarNow); len(out) != 1 {
		t.Fatalf("future-dated quote must not create a bar: %+v", out)
	}
}
