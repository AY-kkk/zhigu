package futures

import (
	"strings"
	"testing"
	"time"
)

func TestCursorIsOpaqueVersionedOwnerAndFilterBound(t *testing.T) {
	identity := Identity{OwnerID: 42, Mode: "live"}
	token, err := NewPageCursor(identity, "notifications", "filter-a", time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC), "notification_b", 7)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(token, "notification_b") || strings.Contains(token, "filter-a") {
		t.Fatalf("cursor is not opaque: %q", token)
	}
	cursor, err := DecodePageCursor(token, identity, "notifications", "filter-a")
	if err != nil {
		t.Fatal(err)
	}
	if cursor.Primary != "2026-10-13T08:00:00Z" || cursor.Secondary != "notification_b" || cursor.Tertiary != "7" {
		t.Fatalf("unexpected cursor: %#v", cursor)
	}

	for name, mutate := range map[string]func(*Identity, *string, *string){
		"owner":  func(id *Identity, kind, filter *string) { id.OwnerID = 43 },
		"mode":   func(id *Identity, kind, filter *string) { id.Mode = "read_only" },
		"kind":   func(id *Identity, kind, filter *string) { *kind = "runs" },
		"filter": func(id *Identity, kind, filter *string) { *filter = "filter-b" },
	} {
		id, kind, filter := identity, "notifications", "filter-a"
		mutate(&id, &kind, &filter)
		if _, err := DecodePageCursor(token, id, kind, filter); err == nil {
			t.Fatalf("%s binding accepted", name)
		}
	}

	tampered := token[:len(token)-1] + "A"
	if _, err := DecodePageCursor(tampered, identity, "notifications", "filter-a"); err == nil {
		t.Fatal("tampered cursor accepted")
	}
}

func TestPageLimitRejectsInvalidAndBoundsValidValues(t *testing.T) {
	for _, raw := range []string{"0", "-1", "101", "x"} {
		if _, err := ParsePageLimit(raw); err == nil {
			t.Fatalf("accepted limit %q", raw)
		}
	}
	for raw, want := range map[string]int{"": 20, "1": 1, "100": 100, "20": 20} {
		got, err := ParsePageLimit(raw)
		if err != nil || got != want {
			t.Fatalf("limit %q got=%d err=%v", raw, got, err)
		}
	}
}

func TestProductionCapabilitiesNeverExposeFixtureProvenance(t *testing.T) {
	capabilities := NewService(true).WithRuntimeState(RuntimeState{
		Enabled: true, Ready: true, Mode: "live",
		DeploymentEnvironment: "production", OperationMode: "live",
		DataProvenance: "fixture", Health: "ready", Reason: "",
	}).Capabilities()
	if capabilities.DataProvenance != "real" {
		t.Fatalf("production leaked fixture provenance: %#v", capabilities)
	}
	if capabilities.Health != "unavailable" || capabilities.OperationMode != "read_only" || capabilities.Reason != "INVALID_DATA_PROVENANCE" {
		t.Fatalf("invalid production state not fail-closed: %#v", capabilities)
	}
}

func TestCapabilitiesExposeFourIndependentDimensions(t *testing.T) {
	capabilities := NewService(true).WithRuntimeState(RuntimeState{
		Enabled: true, Ready: false, Mode: "read_only",
		DeploymentEnvironment: "demo", OperationMode: "read_only",
		DataProvenance: "fixture", Health: "degraded", Reason: "MODEL_UNAVAILABLE",
	}).Capabilities()
	if capabilities.DeploymentEnvironment != "demo" || capabilities.OperationMode != "read_only" ||
		capabilities.DataProvenance != "fixture" || capabilities.Health != "degraded" {
		t.Fatalf("dimensions collapsed: %#v", capabilities)
	}
}
