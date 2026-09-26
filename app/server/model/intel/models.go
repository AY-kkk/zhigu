package intel

import "time"

// Models mirror finance_intel_* tables. The application uses explicit SQL and
// these structs for scanning; GORM AutoMigrate is intentionally not used.
type Namespace struct {
	ID           string
	Kind         string
	PrincipalKey string
	Generation   int64
	ReplayVersion int64
	Branch       string
	StepIndex    int
	SimulatedAt  *time.Time
	ExpiresAt    *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Instrument struct {
	ID          string
	NamespaceID string
	Code        string
	Name        string
	Exchange    string
	Source      string
	VerifiedAt  *time.Time
	CreatedAt   time.Time
}

type SourceRevision struct {
	ID                string
	NamespaceID       string
	SourceID          string
	RevisionNo        int
	ContentHash       string
	Title             string
	AllowedExcerpt    string
	Locator           map[string]any
	DisclosedAt       *time.Time
	DisclosedDate     string
	OccurredAt        *time.Time
	FetchedAt         time.Time
	SourceUpdatedAt   *time.Time
	AccessStatus      string
	IsRepost          bool
	SupersedesRevisionID string
	CreatedAt         time.Time
}

type Event struct {
	ID                  string
	NamespaceID         string
	EventType           string
	SubjectCode         string
	SubjectName         string
	MatterKey           string
	Title               string
	CoreClaimKey        string
	CoreClaimText       string
	CurrentVersion      int
	CurrentSnapshotID   string
	LatestValidDisclosedAt *time.Time
	Muted               bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Evidence struct {
	ID                 string
	NamespaceID        string
	EventID            string
	SourceRevisionID   string
	ClaimKey           string
	Subject            string
	Predicate          string
	Object             string
	Period             string
	Currency           string
	Basis              string
	Modality           string
	Grade              string
	SourceWeight       string
	EffectiveWeight    string
	Direction          string
	ValueJSON          map[string]any
	Quote              string
	Locator            map[string]any
	SourceCluster      string
	Active             bool
	MembershipStatus   string
	SupersedesEvidenceID string
	CreatedAt          time.Time
}

type Snapshot struct {
	ID            string
	NamespaceID   string
	EventID       string
	EventVersion  int
	Verification  string
	Phase         string
	Freshness     string
	SupportScore  string
	SupportLevel  string
	CurrentValues []CurrentValue
	Conclusion    string
	CalcTrace     map[string]any
	Coverage      map[string]any
	AsOf          time.Time
	CreatedAt     time.Time
}

type CurrentValue struct {
	Field       string   `json:"field"`
	Value       string   `json:"value"`
	Currency    string   `json:"currency,omitempty"`
	Unit        string   `json:"unit,omitempty"`
	EvidenceIDs []string `json:"evidence_ids"`
	Status      string   `json:"status,omitempty"`
}

type Change struct {
	ID               string
	NamespaceID      string
	EventID          string
	EventVersion     int
	Kind             string
	Summary          string
	BeforeSnapshotID string
	AfterSnapshotID  string
	Diff             map[string]any
	SourceRevisionID string
	CreatedAt        time.Time
}

type Notification struct {
	ID          string
	NamespaceID string
	PrincipalKey string
	EventID     string
	ChangeID    string
	OutboxID    string
	Title       string
	Reason      string
	Before      map[string]any
	After       map[string]any
	TargetCodes []string
	LinkVersion int
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
