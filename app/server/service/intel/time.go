package intel

import (
	"time"
)

func EvaluateFreshness(in FreshnessInput) Freshness {
	if !in.CoverageComplete || in.LatestValidAt == nil || in.AsOf.IsZero() {
		return FreshnessUnknown
	}
	if in.CandidateAt != nil && !in.CandidateExpected && in.CandidateAt.After(*in.LatestValidAt) {
		return FreshnessUnknown
	}
	if in.AsOf.Before(*in.LatestValidAt) {
		return FreshnessUnknown
	}
	age := in.AsOf.Sub(*in.LatestValidAt)
	switch {
	case age < 14*24*time.Hour:
		return FreshnessFresh
	case age < 30*24*time.Hour:
		return FreshnessStale
	default:
		return FreshnessExpired
	}
}
