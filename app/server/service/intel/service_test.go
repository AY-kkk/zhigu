package intel

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"zhigu/server/testdb"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testdb.StopIfStarted()
	os.Exit(code)
}

func testFixtureDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "..", "contracts", "intel", "fixtures", "events-v1")
}

func TestDemoSessionReplayAndResetAreNamespaceIsolated(t *testing.T) {
	db := testdb.Start(t)
	svc := NewService(db, []byte("intel-test-cookie-secret"), testFixtureDir(t))
	ctx := context.Background()

	first, _, err := svc.CreateDemoSession(ctx, "boot-1", "idem-create-1", "body-1")
	if err != nil {
		t.Fatalf("create first demo: %v", err)
	}
	second, _, err := svc.CreateDemoSession(ctx, "boot-2", "idem-create-2", "body-2")
	if err != nil {
		t.Fatalf("create second demo: %v", err)
	}
	if first.NamespaceID == second.NamespaceID || first.PrincipalKey == second.PrincipalKey {
		t.Fatal("demo sessions must be isolated")
	}

	for _, code := range []string{"DEMO.A", "DEMO.B"} {
		if _, err := svc.AddWatchlist(ctx, first, code); err != nil {
			t.Fatalf("watch %s: %v", code, err)
		}
	}
	if _, err := svc.AddWatchlist(ctx, second, "DEMO.A"); err != nil {
		t.Fatalf("second watch: %v", err)
	}

	step1, err := svc.ReplayAction(ctx, first, ReplayRequest{Action: "step", ExpectedVersion: 0})
	if err != nil {
		t.Fatalf("step S1: %v", err)
	}
	if step1.EventVersion != 1 || step1.NotificationCount != 1 {
		t.Fatalf("S1 = %#v, want version 1 notifications 1", step1)
	}
	step2, err := svc.ReplayAction(ctx, first, ReplayRequest{Action: "step", ExpectedVersion: step1.ReplayVersion})
	if err != nil {
		t.Fatalf("step S2: %v", err)
	}
	if step2.EventVersion != 1 || step2.NotificationCount != 1 {
		t.Fatalf("S2 = %#v, want no business change", step2)
	}

	events, err := svc.ListEvents(ctx, second, EventQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(events.Items) != 0 {
		t.Fatalf("second namespace leaked events: %#v", events.Items)
	}
	notifications, err := svc.ListNotifications(ctx, second, NotificationQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if notifications.UnreadCount != 0 {
		t.Fatalf("second namespace leaked notifications: %#v", notifications)
	}

	reset, err := svc.ReplayAction(ctx, first, ReplayRequest{Action: "reset", ExpectedVersion: step2.ReplayVersion, Branch: "main"})
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if reset.Generation <= first.Generation || reset.StepIndex != 0 || reset.EventVersion != 0 {
		t.Fatalf("reset = %#v, want fenced empty replay", reset)
	}
	events, err = svc.ListEvents(ctx, first, EventQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(events.Items) != 0 {
		t.Fatalf("reset retained events: %#v", events.Items)
	}
}

func TestReplayExpectedVersionAndNotificationStateConflict(t *testing.T) {
	db := testdb.Start(t)
	svc := NewService(db, []byte("intel-test-cookie-secret"), testFixtureDir(t))
	ctx := context.Background()
	scope, _, err := svc.CreateDemoSession(ctx, "boot-version", "idem-version", "body-version")
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
	if _, err := svc.ReplayAction(ctx, scope, ReplayRequest{Action: "step", ExpectedVersion: 0}); err == nil || ErrorCode(err) != "VERSION_CONFLICT" {
		t.Fatalf("stale replay err = %v, want VERSION_CONFLICT", err)
	}
	notifications, err := svc.ListNotifications(ctx, scope, NotificationQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(notifications.Items) != 1 {
		t.Fatalf("notifications = %#v, want one", notifications.Items)
	}
	id := notifications.Items[0].(map[string]any)["id"].(string)
	if _, err := svc.PatchNotification(ctx, scope, id, "ignored"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PatchNotification(ctx, scope, id, "read"); err == nil || ErrorCode(err) != "NOTIFICATION_STATE_CONFLICT" {
		t.Fatalf("ignored->read err = %v, want NOTIFICATION_STATE_CONFLICT", err)
	}
}

func TestMuteStopsSubsequentNotificationsButNotReplay(t *testing.T) {
	db := testdb.Start(t)
	svc := NewService(db, []byte("intel-test-cookie-secret"), testFixtureDir(t))
	ctx := context.Background()
	scope, _, err := svc.CreateDemoSession(ctx, "boot-mute", "idem-mute", "body-mute")
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"DEMO.A", "DEMO.B"} {
		if _, err := svc.AddWatchlist(ctx, scope, code); err != nil {
			t.Fatal(err)
		}
	}
	first, err := svc.ReplayAction(ctx, scope, ReplayRequest{Action: "step", ExpectedVersion: 0})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetMute(ctx, scope, "evt_demo_acq", true); err != nil {
		t.Fatal(err)
	}
	second, err := svc.ReplayAction(ctx, scope, ReplayRequest{Action: "step", ExpectedVersion: first.ReplayVersion})
	if err != nil {
		t.Fatal(err)
	}
	third, err := svc.ReplayAction(ctx, scope, ReplayRequest{Action: "step", ExpectedVersion: second.ReplayVersion})
	if err != nil {
		t.Fatal(err)
	}
	if third.EventVersion != 2 {
		t.Fatalf("mute stopped business replay: %#v", third)
	}
	notifications, err := svc.ListNotifications(ctx, scope, NotificationQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if notifications.UnreadCount != 1 {
		t.Fatalf("muted replay generated notifications: %#v", notifications)
	}
	_ = time.Second
}

func TestCreateDemoSessionIsIdempotentPerBootstrapAndBody(t *testing.T) {
	db := testdb.Start(t)
	svc := NewService(db, []byte("intel-test-cookie-secret"), testFixtureDir(t))
	ctx := context.Background()
	first, replayed, err := svc.CreateDemoSession(ctx, "boot-idem", "idem-key-0001", "body-a")
	if err != nil || replayed {
		t.Fatalf("first create err=%v replayed=%v", err, replayed)
	}
	second, replayed, err := svc.CreateDemoSession(ctx, "boot-idem", "idem-key-0001", "body-a")
	if err != nil || !replayed {
		t.Fatalf("second create err=%v replayed=%v", err, replayed)
	}
	if first.SessionID == "" || first.SessionID != second.SessionID || first.NamespaceID != second.NamespaceID {
		t.Fatalf("idempotent sessions differ: %#v vs %#v", first, second)
	}
	if _, _, err := svc.CreateDemoSession(ctx, "boot-idem", "idem-key-0001", "body-b"); err == nil || ErrorCode(err) != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("different body err=%v, want IDEMPOTENCY_CONFLICT", err)
	}
}

func TestAdminReviewAndReassignPreserveVersionChecks(t *testing.T) {
	db := testdb.Start(t)
	svc := NewService(db, []byte("intel-test-cookie-secret"), testFixtureDir(t))
	ctx := context.Background()
	scope, _, err := svc.CreateDemoSession(ctx, "boot-admin", "idem-admin", "body-admin")
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
	var event map[string]any
	if err := db.Raw(`SELECT id,current_version FROM finance_intel_events WHERE namespace_id=? AND logical_id='evt_demo_acq'`, scope.NamespaceID).Scan(&event).Error; err != nil {
		t.Fatal(err)
	}
	eventID := event["id"].(string)
	reviewID := "review_test"
	if err := db.Exec(`INSERT INTO finance_intel_review_items(id,namespace_id,revision,kind,status,payload,reason,created_at)
VALUES (?,?,1,'merge','pending','{}'::jsonb,'候选关联',now())`, reviewID, scope.NamespaceID).Error; err != nil {
		t.Fatal(err)
	}
	resolved, err := svc.ResolveReviewItem(ctx, scope, reviewID, ReviewResolveRequest{
		Action: "assign", EventID: "evt_demo_acq", ExpectedVersion: 1, Reason: "确认同一事项",
	})
	if err != nil || resolved["status"] != "resolved" {
		t.Fatalf("resolve=%#v err=%v", resolved, err)
	}
	reassigned, err := svc.ReassignEvidence(ctx, scope, "evt_demo_acq", ReassignRequest{
		EvidenceIDs: []string{"ev_s1_core"}, Reason: "纠正证据归属", ExpectedVersion: 1,
	})
	if err != nil || reassigned["source_event_id"] != "evt_demo_acq" {
		t.Fatalf("reassign=%#v err=%v", reassigned, err)
	}
	if _, err := svc.ReassignEvidence(ctx, scope, "evt_demo_acq", ReassignRequest{
		EvidenceIDs: []string{"ev_s1_core"}, Reason: "重复请求版本应冲突", ExpectedVersion: 0,
	}); err == nil || ErrorCode(err) != "VERSION_CONFLICT" {
		t.Fatalf("stale reassign err=%v", err)
	}
	_ = eventID
}

func TestResetGenerationFencesStalePrivateWrites(t *testing.T) {
	db := testdb.Start(t)
	svc := NewService(db, []byte("intel-test-cookie-secret"), testFixtureDir(t))
	ctx := context.Background()
	scope, _, err := svc.CreateDemoSession(ctx, "boot-fence", "idem-fence", "body-fence")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddWatchlist(ctx, scope, "DEMO.A"); err != nil {
		t.Fatal(err)
	}
	step, err := svc.ReplayAction(ctx, scope, ReplayRequest{Action: "step", ExpectedVersion: 0})
	if err != nil {
		t.Fatal(err)
	}
	stale := scope
	reset, err := svc.ReplayAction(ctx, scope, ReplayRequest{Action: "reset", ExpectedVersion: step.ReplayVersion, Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if reset.Generation <= stale.Generation {
		t.Fatalf("reset did not fence generation: %#v", reset)
	}
	if _, err := svc.AddWatchlist(ctx, stale, "DEMO.B"); err == nil || ErrorCode(err) != "VERSION_CONFLICT" {
		t.Fatalf("stale watchlist err=%v", err)
	}
	if _, err := svc.SetMute(ctx, stale, "evt_demo_acq", true); err == nil || ErrorCode(err) != "VERSION_CONFLICT" {
		t.Fatalf("stale mute err=%v", err)
	}
}

func TestImportSourceRevisionIsContentIdempotent(t *testing.T) {
	db := testdb.Start(t)
	svc := NewService(db, []byte("intel-test-cookie-secret"), testFixtureDir(t))
	ctx := context.Background()
	scope, _, err := svc.CreateDemoSession(ctx, "boot-import", "idem-import", "body-import")
	if err != nil {
		t.Fatal(err)
	}
	req := ImportSourceRequest{
		Publisher: "DEMO 公告", DocumentID: "IMPORT-1", URL: "fixture://import/1",
		Title: "导入材料", Text: "这是一条用于验证人工导入的材料。", Rights: "summary", ImportReason: "测试标准材料导入",
	}
	first, err := svc.ImportSourceRevision(ctx, scope, req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.ImportSourceRevision(ctx, scope, req)
	if err != nil {
		t.Fatal(err)
	}
	if first["revision_id"] != second["revision_id"] || second["existing"] != true {
		t.Fatalf("duplicate import first=%#v second=%#v", first, second)
	}
	var jobs int
	if err := db.Raw(`SELECT count(*) FROM finance_intel_jobs WHERE namespace_id=? AND kind='extract'`, scope.NamespaceID).Scan(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	if jobs != 1 {
		t.Fatalf("duplicate import created %d jobs", jobs)
	}
	changed := req
	changed.Text = "同一文档的第二条修订内容。"
	third, err := svc.ImportSourceRevision(ctx, scope, changed)
	if err != nil {
		t.Fatal(err)
	}
	var revisionNo int
	if err := db.Raw(`SELECT revision_no FROM finance_intel_source_revisions WHERE namespace_id=? AND logical_id=?`, scope.NamespaceID, third["revision_id"]).Scan(&revisionNo).Error; err != nil {
		t.Fatal(err)
	}
	if revisionNo != 2 {
		t.Fatalf("changed content revision_no=%d, want 2", revisionNo)
	}
}

func TestHistoricalEvidenceMembershipFollowsSelectedVersion(t *testing.T) {
	db := testdb.Start(t)
	svc := NewService(db, []byte("intel-test-cookie-secret"), testFixtureDir(t))
	ctx := context.Background()
	scope, _, err := svc.CreateDemoSession(ctx, "boot-history", "idem-history", "body-history")
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"DEMO.A", "DEMO.B"} {
		if _, err := svc.AddWatchlist(ctx, scope, code); err != nil {
			t.Fatal(err)
		}
	}
	version := int64(0)
	for i := 0; i < 4; i++ {
		result, err := svc.ReplayAction(ctx, scope, ReplayRequest{Action: "step", ExpectedVersion: version})
		if err != nil {
			t.Fatal(err)
		}
		version = result.ReplayVersion
	}
	v2, err := svc.Evidence(ctx, scope, "evt_demo_acq", 2, "", "", true, 100)
	if err != nil {
		t.Fatal(err)
	}
	v3, err := svc.Evidence(ctx, scope, "evt_demo_acq", 3, "", "", true, 100)
	if err != nil {
		t.Fatal(err)
	}
	v2Items, _ := v2["items"].([]any)
	v3Items, _ := v3["items"].([]any)
	if !evidenceHasStatus(v2Items, "ev_n1_amount", "active") {
		t.Fatalf("version 2 evidence = %#v", v2Items)
	}
	if !evidenceHasStatus(v3Items, "ev_n1_amount", "superseded") || !evidenceHasStatus(v3Items, "ev_n2_amount", "active") {
		t.Fatalf("version 3 evidence = %#v", v3Items)
	}
	for i := 0; i < 2; i++ {
		result, err := svc.ReplayAction(ctx, scope, ReplayRequest{Action: "step", ExpectedVersion: version})
		if err != nil {
			t.Fatal(err)
		}
		version = result.ReplayVersion
	}
	openAt4, err := svc.Conflicts(ctx, scope, "evt_demo_acq", 4, "open", 100)
	if err != nil {
		t.Fatal(err)
	}
	resolvedAt5, err := svc.Conflicts(ctx, scope, "evt_demo_acq", 5, "resolved", 100)
	if err != nil {
		t.Fatal(err)
	}
	openItems, _ := openAt4["items"].([]any)
	resolvedItems, _ := resolvedAt5["items"].([]any)
	if len(openItems) != 1 || len(resolvedItems) != 1 {
		t.Fatalf("history conflicts open=%#v resolved=%#v", openItems, resolvedItems)
	}
}

func evidenceHasStatus(items []any, logicalID, status string) bool {
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if ok && item["evidence_id"] == logicalID && item["status"] == status {
			return true
		}
	}
	return false
}
