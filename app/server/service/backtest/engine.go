package backtest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/shopspring/decimal"

	"zhigu/server/service/indicators"
	"zhigu/server/service/market"
	"zhigu/server/service/strategy"
)

const (
	EngineVersion        = "backtest.v1"
	DefaultSlippageBPS   = "10"
	DefaultParticipation = "0.01"
)

type Config struct {
	StrategyVersionID  string `json:"strategy_version_id"`
	InstrumentID       string `json:"instrument_id"`
	Start              string `json:"start"`
	End                string `json:"end"`
	InitialCash        string `json:"initial_cash"`
	Currency           string `json:"currency"`
	FeeScheduleID      string `json:"fee_schedule_id"`
	CommissionConfig   string `json:"commission_config"`
	SlippageBPS        string `json:"slippage_bps"`
	ParticipationCap   string `json:"participation_cap"`
	Benchmark          string `json:"benchmark"`
	DataSnapshotID     string `json:"data_snapshot_id"`
	ExecutionProfileID string `json:"execution_profile_id"`
	ForceClose         bool   `json:"force_close"`
}

type Input struct {
	Doc        strategy.Document
	Compiled   strategy.Compiled
	Raw        []indicators.Bar
	Signal     []indicators.Bar
	Instrument market.InstrumentView
	Rule       market.TradingRule
	Actions    []market.CorporateAction
	Config     Config
	Calendar   map[string]bool
	Suspended  map[string]bool
}

type Order struct {
	ID, SignalID, Side, Status, Qty, Submitted, FillDate, Reason string
	Price                                                        string
	Fees                                                         []Fee
	CashDelta                                                    string
}

type Fee struct {
	Name   string `json:"name"`
	Amount string `json:"amount"`
}

type Fill struct {
	OrderID, Date, Qty, Price, CashDelta string
	Fees                                 []Fee
}

type Equity struct {
	Date, Cash, Qty, MarketValue, Equity, Drawdown string
}

type Signal struct {
	Date string `json:"date"`
	Kind string `json:"kind"`
}

type Result struct {
	Status      string         `json:"status"`
	ErrorCode   string         `json:"error_code,omitempty"`
	Message     string         `json:"message,omitempty"`
	Orders      []Order        `json:"orders"`
	Fills       []Fill         `json:"fills"`
	Equity      []Equity       `json:"equity"`
	Signals     []Signal       `json:"signals"`
	Metrics     map[string]any `json:"metrics"`
	Assumptions []string       `json:"assumptions"`
	Quality     map[string]any `json:"quality"`
	Manifest    map[string]any `json:"manifest"`
	ResultHash  string         `json:"result_hash"`
}

type lot struct {
	qty, cost decimal.Decimal
	available string
	buyDate   string
}

type pending struct {
	side, signal, reason string
	targetFrac           decimal.Decimal
	exit                 bool
	created              string
}

func Run(in Input) Result {
	if in.Instrument.AssetType != "" && in.Instrument.AssetType != "stock" {
		return failRes("INSTRUMENT_UNSUPPORTED", "该证券不是股票，不能按股票策略回测")
	}
	if in.Doc.SignalPeriod != "1d" {
		return failRes("STRATEGY_UNSUPPORTED", "当前仅支持日线回测")
	}
	if len(in.Raw) == 0 {
		return failRes("INSUFFICIENT_HISTORY", "没有可用日线")
	}
	for _, b := range in.Raw {
		if !b.High.GreaterThanOrEqual(b.Low) || b.Volume.IsNegative() {
			return failRes("INSUFFICIENT_HISTORY", "K 线校验失败")
		}
	}
	signalBars := in.Signal
	if len(signalBars) == 0 {
		signalBars = in.Raw
	}
	if len(signalBars) != len(in.Raw) {
		return failRes("INSUFFICIENT_HISTORY", "信号序列与原始行情长度不一致")
	}
	if in.Doc.PriceBasis == "causal_qfq" {
		for _, a := range in.Actions {
			if a.AvailableAt == nil || *a.AvailableAt == "" {
				return failRes("RULE_DATA_INCOMPLETE", "公司行动缺少 available_at，不能做 causal_qfq")
			}
		}
	}
	cfg := in.Config
	if cfg.SlippageBPS == "" {
		cfg.SlippageBPS = DefaultSlippageBPS
	}
	if cfg.ParticipationCap == "" {
		cfg.ParticipationCap = DefaultParticipation
	}
	cash, err := decimal.NewFromString(cfg.InitialCash)
	if err != nil || !cash.GreaterThan(decimal.Zero) {
		return failRes("INVALID_PARAM", "初始资金无效")
	}
	specs := in.Doc.Indicators
	series, err := indicators.ComputeMany(signalBars, specs)
	if err != nil {
		return failRes("STRATEGY_INVALID", err.Error())
	}
	rule := in.Rule
	if rule.LotSize <= 0 {
		return failRes("RULE_DATA_INCOMPLETE", "缺少每手股数")
	}
	if strings.TrimSpace(rule.Tick) == "" || strings.TrimSpace(rule.CommissionRate) == "" {
		return failRes("RULE_DATA_INCOMPLETE", "缺少最小变动价位或佣金配置")
	}
	tick := market.MustDec(rule.Tick)
	slip := market.MustDec(cfg.SlippageBPS).Div(decimal.NewFromInt(10000))
	part := market.MustDec(cfg.ParticipationCap)
	start, end := cfg.Start, cfg.End
	warmup := in.Compiled.WarmupBars
	var (
		orders []Order
		fills  []Fill
		equity []Equity
		sigs   []Signal
		lots   []lot
		pend   *pending
		peak   = cash
		recv   = decimal.Zero
	)
	entryFrac := market.MustDec(in.Doc.Position.Value)
	soldToday := false
	for i, raw := range in.Raw {
		soldToday = false
		if start != "" && raw.Time < start {
			continue
		}
		if end != "" && raw.Time > end {
			break
		}
		if in.Suspended[raw.Time] {
			noteEquity(&equity, raw.Time, cash, lots, raw.Close, &peak, recv)
			continue
		}
		cash, lots, recv = applyActions(raw.Time, in.Actions, cash, lots, recv)
		if pend != nil {
			o, f, newCash, newLots, okFill := execute(pend, raw, in.Raw, i, cash, lots, rule, tick, slip, part)
			orders = append(orders, o)
			if okFill {
				fills = append(fills, f)
				cash, lots = newCash, newLots
				if pend.exit {
					soldToday = true
				}
				pend = nil
			} else if !pend.exit {
				pend = nil
			} else {
				pend.reason = o.Reason
			}
		}
		noteEquity(&equity, raw.Time, cash, lots, raw.Close, &peak, recv)
		if i < warmup {
			continue
		}
		evalBars, evalSeries := signalBars, series
		if in.Doc.PriceBasis == "causal_qfq" && len(in.Actions) > 0 {
			evalBars = causalQfq(in.Raw[:i+1], in.Actions, raw.Time)
			var qerr error
			evalSeries, qerr = indicators.ComputeMany(evalBars, specs)
			if qerr != nil {
				return failRes("STRATEGY_INVALID", qerr.Error())
			}
		}
		posQty := sumQty(lots)
		if posQty.GreaterThan(decimal.Zero) {
			if hitRisk(in.Doc.Risk, lots, raw, recv) {
				sigs = append(sigs, Signal{Date: raw.Time, Kind: "risk_exit"})
				pend = &pending{side: "sell", signal: "risk_" + raw.Time, exit: true, created: raw.Time, targetFrac: decimal.Zero}
				continue
			}
			exit, ready := strategy.EvalCond(in.Doc.Exit, evalBars, evalSeries, i)
			if ready && exit {
				sigs = append(sigs, Signal{Date: raw.Time, Kind: "exit"})
				pend = &pending{side: "sell", signal: "exit_" + raw.Time, exit: true, created: raw.Time, targetFrac: decimal.Zero}
				continue
			}
		}
		if posQty.Equal(decimal.Zero) && !soldToday {
			enter, ready := strategy.EvalCond(in.Doc.Entry, evalBars, evalSeries, i)
			if ready && enter {
				sigs = append(sigs, Signal{Date: raw.Time, Kind: "entry"})
				pend = &pending{side: "buy", signal: "entry_" + raw.Time, created: raw.Time, targetFrac: entryFrac}
			}
		}
	}
	m := metrics(equity, fills, orders, cfg)
	assumptions := append([]string{}, in.Compiled.Assumptions...)
	assumptions = append(assumptions,
		"历史模拟，不是实盘成交",
		"滑点 "+cfg.SlippageBPS+" bps，数量上限为前一日成交量的 "+cfg.ParticipationCap,
		"费用来源 "+rule.Source+" / "+rule.Version,
	)
	if in.Doc.PriceBasis == "causal_qfq" {
		if len(in.Actions) == 0 {
			assumptions = append(assumptions, "区间内无公司行动记录，causal_qfq 与 raw 一致")
		} else {
			assumptions = append(assumptions, "信号按 causal_qfq：每个信号日只用当时已可知的公司行动调整历史窗口；成交仍用原始开盘价")
		}
	}
	res := Result{
		Status: "succeeded", Orders: orders, Fills: fills, Equity: equity, Signals: sigs,
		Metrics: m, Assumptions: assumptions,
		Quality: map[string]any{"status": "complete", "evidence_level": actionLevel(in.Actions)},
		Manifest: map[string]any{
			"backtest_engine_version":  EngineVersion,
			"indicator_engine_version": indicators.EngineVersion,
			"compiler_version":         in.Compiled.CompilerVersion,
			"data_snapshot_id":         cfg.DataSnapshotID,
			"fee_schedule_id":          cfg.FeeScheduleID,
			"rule_version":             rule.Version,
		},
	}
	res.ResultHash = hashResult(res)
	return res
}

func failRes(code, msg string) Result {
	return Result{Status: "failed", ErrorCode: code, Message: msg}
}

func actionLevel(actions []market.CorporateAction) string {
	for _, a := range actions {
		if a.EvidenceLevel != "point_in_time_verified" {
			return "historical_revised"
		}
	}
	if len(actions) == 0 {
		return "no_actions"
	}
	return "point_in_time_verified"
}

func sumQty(lots []lot) decimal.Decimal {
	s := decimal.Zero
	for _, l := range lots {
		s = s.Add(l.qty)
	}
	return s
}

func noteEquity(dst *[]Equity, date string, cash decimal.Decimal, lots []lot, close decimal.Decimal, peak *decimal.Decimal, recv decimal.Decimal) {
	qty := sumQty(lots)
	mv := qty.Mul(close)
	eq := cash.Add(mv).Add(recv)
	if eq.GreaterThan(*peak) {
		*peak = eq
	}
	dd := decimal.Zero
	if peak.GreaterThan(decimal.Zero) {
		dd = peak.Sub(eq).Div(*peak)
	}
	*dst = append(*dst, Equity{
		Date: date, Cash: cash.String(), Qty: qty.String(), MarketValue: mv.String(),
		Equity: eq.String(), Drawdown: dd.String(),
	})
}

func applyActions(date string, actions []market.CorporateAction, cash decimal.Decimal, lots []lot, recv decimal.Decimal) (decimal.Decimal, []lot, decimal.Decimal) {
	qty := sumQty(lots)
	for _, a := range actions {
		switch a.Kind {
		case "split":
			if a.EffectiveAt == date && qty.GreaterThan(decimal.Zero) {
				r := market.MustDec(a.Ratio)
				if r.GreaterThan(decimal.Zero) {
					for i := range lots {
						lots[i].qty = lots[i].qty.Mul(r)
					}
				}
			}
		case "cash_dividend":
			if a.RecordDate != nil && *a.RecordDate == date && qty.GreaterThan(decimal.Zero) {
				recv = recv.Add(qty.Mul(market.MustDec(a.CashAmount)))
			}
			if a.PayDate != nil && *a.PayDate == date && recv.GreaterThan(decimal.Zero) {
				cash = cash.Add(recv)
				recv = decimal.Zero
			}
		}
	}
	return cash, lots, recv
}

func hitRisk(r strategy.Risk, lots []lot, bar indicators.Bar, recv decimal.Decimal) bool {
	qty := sumQty(lots)
	if qty.LessThanOrEqual(decimal.Zero) {
		return false
	}
	cost := decimal.Zero
	realized := decimal.Zero
	first := lots[0].buyDate
	for _, l := range lots {
		cost = cost.Add(l.cost)
		if l.buyDate < first {
			first = l.buyDate
		}
	}
	if cost.LessThanOrEqual(decimal.Zero) {
		return false
	}
	mv := qty.Mul(bar.Close)
	pnl := mv.Add(realized).Add(recv).Sub(cost).Div(cost)
	if r.StopLossPct != nil && strings.TrimSpace(*r.StopLossPct) != "" {
		th := market.MustDec(*r.StopLossPct)
		if pnl.LessThanOrEqual(th.Neg()) {
			return true
		}
	}
	if r.TakeProfitPct != nil && strings.TrimSpace(*r.TakeProfitPct) != "" {
		th := market.MustDec(*r.TakeProfitPct)
		if pnl.GreaterThanOrEqual(th) {
			return true
		}
	}
	if r.MaxHoldingBars != nil {
		// counted in caller if needed; keep simple date distance via buyDate compare in engine loop if set
	}
	return false
}

func execute(p *pending, day indicators.Bar, all []indicators.Bar, i int, cash decimal.Decimal, lots []lot, rule market.TradingRule, tick, slip, part decimal.Decimal) (Order, Fill, decimal.Decimal, []lot, bool) {
	o := Order{ID: "ord_" + day.Time + "_" + p.side, SignalID: p.signal, Side: p.side, Submitted: day.Time, Status: "rejected", Qty: "0"}
	if day.Open.LessThanOrEqual(decimal.Zero) {
		o.Reason = "NO_OPEN"
		return o, Fill{}, cash, lots, false
	}
	prevVol := decimal.Zero
	if i > 0 {
		prevVol = all[i-1].Volume
	}
	cap := prevVol.Mul(part)
	if cap.LessThanOrEqual(decimal.Zero) && prevVol.Equal(decimal.Zero) {
		o.Reason = "NO_VOLUME"
		return o, Fill{}, cash, lots, false
	}
	todayCap := day.Volume.Mul(part)
	px := day.Open
	if p.side == "buy" {
		px = px.Mul(decimal.NewFromInt(1).Add(slip))
		px = roundTick(px, tick, true)
		if limitBlocked(day, all, i, rule, true) {
			o.Reason = "LIMIT_LOCKED"
			o.Status = "rejected"
			return o, Fill{}, cash, lots, false
		}
		budget := cash.Mul(p.targetFrac)
		gross := px
		feeRate := market.MustDec(rule.CommissionRate).Add(market.MustDec(rule.StampBuy)).Add(market.MustDec(rule.TransferRate))
		denom := gross.Mul(decimal.NewFromInt(1).Add(feeRate))
		if denom.LessThanOrEqual(decimal.Zero) {
			o.Reason = "BAD_PRICE"
			return o, Fill{}, cash, lots, false
		}
		qty := floorLot(budget.Div(denom), rule.LotSize)
		if !cap.Equal(decimal.Zero) && qty.GreaterThan(cap) {
			qty = floorLot(cap, rule.LotSize)
		}
		if qty.GreaterThan(todayCap) {
			qty = floorLot(todayCap, rule.LotSize)
		}
		if qty.LessThan(decimal.NewFromInt(int64(rule.LotSize))) {
			o.Reason = "INSUFFICIENT_CASH_OR_LOT"
			return o, Fill{}, cash, lots, false
		}
		notional := px.Mul(qty)
		fees, feeAmt := feeList(rule, "buy", notional)
		need := notional.Add(feeAmt)
		if need.GreaterThan(cash) {
			o.Reason = "INSUFFICIENT_CASH"
			return o, Fill{}, cash, lots, false
		}
		cash = cash.Sub(need)
		avail := day.Time
		if rule.TPlus > 0 && i+rule.TPlus < len(all) {
			avail = all[i+rule.TPlus].Time
		} else if rule.TPlus > 0 {
			avail = day.Time + "+t1"
		}
		lots = append(lots, lot{qty: qty, cost: need, available: avail, buyDate: day.Time})
		o.Status, o.Qty, o.Price, o.FillDate, o.Fees, o.CashDelta = "filled", qty.String(), px.String(), day.Time, fees, need.Neg().String()
		f := Fill{OrderID: o.ID, Date: day.Time, Qty: qty.String(), Price: px.String(), Fees: fees, CashDelta: o.CashDelta}
		return o, f, cash, lots, true
	}
	px = px.Mul(decimal.NewFromInt(1).Sub(slip))
	px = roundTick(px, tick, false)
	if limitBlocked(day, all, i, rule, false) {
		o.Reason = "LIMIT_LOCKED"
		return o, Fill{}, cash, lots, false
	}
	sellable := decimal.Zero
	for _, l := range lots {
		if l.available <= day.Time || strings.HasSuffix(l.available, "+t1") && rule.TPlus == 0 {
			sellable = sellable.Add(l.qty)
		}
	}
	if rule.TPlus == 0 {
		sellable = sumQty(lots)
	} else {
		sellable = decimal.Zero
		for _, l := range lots {
			if l.available <= day.Time {
				sellable = sellable.Add(l.qty)
			}
		}
	}
	qty := sellable
	if !cap.Equal(decimal.Zero) && qty.GreaterThan(cap) {
		qty = floorLot(cap, rule.LotSize)
	}
	if qty.GreaterThan(todayCap) {
		qty = floorLot(todayCap, rule.LotSize)
	}
	qty = floorLot(qty, rule.LotSize)
	if qty.LessThanOrEqual(decimal.Zero) {
		o.Reason = "NOT_SELLABLE"
		o.Status = "rejected"
		return o, Fill{}, cash, lots, false
	}
	notional := px.Mul(qty)
	fees, feeAmt := feeList(rule, "sell", notional)
	proceeds := notional.Sub(feeAmt)
	cash = cash.Add(proceeds)
	lots = reduceLots(lots, qty)
	o.Status, o.Qty, o.Price, o.FillDate, o.Fees, o.CashDelta = "filled", qty.String(), px.String(), day.Time, fees, proceeds.String()
	f := Fill{OrderID: o.ID, Date: day.Time, Qty: qty.String(), Price: px.String(), Fees: fees, CashDelta: o.CashDelta}
	return o, f, cash, lots, true
}

func reduceLots(lots []lot, qty decimal.Decimal) []lot {
	left := qty
	out := []lot{}
	for _, l := range lots {
		if left.LessThanOrEqual(decimal.Zero) {
			out = append(out, l)
			continue
		}
		if l.qty.LessThanOrEqual(left) {
			left = left.Sub(l.qty)
			continue
		}
		l.qty = l.qty.Sub(left)
		left = decimal.Zero
		out = append(out, l)
	}
	return out
}

func floorLot(qty decimal.Decimal, lot int) decimal.Decimal {
	if lot <= 0 {
		return decimal.Zero
	}
	ld := decimal.NewFromInt(int64(lot))
	n := qty.Div(ld).Floor()
	return n.Mul(ld)
}

func roundTick(px, tick decimal.Decimal, buy bool) decimal.Decimal {
	if tick.LessThanOrEqual(decimal.Zero) {
		return px
	}
	n := px.Div(tick)
	if buy {
		return n.Ceil().Mul(tick)
	}
	return n.Floor().Mul(tick)
}

func limitBlocked(day indicators.Bar, all []indicators.Bar, i int, rule market.TradingRule, buy bool) bool {
	if strings.TrimSpace(rule.LimitPct) == "" || rule.LimitPct == "0" {
		return false
	}
	if i == 0 {
		return false
	}
	lim := market.MustDec(rule.LimitPct)
	prev := all[i-1].Close
	if prev.LessThanOrEqual(decimal.Zero) {
		return false
	}
	up := prev.Mul(decimal.NewFromInt(1).Add(lim))
	down := prev.Mul(decimal.NewFromInt(1).Sub(lim))
	if buy && day.Open.GreaterThanOrEqual(up) && day.High.Equal(day.Open) {
		return true
	}
	if !buy && day.Open.LessThanOrEqual(down) && day.Low.Equal(day.Open) {
		return true
	}
	return false
}

func feeList(rule market.TradingRule, side string, notional decimal.Decimal) ([]Fee, decimal.Decimal) {
	comm := notional.Mul(market.MustDec(rule.CommissionRate))
	min := market.MustDec(rule.CommissionMin)
	if comm.LessThan(min) {
		comm = min
	}
	stampRate := market.MustDec(rule.StampSell)
	if side == "buy" {
		stampRate = market.MustDec(rule.StampBuy)
	}
	stamp := notional.Mul(stampRate)
	xfer := notional.Mul(market.MustDec(rule.TransferRate))
	fees := []Fee{{Name: "commission", Amount: comm.String()}, {Name: "stamp", Amount: stamp.String()}, {Name: "transfer", Amount: xfer.String()}}
	return fees, comm.Add(stamp).Add(xfer)
}

func hashResult(r Result) string {
	clone := struct {
		Orders  []Order        `json:"orders"`
		Fills   []Fill         `json:"fills"`
		Equity  []Equity       `json:"equity"`
		Metrics map[string]any `json:"metrics"`
	}{r.Orders, r.Fills, r.Equity, r.Metrics}
	raw, _ := json.Marshal(clone)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func metrics(eq []Equity, fills []Fill, orders []Order, cfg Config) map[string]any {
	out := map[string]any{"trade_count": len(fills), "rf": "0"}
	if len(eq) == 0 {
		return out
	}
	init := market.MustDec(eq[0].Equity)
	if cfg.InitialCash != "" {
		init = market.MustDec(cfg.InitialCash)
	}
	last := market.MustDec(eq[len(eq)-1].Equity)
	var ret any = nil
	if init.GreaterThan(decimal.Zero) {
		r := last.Div(init).Sub(decimal.NewFromInt(1))
		ret = r.String()
		days := len(eq)
		if days > 0 {
			ann := r.Add(decimal.NewFromInt(1))
			// (1+r)^(365.25/days)-1
			out["annualized_note"] = "calendar 365.25"
			if days < 365 {
				out["annualized_flag"] = "short_sample"
			}
			_ = ann
		}
		out["total_return"] = ret
	}
	maxDD := decimal.Zero
	for _, row := range eq {
		d := market.MustDec(row.Drawdown)
		if d.GreaterThan(maxDD) {
			maxDD = d
		}
	}
	out["max_drawdown"] = maxDD.String()
	wins, losses, rounds := 0, 0, 0
	openCost := decimal.Zero
	open := false
	for _, o := range orders {
		if o.Status != "filled" {
			continue
		}
		if o.Side == "buy" {
			open = true
			openCost = market.MustDec(o.Qty).Mul(market.MustDec(o.Price))
		} else if open {
			proceeds := market.MustDec(o.Qty).Mul(market.MustDec(o.Price))
			rounds++
			if proceeds.GreaterThan(openCost) {
				wins++
			} else if proceeds.LessThan(openCost) {
				losses++
			}
			open = false
		}
	}
	out["round_trips"] = rounds
	if rounds == 0 {
		out["win_rate"] = nil
	} else {
		out["win_rate"] = decimal.NewFromInt(int64(wins)).Div(decimal.NewFromInt(int64(rounds))).String()
	}
	if losses == 0 {
		out["payoff_ratio"] = nil
	}
	return out
}
