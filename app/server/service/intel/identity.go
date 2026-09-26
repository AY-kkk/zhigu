package intel

type IdentityAction string

const (
	IdentityMerge  IdentityAction = "merge"
	IdentityReview IdentityAction = "review"
	IdentityNew    IdentityAction = "new"
)

type CandidateIdentity struct {
	SubjectCode           string
	EventType             EventType
	MatterKey             string
	ObjectKey             string
	Period                string
	ExplicitRefs          []string
	CandidateRefs         []string
	SameMatterDescription bool
	NameSimilar           bool
	VariableAmount        string
	VariableDate          string
}

type IdentityDecision struct {
	Action       IdentityAction
	MatchedIndex int
	Reason       string
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v != "" && v == want {
			return true
		}
	}
	return false
}

func identityBaseEqual(a CandidateIdentity, b CandidateIdentity) bool {
	return a.SubjectCode == b.SubjectCode && a.EventType == b.EventType &&
		a.ObjectKey == b.ObjectKey && a.Period == b.Period
}

// ClassifyIdentity implements PRD R1. recallWindowSeconds affects candidate
// retrieval only and never turns elapsed time into an identity condition.
func ClassifyIdentity(existing []CandidateIdentity, next CandidateIdentity, _ int64) IdentityDecision {
	matches := make([]int, 0)
	for i, candidate := range existing {
		switch {
		case next.MatterKey != "" && candidate.MatterKey == next.MatterKey:
			matches = append(matches, i)
		case candidate.MatterKey != "" && (contains(next.ExplicitRefs, candidate.MatterKey) || contains(candidate.ExplicitRefs, next.MatterKey)):
			matches = append(matches, i)
		case identityBaseEqual(candidate, next) && next.SameMatterDescription && candidate.SameMatterDescription:
			matches = append(matches, i)
		}
	}
	if len(matches) == 1 {
		return IdentityDecision{Action: IdentityMerge, MatchedIndex: matches[0], Reason: "exact_or_verified_same_matter"}
	}
	if len(matches) > 1 {
		return IdentityDecision{Action: IdentityReview, MatchedIndex: -1, Reason: "multiple_candidates"}
	}
	if len(next.CandidateRefs) > 0 {
		return IdentityDecision{Action: IdentityReview, MatchedIndex: -1, Reason: "candidate_reference_unresolved"}
	}
	if next.NameSimilar {
		return IdentityDecision{Action: IdentityReview, MatchedIndex: -1, Reason: "name_similarity_only"}
	}
	if next.MatterKey != "" {
		return IdentityDecision{Action: IdentityNew, MatchedIndex: -1, Reason: "distinct_matter"}
	}
	if len(existing) == 0 {
		return IdentityDecision{Action: IdentityNew, MatchedIndex: -1, Reason: "first_event"}
	}
	return IdentityDecision{Action: IdentityReview, MatchedIndex: -1, Reason: "insufficient_identity"}
}
