package futures

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestScaffoldCannotAdvertiseLive(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		got := NewService(enabled).Capabilities()
		if got.Ready || got.Mode != "off" || got.Features.Research || len(got.Products) != 0 {
			t.Fatalf("unfinished module advertised live capability: %+v", got)
		}
		if got.Enabled != enabled || got.Reason != "FUTURES_NOT_IMPLEMENTED" {
			t.Fatalf("configuration/status lost: %+v", got)
		}
	}
}

func TestCapabilityPayloadMatchesSharedContractFixture(t *testing.T) {
	fixture, err := os.ReadFile("../../../../contracts/futures/v1/fixtures/capabilities-off.json")
	if err != nil {
		t.Fatal(err)
	}
	actual, err := json.Marshal(NewService(false).Capabilities())
	if err != nil {
		t.Fatal(err)
	}
	var expectedJSON, actualJSON any
	if err := json.Unmarshal(fixture, &expectedJSON); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(actual, &actualJSON); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expectedJSON, actualJSON) {
		t.Fatalf("Go capability payload drifted from shared fixture: %s", actual)
	}
}
