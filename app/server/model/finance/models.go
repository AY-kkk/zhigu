package finance

import (
	"time"

	"gorm.io/datatypes"
)

type User struct {
	ID           uint      `gorm:"column:id;primaryKey"`
	Username     string    `gorm:"column:username"`
	PasswordHash string    `gorm:"column:password_hash"`
	Role         string    `gorm:"column:role"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (User) TableName() string { return "finance_users" }

type ClaimDraft struct {
	ID                 string         `gorm:"column:id;primaryKey"`
	OwnerID            uint           `gorm:"column:owner_id"`
	Text               string         `gorm:"column:text"`
	Horizon            *string        `gorm:"column:horizon"`
	HorizonStart       *string        `gorm:"column:horizon_start"`
	HorizonEnd         *string        `gorm:"column:horizon_end"`
	InstrumentID       *string        `gorm:"column:instrument_id"`
	Items              datatypes.JSON `gorm:"column:items"`
	SourceMode         string         `gorm:"column:source_mode;default:claim_only"`
	DocumentID         *string        `gorm:"column:document_id"`
	FocusText          *string        `gorm:"column:focus_text"`
	Revision           int            `gorm:"column:revision"`
	AsOf               time.Time      `gorm:"column:as_of"`
	Mode               string         `gorm:"column:mode"`
	ParseStatus        string         `gorm:"column:parse_status"`
	ModelConfigVersion *string        `gorm:"column:model_config_version"`
	Protocol           *string        `gorm:"column:protocol"`
	ScopeOrigin        *string        `gorm:"column:scope_origin"`
	ParseUsageID       *string        `gorm:"column:parse_usage_id"`
	ConfirmedRunID     *string        `gorm:"column:confirmed_run_id"`
	ConfigVersions     datatypes.JSON `gorm:"column:config_versions"`
	CreatedAt          time.Time      `gorm:"column:created_at"`
	UpdatedAt          time.Time      `gorm:"column:updated_at"`
}

func (ClaimDraft) TableName() string { return "finance_claim_drafts" }

type ResearchRun struct {
	ID             string         `gorm:"column:id;primaryKey"`
	OwnerID        uint           `gorm:"column:owner_id"`
	DraftID        string         `gorm:"column:draft_id"`
	ParentRunID    *string        `gorm:"column:parent_run_id"`
	ClaimSnapshot  datatypes.JSON `gorm:"column:claim_snapshot"`
	DocumentID     *string        `gorm:"column:document_id"`
	InputMode      string         `gorm:"column:input_mode;default:claim_only"`
	InstrumentID   string         `gorm:"column:instrument_id"`
	Horizon        string         `gorm:"column:horizon"`
	AsOf           time.Time      `gorm:"column:as_of"`
	Mode           string         `gorm:"column:mode"`
	Status         string         `gorm:"column:status"`
	Stage          string         `gorm:"column:stage"`
	ConfigVersions datatypes.JSON `gorm:"column:config_versions"`
	BudgetSnapshot datatypes.JSON `gorm:"column:budget_snapshot"`
	IdempotencyKey string         `gorm:"column:idempotency_key"`
	RequestHash    string         `gorm:"column:request_hash"`
	StartedAt      *time.Time     `gorm:"column:started_at"`
	DeadlineAt     *time.Time     `gorm:"column:deadline_at"`
	LeaseOwner     *string        `gorm:"column:lease_owner"`
	LeaseUntil     *time.Time     `gorm:"column:lease_until"`
	Version        int64          `gorm:"column:version"`
	ExecutionEpoch int64          `gorm:"column:execution_epoch"`
	ClaimResults   datatypes.JSON `gorm:"column:claim_results"`
	DeletedAt      *time.Time     `gorm:"column:deleted_at"`
	CreatedAt      time.Time      `gorm:"column:created_at"`
	UpdatedAt      time.Time      `gorm:"column:updated_at"`
}

func (ResearchRun) TableName() string { return "finance_research_runs" }

type ResearchDocument struct {
	ID               string     `gorm:"column:id;primaryKey"`
	OwnerID          uint       `gorm:"column:owner_id"`
	DraftID          *string    `gorm:"column:draft_id"`
	RunID            *string    `gorm:"column:run_id"`
	Filename         string     `gorm:"column:filename"`
	MediaType        string     `gorm:"column:media_type"`
	ByteSize         int64      `gorm:"column:byte_size"`
	ContentHash      string     `gorm:"column:content_hash"`
	StorageKey       string     `gorm:"column:storage_key"`
	ExtractionStatus string     `gorm:"column:extraction_status"`
	ExtractedText    *string    `gorm:"column:extracted_text"`
	ExtractionError  *string    `gorm:"column:extraction_error"`
	PageCount        *int       `gorm:"column:page_count"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
	DeletedAt        *time.Time `gorm:"column:deleted_at"`
}

func (ResearchDocument) TableName() string { return "finance_research_documents" }

type ResearchDocumentSpan struct {
	ID             string    `gorm:"column:id;primaryKey"`
	DocumentID     string    `gorm:"column:document_id"`
	PageNumber     *int      `gorm:"column:page_number"`
	ParagraphIndex *int      `gorm:"column:paragraph_index"`
	StartOffset    int       `gorm:"column:start_offset"`
	EndOffset      int       `gorm:"column:end_offset"`
	Text           string    `gorm:"column:text"`
	ContentHash    string    `gorm:"column:content_hash"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (ResearchDocumentSpan) TableName() string { return "finance_research_document_spans" }

type ClaimFactCheck struct {
	ID          string         `gorm:"column:id;primaryKey"`
	RunID       string         `gorm:"column:run_id"`
	ClaimID     string         `gorm:"column:claim_id"`
	Status      string         `gorm:"column:status"`
	Reason      string         `gorm:"column:reason"`
	EvidenceIDs datatypes.JSON `gorm:"column:evidence_ids"`
	CreatedAt   time.Time      `gorm:"column:created_at"`
}

func (ClaimFactCheck) TableName() string { return "finance_claim_fact_checks" }

type ResearchTask struct {
	ID           string         `gorm:"column:id;primaryKey"`
	RunID        string         `gorm:"column:run_id"`
	Role         string         `gorm:"column:role"`
	Status       string         `gorm:"column:status"`
	PayloadHash  string         `gorm:"column:payload_hash"`
	Attempt      int            `gorm:"column:attempt"`
	WorkerTaskID *string        `gorm:"column:worker_task_id"`
	Result       datatypes.JSON `gorm:"column:result"`
	HeartbeatAt  *time.Time     `gorm:"column:heartbeat_at"`
	CreatedAt    time.Time      `gorm:"column:created_at"`
	UpdatedAt    time.Time      `gorm:"column:updated_at"`
}

func (ResearchTask) TableName() string { return "finance_research_tasks" }

type Evidence struct {
	ID           string         `gorm:"column:id;primaryKey"`
	RunID        string         `gorm:"column:run_id"`
	InstrumentID string         `gorm:"column:instrument_id"`
	SourceID     string         `gorm:"column:source_id"`
	SourceURL    string         `gorm:"column:source_url"`
	SourceKind   string         `gorm:"column:source_kind"`
	SourceGrade  string         `gorm:"column:source_grade;default:structured_data"`
	VerificationStatus string   `gorm:"column:verification_status;default:independent_verified"`
	Title        string         `gorm:"column:title"`
	Locator      string         `gorm:"column:locator"`
	Text         string         `gorm:"column:text"`
	Metrics      datatypes.JSON `gorm:"column:metrics"`
	PublishedAt  time.Time      `gorm:"column:published_at"`
	AvailableAt  time.Time      `gorm:"column:available_at"`
	RetrievedAt  time.Time      `gorm:"column:retrieved_at"`
	ContentHash  string         `gorm:"column:content_hash"`
	DataVersion  string         `gorm:"column:data_version"`
	Mode         string         `gorm:"column:mode"`
	CreatedAt    time.Time      `gorm:"column:created_at"`
	UpdatedAt    time.Time      `gorm:"column:updated_at"`
}

func (Evidence) TableName() string { return "finance_evidence" }

type TaskEvidence struct {
	TaskID     string `gorm:"column:task_id;primaryKey"`
	EvidenceID string `gorm:"column:evidence_id;primaryKey"`
}

func (TaskEvidence) TableName() string { return "finance_task_evidence" }

type Calculation struct {
	ID        string         `gorm:"column:id;primaryKey"`
	RunID     string         `gorm:"column:run_id"`
	TaskID    string         `gorm:"column:task_id"`
	GrantID   string         `gorm:"column:grant_id"`
	Operation string         `gorm:"column:operation"`
	Inputs    datatypes.JSON `gorm:"column:inputs"`
	Formula   string         `gorm:"column:formula"`
	Precision int            `gorm:"column:precision"`
	Result    string         `gorm:"column:result;type:numeric"`
	Unit      string         `gorm:"column:unit"`
	CreatedAt time.Time      `gorm:"column:created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
}

func (Calculation) TableName() string { return "finance_calculations" }

type Report struct {
	ID               string         `gorm:"column:id;primaryKey"`
	RunID            string         `gorm:"column:run_id"`
	Version          int            `gorm:"column:version"`
	QualityStatus    string         `gorm:"column:quality_status"`
	Body             datatypes.JSON `gorm:"column:body"`
	ValidatorVersion string         `gorm:"column:validator_version"`
	Verification     datatypes.JSON `gorm:"column:verification"`
	PublishedAt      *time.Time     `gorm:"column:published_at"`
	CreatedAt        time.Time      `gorm:"column:created_at"`
	UpdatedAt        time.Time      `gorm:"column:updated_at"`
}

func (Report) TableName() string { return "finance_reports" }

type Question struct {
	ID             string         `gorm:"column:id;primaryKey"`
	RunID          string         `gorm:"column:run_id"`
	OwnerID        uint           `gorm:"column:owner_id"`
	IdempotencyKey string         `gorm:"column:idempotency_key"`
	RequestHash    string         `gorm:"column:request_hash"`
	Body           string         `gorm:"column:body"`
	Response       datatypes.JSON `gorm:"column:response"`
	Status         string         `gorm:"column:status"`
	CreatedAt      time.Time      `gorm:"column:created_at"`
	UpdatedAt      time.Time      `gorm:"column:updated_at"`
}

func (Question) TableName() string { return "finance_questions" }

type ConfigVersion struct {
	ID               string         `gorm:"column:id;primaryKey"`
	Kind             string         `gorm:"column:kind"`
	ConfigDigest     string         `gorm:"column:config_digest"`
	PublicConfig     datatypes.JSON `gorm:"column:public_config"`
	SecretCiphertext []byte         `gorm:"column:secret_ciphertext"`
	SecretKeyVersion *string        `gorm:"column:secret_key_version"`
	TestStatus       string         `gorm:"column:test_status"`
	TestDigest       *string        `gorm:"column:test_digest"`
	ActiveAt         *time.Time     `gorm:"column:active_at"`
	CreatedAt        time.Time      `gorm:"column:created_at"`
	UpdatedAt        time.Time      `gorm:"column:updated_at"`
}

func (ConfigVersion) TableName() string { return "finance_config_versions" }

type UsageLedger struct {
	ID                string    `gorm:"column:id;primaryKey"`
	OwnerID           uint      `gorm:"column:owner_id"`
	RunID             *string   `gorm:"column:run_id"`
	TaskID            *string   `gorm:"column:task_id"`
	QuestionID        *string   `gorm:"column:question_id"`
	ParseID           *string   `gorm:"column:parse_id"`
	RequestID         string    `gorm:"column:request_id"`
	Purpose           string    `gorm:"column:purpose"`
	Kind              string    `gorm:"column:kind"`
	Status            string    `gorm:"column:status"`
	ReservedTokens    int       `gorm:"column:reserved_tokens"`
	ActualTokens      *int      `gorm:"column:actual_tokens"`
	UpstreamRequestID *string   `gorm:"column:upstream_request_id"`
	ErrorCode         *string   `gorm:"column:error_code"`
	CreatedAt         time.Time `gorm:"column:created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at"`
}

func (UsageLedger) TableName() string { return "finance_usage_ledger" }

type ToolGrant struct {
	ID         string    `gorm:"column:id;primaryKey"`
	RequestID  string    `gorm:"column:request_id"`
	RunID      string    `gorm:"column:run_id"`
	TaskID     string    `gorm:"column:task_id"`
	ToolName   string    `gorm:"column:tool_name"`
	ArgsHash   string    `gorm:"column:args_hash"`
	Status     string    `gorm:"column:status"`
	ExpiresAt  time.Time `gorm:"column:expires_at"`
	OutputHash *string   `gorm:"column:output_hash"`
	Executed   bool      `gorm:"column:executed"`
	ParamsHash string    `gorm:"column:params_hash"`
	CreatedAt  time.Time `gorm:"column:created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

func (ToolGrant) TableName() string { return "finance_tool_grants" }

type DataRecord struct {
	ID         string         `gorm:"column:id;primaryKey"`
	GrantID    string         `gorm:"column:grant_id"`
	RecordHash string         `gorm:"column:record_hash"`
	Payload    datatypes.JSON `gorm:"column:payload"`
	CreatedAt  time.Time      `gorm:"column:created_at"`
}

func (DataRecord) TableName() string { return "finance_data_records" }

type ProviderRecordRow struct {
	ID         string         `gorm:"column:id;primaryKey"`
	GrantID    string         `gorm:"column:grant_id"`
	RecordHash string         `gorm:"column:record_hash"`
	Payload    datatypes.JSON `gorm:"column:payload"`
	CreatedAt  time.Time      `gorm:"column:created_at"`
}

func (ProviderRecordRow) TableName() string { return "finance_provider_records" }

type ReportCheck struct {
	ID              string         `gorm:"column:id;primaryKey"`
	RunID           string         `gorm:"column:run_id"`
	Attempt         int            `gorm:"column:attempt"`
	CandidateHash   string         `gorm:"column:candidate_hash"`
	Pointer         string         `gorm:"column:pointer"`
	ClaimType       string         `gorm:"column:claim_type"`
	EvidenceIDs     datatypes.JSON `gorm:"column:evidence_ids"`
	NumericBindings datatypes.JSON `gorm:"column:numeric_bindings"`
	RuleVersion     string         `gorm:"column:rule_version"`
	ProgramResult   string         `gorm:"column:program_result"`
	SemanticResult  *string        `gorm:"column:semantic_result"`
	FailureReason   *string        `gorm:"column:failure_reason"`
	CreatedAt       time.Time      `gorm:"column:created_at"`
}

func (ReportCheck) TableName() string { return "finance_report_checks" }

type Scheduler struct {
	SingletonID   string    `gorm:"column:singleton_id;primaryKey"`
	PolicyVersion string    `gorm:"column:policy_version"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (Scheduler) TableName() string { return "finance_scheduler" }

type InternalToken struct {
	TokenHash string     `gorm:"column:token_hash;primaryKey"`
	RunID     string     `gorm:"column:run_id"`
	TaskID    string     `gorm:"column:task_id"`
	Purpose   string     `gorm:"column:purpose"`
	ExpiresAt time.Time  `gorm:"column:expires_at"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
}

func (InternalToken) TableName() string { return "finance_internal_tokens" }

type ModelCache struct {
	RunID     string         `gorm:"column:run_id;primaryKey"`
	TaskID    string         `gorm:"column:task_id;primaryKey"`
	RequestID string         `gorm:"column:request_id;primaryKey"`
	OwnerID   uint           `gorm:"column:owner_id"`
	Purpose   string         `gorm:"column:purpose"`
	BodyHash  string         `gorm:"column:body_hash"`
	Status    string         `gorm:"column:status"`
	Response  datatypes.JSON `gorm:"column:response"`
	CreatedAt time.Time      `gorm:"column:created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
}

func (ModelCache) TableName() string { return "finance_model_cache" }
