package intel

import (
	"time"

	"github.com/shopspring/decimal"
)

const CoreClaimKey = "__core__"

type EventType string

const (
	EventAcquisition             EventType = "acquisition"
	EventEarningsForecast        EventType = "earnings_forecast"
	EventRegulatoryInvestigation EventType = "regulatory_investigation"
)

type Grade string

const (
	GradeFact      Grade = "fact"
	GradeOpinion   Grade = "opinion"
	GradeInference Grade = "inference"
	GradeRumor     Grade = "rumor"
)

func (g Grade) Factor() decimal.Decimal {
	switch g {
	case GradeFact:
		return decimal.NewFromInt(10).Div(decimal.NewFromInt(10))
	case GradeOpinion:
		return decimal.NewFromInt(6).Div(decimal.NewFromInt(10))
	case GradeInference:
		return decimal.NewFromInt(3).Div(decimal.NewFromInt(10))
	case GradeRumor:
		return decimal.NewFromInt(1).Div(decimal.NewFromInt(10))
	default:
		return decimal.Zero
	}
}

type Stance string

const (
	StanceSupport Stance = "support"
	StanceRefute  Stance = "refute"
	StanceContext Stance = "context"
)

type EvidenceStatus string

const (
	EvidenceActive      EvidenceStatus = "active"
	EvidenceSuperseded  EvidenceStatus = "superseded"
	EvidenceRetracted   EvidenceStatus = "retracted"
	EvidenceQuarantined EvidenceStatus = "quarantined"
)

type SupportLevel string

const (
	SupportLow     SupportLevel = "low"
	SupportMedium  SupportLevel = "medium"
	SupportHigh    SupportLevel = "high"
	SupportUnknown SupportLevel = "unknown"
)

type Verification string

const (
	VerificationUnverified Verification = "unverified"
	VerificationConfirmed  Verification = "confirmed"
	VerificationDenied     Verification = "denied"
	VerificationDisputed   Verification = "disputed"
)

type Freshness string

const (
	FreshnessFresh   Freshness = "fresh"
	FreshnessStale   Freshness = "stale"
	FreshnessExpired Freshness = "expired"
	FreshnessUnknown Freshness = "unknown"
)

type EvidenceRule struct {
	ID               string
	ClaimKey         string
	Value            string
	SourceCluster    string
	SourceWeight     decimal.Decimal
	Grade            Grade
	Stance           Stance
	Status           EvidenceStatus
	Authoritative    bool
	Direct           bool
	SubjectUnique    bool
	ModalityExplicit bool
	Supersedes       []string
}

type ClaimEvaluation struct {
	SupportScore           decimal.Decimal
	RefuteScore            decimal.Decimal
	SupportLevel           SupportLevel
	SupportEvidenceIDs     []string
	RefuteEvidenceIDs      []string
	ConflictingEvidenceIDs []string
	ConflictOpen           bool
	CapReason              string
	Uncertain              bool
}

type FreshnessInput struct {
	LatestValidAt     *time.Time
	CandidateAt       *time.Time
	CandidateExpected bool
	AsOf              time.Time
	CoverageComplete  bool
}
