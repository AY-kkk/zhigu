package futures

import (
	"context"

	"gorm.io/gorm"
	"strings"
	"testing"
	"time"
)

func TestSourceConfigDigestCoversConfigAndSecretRevision(t *testing.T) {
	config := map[string]any{"source_id": "source_a", "adapter_version": "adapter-v1"}
	base := ComputeSourceConfigDigest(config, 3, "fingerprint-a")
	if base != ComputeSourceConfigDigest(config, 3, "fingerprint-a") {
		t.Fatal("digest is not deterministic")
	}
	if base == ComputeSourceConfigDigest(config, 4, "fingerprint-a") {
		t.Fatal("secret revision is not covered")
	}
	if base == ComputeSourceConfigDigest(config, 3, "fingerprint-b") {
		t.Fatal("secret fingerprint is not covered")
	}
	if base == ComputeSourceConfigDigest(map[string]any{"source_id": "source_b", "adapter_version": "adapter-v1"}, 3, "fingerprint-a") {
		t.Fatal("non-secret config is not covered")
	}
	if len(base) != 64 || strings.ToLower(base) != base {
		t.Fatalf("invalid digest %q", base)
	}
}

func TestReleaseGateLifetimeIsAtMost24Hours(t *testing.T) {
	now := time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)
	gate := newSourceReleaseGate(Identity{OwnerID: 7, Mode: "live"}, "probe_1", "digest", 2, "fingerprint", now)
	if gate.ExpiresAt.After(now.Add(24*time.Hour)) || !gate.ExpiresAt.After(now) {
		t.Fatalf("invalid expiry %s", gate.ExpiresAt)
	}
}

func TestSourceProbeAndReleaseGateRequireMatchingOwnershipAndDigest(t *testing.T) {
	db := repoTestDB(t)
	domain := NewDomain(db, "").WithMode("live").WithNow(func() time.Time {
		return time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)
	})
	insertTestSource(t, db, "source_gate")
	owner := Identity{OwnerID: 11, Mode: "live"}
	probe, err := domain.CreateSourceProbe(context.Background(), owner, "probe-key", SourceProbeInput{
		SourceID: "source_gate", SourceVersion: 1, AdapterVersion: "adapter-v1", Result: "PASS", Reason: "fixture probe",
		SecretRevision: 2, SecretFingerprint: strings.Repeat("a", 64), EvidenceHash: strings.Repeat("b", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	if probe["release_gate_id"] == "" || probe["config_digest"] == "" {
		t.Fatalf("PASS probe did not create gate: %#v", probe)
	}
	page, err := domain.ListSourceProbesPage(context.Background(), owner, SourceProbeFilter{SourceID: "source_gate"}, PageOptions{Limit: 20})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("owner probe missing: page=%#v err=%v", page, err)
	}
	other := Identity{OwnerID: 12, Mode: "live"}
	page, err = domain.ListSourceProbesPage(context.Background(), other, SourceProbeFilter{SourceID: "source_gate"}, PageOptions{Limit: 20})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("cross-owner probe leaked: page=%#v err=%v", page, err)
	}

	digest := probe["config_digest"].(string)
	gateID := probe["release_gate_id"].(string)
	if _, err := domain.PatchOperationsWithGate(context.Background(), owner, 1, "live", "activate", []int64{11}, gateID, "wrong-digest"); err != ErrReleaseGate {
		t.Fatalf("wrong digest err=%v", err)
	}
	operations, err := domain.GetOperations(context.Background())
	if err != nil || operations["version"] != int64(1) || operations["mode"] != "off" {
		t.Fatalf("failed activation changed state: %#v err=%v", operations, err)
	}
	if _, err := domain.PatchOperationsWithGate(context.Background(), owner, 2, "live", "activate", []int64{11}, gateID, digest); err != ErrRevisionConflict {
		t.Fatalf("version mismatch err=%v", err)
	}
	activated, err := domain.PatchOperationsWithGate(context.Background(), owner, 1, "live", "activate", []int64{11}, gateID, digest)
	if err != nil {
		t.Fatal(err)
	}
	if activated["mode"] != "live" || activated["version"] != int64(2) {
		t.Fatalf("activation failed: %#v", activated)
	}
	if _, err := domain.PatchOperationsWithGate(context.Background(), owner, 2, "live", "reactivate", []int64{11}, gateID, digest); err != ErrReleaseGate {
		t.Fatalf("consumed gate reused: %v", err)
	}
}

func insertTestSource(t *testing.T, db *gorm.DB, sourceID string) {
	t.Helper()
	value := SourceValue{ID: sourceID, SourceID: sourceID, Version: 1, Publisher: "publisher", URL: "https://example.com",
		Status: "enabled", CredentialRef: &[]string{"secret-ref"}[0], Rights: []byte(`{"display":true,"model_use":true,"export":true}`), Metrics: []byte(`["price"]`),
		ScheduleVersion: "calendar-v1", AdapterVersion: "adapter-v1", Health: "healthy", Manifest: []byte(`{"source_id":"` + sourceID + `","adapter_version":"adapter-v1","credential_ref":"secret-ref"}`), CreatedAt: time.Date(2026, 10, 13, 7, 0, 0, 0, time.UTC)}
	if err := db.Create(&value).Error; err != nil {
		t.Fatal(err)
	}
}

func TestSourceConfigChangeInvalidatesActiveGate(t *testing.T) {
	db := repoTestDB(t)
	domain := NewDomain(db, "").WithMode("live").WithNow(func() time.Time {
		return time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)
	})
	insertTestSource(t, db, "source_invalidate")
	owner := Identity{OwnerID: 21, Mode: "live"}
	probe, err := domain.CreateSourceProbe(context.Background(), owner, "probe-invalidate", SourceProbeInput{
		SourceID: "source_invalidate", SourceVersion: 1, AdapterVersion: "adapter-v1", Result: "PASS",
		Reason: "before config change", SecretRevision: 1, SecretFingerprint: strings.Repeat("c", 64), EvidenceHash: strings.Repeat("d", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.PatchSource(context.Background(), "source_invalidate", 1, "paused", "change source"); err != nil {
		t.Fatal(err)
	}
	_, err = domain.PatchOperationsWithGate(context.Background(), owner, 1, "live", "activate", []int64{21},
		probe["release_gate_id"].(string), probe["config_digest"].(string))
	if err != ErrReleaseGate {
		t.Fatalf("invalidated gate accepted: %v", err)
	}
}

func TestConcurrentLiveActivationConsumesGateExactlyOnce(t *testing.T) {
	db := repoTestDB(t)
	domain := NewDomain(db, "").WithMode("live").WithNow(func() time.Time {
		return time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)
	})
	insertTestSource(t, db, "source_concurrent")
	owner := Identity{OwnerID: 31, Mode: "live"}
	probe, err := domain.CreateSourceProbe(context.Background(), owner, "probe-concurrent", SourceProbeInput{
		SourceID: "source_concurrent", SourceVersion: 1, AdapterVersion: "adapter-v1", Result: "PASS",
		Reason: "concurrent activation", SecretRevision: 1, SecretFingerprint: strings.Repeat("e", 64), EvidenceHash: strings.Repeat("f", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	gateID := probe["release_gate_id"].(string)
	digest := probe["config_digest"].(string)
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := domain.PatchOperationsWithGate(context.Background(), owner, 1, "live", "activate", []int64{31}, gateID, digest)
			results <- err
		}()
	}
	successes, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		switch err := <-results; err {
		case nil:
			successes++
		case ErrRevisionConflict, ErrReleaseGate:
			conflicts++
		default:
			t.Fatalf("unexpected concurrent result: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
}
