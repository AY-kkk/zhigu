package strategy

import (
	"time"

	"gorm.io/datatypes"
)

type Workspace struct {
	OwnerID          uint           `gorm:"column:owner_id;primaryKey"`
	Revision         int            `gorm:"column:revision"`
	Watchlist        datatypes.JSON `gorm:"column:watchlist"`
	LastInstrumentID *string        `gorm:"column:last_instrument_id"`
	Layout           datatypes.JSON `gorm:"column:layout"`
	ChartIndicators  datatypes.JSON `gorm:"column:chart_indicators"`
	UpdatedAt        time.Time      `gorm:"column:updated_at"`
}

func (Workspace) TableName() string { return "finance_strategy_workspace" }

type Draft struct {
	ID               string         `gorm:"column:id;primaryKey"`
	OwnerID          uint           `gorm:"column:owner_id"`
	Text             string         `gorm:"column:text"`
	InstrumentID     *string        `gorm:"column:instrument_id"`
	Status           string         `gorm:"column:status"`
	Revision         int            `gorm:"column:revision"`
	GenerationID     *string        `gorm:"column:generation_id"`
	DSL              datatypes.JSON `gorm:"column:dsl"`
	Assumptions      datatypes.JSON `gorm:"column:assumptions"`
	CapabilityErrors datatypes.JSON `gorm:"column:capability_errors"`
	Clarification    datatypes.JSON `gorm:"column:clarification"`
	Compiled         datatypes.JSON `gorm:"column:compiled"`
	CreatedAt        time.Time      `gorm:"column:created_at"`
	UpdatedAt        time.Time      `gorm:"column:updated_at"`
}

func (Draft) TableName() string { return "finance_strategy_drafts" }

type Generation struct {
	ID                 string         `gorm:"column:id;primaryKey"`
	OwnerID            uint           `gorm:"column:owner_id"`
	DraftID            string         `gorm:"column:draft_id"`
	Status             string         `gorm:"column:status"`
	Text               string         `gorm:"column:text"`
	InstrumentID       *string        `gorm:"column:instrument_id"`
	BaseRevision       *int           `gorm:"column:base_revision"`
	ModelConfigVersion *string        `gorm:"column:model_config_version"`
	PromptVersion      *string        `gorm:"column:prompt_version"`
	Protocol           *string        `gorm:"column:protocol"`
	Usage              datatypes.JSON `gorm:"column:usage"`
	ErrorCode          *string        `gorm:"column:error_code"`
	ErrorMessage       *string        `gorm:"column:error_message"`
	IdempotencyKey     string         `gorm:"column:idempotency_key"`
	RequestHash        string         `gorm:"column:request_hash"`
	LeaseOwner         *string        `gorm:"column:lease_owner"`
	LeaseUntil         *time.Time     `gorm:"column:lease_until"`
	ExecutionEpoch     int            `gorm:"column:execution_epoch"`
	CreatedAt          time.Time      `gorm:"column:created_at"`
	UpdatedAt          time.Time      `gorm:"column:updated_at"`
}

func (Generation) TableName() string { return "finance_strategy_generations" }

type Strategy struct {
	ID               string     `gorm:"column:id;primaryKey" json:"id"`
	OwnerID          uint       `gorm:"column:owner_id" json:"-"`
	Name             string     `gorm:"column:name" json:"name"`
	CurrentVersionID *string    `gorm:"column:current_version_id" json:"current_version_id"`
	DeletedAt        *time.Time `gorm:"column:deleted_at" json:"deleted_at,omitempty"`
	CreatedAt        time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (Strategy) TableName() string { return "finance_strategies" }

type Version struct {
	ID              string         `gorm:"column:id;primaryKey" json:"id"`
	StrategyID      string         `gorm:"column:strategy_id" json:"strategy_id"`
	OwnerID         uint           `gorm:"column:owner_id" json:"-"`
	Revision        int            `gorm:"column:revision" json:"revision"`
	Name            string         `gorm:"column:name" json:"name"`
	DSL             datatypes.JSON `gorm:"column:dsl" json:"dsl"`
	DSLHash         string         `gorm:"column:dsl_hash" json:"dsl_hash"`
	Compiled        datatypes.JSON `gorm:"column:compiled" json:"compiled"`
	CompilerVersion string         `gorm:"column:compiler_version" json:"compiler_version"`
	CreatedAt       time.Time      `gorm:"column:created_at" json:"created_at"`
}

func (Version) TableName() string { return "finance_strategy_versions" }

type BacktestRun struct {
	ID                string         `gorm:"column:id;primaryKey" json:"id"`
	OwnerID           uint           `gorm:"column:owner_id" json:"-"`
	StrategyID        string         `gorm:"column:strategy_id" json:"strategy_id"`
	StrategyVersionID string         `gorm:"column:strategy_version_id" json:"strategy_version_id"`
	Status            string         `gorm:"column:status" json:"status"`
	Progress          datatypes.JSON `gorm:"column:progress" json:"progress"`
	Config            datatypes.JSON `gorm:"column:config" json:"config"`
	ConfigHash        string         `gorm:"column:config_hash" json:"config_hash"`
	Manifest          datatypes.JSON `gorm:"column:manifest" json:"manifest"`
	ResultSummary     datatypes.JSON `gorm:"column:result_summary" json:"result_summary"`
	ResultHash        *string        `gorm:"column:result_hash" json:"result_hash"`
	ErrorCode         *string        `gorm:"column:error_code" json:"error_code"`
	ErrorMessage      *string        `gorm:"column:error_message" json:"error_message"`
	IdempotencyKey    string         `gorm:"column:idempotency_key" json:"-"`
	RequestHash       string         `gorm:"column:request_hash" json:"-"`
	LeaseOwner        *string        `gorm:"column:lease_owner" json:"-"`
	LeaseUntil        *time.Time     `gorm:"column:lease_until" json:"-"`
	ExecutionEpoch    int            `gorm:"column:execution_epoch" json:"-"`
	HeartbeatAt       *time.Time     `gorm:"column:heartbeat_at" json:"-"`
	StartedAt         *time.Time     `gorm:"column:started_at" json:"started_at"`
	FinishedAt        *time.Time     `gorm:"column:finished_at" json:"finished_at"`
	CreatedAt         time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt         time.Time      `gorm:"column:updated_at" json:"updated_at"`
}

func (BacktestRun) TableName() string { return "finance_backtest_runs" }

type Order struct {
	ID            string         `gorm:"column:id;primaryKey" json:"id"`
	RunID         string         `gorm:"column:run_id" json:"run_id"`
	SignalID      *string        `gorm:"column:signal_id" json:"signal_id"`
	Side          string         `gorm:"column:side" json:"side"`
	Status        string         `gorm:"column:status" json:"status"`
	Qty           string         `gorm:"column:qty" json:"qty"`
	SubmittedDate *string        `gorm:"column:submitted_date" json:"submitted_date"`
	FillDate      *string        `gorm:"column:fill_date" json:"fill_date"`
	Reason        *string        `gorm:"column:reason" json:"reason"`
	Payload       datatypes.JSON `gorm:"column:payload" json:"payload"`
	CreatedAt     time.Time      `gorm:"column:created_at" json:"created_at"`
}

func (Order) TableName() string { return "finance_backtest_orders" }

type Fill struct {
	ID        string         `gorm:"column:id;primaryKey" json:"id"`
	RunID     string         `gorm:"column:run_id" json:"run_id"`
	OrderID   string         `gorm:"column:order_id" json:"order_id"`
	FillDate  string         `gorm:"column:fill_date" json:"fill_date"`
	Qty       string         `gorm:"column:qty" json:"qty"`
	Price     string         `gorm:"column:price" json:"price"`
	Fees      datatypes.JSON `gorm:"column:fees" json:"fees"`
	CashDelta string         `gorm:"column:cash_delta" json:"cash_delta"`
	Payload   datatypes.JSON `gorm:"column:payload" json:"payload"`
	CreatedAt time.Time      `gorm:"column:created_at" json:"created_at"`
}

func (Fill) TableName() string { return "finance_backtest_fills" }

type EquityRow struct {
	RunID       string  `gorm:"column:run_id;primaryKey" json:"run_id"`
	TradeDate   string  `gorm:"column:trade_date;primaryKey" json:"trade_date"`
	Cash        string  `gorm:"column:cash" json:"cash"`
	PositionQty string  `gorm:"column:position_qty" json:"position_qty"`
	MarketValue string  `gorm:"column:market_value" json:"market_value"`
	Equity      string  `gorm:"column:equity" json:"equity"`
	Drawdown    *string `gorm:"column:drawdown" json:"drawdown"`
}

func (EquityRow) TableName() string { return "finance_backtest_equity" }

type Result struct {
	RunID       string         `gorm:"column:run_id;primaryKey"`
	Metrics     datatypes.JSON `gorm:"column:metrics"`
	Signals     datatypes.JSON `gorm:"column:signals"`
	Assumptions datatypes.JSON `gorm:"column:assumptions"`
	Quality     datatypes.JSON `gorm:"column:quality"`
	ResultHash  string         `gorm:"column:result_hash"`
	CreatedAt   time.Time      `gorm:"column:created_at"`
}

func (Result) TableName() string { return "finance_backtest_results" }

type Idempotency struct {
	OwnerID        uint      `gorm:"column:owner_id;primaryKey"`
	Operation      string    `gorm:"column:operation;primaryKey"`
	IdempotencyKey string    `gorm:"column:idempotency_key;primaryKey"`
	RequestHash    string    `gorm:"column:request_hash"`
	ObjectID       string    `gorm:"column:object_id"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (Idempotency) TableName() string { return "finance_strategy_idempotency" }
