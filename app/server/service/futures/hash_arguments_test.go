package futures

import (
	"encoding/json"
	"os"
	"testing"
)

func TestHashArgumentsMatchesCrossLanguageCanonicalVector(t *testing.T) {
	raw, err := os.ReadFile("../../../../contracts/futures/v1/test-vectors/canonical-arguments.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Arguments map[string]any `json:"arguments"`
		SHA256    string         `json:"sha256"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if got := HashArguments(fixture.Arguments); got != fixture.SHA256 {
		t.Fatalf("hash=%s want=%s", got, fixture.SHA256)
	}
}
