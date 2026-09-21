package backtest

import (
	"github.com/shopspring/decimal"

	"zhigu/server/service/indicators"
	"zhigu/server/service/market"
)

func causalQfq(raw []indicators.Bar, actions []market.CorporateAction, asOf string) []indicators.Bar {
	out := make([]indicators.Bar, len(raw))
	for i, b := range raw {
		pf, vf := decimal.NewFromInt(1), decimal.NewFromInt(1)
		for _, a := range actions {
			if a.AvailableAt == nil || *a.AvailableAt == "" || *a.AvailableAt > asOf {
				continue
			}
			if a.EffectiveAt == "" || a.EffectiveAt > asOf {
				continue
			}
			if b.Time >= a.EffectiveAt {
				continue
			}
			switch a.Kind {
			case "split":
				r := market.MustDec(a.Ratio)
				if r.GreaterThan(decimal.Zero) {
					pf = pf.Div(r)
					vf = vf.Mul(r)
				}
			case "cash_dividend":
				cash := market.MustDec(a.CashAmount)
				prev := closeBefore(raw, a.EffectiveAt)
				if prev.GreaterThan(cash) && prev.GreaterThan(decimal.Zero) {
					pf = pf.Mul(prev.Sub(cash).Div(prev))
				}
			}
		}
		nb := b
		nb.Open, nb.High, nb.Low, nb.Close = b.Open.Mul(pf), b.High.Mul(pf), b.Low.Mul(pf), b.Close.Mul(pf)
		nb.Volume = b.Volume.Mul(vf)
		out[i] = nb
	}
	return out
}

func closeBefore(raw []indicators.Bar, day string) decimal.Decimal {
	for i := len(raw) - 1; i >= 0; i-- {
		if raw[i].Time < day {
			return raw[i].Close
		}
	}
	return decimal.Zero
}
