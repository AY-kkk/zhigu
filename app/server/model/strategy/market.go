package strategy

import (
	"time"

	"gorm.io/datatypes"
)

// Market item ID prefixes are app-generated: smi_ (items), smv_ (versions),
// sme_ (evidence), sma_ (audit). Content is append-only; withdrawn items are
// never physically deleted. HTTP layers return dedicated DTOs, not these rows.

type MarketItem struct {
	ID               string     `gorm:"column:id;primaryKey"`
	Slug             string     `gorm:"column:slug"`
	Status           string     `gorm:"column:status"`
	CurrentVersionID *string    `gorm:"column:current_version_id"`
	Revision         int        `gorm:"column:revision"`
	CreatedBy        uint       `gorm:"column:created_by"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
	PublishedAt      *time.Time `gorm:"column:published_at"`
	WithdrawnAt      *time.Time `gorm:"column:withdrawn_at"`
}

func (MarketItem) TableName() string { return "finance_strategy_market_items" }

type MarketVersion struct {
	ID                  string         `gorm:"column:id;primaryKey"`
	ItemID              string         `gorm:"column:item_id"`
	VersionNo           int            `gorm:"column:version_no"`
	Name                string         `gorm:"column:name"`
	Summary             string         `gorm:"column:summary"`
	Category            string         `gorm:"column:category"`
	Tags                datatypes.JSON `gorm:"column:tags;default:'[]'"`
	Markets             datatypes.JSON `gorm:"column:markets;default:'[]'"`
	SignalPeriod        string         `gorm:"column:signal_period"`
	Description         string         `gorm:"column:description"`
	Hypothesis          string         `gorm:"column:hypothesis"`
	FailureCases        string         `gorm:"column:failure_cases"`
	Sources             datatypes.JSON `gorm:"column:sources;default:'[]'"`
	RightsNote          string         `gorm:"column:rights_note"`
	EditorSchemaVersion string         `gorm:"column:editor_schema_version"`
	RuleTemplate        datatypes.JSON `gorm:"column:rule_template"`
	BacktestDefaults    datatypes.JSON `gorm:"column:backtest_defaults;default:'{}'"`
	ContentHash         string         `gorm:"column:content_hash"`
	ValidationStatus    string         `gorm:"column:validation_status;default:pending"`
	ValidationReport    datatypes.JSON `gorm:"column:validation_report;default:'{}'"`
	CreatedBy           uint           `gorm:"column:created_by"`
	CreatedAt           time.Time      `gorm:"column:created_at"`
	ValidatedAt         *time.Time     `gorm:"column:validated_at"`
	PublishedAt         *time.Time     `gorm:"column:published_at"`
}

func (MarketVersion) TableName() string { return "finance_strategy_market_versions" }

type MarketEvidence struct {
	ID                     string         `gorm:"column:id;primaryKey"`
	ItemID                 string         `gorm:"column:item_id"`
	MarketVersionID        string         `gorm:"column:market_version_id"`
	Status                 string         `gorm:"column:status;default:pending"`
	ValidationInstrumentID string         `gorm:"column:validation_instrument_id"`
	BoundDSLHash           string         `gorm:"column:bound_dsl_hash"`
	Config                 datatypes.JSON `gorm:"column:config;default:'{}'"`
	Manifest               datatypes.JSON `gorm:"column:manifest;default:'{}'"`
	Metrics                datatypes.JSON `gorm:"column:metrics;default:'{}'"`
	Equity                 datatypes.JSON `gorm:"column:equity;default:'[]'"`
	Trades                 datatypes.JSON `gorm:"column:trades;default:'[]'"`
	Limitations            datatypes.JSON `gorm:"column:limitations;default:'[]'"`
	ResultHash             string         `gorm:"column:result_hash"`
	EvidenceHash           string         `gorm:"column:evidence_hash"`
	ReviewNote             string         `gorm:"column:review_note"`
	CreatedBy              uint           `gorm:"column:created_by"`
	ReviewedBy             *uint          `gorm:"column:reviewed_by"`
	CreatedAt              time.Time      `gorm:"column:created_at"`
	ReviewedAt             *time.Time     `gorm:"column:reviewed_at"`
}

func (MarketEvidence) TableName() string { return "finance_strategy_market_evidence" }

// MarketAudit rows are append-only; the database rejects UPDATE and DELETE.
type MarketAudit struct {
	ID              string    `gorm:"column:id;primaryKey"`
	ItemID          string    `gorm:"column:item_id"`
	MarketVersionID *string   `gorm:"column:market_version_id"`
	ActorID         uint      `gorm:"column:actor_id"`
	Action          string    `gorm:"column:action"`
	RequestID       string    `gorm:"column:request_id"`
	BeforeRevision  int       `gorm:"column:before_revision"`
	AfterRevision   int       `gorm:"column:after_revision"`
	Reason          string    `gorm:"column:reason"`
	PayloadHash     string    `gorm:"column:payload_hash"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}

func (MarketAudit) TableName() string { return "finance_strategy_market_audit" }
