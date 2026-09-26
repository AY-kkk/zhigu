package indicators

import "github.com/shopspring/decimal"

func dec(v int64) decimal.Decimal { return decimal.NewFromInt(v) }

func ptr(d decimal.Decimal) *decimal.Decimal { return &d }

func round(d decimal.Decimal) decimal.Decimal {
	return d.RoundBank(34)
}

func smaClose(bars []Bar, n int, ready []bool) []*decimal.Decimal {
	out := make([]*decimal.Decimal, len(bars))
	if n <= 0 {
		return out
	}
	var sum decimal.Decimal
	for i := range bars {
		sum = sum.Add(bars[i].Close)
		if i >= n {
			sum = sum.Sub(bars[i-n].Close)
		}
		if i+1 >= n {
			v := round(sum.Div(dec(int64(n))))
			out[i] = &v
			ready[i] = true
		}
	}
	return out
}

func emaClose(bars []Bar, n int, ready []bool) []*decimal.Decimal {
	out := make([]*decimal.Decimal, len(bars))
	if len(bars) == 0 || n <= 0 {
		return out
	}
	k := decimal.NewFromInt(2).Div(decimal.NewFromInt(int64(n + 1)))
	one := decimal.NewFromInt(1)
	ema := bars[0].Close
	out[0] = ptr(round(ema))
	for i := 1; i < len(bars); i++ {
		ema = round(ema.Mul(one.Sub(k)).Add(bars[i].Close.Mul(k)))
		out[i] = ptr(ema)
		if i+1 >= n {
			ready[i] = true
		}
	}
	return out
}

func emaSeries(values []*decimal.Decimal, n int) []*decimal.Decimal {
	out := make([]*decimal.Decimal, len(values))
	k := decimal.NewFromInt(2).Div(decimal.NewFromInt(int64(n + 1)))
	one := decimal.NewFromInt(1)
	var ema decimal.Decimal
	seeded := false
	count := 0
	for i, v := range values {
		if v == nil {
			continue
		}
		count++
		if !seeded {
			ema = *v
			seeded = true
			out[i] = ptr(round(ema))
			continue
		}
		ema = round(ema.Mul(one.Sub(k)).Add(v.Mul(k)))
		out[i] = ptr(ema)
		_ = count
	}
	return out
}

func boll(bars []Bar, n, k int, ready []bool) (mid, upper, lower []*decimal.Decimal) {
	mid = make([]*decimal.Decimal, len(bars))
	upper = make([]*decimal.Decimal, len(bars))
	lower = make([]*decimal.Decimal, len(bars))
	if n <= 0 {
		return
	}
	kd := decimal.NewFromInt(int64(k))
	for i := n - 1; i < len(bars); i++ {
		var sum decimal.Decimal
		for j := i - n + 1; j <= i; j++ {
			sum = sum.Add(bars[j].Close)
		}
		mean := round(sum.Div(dec(int64(n))))
		var acc decimal.Decimal
		for j := i - n + 1; j <= i; j++ {
			d := bars[j].Close.Sub(mean)
			acc = acc.Add(d.Mul(d))
		}
		// population stddev, denominator N
		variance := acc.Div(dec(int64(n)))
		sd := round(sqrt(variance))
		u := round(mean.Add(kd.Mul(sd)))
		l := round(mean.Sub(kd.Mul(sd)))
		mid[i], upper[i], lower[i] = ptr(mean), ptr(u), ptr(l)
		ready[i] = true
	}
	return
}

func volSeries(bars []Bar, n5, n10 int, ready []bool) (vol, ma5, ma10 []*decimal.Decimal) {
	vol = make([]*decimal.Decimal, len(bars))
	ma5 = make([]*decimal.Decimal, len(bars))
	ma10 = make([]*decimal.Decimal, len(bars))
	var s5, s10 decimal.Decimal
	need := n5
	if n10 > need {
		need = n10
	}
	for i := range bars {
		v := bars[i].Volume
		vol[i] = ptr(v)
		s5 = s5.Add(v)
		s10 = s10.Add(v)
		if i >= n5 {
			s5 = s5.Sub(bars[i-n5].Volume)
		}
		if i >= n10 {
			s10 = s10.Sub(bars[i-n10].Volume)
		}
		if i+1 >= n5 {
			x := round(s5.Div(dec(int64(n5))))
			ma5[i] = &x
		}
		if i+1 >= n10 {
			x := round(s10.Div(dec(int64(n10))))
			ma10[i] = &x
		}
		if i+1 >= need {
			ready[i] = true
		}
	}
	return
}

func macd(bars []Bar, fast, slow, signal int, ready []bool) (dif, dea, hist []*decimal.Decimal) {
	n := len(bars)
	dif = make([]*decimal.Decimal, n)
	dea = make([]*decimal.Decimal, n)
	hist = make([]*decimal.Decimal, n)
	fastEMA := emaClose(bars, fast, make([]bool, n))
	slowEMA := emaClose(bars, slow, make([]bool, n))
	for i := 0; i < n; i++ {
		if fastEMA[i] == nil || slowEMA[i] == nil {
			continue
		}
		v := round(fastEMA[i].Sub(*slowEMA[i]))
		dif[i] = &v
	}
	dea = emaSeries(dif, signal)
	two := decimal.NewFromInt(2)
	warmup := slow + signal - 1
	for i := 0; i < n; i++ {
		if dif[i] == nil || dea[i] == nil {
			continue
		}
		h := round(two.Mul(dif[i].Sub(*dea[i])))
		hist[i] = &h
		if i+1 >= warmup {
			ready[i] = true
		}
	}
	return
}

func kdj(bars []Bar, n, m1, m2 int, ready []bool) (kArr, dArr, jArr []*decimal.Decimal) {
	kArr = make([]*decimal.Decimal, len(bars))
	dArr = make([]*decimal.Decimal, len(bars))
	jArr = make([]*decimal.Decimal, len(bars))
	if n <= 0 || m1 <= 0 || m2 <= 0 {
		return
	}
	fifty := decimal.NewFromInt(50)
	hundred := decimal.NewFromInt(100)
	three := decimal.NewFromInt(3)
	two := decimal.NewFromInt(2)
	k := fifty
	d := fifty
	m1d := decimal.NewFromInt(int64(m1))
	m2d := decimal.NewFromInt(int64(m2))
	for i := range bars {
		if i+1 < n {
			continue
		}
		llv := bars[i].Low
		hhv := bars[i].High
		for j := i - n + 1; j <= i; j++ {
			if bars[j].Low.LessThan(llv) {
				llv = bars[j].Low
			}
			if bars[j].High.GreaterThan(hhv) {
				hhv = bars[j].High
			}
		}
		var rsv decimal.Decimal
		if hhv.Equal(llv) {
			rsv = fifty
		} else {
			rsv = round(hundred.Mul(bars[i].Close.Sub(llv)).Div(hhv.Sub(llv)))
		}
		k = round(m1d.Sub(decimal.NewFromInt(1)).Div(m1d).Mul(k).Add(rsv.Div(m1d)))
		d = round(m2d.Sub(decimal.NewFromInt(1)).Div(m2d).Mul(d).Add(k.Div(m2d)))
		j := round(three.Mul(k).Sub(two.Mul(d)))
		kArr[i], dArr[i], jArr[i] = ptr(k), ptr(d), ptr(j)
		ready[i] = true
	}
	return
}

func rsi(bars []Bar, n int, ready []bool) []*decimal.Decimal {
	out := make([]*decimal.Decimal, len(bars))
	if len(bars) < n+1 || n <= 0 {
		return out
	}
	hundred := decimal.NewFromInt(100)
	var gain, loss decimal.Decimal
	for i := 1; i <= n; i++ {
		chg := bars[i].Close.Sub(bars[i-1].Close)
		if chg.GreaterThan(decimal.Zero) {
			gain = gain.Add(chg)
		} else {
			loss = loss.Add(chg.Abs())
		}
	}
	avgGain := gain.Div(dec(int64(n)))
	avgLoss := loss.Div(dec(int64(n)))
	out[n] = ptr(rsiValue(avgGain, avgLoss, hundred))
	ready[n] = true
	nm1 := dec(int64(n - 1))
	nd := dec(int64(n))
	for i := n + 1; i < len(bars); i++ {
		chg := bars[i].Close.Sub(bars[i-1].Close)
		g, l := decimal.Zero, decimal.Zero
		if chg.GreaterThan(decimal.Zero) {
			g = chg
		} else {
			l = chg.Abs()
		}
		avgGain = round(avgGain.Mul(nm1).Add(g).Div(nd))
		avgLoss = round(avgLoss.Mul(nm1).Add(l).Div(nd))
		out[i] = ptr(rsiValue(avgGain, avgLoss, hundred))
		ready[i] = true
	}
	return out
}

func rsiValue(avgGain, avgLoss, hundred decimal.Decimal) decimal.Decimal {
	if avgLoss.Equal(decimal.Zero) && !avgGain.Equal(decimal.Zero) {
		return hundred
	}
	if avgLoss.Equal(decimal.Zero) && avgGain.Equal(decimal.Zero) {
		return decimal.NewFromInt(50)
	}
	rs := avgGain.Div(avgLoss)
	return round(hundred.Sub(hundred.Div(decimal.NewFromInt(1).Add(rs))))
}

func wr(bars []Bar, n int, ready []bool) []*decimal.Decimal {
	out := make([]*decimal.Decimal, len(bars))
	hundred := decimal.NewFromInt(100)
	fifty := decimal.NewFromInt(50)
	for i := n - 1; i < len(bars); i++ {
		llv := bars[i].Low
		hhv := bars[i].High
		for j := i - n + 1; j <= i; j++ {
			if bars[j].Low.LessThan(llv) {
				llv = bars[j].Low
			}
			if bars[j].High.GreaterThan(hhv) {
				hhv = bars[j].High
			}
		}
		var v decimal.Decimal
		if hhv.Equal(llv) {
			v = fifty
		} else {
			v = round(hundred.Mul(hhv.Sub(bars[i].Close)).Div(hhv.Sub(llv)))
		}
		out[i] = &v
		ready[i] = true
	}
	return out
}

func bias(bars []Bar, n int, ready []bool) []*decimal.Decimal {
	sma := smaClose(bars, n, make([]bool, len(bars)))
	out := make([]*decimal.Decimal, len(bars))
	hundred := decimal.NewFromInt(100)
	for i := range bars {
		if sma[i] == nil {
			continue
		}
		if sma[i].Equal(decimal.Zero) {
			ready[i] = false
			continue
		}
		v := round(hundred.Mul(bars[i].Close.Sub(*sma[i])).Div(*sma[i]))
		out[i] = &v
		ready[i] = true
	}
	return out
}

func cci(bars []Bar, n int, ready []bool) []*decimal.Decimal {
	out := make([]*decimal.Decimal, len(bars))
	three := decimal.NewFromInt(3)
	tp := make([]decimal.Decimal, len(bars))
	for i := range bars {
		tp[i] = round(bars[i].High.Add(bars[i].Low).Add(bars[i].Close).Div(three))
	}
	factor := decimal.RequireFromString("0.015")
	for i := n - 1; i < len(bars); i++ {
		var sum decimal.Decimal
		for j := i - n + 1; j <= i; j++ {
			sum = sum.Add(tp[j])
		}
		mean := round(sum.Div(dec(int64(n))))
		var dev decimal.Decimal
		for j := i - n + 1; j <= i; j++ {
			dev = dev.Add(tp[j].Sub(mean).Abs())
		}
		md := round(dev.Div(dec(int64(n))))
		if md.Equal(decimal.Zero) {
			z := decimal.Zero
			out[i] = &z
			ready[i] = true
			continue
		}
		v := round(tp[i].Sub(mean).Div(factor.Mul(md)))
		out[i] = &v
		ready[i] = true
	}
	return out
}

func atr(bars []Bar, n int, ready []bool) []*decimal.Decimal {
	out := make([]*decimal.Decimal, len(bars))
	if len(bars) == 0 || n <= 0 {
		return out
	}
	tr := make([]decimal.Decimal, len(bars))
	tr[0] = bars[0].High.Sub(bars[0].Low)
	for i := 1; i < len(bars); i++ {
		hl := bars[i].High.Sub(bars[i].Low)
		hc := bars[i].High.Sub(bars[i-1].Close).Abs()
		lc := bars[i].Low.Sub(bars[i-1].Close).Abs()
		m := hl
		if hc.GreaterThan(m) {
			m = hc
		}
		if lc.GreaterThan(m) {
			m = lc
		}
		tr[i] = m
	}
	if len(bars) < n {
		return out
	}
	var sum decimal.Decimal
	for i := 0; i < n; i++ {
		sum = sum.Add(tr[i])
	}
	atrV := round(sum.Div(dec(int64(n))))
	out[n-1] = ptr(atrV)
	ready[n-1] = true
	nm1 := dec(int64(n - 1))
	nd := dec(int64(n))
	for i := n; i < len(bars); i++ {
		atrV = round(atrV.Mul(nm1).Add(tr[i]).Div(nd))
		out[i] = ptr(atrV)
		ready[i] = true
	}
	return out
}

func obv(bars []Bar, ready []bool) []*decimal.Decimal {
	out := make([]*decimal.Decimal, len(bars))
	if len(bars) == 0 {
		return out
	}
	z := decimal.Zero
	out[0] = &z
	cur := decimal.Zero
	for i := 1; i < len(bars); i++ {
		cmp := bars[i].Close.Cmp(bars[i-1].Close)
		if cmp > 0 {
			cur = cur.Add(bars[i].Volume)
		} else if cmp < 0 {
			cur = cur.Sub(bars[i].Volume)
		}
		v := cur
		out[i] = &v
		ready[i] = true
	}
	return out
}

func sqrt(d decimal.Decimal) decimal.Decimal {
	if d.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}
	x := d
	if x.GreaterThan(decimal.NewFromInt(1)) {
		x = d.Div(decimal.NewFromInt(2)).Add(decimal.NewFromInt(1))
	} else {
		x = decimal.NewFromInt(1)
	}
	for i := 0; i < 48; i++ {
		if x.Equal(decimal.Zero) {
			break
		}
		nx := round(x.Add(d.Div(x)).Div(decimal.NewFromInt(2)))
		if nx.Equal(x) {
			return nx
		}
		x = nx
	}
	return round(x)
}
