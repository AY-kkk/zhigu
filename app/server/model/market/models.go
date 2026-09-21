package market

import (
	"time"

	"gorm.io/datatypes"
)

type CatalogRelease struct {
	CatalogVersion        string         `gorm:"column:catalog_version;primaryKey"`
	CatalogAsOf           string         `gorm:"column:catalog_as_of"`
	FreshnessStatus       string         `gorm:"column:freshness_status"`
	SourceID              string         `gorm:"column:source_id"`
	SourceContractVersion string         `gorm:"column:source_contract_version"`
	InstrumentCount       int            `gorm:"column:instrument_count"`
	Payload               datatypes.JSON `gorm:"column:payload"`
	PublishedAt           time.Time      `gorm:"column:published_at"`
}

func (CatalogRelease) TableName() string { return "finance_market_catalog_releases" }

type Pointer struct {
	Name      string    `gorm:"column:name;primaryKey"`
	Value     string    `gorm:"column:value"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (Pointer) TableName() string { return "finance_market_pointers" }

type Instrument struct {
	InstrumentID  string         `gorm:"column:instrument_id;primaryKey"`
	SecurityID    string         `gorm:"column:security_id"`
	Exchange      string         `gorm:"column:exchange"`
	Board         string         `gorm:"column:board"`
	AssetType     string         `gorm:"column:asset_type"`
	Code          string         `gorm:"column:code"`
	Name          string         `gorm:"column:name"`
	NameEN        string         `gorm:"column:name_en"`
	Aliases       datatypes.JSON `gorm:"column:aliases"`
	Pinyin        string         `gorm:"column:pinyin"`
	PinyinAbbr    string         `gorm:"column:pinyin_abbr"`
	Currency      string         `gorm:"column:currency"`
	CalendarID    string         `gorm:"column:calendar_id"`
	ListingDate   *string        `gorm:"column:listing_date"`
	DelistingDate *string        `gorm:"column:delisting_date"`
	TradingStatus string         `gorm:"column:trading_status"`
	LotSize       int            `gorm:"column:lot_size"`
	LotRuleID     string         `gorm:"column:lot_rule_id"`
	TickRuleID    string         `gorm:"column:tick_rule_id"`
	ValidFrom     string         `gorm:"column:valid_from"`
	ValidTo       *string        `gorm:"column:valid_to"`
	CatalogVersion string        `gorm:"column:catalog_version"`
	SourcePayload datatypes.JSON `gorm:"column:source_payload"`
	CreatedAt     time.Time      `gorm:"column:created_at"`
	UpdatedAt     time.Time      `gorm:"column:updated_at"`
}

func (Instrument) TableName() string { return "finance_market_instruments" }

type Alias struct {
	Alias          string  `gorm:"column:alias;primaryKey"`
	Normalized     string  `gorm:"column:normalized;primaryKey"`
	InstrumentID   string  `gorm:"column:instrument_id;primaryKey"`
	Kind           string  `gorm:"column:kind;primaryKey"`
	ValidFrom      string  `gorm:"column:valid_from;primaryKey"`
	ValidTo        *string `gorm:"column:valid_to"`
	CatalogVersion string  `gorm:"column:catalog_version"`
}

func (Alias) TableName() string { return "finance_market_instrument_aliases" }

type Snapshot struct {
	ID                    string         `gorm:"column:id;primaryKey"`
	InstrumentID          string         `gorm:"column:instrument_id"`
	Period                string         `gorm:"column:period"`
	Adjust                string         `gorm:"column:adjust"`
	SourceID              string         `gorm:"column:source_id"`
	SourceContractVersion string         `gorm:"column:source_contract_version"`
	CatalogVersion        *string        `gorm:"column:catalog_version"`
	AdjustmentVersion     *string        `gorm:"column:adjustment_version"`
	CalendarVersion       *string        `gorm:"column:calendar_version"`
	EarliestDate          *string        `gorm:"column:earliest_date"`
	LatestCompleteDate    *string        `gorm:"column:latest_complete_date"`
	Quality               datatypes.JSON `gorm:"column:quality"`
	BarsHash              string         `gorm:"column:bars_hash"`
	Immutable             bool           `gorm:"column:immutable"`
	CreatedAt             time.Time      `gorm:"column:created_at"`
}

func (Snapshot) TableName() string { return "finance_market_snapshots" }

type BarRow struct {
	SnapshotID   string  `gorm:"column:snapshot_id;primaryKey"`
	InstrumentID string  `gorm:"column:instrument_id;primaryKey"`
	Period       string  `gorm:"column:period;primaryKey"`
	TradeDate    string  `gorm:"column:trade_date;primaryKey"`
	PeriodStart  *string `gorm:"column:period_start"`
	PeriodEnd    *string `gorm:"column:period_end"`
	Open         string  `gorm:"column:open_px"`
	High         string  `gorm:"column:high_px"`
	Low          string  `gorm:"column:low_px"`
	Close        string  `gorm:"column:close_px"`
	Volume       string  `gorm:"column:volume"`
	IsFinal      bool    `gorm:"column:is_final"`
}

func (BarRow) TableName() string { return "finance_market_bars" }

type Action struct {
	ID            string         `gorm:"column:id;primaryKey"`
	InstrumentID  string         `gorm:"column:instrument_id"`
	Kind          string         `gorm:"column:kind"`
	EffectiveAt   string         `gorm:"column:effective_at"`
	AvailableAt   *string        `gorm:"column:available_at"`
	RecordDate    *string        `gorm:"column:record_date"`
	PayDate       *string        `gorm:"column:pay_date"`
	Factor        *string        `gorm:"column:factor"`
	CashAmount    *string        `gorm:"column:cash_amount"`
	Ratio         *string        `gorm:"column:ratio"`
	SourceID      string         `gorm:"column:source_id"`
	Version       string         `gorm:"column:version"`
	EvidenceLevel string         `gorm:"column:evidence_level"`
	Payload       datatypes.JSON `gorm:"column:payload"`
	CreatedAt     time.Time      `gorm:"column:created_at"`
}

func (Action) TableName() string { return "finance_market_actions" }

type CalendarDay struct {
	CalendarID      string `gorm:"column:calendar_id;primaryKey"`
	CalendarVersion string `gorm:"column:calendar_version;primaryKey"`
	TradeDate       string `gorm:"column:trade_date;primaryKey"`
	Session         string `gorm:"column:session"`
	IsHalfDay       bool   `gorm:"column:is_half_day"`
}

func (CalendarDay) TableName() string { return "finance_market_calendars" }

type Rule struct {
	ID            string         `gorm:"column:id;primaryKey"`
	Exchange      string         `gorm:"column:exchange"`
	Board         string         `gorm:"column:board"`
	RuleKind      string         `gorm:"column:rule_kind"`
	EffectiveFrom string         `gorm:"column:effective_from"`
	EffectiveTo   *string        `gorm:"column:effective_to"`
	Version       string         `gorm:"column:version"`
	Source        string         `gorm:"column:source"`
	Payload       datatypes.JSON `gorm:"column:payload"`
	CreatedAt     time.Time      `gorm:"column:created_at"`
}

func (Rule) TableName() string { return "finance_market_rules" }
