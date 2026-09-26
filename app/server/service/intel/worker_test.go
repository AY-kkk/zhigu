package intel

import (
	"context"
	"encoding/json"
	"testing"

	"zhigu/server/service/finance"

	"zhigu/server/testdb"
)

func TestJobLeaseRenewAndEpochFencing(t *testing.T) {
	db := testdb.Start(t)
	svc := NewService(db, []byte("intel-test-cookie-secret"), testFixtureDir(t))
	ctx := context.Background()
	scope, _, err := svc.CreateDemoSession(ctx, "boot-worker", "idem-worker", "body-worker")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateIngestionJob(ctx, scope, IngestionJobRequest{Provider: "fixture", Codes: []string{"DEMO.A"}, From: "2026-09-01", To: "2026-09-02"}); err != nil {
		t.Fatal(err)
	}
	lease, err := svc.ClaimJob(ctx, "worker-a")
	if err != nil {
		t.Fatal(err)
	}
	if lease.LeaseEpoch != 1 || lease.Generation != scope.Generation {
		t.Fatalf("lease = %#v", lease)
	}
	if _, err := svc.ClaimJob(ctx, "worker-b"); err == nil {
		t.Fatal("leased job must not be claimable")
	}
	if err := svc.RenewJob(ctx, lease); err != nil {
		t.Fatal(err)
	}
	stale := lease
	stale.LeaseEpoch = 0
	if err := svc.CompleteJob(ctx, stale, "succeeded", 1, 1, 0, ""); err == nil || ErrorCode(err) != "VERSION_CONFLICT" {
		t.Fatalf("stale complete err=%v", err)
	}
	if err := svc.CompleteJob(ctx, lease, "partial", 1, 0, 1, "fixture job requires explicit replay"); err != nil {
		t.Fatal(err)
	}
}

func TestOutboxLeaseAndCompletionAreFenced(t *testing.T) {
	db := testdb.Start(t)
	svc := NewService(db, []byte("intel-test-cookie-secret"), testFixtureDir(t))
	ctx := context.Background()
	scope, _, err := svc.CreateDemoSession(ctx, "boot-outbox", "idem-outbox", "body-outbox")
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"DEMO.A", "DEMO.B"} {
		if _, err := svc.AddWatchlist(ctx, scope, code); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.ReplayAction(ctx, scope, ReplayRequest{Action: "step", ExpectedVersion: 0}); err != nil {
		t.Fatal(err)
	}
	lease, err := svc.ClaimOutbox(ctx, "outbox-a")
	if err != nil {
		t.Fatal(err)
	}
	stale := lease
	stale.LeaseEpoch = 0
	if err := svc.CompleteOutbox(ctx, stale, "sent"); err == nil || ErrorCode(err) != "VERSION_CONFLICT" {
		t.Fatalf("stale outbox err=%v", err)
	}
	if err := svc.CompleteOutbox(ctx, lease, "sent"); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := db.Raw(`SELECT status FROM finance_intel_outbox WHERE id=?`, lease.ID).Scan(&status).Error; err != nil {
		t.Fatal(err)
	}
	if status != "sent" {
		t.Fatalf("outbox status=%q", status)
	}
}

func TestWorkerExecutesFrozenModelExtraction(t *testing.T) {
	db := testdb.Start(t)
	configs := finance.NewConfigService(db)
	svc := NewService(db, []byte("intel-test-cookie-secret"), testFixtureDir(t)).WithConfigService(configs)
	ctx := context.Background()
	saved, err := configs.SaveModel(ctx, map[string]any{
		"base_url": "https://example.com/v1", "protocol": "openai_chat_completions", "model": "model-x",
	}, "secret-key")
	if err != nil {
		t.Fatal(err)
	}
	if err := configs.MarkTested(ctx, saved.ID, saved.ConfigDigest); err != nil {
		t.Fatal(err)
	}
	if err := configs.Activate(ctx, saved.ID); err != nil {
		t.Fatal(err)
	}
	scope, err := svc.LiveScope(ctx, 99, "admin")
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.ImportSourceRevision(ctx, scope, ImportSourceRequest{
		Publisher: "授权公告", DocumentID: "LIVE-1", URL: "https://example.com/live-1",
		Title: "真实材料占位", Text: "公司披露一项交易进展。", Rights: "summary", ImportReason: "授权样本",
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload extractionJobPayload
	var payloadRaw string
	if err := db.Raw(`SELECT payload::text FROM finance_intel_jobs WHERE namespace_id=? AND id=(SELECT id FROM finance_intel_jobs WHERE namespace_id=? ORDER BY created_at DESC LIMIT 1)`, scope.NamespaceID, scope.NamespaceID).Scan(&payloadRaw).Error; err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ModelConfig.ConfigID != saved.ID || payload.ModelConfig.ConfigDigest != saved.ConfigDigest {
		t.Fatalf("payload=%#v", payload)
	}
	svc.ModelCall = func(_ context.Context, cfg FrozenModelConfig, _ []byte) ([]byte, error) {
		output := extractionFixtureOutput(payload.SourceRevisionID, "公司披露一项交易进展。")
		raw, _ := json.Marshal(output)
		response, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(raw)}}}, "usage": map[string]any{"total_tokens": 12}})
		return response, nil
	}
	lease, err := svc.ClaimJob(ctx, "worker-model")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ProcessJob(ctx, lease); err != nil {
		t.Fatal(err)
	}
	var runs int
	if err := db.Raw(`SELECT count(*) FROM finance_intel_extraction_runs WHERE namespace_id=?`, scope.NamespaceID).Scan(&runs).Error; err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("extraction runs=%d", runs)
	}
	_ = created
}
