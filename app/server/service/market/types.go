package market

import "github.com/shopspring/decimal"

const (
	SourceContractVersion = "eastmoney_push2_kline_v3"
	CalendarVersion       = "cn_hk_weekends_holidays_v1"
	RuleVersion           = "exchange_board_rules_v1"
	AdjustmentVersion     = "actions_v1"
)

type QuoteBar struct {
	Time        string `json:"time"`
	Open        string `json:"open"`
	High        string `json:"high"`
	Low         string `json:"low"`
	Close       string `json:"close"`
	Volume      string `json:"volume"`
	IsFinal     bool   `json:"is_final"`
	PeriodStart string `json:"period_start,omitempty"`
	PeriodEnd   string `json:"period_end,omitempty"`
}

type InstrumentView struct {
	InstrumentID   string   `json:"instrument_id"`
	SecurityID     string   `json:"security_id"`
	Name           string   `json:"name"`
	Code           string   `json:"code"`
	Exchange       string   `json:"exchange"`
	Board          string   `json:"board"`
	AssetType      string   `json:"asset_type"`
	Currency       string   `json:"currency"`
	CalendarID     string   `json:"calendar_id"`
	TradingStatus  string   `json:"trading_status"`
	LotSize        int      `json:"lot_size"`
	ListingDate    *string  `json:"listing_date"`
	DelistingDate  *string  `json:"delisting_date"`
	Aliases        []string `json:"aliases"`
	CatalogVersion string   `json:"catalog_version"`
	Unsupported    string   `json:"unsupported_reason,omitempty"`
}

type TradingRule struct {
	LotSize        int    `json:"lot_size"`
	Tick           string `json:"tick"`
	LimitPct       string `json:"limit_pct"`
	TPlus          int    `json:"t_plus"`
	CommissionRate string `json:"commission_rate"`
	CommissionMin  string `json:"commission_min"`
	StampBuy       string `json:"stamp_buy"`
	StampSell      string `json:"stamp_sell"`
	TransferRate   string `json:"transfer_rate"`
	Source         string `json:"source"`
	Version        string `json:"version"`
	EffectiveFrom  string `json:"effective_from"`
}

type CorporateAction struct {
	Kind          string  `json:"kind"`
	EffectiveAt   string  `json:"effective_at"`
	AvailableAt   *string `json:"available_at"`
	RecordDate    *string `json:"record_date"`
	PayDate       *string `json:"pay_date"`
	CashAmount    string  `json:"cash_amount,omitempty"`
	Ratio         string  `json:"ratio,omitempty"`
	EvidenceLevel string  `json:"evidence_level"`
}

func MustDec(s string) decimal.Decimal {
	if s == "" {
		return decimal.Zero
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero
	}
	return d
}
