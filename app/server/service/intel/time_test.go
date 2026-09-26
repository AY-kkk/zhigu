package intel

import (
	"testing"
	"time"
)

func TestFreshnessBoundariesAndCoverageGate(t *testing.T) {
	base := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		at       time.Time
		complete bool
		want     Freshness
	}{
		{"same day", base, true, FreshnessFresh},
		{"just before 14 days", base.Add(14*24*time.Hour - time.Nanosecond), true, FreshnessFresh},
		{"14 days", base.Add(14 * 24 * time.Hour), true, FreshnessStale},
		{"just before 30 days", base.Add(30*24*time.Hour - time.Nanosecond), true, FreshnessStale},
		{"30 days", base.Add(30 * 24 * time.Hour), true, FreshnessExpired},
		{"coverage incomplete", base, false, FreshnessUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateFreshness(FreshnessInput{LatestValidAt: &base, AsOf: tc.at, CoverageComplete: tc.complete})
			if got != tc.want {
				t.Fatalf("freshness = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFreshnessUnknownTimestampAndFutureExpectedDoNotRefresh(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	asOf := base.Add(40 * 24 * time.Hour)
	future := base.Add(100 * 24 * time.Hour)
	got := EvaluateFreshness(FreshnessInput{LatestValidAt: &base, CandidateAt: &future, CandidateExpected: true, AsOf: asOf, CoverageComplete: true})
	if got != FreshnessExpired {
		t.Fatalf("future expected refreshed freshness to %q", got)
	}
	unknown := EvaluateFreshness(FreshnessInput{AsOf: asOf, CoverageComplete: true})
	if unknown != FreshnessUnknown {
		t.Fatalf("missing timestamp freshness = %q, want unknown", unknown)
	}
}

func TestFreshnessLateOldMaterialDoesNotRewindLatest(t *testing.T) {
	latest := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	old := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	got := EvaluateFreshness(FreshnessInput{LatestValidAt: &latest, CandidateAt: &old, AsOf: latest.Add(24 * time.Hour), CoverageComplete: true})
	if got != FreshnessFresh {
		t.Fatalf("late old material changed freshness to %q", got)
	}
}
