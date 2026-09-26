package intel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFixtureEventsV1ManualGold(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "contracts", "intel", "fixtures", "events-v1")
	for _, branch := range []string{"main", "denial"} {
		t.Run(branch, func(t *testing.T) {
			got, err := RunFixture(dir, branch)
			if err != nil {
				t.Fatalf("run fixture: %v", err)
			}
			want := fixtureExpected(t, dir, branch)
			if len(got) != len(want) {
				t.Fatalf("got %d steps, want %d", len(got), len(want))
			}
			for i, step := range got {
				w := want[i]
				if step.ID != w.ID {
					t.Fatalf("step %d id = %q, want %q", i, step.ID, w.ID)
				}
				if step.EventVersion != w.EventVersion || step.NotificationCount != w.NotificationCount ||
					step.Verification != w.Verification || step.Phase != w.Phase || step.Freshness != w.Freshness ||
					step.SupportLevel != w.SupportLevel || step.TransactionAmount != w.TransactionAmount ||
					step.OpenConflicts != w.OpenConflicts {
					t.Fatalf("%s = %#v, want %#v", step.ID, step, w)
				}
			}
		})
	}
}

func TestFixtureReplayIsIdempotentAndStable(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "contracts", "intel", "fixtures", "events-v1")
	first, err := RunFixture(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunFixture(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != len(second) {
		t.Fatalf("length changed: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("step %d changed: %#v vs %#v", i, first[i], second[i])
		}
	}
}

type fixtureExpectedResult struct {
	ID                string       `json:"-"`
	EventVersion      int          `json:"event_version"`
	NotificationCount int          `json:"notification_count"`
	Verification      Verification `json:"verification"`
	Phase             string       `json:"phase"`
	Freshness         Freshness    `json:"freshness"`
	SupportLevel      SupportLevel `json:"support_level"`
	TransactionAmount string       `json:"transaction_amount"`
	OpenConflicts     int          `json:"open_conflicts"`
}

func fixtureExpected(t *testing.T, dir, branch string) []fixtureExpectedResult {
	t.Helper()
	manifestRaw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Branches map[string]struct {
			Steps []string `json:"steps"`
		} `json:"branches"`
	}
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	expectedRaw, err := os.ReadFile(filepath.Join(dir, "expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	var expected struct {
		Steps map[string]fixtureExpectedResult `json:"steps"`
	}
	if err := json.Unmarshal(expectedRaw, &expected); err != nil {
		t.Fatal(err)
	}
	out := make([]fixtureExpectedResult, 0, len(manifest.Branches[branch].Steps))
	for _, id := range manifest.Branches[branch].Steps {
		item := expected.Steps[id]
		item.ID = id
		out = append(out, item)
	}
	return out
}
