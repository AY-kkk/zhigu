package intel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type fixtureStep struct {
	ID               string            `json:"id"`
	Branch           string            `json:"branch"`
	SimulatedAt      time.Time         `json:"simulated_at"`
	SourceRevisionID string            `json:"source_revision_id"`
	Repost           bool              `json:"repost"`
	SourceCluster    string            `json:"source_cluster"`
	Simulation       bool              `json:"simulation"`
	CoverageComplete *bool             `json:"coverage_complete"`
	Text             string            `json:"text"`
	Identity         fixtureIdentity   `json:"identity"`
	Evidence         []fixtureEvidence `json:"evidence"`
	Phase            *fixturePhase     `json:"phase"`
}

type fixtureIdentity struct {
	EventType             EventType `json:"event_type"`
	MatterKey             string    `json:"matter_key"`
	SubjectCode           string    `json:"subject_code"`
	ObjectKey             string    `json:"object_key"`
	Period                string    `json:"period"`
	SameMatterDescription bool      `json:"same_matter_description"`
	ExplicitRefs          []string  `json:"explicit_refs"`
}

type fixtureEvidence struct {
	ID               string         `json:"id"`
	ClaimKey         string         `json:"claim_key"`
	Value            string         `json:"value"`
	SourceCluster    string         `json:"source_cluster"`
	SourceWeight     string         `json:"source_weight"`
	Grade            Grade          `json:"grade"`
	Stance           Stance         `json:"stance"`
	Status           EvidenceStatus `json:"status"`
	Authoritative    bool           `json:"authoritative"`
	Direct           bool           `json:"direct"`
	SubjectUnique    bool           `json:"subject_unique"`
	ModalityExplicit bool           `json:"modality_explicit"`
	Supersedes       []string       `json:"supersedes"`
}

type fixturePhase struct {
	Value         string    `json:"value"`
	Authoritative bool      `json:"authoritative"`
	At            time.Time `json:"at"`
}

type FixtureResult struct {
	ID                string       `json:"id"`
	EventVersion      int          `json:"event_version"`
	NotificationCount int          `json:"notification_count"`
	Verification      Verification `json:"verification"`
	Phase             string       `json:"phase"`
	Freshness         Freshness    `json:"freshness"`
	SupportLevel      SupportLevel `json:"support_level"`
	TransactionAmount string       `json:"transaction_amount"`
	OpenConflicts     int          `json:"open_conflicts"`
}

type fixtureSourceDoc struct {
	Items []struct {
		SourceRevisionID string    `json:"source_revision_id"`
		DisclosedAt      time.Time `json:"disclosed_at"`
		IsRepost         bool      `json:"is_repost"`
	} `json:"items"`
}

func loadFixtureSteps(dir, branch string) ([]fixtureStep, map[string]time.Time, error) {
	manifestRaw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, nil, err
	}
	var manifest struct {
		Branches map[string]struct {
			Steps []string `json:"steps"`
		} `json:"branches"`
	}
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return nil, nil, fmt.Errorf("decode manifest: %w", err)
	}
	branchSteps, ok := manifest.Branches[branch]
	if !ok || len(branchSteps.Steps) == 0 {
		return nil, nil, fmt.Errorf("fixture branch %q has no steps", branch)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "extractions.json"))
	if err != nil {
		return nil, nil, err
	}
	var doc struct {
		SchemaVersion string        `json:"schema_version"`
		Steps         []fixtureStep `json:"steps"`
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return nil, nil, fmt.Errorf("decode extractions: %w", err)
	}
	if doc.SchemaVersion != "intel.fixture-steps.v1" {
		return nil, nil, fmt.Errorf("unsupported fixture schema %q", doc.SchemaVersion)
	}
	sourceRaw, err := os.ReadFile(filepath.Join(dir, "sources.json"))
	if err != nil {
		return nil, nil, err
	}
	var sourceDoc fixtureSourceDoc
	if err := json.Unmarshal(sourceRaw, &sourceDoc); err != nil {
		return nil, nil, fmt.Errorf("decode sources: %w", err)
	}
	disclosed := make(map[string]time.Time, len(sourceDoc.Items))
	for _, item := range sourceDoc.Items {
		disclosed[item.SourceRevisionID] = item.DisclosedAt
	}
	byID := make(map[string]fixtureStep, len(doc.Steps))
	for _, step := range doc.Steps {
		byID[step.ID] = step
	}
	out := make([]fixtureStep, 0, len(branchSteps.Steps))
	for _, id := range branchSteps.Steps {
		step, ok := byID[id]
		if !ok {
			return nil, nil, fmt.Errorf("fixture step %q not found", id)
		}
		out = append(out, step)
	}
	return out, disclosed, nil
}

func toEvidenceRule(in fixtureEvidence) EvidenceRule {
	return EvidenceRule{
		ID: in.ID, ClaimKey: in.ClaimKey, Value: in.Value,
		SourceCluster: in.SourceCluster, SourceWeight: decimal.RequireFromString(in.SourceWeight),
		Grade: in.Grade, Stance: in.Stance, Status: in.Status,
		Authoritative: in.Authoritative, Direct: in.Direct,
		SubjectUnique: in.SubjectUnique, ModalityExplicit: in.ModalityExplicit,
		Supersedes: in.Supersedes,
	}
}

func activeEvidence(evidence []EvidenceRule) []EvidenceRule {
	out := make([]EvidenceRule, 0, len(evidence))
	for _, e := range evidence {
		if e.Status == EvidenceActive {
			out = append(out, e)
		}
	}
	return out
}

func amountValues(evidence []EvidenceRule) string {
	type candidate struct {
		value string
		score decimal.Decimal
	}
	byValue := make(map[string]decimal.Decimal)
	for _, e := range activeEvidence(evidence) {
		if e.ClaimKey != "transaction_amount" || e.Stance != StanceSupport || e.Value == "" {
			continue
		}
		w := effectiveWeight(e)
		if cur, ok := byValue[e.Value]; !ok || w.GreaterThan(cur) {
			byValue[e.Value] = w
		}
	}
	if len(byValue) == 0 {
		return ""
	}
	max := decimal.Zero
	for _, w := range byValue {
		if w.GreaterThan(max) {
			max = w
		}
	}
	candidates := make([]candidate, 0, len(byValue))
	for value, score := range byValue {
		if score.Equal(max) {
			candidates = append(candidates, candidate{value: value, score: score})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, aerr := decimal.NewFromString(candidates[i].value)
		b, berr := decimal.NewFromString(candidates[j].value)
		if aerr == nil && berr == nil {
			if a.Equal(b) {
				return candidates[i].value < candidates[j].value
			}
			return a.GreaterThan(b)
		}
		return candidates[i].value > candidates[j].value
	})
	values := make([]string, 0, len(candidates))
	for _, c := range candidates {
		values = append(values, c.value)
	}
	return strings.Join(values, "|")
}

func conflictClaimCount(evidence []EvidenceRule) int {
	claims := make(map[string]map[string]struct{})
	for _, e := range activeEvidence(evidence) {
		if e.Stance == StanceContext || e.Value == "" {
			continue
		}
		if claims[e.ClaimKey] == nil {
			claims[e.ClaimKey] = make(map[string]struct{})
		}
		claims[e.ClaimKey][e.Value] = struct{}{}
	}
	n := 0
	for _, values := range claims {
		if len(values) > 1 {
			n++
		}
	}
	return n
}

func phaseAfter(current string, p *fixturePhase) string {
	if p == nil {
		return current
	}
	if !p.Authoritative && current != "" {
		return current
	}
	return p.Value
}

func maxLatestValid(evidence []EvidenceRule, disclosed map[string]time.Time, sourceByEvidence map[string]string) *time.Time {
	var latest *time.Time
	for _, e := range evidence {
		if e.Status != EvidenceActive {
			continue
		}
		src := sourceByEvidence[e.ID]
		if src == "" {
			continue
		}
		ts, ok := disclosed[src]
		if !ok {
			continue
		}
		if latest == nil || ts.After(*latest) {
			t := ts
			latest = &t
		}
	}
	return latest
}

// RunFixture executes the manually-labelled events-v1 branch through the pure
// deterministic rules and returns one result after every injected step.
func RunFixture(dir, branch string) ([]FixtureResult, error) {
	steps, disclosed, err := loadFixtureSteps(dir, branch)
	if err != nil {
		return nil, err
	}
	var evidence []EvidenceRule
	sourceByEvidence := make(map[string]string)
	version, notifications := 0, 0
	phase := "unknown"
	var lastState string
	out := make([]FixtureResult, 0, len(steps))

	for _, step := range steps {
		if !step.Simulation {
			for _, raw := range step.Evidence {
				e := toEvidenceRule(raw)
				for _, oldID := range e.Supersedes {
					for i := range evidence {
						if evidence[i].ID == oldID {
							evidence[i].Status = EvidenceSuperseded
						}
					}
				}
				if e.Authoritative && e.Status == EvidenceActive {
					// A later authoritative original folds an earlier
					// unverified report into history for the same claim; it
					// must not keep manufacturing a live conflict.
					for i := range evidence {
						if evidence[i].ClaimKey == e.ClaimKey && !evidence[i].Authoritative {
							evidence[i].Status = EvidenceSuperseded
						}
					}
				}
				evidence = append(evidence, e)
				sourceByEvidence[e.ID] = step.SourceRevisionID
			}
			if !step.Repost {
				phase = phaseAfter(phase, step.Phase)
			}
		} else {
			phase = phaseAfter(phase, step.Phase)
		}

		active := activeEvidence(evidence)
		core := EvaluateClaim(filterClaim(active, CoreClaimKey))
		verification := EvaluateVerification(active)
		latest := maxLatestValid(evidence, disclosed, sourceByEvidence)
		freshness := EvaluateFreshness(FreshnessInput{
			LatestValidAt:    latest,
			AsOf:             step.SimulatedAt,
			CoverageComplete: step.CoverageComplete == nil || *step.CoverageComplete,
		})
		supportLevel := core.SupportLevel
		if verification == VerificationDenied {
			if len(core.RefuteEvidenceIDs) > 0 {
				supportLevel = level(core.RefuteScore)
			} else {
				supportLevel = SupportLow
			}
		}
		amount := amountValues(active)
		openConflicts := conflictClaimCount(active)
		if openConflicts > 0 && supportLevel == SupportHigh {
			supportLevel = SupportMedium
		}
		state := fmt.Sprintf("%s|%s|%s|%s|%s|%d", verification, phase, freshness, supportLevel, amount, openConflicts)
		if version == 0 || state != lastState {
			if !step.Repost || version == 0 {
				version++
				notifications++
			}
		}
		lastState = state
		out = append(out, FixtureResult{
			ID: step.ID, EventVersion: version, NotificationCount: notifications,
			Verification: verification, Phase: phase, Freshness: freshness,
			SupportLevel: supportLevel, TransactionAmount: amount, OpenConflicts: openConflicts,
		})
	}
	return out, nil
}

func filterClaim(evidence []EvidenceRule, claim string) []EvidenceRule {
	out := make([]EvidenceRule, 0, len(evidence))
	for _, e := range evidence {
		if e.ClaimKey == claim {
			out = append(out, e)
		}
	}
	return out
}
