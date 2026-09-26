package intel

import (
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

var (
	mediumThreshold = decimal.RequireFromString("0.4")
	highThreshold   = decimal.RequireFromString("0.8")
)

func effectiveWeight(e EvidenceRule) decimal.Decimal {
	if e.Status != EvidenceActive {
		return decimal.Zero
	}
	w := e.SourceWeight.Mul(e.Grade.Factor())
	if w.IsNegative() {
		return decimal.Zero
	}
	return w
}

func normalizedValue(v string) string {
	return strings.TrimSpace(v)
}

func maxClusterScore(evidence []EvidenceRule, stance Stance) (decimal.Decimal, []string) {
	clusters := make(map[string]decimal.Decimal)
	for _, e := range evidence {
		if e.Stance != stance || e.Status != EvidenceActive {
			continue
		}
		w := effectiveWeight(e)
		if cur, ok := clusters[e.SourceCluster]; !ok || w.GreaterThan(cur) {
			clusters[e.SourceCluster] = w
		}
	}
	max := decimal.Zero
	for _, w := range clusters {
		if w.GreaterThan(max) {
			max = w
		}
	}
	ids := make([]string, 0)
	if max.IsZero() {
		return max, ids
	}
	for _, e := range evidence {
		if e.Stance == stance && e.Status == EvidenceActive && effectiveWeight(e).Equal(max) {
			ids = append(ids, e.ID)
		}
	}
	sort.Strings(ids)
	return max, ids
}

func evidenceIDs(evidence []EvidenceRule, predicate func(EvidenceRule) bool) []string {
	ids := make([]string, 0)
	for _, e := range evidence {
		if predicate(e) {
			ids = append(ids, e.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func openConflictIDs(evidence []EvidenceRule) []string {
	values := make(map[string][]EvidenceRule)
	for _, e := range evidence {
		if e.Status != EvidenceActive || e.Stance == StanceContext {
			continue
		}
		v := normalizedValue(e.Value)
		if v == "" {
			continue
		}
		values[e.ClaimKey+"\x00"+v] = append(values[e.ClaimKey+"\x00"+v], e)
	}
	byClaim := make(map[string]map[string][]EvidenceRule)
	for key, group := range values {
		parts := strings.SplitN(key, "\x00", 2)
		claim := parts[0]
		value := parts[1]
		if byClaim[claim] == nil {
			byClaim[claim] = make(map[string][]EvidenceRule)
		}
		byClaim[claim][value] = group
	}
	ids := make([]string, 0)
	for _, valuesByClaim := range byClaim {
		if len(valuesByClaim) < 2 {
			continue
		}
		for _, group := range valuesByClaim {
			for _, e := range group {
				ids = append(ids, e.ID)
			}
		}
	}
	sort.Strings(ids)
	return ids
}

func level(score decimal.Decimal) SupportLevel {
	switch {
	case score.GreaterThanOrEqual(highThreshold):
		return SupportHigh
	case score.GreaterThanOrEqual(mediumThreshold):
		return SupportMedium
	default:
		return SupportLow
	}
}

func EvaluateClaim(evidence []EvidenceRule) ClaimEvaluation {
	support, supportIDs := maxClusterScore(evidence, StanceSupport)
	refute, refuteIDs := maxClusterScore(evidence, StanceRefute)
	out := ClaimEvaluation{
		SupportScore:       support,
		RefuteScore:        refute,
		SupportEvidenceIDs: supportIDs,
		RefuteEvidenceIDs:  refuteIDs,
	}
	out.ConflictingEvidenceIDs = openConflictIDs(evidence)
	out.ConflictOpen = len(out.ConflictingEvidenceIDs) > 0
	out.SupportLevel = level(support)
	if out.ConflictOpen {
		out.SupportLevel = SupportMedium
		out.CapReason = "open_claim_conflict"
	}
	out.Uncertain = support.IsZero() && refute.IsZero()
	if out.Uncertain {
		out.SupportLevel = SupportUnknown
	}
	return out
}

func EvaluateVerification(evidence []EvidenceRule) Verification {
	core := evidenceIDs(evidence, func(e EvidenceRule) bool {
		return e.ClaimKey == CoreClaimKey &&
			e.Status == EvidenceActive &&
			e.Grade == GradeFact &&
			e.Authoritative && e.Direct && e.SubjectUnique && e.ModalityExplicit
	})
	filtered := make([]EvidenceRule, 0, len(core))
	for _, e := range evidence {
		for _, id := range core {
			if e.ID == id {
				filtered = append(filtered, e)
				break
			}
		}
	}
	support, _ := maxClusterScore(filtered, StanceSupport)
	refute, _ := maxClusterScore(filtered, StanceRefute)
	switch {
	case support.GreaterThan(refute):
		return VerificationConfirmed
	case refute.GreaterThan(support):
		return VerificationDenied
	case support.GreaterThan(decimal.Zero):
		return VerificationDisputed
	default:
		return VerificationUnverified
	}
}
