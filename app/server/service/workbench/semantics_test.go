package workbench_test

// 正式回归集（reviews/stage-b-2026-09-24 反例的等价用例，R7-H4）：
// 局部修改保真、无模型解释不伪成功、已有策略强制 base_version。

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"gorm.io/datatypes"

	model "zhigu/server/model/strategy"
	"zhigu/server/service/workbench"
)

const localityState = `{"name":"保留原策略","instrument_id":"600519.SH","signal_period":"1d","price_basis":"raw","indicators":[{"id":"kdj","type":"KDJ","params":{"n":9,"m1":3,"m2":3}}],"entry":{"op":"lt","left":"kdj.j","right":{"constant":"17"}},"exit":{"op":"gt","left":"kdj.j","right":{"constant":"83"}},"position":{"type":"equity_fraction","value":"0.8"},"risk":{"check":"close"},"execution":{"timing":"next_session_open","priority":"exit_first"}}`

func localitySetup(t *testing.T) (*workbench.Hub, context.Context, string) {
	t.Helper()
	hub, ctx, draftID := setupHub(t)
	if _, err := hub.PatchDraftV2(ctx, draftID, workbench.DraftPatch{
		Revision: 1, HasEditor: true, EditorSchemaVersion: "strategy.editor.v1", EditorState: []byte(localityState),
	}); err != nil {
		t.Fatal(err)
	}
	return hub, ctx, draftID
}

// canonDropNull normalizes JSON semantics: absent fields and explicit nulls are
// both "明确未启用/未设置" per contract, so they must compare equal.
func canonDropNull(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	b, _ := json.Marshal(dropNulls(v))
	return string(b)
}

func dropNulls(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, val := range t {
			if val == nil {
				continue
			}
			out[k] = dropNulls(val)
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, val := range t {
			out = append(out, dropNulls(val))
		}
		return out
	default:
		return v
	}
}

func waitTerminal(t *testing.T, hub *workbench.Hub, ctx context.Context, id string) map[string]any {
	t.Helper()
	for i := 0; i < 500; i++ {
		g, err := hub.GetGeneration(ctx, id)
		if err != nil {
			t.Fatalf("get generation: %v", err)
		}
		switch g["status"] {
		case "ready", "failed", "unsupported", "needs_clarification", "canceled":
			return g
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("generation timeout")
	return nil
}

// TestModifyPreservesUnrelatedRule：只改仓位时，其余交易语义逐字段不变，
// dsl 与 editor_state 同步更新（R2）。
func TestModifyPreservesUnrelatedRule(t *testing.T) {
	hub, ctx, draftID := localitySetup(t)
	rev := 2
	started, err := hub.StartGenerate(ctx, "modify-locality", workbench.GenerateReq{
		DraftID: draftID, BaseRevision: &rev, InstrumentID: "600519.SH",
		Mode: "modify", Text: "只把仓位改为半仓，其他规则不变",
	})
	if err != nil {
		t.Fatal(err)
	}
	g := waitTerminal(t, hub, ctx, started["generation_id"].(string))
	if g["status"] != "ready" {
		t.Fatalf("status %v", g)
	}
	var d model.Draft
	if err := hub.DB.Where("id = ?", draftID).Take(&d).Error; err != nil {
		t.Fatal(err)
	}
	var got, want map[string]json.RawMessage
	_ = json.Unmarshal(d.DSL, &got)
	_ = json.Unmarshal([]byte(localityState), &want)
	for _, key := range []string{"entry", "exit", "risk", "execution", "indicators", "signal_period", "price_basis"} {
		if canonDropNull(got[key]) != canonDropNull(want[key]) {
			t.Errorf("unrelated %s changed: before=%s after=%s", key, canonDropNull(want[key]), canonDropNull(got[key]))
		}
	}
	var pos struct {
		Value string `json:"value"`
	}
	_ = json.Unmarshal(got["position"], &pos)
	if pos.Value != "0.5" {
		t.Fatalf("dsl position = %q, want 0.5", pos.Value)
	}
	// editor_state 与 dsl 同一套表示，不再分裂（R2-E2）。
	var editor map[string]json.RawMessage
	_ = json.Unmarshal(d.EditorState, &editor)
	_ = json.Unmarshal(editor["position"], &pos)
	if pos.Value != "0.5" {
		t.Fatalf("editor position = %q, want 0.5", pos.Value)
	}
}

// TestExistingStrategyRequiresBaseVersion：已有策略追加版本必须携带
// base_version_id，缺省拒绝、过期 409（R4）。
func TestExistingStrategyRequiresBaseVersion(t *testing.T) {
	hub, ctx, draftID := localitySetup(t)
	first, err := hub.SaveStrategy(ctx, "base-first", draftID, 2, "", "")
	if err != nil {
		t.Fatal(err)
	}
	strategyID, _ := first["strategy_id"].(string)
	if _, err := hub.SaveStrategy(ctx, "base-missing", draftID, 2, strategyID, ""); err == nil {
		t.Fatal("missing base_version accepted on existing strategy")
	}
	if _, err := hub.SaveStrategy(ctx, "base-stale", draftID, 2, strategyID, "stv_stale"); codeOf(err) != "REVISION_CONFLICT" {
		t.Fatalf("want REVISION_CONFLICT, got %v", err)
	}
	if _, err := hub.SaveStrategy(ctx, "base-ok", draftID, 2, strategyID, first["version_id"].(string)); err != nil {
		t.Fatalf("valid base rejected: %v", err)
	}
	// 失败请求不留下版本／指针／幂等副作用（R4-H2）。
	var count int64
	hub.DB.Model(&model.Version{}).Where("strategy_id = ?", strategyID).Count(&count)
	if count != 2 {
		t.Fatalf("version rows = %d, want 2", count)
	}
	var rec model.Idempotency
	if err := hub.DB.Where("operation = ? AND idempotency_key = ?", "save_strategy", "base-missing").Take(&rec).Error; err == nil {
		t.Fatal("failed save left idempotency side effect")
	}
}

// TestLateModifyDoesNotClobberWindowEdit：迟到修改按 base_revision CAS，
// 不覆盖窗口编辑（R2/R4 同源守护）。
func TestLateModifyDoesNotClobberWindowEdit(t *testing.T) {
	hub, ctx, draftID := localitySetup(t)
	base := 2
	started, err := hub.StartGenerate(ctx, "late-modify", workbench.GenerateReq{
		DraftID: draftID, BaseRevision: &base, Mode: "modify", Text: "只把仓位改为半仓，其他规则不变",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 在生成完成前由窗口抢先编辑（revision 2 → 3）。
	if _, err := hub.PatchDraftV2(ctx, draftID, workbench.DraftPatch{
		Revision: 2, HasEditor: true, EditorSchemaVersion: "strategy.editor.v1", EditorState: []byte(localityState),
	}); err != nil {
		t.Fatal(err)
	}
	g := waitTerminal(t, hub, ctx, started["generation_id"].(string))
	if g["status"] != "failed" || g["error_code"] != "REVISION_CONFLICT" {
		t.Fatalf("late modify must be rejected, got %v", g)
	}
	var d model.Draft
	if err := hub.DB.Where("id = ?", draftID).Take(&d).Error; err != nil {
		t.Fatal(err)
	}
	var editor map[string]json.RawMessage
	_ = json.Unmarshal(d.EditorState, &editor)
	var pos struct {
		Value string `json:"value"`
	}
	_ = json.Unmarshal(editor["position"], &pos)
	if pos.Value != "0.8" {
		t.Fatalf("window edit clobbered by late modify: position %q", pos.Value)
	}
	_ = datatypes.JSON{}
}
