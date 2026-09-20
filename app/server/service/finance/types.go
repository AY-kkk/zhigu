package finance

import "time"

type ClaimItem struct {
	ClaimID   string `json:"claim_id"`
	Text      string `json:"text"`
	ClaimType string `json:"claim_type"`
}

type Claim struct {
	Text    string      `json:"text"`
	Horizon string      `json:"horizon"`
	Items   []ClaimItem `json:"items"`
}

type ResearchTask struct {
	SchemaVersion       string    `json:"schema_version"`
	RunID               string    `json:"run_id"`
	TaskID              string    `json:"task_id"`
	Role                string    `json:"role"`
	Claim               Claim     `json:"claim"`
	InstrumentID        string    `json:"instrument_id"`
	AsOf                time.Time `json:"as_of"`
	Mode                string    `json:"mode"`
	SourcePolicyVersion string    `json:"source_policy_version"`
	ModelConfigVersion  string    `json:"model_config_version"`
	PromptVersion       string    `json:"prompt_version"`
	MaxModelCalls       int       `json:"max_model_calls"`
	MaxToolCalls        int       `json:"max_tool_calls"`
	DeadlineAt          time.Time `json:"deadline_at"`
	TaskToken           string    `json:"-"`
}

type Argument struct {
	ClaimType   string   `json:"claim_type"`
	Text        string   `json:"text"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type Usage struct {
	ModelCalls   int  `json:"model_calls"`
	ToolCalls    int  `json:"tool_calls"`
	InputTokens  int  `json:"input_tokens"`
	OutputTokens int  `json:"output_tokens"`
	UsageUnknown bool `json:"usage_unknown"`
	Simulated    bool `json:"simulated"`
}

type ResultError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type ResearchResult struct {
	SchemaVersion   string        `json:"schema_version"`
	RunID           string        `json:"run_id"`
	TaskID          string        `json:"task_id"`
	Status          string        `json:"status"`
	Arguments       []Argument    `json:"arguments"`
	EvidenceIDs     []string      `json:"evidence_ids"`
	Unknowns        []string      `json:"unknowns"`
	Counterevidence []Argument    `json:"counterevidence"`
	Usage           Usage         `json:"usage"`
	Errors          []ResultError `json:"errors"`
}

type VerifiedReport struct {
	SchemaVersion       string     `json:"schema_version"`
	RunID               string     `json:"run_id"`
	Version             int        `json:"version"`
	Mode                string     `json:"mode"`
	AsOf                time.Time  `json:"as_of"`
	QualityStatus       string     `json:"quality_status"`
	Verdict             *string    `json:"verdict"`
	Summary             string     `json:"summary"`
	Support             []Argument `json:"support"`
	Challenge           []Argument `json:"challenge"`
	Assumptions         []string   `json:"assumptions"`
	ChangeConditions    []string   `json:"change_conditions"`
	Unknowns            []string   `json:"unknowns"`
	EvidenceIDs         []string   `json:"evidence_ids"`
	ModelConfigVersion  string     `json:"model_config_version"`
	SourcePolicyVersion string     `json:"source_policy_version"`
	PromptVersion       string     `json:"prompt_version"`
}

type RunSnapshot struct {
	ID             string
	TaskID         string
	OwnerID        uint
	InstrumentID   string
	Horizon        string
	AsOf           time.Time
	Mode           string
	Status         string
	Stage          string
	Version        int64
	DeletedAt      *time.Time
	ConfigVersions map[string]string
	Claim          Claim
	DeadlineAt     *time.Time
}

type ValidatedRole struct {
	role      string
	result    ResearchResult
	arguments []Argument
	evidence  []string
}

func NewValidatedRole(role string, result ResearchResult) ValidatedRole {
	return ValidatedRole{role: role, result: result, arguments: result.Arguments, evidence: result.EvidenceIDs}
}

func (v ValidatedRole) Role() string            { return v.role }
func (v ValidatedRole) Result() ResearchResult  { return v.result }
func (v ValidatedRole) Arguments() []Argument   { return v.arguments }
func (v ValidatedRole) EvidenceIDs() []string   { return v.evidence }

type CreateResearchInput struct {
	DraftID      string
	Revision     int
	InstrumentID string
	Horizon      string
	AsOf         time.Time
	ParentRunID  string
	ClaimText    string
}

type CreateResearchOutput struct {
	RunID   string `json:"run_id"`
	Status  string `json:"status"`
	PollURL string `json:"poll_url"`
}

type ParseInput struct {
	Text string
	AsOf *time.Time
}

type ParseOutput struct {
	DraftID           string         `json:"draft_id"`
	Revision          int            `json:"revision"`
	Candidates        []Instrument   `json:"candidates"`
	SuggestedHorizon  string         `json:"suggested_horizon"`
	Items             []ClaimItem    `json:"items"`
	NeedsConfirmation bool           `json:"needs_confirmation"`
	Mode              string         `json:"mode"`
}

type Instrument struct {
	ID     string `json:"instrument_id"`
	Symbol string `json:"symbol"`
	Name   string `json:"name"`
}

type ResearchView struct {
	RunID        string          `json:"run_id"`
	Status       string          `json:"status"`
	Stage        string          `json:"stage"`
	Mode         string          `json:"mode"`
	AsOf         time.Time       `json:"as_of"`
	InstrumentID string          `json:"instrument_id,omitempty"`
	Horizon      string          `json:"horizon,omitempty"`
	Claim        *Claim          `json:"claim,omitempty"`
	Report       *VerifiedReport `json:"report"`
	Warnings     []string        `json:"warnings"`
	Error        *APIErrorBody   `json:"error"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type APIErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type HistoryPage struct {
	Items      []HistoryItem `json:"items"`
	NextCursor string        `json:"next_cursor"`
}

type HistoryItem struct {
	RunID        string    `json:"run_id"`
	Status       string    `json:"status"`
	InstrumentID string    `json:"instrument_id"`
	Mode         string    `json:"mode"`
	CreatedAt    time.Time `json:"created_at"`
}

type AdminRunItem struct {
	RunID       string `json:"run_id"`
	UserRef     string `json:"user_id"`
	Stage       string `json:"stage"`
	Status      string `json:"status"`
	Mode        string `json:"mode"`
	DurationMS  int64  `json:"duration_ms"`
	UsageTokens int64  `json:"usage_tokens"`
	ErrorCode   string `json:"error_code"`
}

type PolicyView struct {
	MaxModelCalls        int    `json:"max_model_calls"`
	MaxToolCalls         int    `json:"max_tool_calls"`
	RunTimeoutSeconds    int    `json:"run_timeout_seconds"`
	QueueTimeoutSeconds  int    `json:"queue_timeout_seconds"`
	Note                 string `json:"note"`
}

const (
	StatusQueued      = "queued"
	StatusResearching = "researching"
	StatusVerifying   = "verifying"
	StatusCompleted   = "completed"
	StatusIncomplete  = "incomplete"
	StatusFailed      = "failed"
	StatusCanceling   = "canceling"
	StatusCanceled    = "canceled"

	ModeFixture = "fixture"
	ModeLive    = "live"

	RoleSupporter  = "supporter"
	RoleChallenger = "challenger"

	InstrumentDemo = "DEMO:COMPANY"
)
