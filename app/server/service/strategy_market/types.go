package strategy_market

import (
	"encoding/json"
	"time"
)

// Idempotency operation names (finance_strategy_idempotency, one namespace per operation).
const (
	OpCreateItem     = "market_create_item"
	OpCreateVersion  = "market_create_version"
	OpValidate       = "market_validate"
	OpPublish        = "market_publish"
	OpWithdraw       = "market_withdraw"
	OpCopy           = "market_copy"
	OpEvidenceImport = "market_evidence_import"
	OpEvidenceReview = "market_evidence_review"
)

const (
	ItemDraft     = "draft"
	ItemPublished = "published"
	ItemWithdrawn = "withdrawn"

	ValidationPending = "pending"
	ValidationPassed  = "passed"
	ValidationFailed  = "failed"

	EvidencePending  = "pending"
	EvidenceApproved = "approved"
	EvidenceRejected = "rejected"
	EvidenceRevoked  = "revoked"

	BacktestNotTested   = "not_tested"
	BacktestHasEvidence = "has_evidence"
)

// EngineSupported reports whether the running strategy engine can execute the
// rules compiled with the given compiler version.
func EngineSupported(compilerVersion string) bool {
	switch compilerVersion {
	case "strategy.compile.v1", "strategy.compile.v2":
		return true
	}
	return false
}

// MarketCard is one published market entry (§12.5 DTO whitelist).
type MarketCard struct {
	ID                 string          `json:"id"`
	MarketVersionID    string          `json:"market_version_id"`
	VersionNo          int             `json:"version_no"`
	Name               string          `json:"name"`
	Summary            string          `json:"summary"`
	Category           string          `json:"category"`
	Tags               json.RawMessage `json:"tags"`
	Markets            json.RawMessage `json:"markets"`
	SignalPeriod       string          `json:"signal_period"`
	ValidationStatus   string          `json:"validation_status"`
	BacktestStatus     string          `json:"backtest_status"`
	PublishedAt        *time.Time      `json:"published_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	Copyable           bool            `json:"copyable"`
	CopyDisabledReason string          `json:"copy_disabled_reason,omitempty"`
}

type EvidenceSummary struct {
	EvidenceID             string    `json:"evidence_id"`
	ValidationInstrumentID string    `json:"validation_instrument_id"`
	ResultHash             string    `json:"result_hash"`
	CreatedAt              time.Time `json:"created_at"`
}

type MarketDetail struct {
	MarketCard
	Description         string            `json:"description"`
	Hypothesis          string            `json:"hypothesis"`
	FailureCases        string            `json:"failure_cases"`
	Sources             json.RawMessage   `json:"sources"`
	RightsNote          string            `json:"rights_note"`
	EditorSchemaVersion string            `json:"editor_schema_version"`
	EditorState         json.RawMessage   `json:"editor_state"`
	BacktestDefaults    json.RawMessage   `json:"backtest_defaults"`
	EvidenceSummaries   []EvidenceSummary `json:"evidence_summaries"`
}

// SourceEntry is one sources[] record: title + URL or internal record ref +
// collection date + original/adapted note (§12.2).
type SourceEntry struct {
	Title       string `json:"title"`
	URL         string `json:"url,omitempty"`
	RecordRef   string `json:"record_ref,omitempty"`
	CollectedAt string `json:"collected_at"`
	Adaptation  string `json:"adaptation"`
}

// BacktestDefaults is the whitelist of market-provided backtest suggestions.
type BacktestDefaults struct {
	InitialCash      *string `json:"initial_cash,omitempty"`
	Currency         *string `json:"currency,omitempty"`
	Start            *string `json:"start,omitempty"`
	End              *string `json:"end,omitempty"`
	SlippageBps      *string `json:"slippage_bps,omitempty"`
	ParticipationCap *string `json:"participation_cap,omitempty"`
	CommissionConfig *string `json:"commission_config,omitempty"`
	FeeScheduleID    *string `json:"fee_schedule_id,omitempty"`
	Benchmark        *string `json:"benchmark,omitempty"`
}

type VersionContent struct {
	Name                string          `json:"name"`
	Summary             string          `json:"summary"`
	Category            string          `json:"category"`
	Tags                json.RawMessage `json:"tags"`
	Markets             json.RawMessage `json:"markets"`
	SignalPeriod        string          `json:"signal_period"`
	Description         string          `json:"description"`
	Hypothesis          string          `json:"hypothesis"`
	FailureCases        string          `json:"failure_cases"`
	Sources             json.RawMessage `json:"sources"`
	RightsNote          string          `json:"rights_note"`
	EditorSchemaVersion string          `json:"editor_schema_version"`
	RuleTemplate        json.RawMessage `json:"rule_template"`
	BacktestDefaults    json.RawMessage `json:"backtest_defaults"`
}

type CreateItemReq struct {
	Slug    string         `json:"slug"`
	Version VersionContent `json:"version"`
}

type CreateVersionReq struct {
	Revision int            `json:"revision"`
	Version  VersionContent `json:"version"`
}

type ValidateReq struct {
	Revision               int    `json:"revision"`
	ValidationInstrumentID string `json:"validation_instrument_id"`
}

type PublishReq struct {
	Revision        int    `json:"revision"`
	MarketVersionID string `json:"market_version_id"`
	Reason          string `json:"reason"`
}

type WithdrawReq struct {
	Revision int    `json:"revision"`
	Reason   string `json:"reason"`
}

type CopyOverrides struct {
	InstrumentID string `json:"instrument_id,omitempty"`
	InitialCash  string `json:"initial_cash,omitempty"`
	Currency     string `json:"currency,omitempty"`
	Start        string `json:"start,omitempty"`
	End          string `json:"end,omitempty"`
}

type CopyReq struct {
	MarketVersionID string         `json:"market_version_id"`
	Overrides       *CopyOverrides `json:"overrides,omitempty"`
}

type EvidenceImportReq struct {
	Revision    int    `json:"revision"`
	SourceRunID string `json:"source_run_id"`
	RightsNote  string `json:"rights_note"`
}

type EvidenceReviewReq struct {
	Revision int    `json:"revision"`
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}
