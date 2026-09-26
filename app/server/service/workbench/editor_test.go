package workbench_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"gorm.io/datatypes"

	model "zhigu/server/model/strategy"
	f "zhigu/server/service/finance"
	"zhigu/server/service/market"
	"zhigu/server/service/workbench"
	"zhigu/server/testdb"
)

const editorStateReady = `{"name":"测试策略","instrument_id":"600519.SH","signal_period":"1d","price_basis":"raw","indicators":[{"id":"kdj","type":"KDJ","params":{"n":9,"m1":3,"m2":3}}],"entry":{"op":"lt","left":"kdj.j","right":{"constant":"20"}},"exit":{"op":"gt","left":"kdj.j","right":{"constant":"80"}},"position":{"type":"equity_fraction","value":"0.5"},"risk":{"check":"close"},"execution":{"timing":"next_session_open","priority":"exit_first"}}`

const editorStateMissing = `{"name":"测试策略","instrument_id":null,"signal_period":"1d","price_basis":"raw","indicators":[],"entry":{"op":"lt","left":"kdj.j","right":null},"exit":{"op":"gt","left":"kdj.j","right":{"constant":"80"}},"position":{"type":"equity_fraction","value":"0.5"},"risk":{"check":"close"},"execution":{"timing":"next_session_open","priority":"exit_first"}}`

func setupHub(t *testing.T) (*workbench.Hub, context.Context, string) {
	t.Helper()
	t.Setenv("ZHIGU_MARKET_MODE", "fixture")
	db := testdb.Start(t)
	db.Exec(`INSERT INTO finance_users (id, username, password_hash, role) VALUES (1001, 'u', 'x', 'user')`)
	hub := workbench.NewHub(db, market.NewService(db))
	ctx := f.WithUser(context.Background(), 1001, "user")
	draftID := "sdr_test1"
	now := time.Now().UTC()
	if err := db.Create(&model.Draft{
		ID: draftID, OwnerID: 1001, Text: "想法", Status: "queued", Revision: 1,
		Assumptions: datatypes.JSON([]byte("[]")), CapabilityErrors: datatypes.JSON([]byte("[]")),
		Clarification: datatypes.JSON([]byte("[]")), CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	return hub, ctx, draftID
}

func codeOf(err error) string {
	if err == nil {
		return ""
	}
	return f.ErrorCode(err)
}

func TestDraftEditorPatchCAS(t *testing.T) {
	hub, ctx, draftID := setupHub(t)

	// 两种表示不可同时提交。
	if _, err := hub.PatchDraftV2(ctx, draftID, workbench.DraftPatch{
		Revision: 1, HasEditor: true, EditorState: []byte(editorStateReady),
		HasDSL: true, DSL: []byte(`{"schema_version":"strategy.v1"}`),
	}); codeOf(err) != "INVALID_PARAM" {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
	// 缺少表示 → 400。
	if _, err := hub.PatchDraftV2(ctx, draftID, workbench.DraftPatch{Revision: 1}); codeOf(err) != "INVALID_PARAM" {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
	// 错误 revision → 409（SQL 级 CAS）。
	if _, err := hub.PatchDraftV2(ctx, draftID, workbench.DraftPatch{
		Revision: 99, HasEditor: true, EditorSchemaVersion: "strategy.editor.v1", EditorState: []byte(editorStateReady),
	}); codeOf(err) != "REVISION_CONFLICT" {
		t.Fatalf("want REVISION_CONFLICT, got %v", err)
	}
	// 正常编辑：revision+1、ready、field_sources 记录 user 来源。
	out, err := hub.PatchDraftV2(ctx, draftID, workbench.DraftPatch{
		Revision: 1, HasEditor: true, EditorSchemaVersion: "strategy.editor.v1", EditorState: []byte(editorStateReady),
	})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if out["revision"] != 2 || out["status"] != "ready" {
		t.Fatalf("out = %v", out)
	}
	if out["missing_fields"].([]string) == nil {
		t.Fatalf("missing_fields should be present: %v", out)
	}
	sources := out["field_sources"].(json.RawMessage)
	var fs map[string]string
	_ = json.Unmarshal(sources, &fs)
	if fs["entry"] != "user" || fs["instrument_id"] != "user" {
		t.Fatalf("field_sources = %v", fs)
	}
	if o, ok := out["origin"].(map[string]any); ok && len(o) > 0 {
		t.Fatalf("origin should be null for plain draft: %v", out["origin"])
	}

	// 缺字段编辑：needs_clarification、DSL 清空、字段级 missing_fields。
	out2, err := hub.PatchDraftV2(ctx, draftID, workbench.DraftPatch{
		Revision: 2, HasEditor: true, EditorSchemaVersion: "strategy.editor.v1", EditorState: []byte(editorStateMissing),
	})
	if err != nil {
		t.Fatalf("patch missing: %v", err)
	}
	if out2["status"] != "needs_clarification" {
		t.Fatalf("status = %v", out2["status"])
	}
	missing, _ := out2["missing_fields"].([]string)
	joined := ""
	for _, m := range missing {
		joined += m + ","
	}
	if !containsAll(joined, "instrument_id", "entry") {
		t.Fatalf("missing_fields = %v", missing)
	}
	if string(out2["dsl"].(json.RawMessage)) != "null" {
		t.Fatalf("dsl should be null when incomplete: %v", out2["dsl"])
	}
	// 陈旧 revision 再写 → 409，内容不被覆盖。
	if _, err := hub.PatchDraftV2(ctx, draftID, workbench.DraftPatch{
		Revision: 2, HasEditor: true, EditorSchemaVersion: "strategy.editor.v1", EditorState: []byte(editorStateReady),
	}); codeOf(err) != "REVISION_CONFLICT" {
		t.Fatalf("want REVISION_CONFLICT on stale write, got %v", err)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		found := false
		for _, seg := range splitComma(s) {
			if seg == p {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func splitComma(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func readyDraftRevision(t *testing.T, hub *workbench.Hub, ctx context.Context, draftID string, rev int) int {
	t.Helper()
	out, err := hub.PatchDraftV2(ctx, draftID, workbench.DraftPatch{
		Revision: rev, HasEditor: true, EditorSchemaVersion: "strategy.editor.v1", EditorState: []byte(editorStateReady),
	})
	if err != nil {
		t.Fatalf("ready patch: %v", err)
	}
	return out["revision"].(int)
}

func TestSaveStrategyTransaction(t *testing.T) {
	hub, ctx, draftID := setupHub(t)
	rev := readyDraftRevision(t, hub, ctx, draftID, 1)

	saved, err := hub.SaveStrategy(ctx, "save-k1", draftID, rev, "", "")
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	versionID, _ := saved["version_id"].(string)
	// 同 key 同体重放 → 同一版本。
	replay, err := hub.SaveStrategy(ctx, "save-k1", draftID, rev, "", "")
	if err != nil || replay["version_id"] != versionID {
		t.Fatalf("replay: %v %v", err, replay)
	}
	// 同 key 异体 → 409。
	if _, err := hub.SaveStrategy(ctx, "save-k1", draftID, rev, "st_other", ""); codeOf(err) != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("want IDEMPOTENCY_CONFLICT, got %v", err)
	}
	// base_version 冲突：以过期指针保存新版本 → 409，不留空版本。
	strategyID, _ := saved["strategy_id"].(string)
	rev2 := readyDraftRevision(t, hub, ctx, draftID, rev)
	if _, err := hub.SaveStrategy(ctx, "save-k2", draftID, rev2, strategyID, "stv_stale"); codeOf(err) != "REVISION_CONFLICT" {
		t.Fatalf("want REVISION_CONFLICT on base_version, got %v", err)
	}
	// 正确 base_version 可继续保存。
	saved2, err := hub.SaveStrategy(ctx, "save-k3", draftID, rev2, strategyID, versionID)
	if err != nil || saved2["revision"] != 2 {
		t.Fatalf("save2: %v %v", err, saved2)
	}
}

func TestSaveStrategyConcurrentSameKey(t *testing.T) {
	hub, ctx, draftID := setupHub(t)
	rev := readyDraftRevision(t, hub, ctx, draftID, 1)

	var wg sync.WaitGroup
	ids := make(chan string, 4)
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := hub.SaveStrategy(ctx, "save-race", draftID, rev, "", "")
			if err != nil {
				errs <- err
				return
			}
			ids <- out["version_id"].(string)
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent save: %v", err)
	}
	first := ""
	n := 0
	for id := range ids {
		n++
		if first == "" {
			first = id
		} else if id != first {
			t.Fatalf("concurrent saves created %s and %s", first, id)
		}
	}
	if n != 4 {
		t.Fatalf("got %d results", n)
	}
}

func TestExplainWithoutModelFailsAndKeepsDraft(t *testing.T) {
	hub, ctx, draftID := setupHub(t)
	rev := readyDraftRevision(t, hub, ctx, draftID, 1)
	base := rev

	started, err := hub.StartGenerate(ctx, "gen-explain", workbench.GenerateReq{
		Text: "解释一下", DraftID: draftID, Mode: "explain", BaseRevision: &base,
	})
	if err != nil {
		t.Fatalf("start explain: %v", err)
	}
	gid, _ := started["generation_id"].(string)
	var g map[string]any
	for i := 0; i < 40; i++ {
		time.Sleep(25 * time.Millisecond)
		out, err := hub.GetGeneration(ctx, gid)
		if err != nil {
			t.Fatalf("get generation: %v", err)
		}
		g = out
		status, _ := g["status"].(string)
		if status == "ready" || status == "failed" || status == "canceled" {
			break
		}
	}
	// R3：无模型配置时不得伪成功；AI 解释只认模型输出。
	if g["status"] != "failed" || g["mode"] != "explain" {
		t.Fatalf("generation = %v", g)
	}
	if g["error_code"] != "MODEL_UNAVAILABLE" {
		t.Fatalf("error_code = %v", g["error_code"])
	}
	if expl, _ := g["explanation"].(string); expl != "" {
		t.Fatalf("no model but explanation filled: %q", expl)
	}
	// 不改草稿规则与 revision。
	draft, err := hub.GetDraft(ctx, draftID)
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	if draft["revision"] != base {
		t.Fatalf("explain must not bump draft revision: %v vs %d", draft["revision"], base)
	}
	if string(draft["dsl"].(json.RawMessage)) == "null" {
		t.Fatal("explain must not clear dsl")
	}

	// 非法 mode → 400。
	if _, err := hub.StartGenerate(ctx, "gen-bad", workbench.GenerateReq{Text: "x", DraftID: draftID, Mode: "optimize"}); codeOf(err) != "INVALID_PARAM" {
		t.Fatalf("want INVALID_PARAM, got %v", err)
	}
}
