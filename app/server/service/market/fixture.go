package market

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

type SeedInstrument struct {
	InstrumentID, SecurityID, Exchange, Board, AssetType, Code, Name, NameEN, Currency, Status string
	Lot                                                                                        int
	Aliases                                                                                    []string
}

func FixtureUniverse() []SeedInstrument {
	return []SeedInstrument{
		{InstrumentID: "600519.SH", SecurityID: "sec_600519", Exchange: "SSE", Board: "MAIN", AssetType: "stock", Code: "600519", Name: "贵州茅台", NameEN: "Kweichow Moutai", Currency: "CNY", Status: "listed", Lot: 100, Aliases: []string{"茅台", "moutai"}},
		{InstrumentID: "300750.SZ", SecurityID: "sec_300750", Exchange: "SZSE", Board: "CHINEXT", AssetType: "stock", Code: "300750", Name: "宁德时代", NameEN: "CATL", Currency: "CNY", Status: "listed", Lot: 100, Aliases: []string{"宁德", "catl"}},
		{InstrumentID: "000333.SZ", SecurityID: "sec_000333", Exchange: "SZSE", Board: "MAIN", AssetType: "stock", Code: "000333", Name: "美的集团", NameEN: "Midea", Currency: "CNY", Status: "listed", Lot: 100, Aliases: []string{"美的"}},
		{InstrumentID: "835185.BJ", SecurityID: "sec_835185", Exchange: "BSE", Board: "BSE", AssetType: "stock", Code: "835185", Name: "贝特瑞", NameEN: "BTR", Currency: "CNY", Status: "listed", Lot: 100, Aliases: []string{"贝特瑞"}},
		{InstrumentID: "00700.HK", SecurityID: "sec_00700", Exchange: "HKEX", Board: "MAIN", AssetType: "stock", Code: "00700", Name: "腾讯控股", NameEN: "Tencent", Currency: "HKD", Status: "listed", Lot: 100, Aliases: []string{"腾讯", "0700", "700", "tencent"}},
		{InstrumentID: "80700.HK", SecurityID: "sec_00700", Exchange: "HKEX", Board: "MAIN", AssetType: "stock", Code: "80700", Name: "腾讯控股-R", NameEN: "Tencent-R", Currency: "CNY", Status: "listed", Lot: 100, Aliases: []string{"腾讯人民币"}},
		{InstrumentID: "510300.SH", SecurityID: "sec_510300", Exchange: "SSE", Board: "MAIN", AssetType: "fund", Code: "510300", Name: "沪深300ETF", NameEN: "CSI 300 ETF", Currency: "CNY", Status: "listed", Lot: 100, Aliases: []string{"300ETF"}},
	}
}

func SyntheticBars(instrumentID string, n int, last time.Time) []QuoteBar {
	if n <= 0 {
		n = 250
	}
	sum := sha256.Sum256([]byte(instrumentID))
	seed := int(sum[0])*256 + int(sum[1])
	px := decimal.NewFromInt(int64(50 + seed%400)).Add(decimal.RequireFromString("0.12"))
	out := make([]QuoteBar, 0, n)
	day := last
	for len(out) < n {
		if day.Weekday() != time.Saturday && day.Weekday() != time.Sunday {
			chg := decimal.NewFromInt(int64((seed+len(out)*17)%21 - 10)).Div(decimal.NewFromInt(1000))
			open := px
			close := px.Mul(decimal.NewFromInt(1).Add(chg))
			if close.LessThan(decimal.NewFromInt(1)) {
				close = decimal.NewFromInt(1)
			}
			high := close
			if open.GreaterThan(high) {
				high = open
			}
			low := close
			if open.LessThan(low) {
				low = open
			}
			vol := 1000000 + (seed*len(out))%500000
			out = append(out, QuoteBar{
				Time: day.Format("2006-01-02"),
				Open: open.StringFixed(2), High: high.StringFixed(2), Low: low.StringFixed(2), Close: close.StringFixed(2),
				Volume: fmt.Sprintf("%d", vol), IsFinal: true,
			})
			px = close
		}
		day = day.AddDate(0, 0, -1)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func PreviewDate() time.Time {
	return time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
}
