package market

import "strings"

func RuleFor(exchange, board, date string) TradingRule {
	rule := TradingRule{
		LotSize: 100, Tick: "0.01", TPlus: 1,
		CommissionRate: "0.00025", CommissionMin: "5",
		StampBuy: "0", StampSell: stampA(date), TransferRate: "0.00001",
		Source: "product_default_assumption", Version: RuleVersion, EffectiveFrom: "2018-01-01",
	}
	switch exchange {
	case "SSE":
		if board == "STAR" {
			rule.LimitPct = "0.20"
		} else {
			rule.LimitPct = "0.10"
		}
	case "SZSE":
		if board == "CHINEXT" {
			rule.LimitPct = "0.20"
		} else {
			rule.LimitPct = "0.10"
		}
	case "BSE":
		rule.LimitPct = "0.30"
		rule.LotSize = 100
	case "HKEX":
		rule.TPlus = 0
		rule.LimitPct = ""
		rule.Tick = "0.1"
		rule.LotSize = 100
		rule.CommissionMin = "0"
		rule.StampBuy = stampHK(date)
		rule.StampSell = stampHK(date)
		rule.TransferRate = "0.00005"
		rule.Tick = hkTickPlaceholder
	}
	return rule
}

const hkTickPlaceholder = "0.01"

func stampA(date string) string {
	if date >= "2023-08-28" {
		return "0.0005"
	}
	return "0.001"
}

func stampHK(date string) string {
	if date >= "2023-11-17" {
		return "0.001"
	}
	return "0.0013"
}

func BoardOf(exchange, code string) string {
	switch exchange {
	case "SSE":
		if strings.HasPrefix(code, "688") {
			return "STAR"
		}
		return "MAIN"
	case "SZSE":
		if strings.HasPrefix(code, "300") || strings.HasPrefix(code, "301") {
			return "CHINEXT"
		}
		return "MAIN"
	case "BSE":
		return "BSE"
	case "HKEX":
		return "MAIN"
	}
	return "MAIN"
}
