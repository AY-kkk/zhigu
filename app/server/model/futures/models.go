// Package futuresmodels contains futures-only persistence records. No finance
// model is referenced here, keeping the old research domain isolated.
package futuresmodels

import (
	"time"

	"gorm.io/datatypes"
)

type Scope struct {
	Domain  string `json:"domain"`
	Mode    string `json:"mode"`
	OwnerID uint   `json:"owner_id"`
}

type DraftInput struct {
	ProductID   string  `json:"product_id"`
	ContractID  *string `json:"contract_id"`
	HorizonDays int     `json:"horizon_days"`
	Text        string  `json:"text"`
	DocumentID  *string `json:"document_id"`
}

type Draft struct {
	ID                 string         `gorm:"column:id;primaryKey" json:"id"`
	OwnerID            uint           `gorm:"column:owner_id" json:"-"`
	Mode               string         `gorm:"column:mode" json:"-"`
	Revision           int            `gorm:"column:revision" json:"revision"`
	Input              DraftInput     `gorm:"column:input;serializer:json" json:"input"`
	ParseState         string         `gorm:"column:parse_state" json:"parse_state"`
	ParsedRevision     *int           `gorm:"column:parsed_revision" json:"parsed_revision"`
	Claims             datatypes.JSON `gorm:"column:claims" json:"claims"`
	ClaimsOverflow     bool           `gorm:"column:claims_overflow" json:"claims_overflow"`
	ConsumedModelCalls int            `gorm:"column:consumed_model_calls" json:"consumed_model_calls"`
	ConsumedTokens     int            `gorm:"column:consumed_tokens" json:"consumed_tokens"`
	CreatedAt          time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt          time.Time      `gorm:"column:updated_at" json:"-"`
	DeletedAt          *time.Time     `gorm:"column:deleted_at" json:"-"`
}

func (Draft) TableName() string { return "futures_drafts" }

type Run struct {
	ID              string         `gorm:"column:id;primaryKey" json:"id"`
	OwnerID         uint           `gorm:"column:owner_id" json:"-"`
	Mode            string         `gorm:"column:mode" json:"-"`
	DraftID         string         `gorm:"column:draft_id" json:"draft_id"`
	DraftRevision   int            `gorm:"column:draft_revision" json:"draft_revision"`
	Status          string         `gorm:"column:status" json:"status"`
	Stage           string         `gorm:"column:stage" json:"stage"`
	AsOf            time.Time      `gorm:"column:as_of" json:"as_of"`
	HorizonEnd      time.Time      `gorm:"column:horizon_end" json:"horizon_end"`
	Report          datatypes.JSON `gorm:"column:report" json:"report"`
	FailureCode     *string        `gorm:"column:failure_code" json:"failure_code"`
	IdempotencyKey  string         `gorm:"column:idempotency_key" json:"-"`
	RequestHash     string         `gorm:"column:request_hash" json:"-"`
	ManifestID      string         `gorm:"column:manifest_id" json:"-"`
	ClaimsSnapshot  datatypes.JSON `gorm:"column:claims_snapshot" json:"-"`
	Versions        datatypes.JSON `gorm:"column:versions" json:"-"`
	TaskSnapshot    datatypes.JSON `gorm:"column:task_snapshot" json:"-"`
	Generation      int64          `gorm:"column:generation" json:"-"`
	CancelRequested bool           `gorm:"column:cancel_requested" json:"-"`
	LeaseUntil      *time.Time     `gorm:"column:lease_until" json:"-"`
	LeaseOwner      *string        `gorm:"column:lease_owner" json:"-"`
	DeadlineAt      *time.Time     `gorm:"column:deadline_at" json:"-"`
	Version         int64          `gorm:"column:version" json:"-"`
	CreatedAt       time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time      `gorm:"column:updated_at" json:"-"`
	DeletedAt       *time.Time     `gorm:"column:deleted_at" json:"-"`
}

func (Run) TableName() string { return "futures_runs" }

type EvidenceLink struct {
	ID        string     `gorm:"column:id;primaryKey"`
	OwnerID   uint       `gorm:"column:owner_id"`
	Mode      string     `gorm:"column:mode"`
	RunID     string     `gorm:"column:run_id"`
	RecordID  string     `gorm:"column:record_id"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	DeletedAt *time.Time `gorm:"column:deleted_at"`
}

type Document struct {
	ID               string     `gorm:"column:id;primaryKey" json:"id"`
	OwnerID          uint       `gorm:"column:owner_id" json:"-"`
	Mode             string     `gorm:"column:mode" json:"-"`
	Filename         string     `gorm:"column:filename" json:"-"`
	MediaType        string     `gorm:"column:media_type" json:"mime"`
	ByteSize         int64      `gorm:"column:byte_size" json:"bytes"`
	ContentHash      string     `gorm:"column:content_hash" json:"-"`
	StorageKey       string     `gorm:"column:storage_key" json:"-"`
	ExtractionStatus string     `gorm:"column:extraction_status" json:"status"`
	PageCount        *int       `gorm:"column:page_count" json:"page_count"`
	WordCount        *int       `gorm:"column:word_count" json:"character_count"`
	FailureCode      *string    `gorm:"column:failure_code" json:"failure_code"`
	ExtractedText    string     `gorm:"column:extracted_text" json:"-"`
	PurgeAt          time.Time  `gorm:"column:purge_at" json:"-"`
	CreatedAt        time.Time  `gorm:"column:created_at" json:"-"`
	DeletedAt        *time.Time `gorm:"column:deleted_at" json:"-"`
}

func (Document) TableName() string { return "futures_documents" }

func (EvidenceLink) TableName() string { return "futures_evidence_links" }

type Hypothesis struct {
	ID                   string         `gorm:"column:id;primaryKey" json:"id"`
	OwnerID              uint           `gorm:"column:owner_id" json:"-"`
	Mode                 string         `gorm:"column:mode" json:"-"`
	Version              int            `gorm:"column:version" json:"version"`
	RunID                string         `gorm:"column:run_id" json:"run_id"`
	ReportID             string         `gorm:"column:report_id" json:"report_id"`
	ClaimIDs             datatypes.JSON `gorm:"column:claim_ids" json:"claim_ids"`
	Proposition          string         `gorm:"column:proposition" json:"proposition"`
	ProductID            string         `gorm:"column:product_id" json:"product_id"`
	ContractID           *string        `gorm:"column:contract_id" json:"contract_id"`
	ExpiresAt            time.Time      `gorm:"column:expires_at" json:"expires_at"`
	RetentionDeadline    time.Time      `gorm:"column:retention_deadline" json:"retention_deadline"`
	Lifecycle            string         `gorm:"column:lifecycle" json:"lifecycle"`
	UserView             string         `gorm:"column:user_view" json:"user_view"`
	ViewReason           *string        `gorm:"column:view_reason" json:"view_reason"`
	ReviewedCheckVersion *int           `gorm:"column:reviewed_check_version" json:"reviewed_check_version"`
	NeedsReview          bool           `gorm:"column:needs_review" json:"needs_review"`
	Conditions           datatypes.JSON `gorm:"column:conditions" json:"conditions"`
	CreatedAt            time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt            time.Time      `gorm:"column:updated_at" json:"-"`
	DeletedAt            *time.Time     `gorm:"column:deleted_at" json:"-"`
}

func (Hypothesis) TableName() string { return "futures_hypotheses" }

type Check struct {
	ID                string         `gorm:"column:id;primaryKey" json:"id"`
	OwnerID           uint           `gorm:"column:owner_id" json:"-"`
	Mode              string         `gorm:"column:mode" json:"-"`
	HypothesisID      string         `gorm:"column:hypothesis_id" json:"hypothesis_id"`
	HypothesisVersion int64          `gorm:"column:hypothesis_version" json:"hypothesis_version"`
	ConditionID       string         `gorm:"column:condition_id" json:"condition_id"`
	Version           int64          `gorm:"column:version" json:"version"`
	Result            string         `gorm:"column:result" json:"result"`
	EvidenceIDs       datatypes.JSON `gorm:"column:evidence_ids" json:"evidence_ids"`
	Reason            string         `gorm:"column:reason" json:"reason"`
	CheckedAt         time.Time      `gorm:"column:checked_at" json:"checked_at"`
	CreatedAt         time.Time      `gorm:"column:created_at" json:"-"`
}

func (Check) TableName() string { return "futures_checks" }

type Idempotency struct {
	OwnerID     uint           `gorm:"column:owner_id;primaryKey"`
	Mode        string         `gorm:"column:mode;primaryKey"`
	Operation   string         `gorm:"column:operation;primaryKey"`
	Key         string         `gorm:"column:key;primaryKey"`
	RequestHash string         `gorm:"column:request_hash"`
	Response    datatypes.JSON `gorm:"column:response"`
	Status      string         `gorm:"column:status"`
	ExpiresAt   time.Time      `gorm:"column:expires_at"`
	CreatedAt   time.Time      `gorm:"column:created_at"`
}

func (Idempotency) TableName() string { return "futures_idempotency_keys" }
