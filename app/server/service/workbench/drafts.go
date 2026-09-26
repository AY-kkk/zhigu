package workbench

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	model "zhigu/server/model/strategy"
	"zhigu/server/service/finance"
	"zhigu/server/service/strategy"
)

// DraftPatch covers both §12.4.5 representations; they are mutually exclusive.
type DraftPatch struct {
	Revision            int
	HasEditor           bool
	EditorSchemaVersion string
	EditorState         json.RawMessage
	HasConfig           bool
	BacktestConfigDraft json.RawMessage
	HasDSL              bool
	DSL                 json.RawMessage
}

// PatchDraftV2 updates a draft with SQL-level CAS (id + owner + revision) —
// never read-then-write. Success bumps revision; a mismatch returns 409 and the
// client re-reads the latest draft through the owner-checked GET.
func (h *Hub) PatchDraftV2(ctx context.Context, id string, p DraftPatch) (map[string]any, error) {
	uid := finance.UserIDFrom(ctx)
	if p.HasEditor && p.HasDSL {
		return nil, finance.NewError(400, "validation", "INVALID_PARAM", "editor_state 与 dsl 不可同时提交")
	}
	if !p.HasEditor && !p.HasDSL {
		return nil, finance.NewError(400, "validation", "INVALID_PARAM", "缺少 editor_state 或 dsl")
	}
	var current model.Draft
	if err := h.DB.WithContext(ctx).Where("id = ? AND owner_id = ?", id, uid).Take(&current).Error; err != nil {
		return nil, finance.NewError(404, "not_found", "NOT_FOUND", "草稿不存在")
	}

	updates := map[string]any{"updated_at": time.Now().UTC()}
	if p.HasEditor {
		if strings.TrimSpace(p.EditorSchemaVersion) != strategy.EditorSchemaVersion {
			return nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "editor_schema_version 必须为 "+strategy.EditorSchemaVersion)
		}
		state, err := strategy.ParseEditorState(p.EditorState)
		if err != nil {
			return nil, err
		}
		res, err := strategy.CompileEditor(state, "")
		if err != nil {
			return nil, err
		}
		stateRaw, err := json.Marshal(state)
		if err != nil {
			return nil, err
		}
		updates["editor_schema_version"] = p.EditorSchemaVersion
		updates["editor_state"] = datatypes.JSON(stateRaw)
		updates["field_sources"] = datatypes.JSON(fieldSourcesAfterEdit(current, stateRaw))
		if state.InstrumentID != nil {
			updates["instrument_id"] = *state.InstrumentID
		} else {
			updates["instrument_id"] = nil
		}
		if res.Status != "ready" {
			// 缺字段草稿：DSL/compiled 不可执行（§12.4.4）。
			updates["status"] = "needs_clarification"
			clar, _ := json.Marshal(map[string]any{"missing_fields": res.Missing})
			updates["clarification"] = datatypes.JSON(clar)
			updates["dsl"] = gorm.Expr("NULL")
			updates["compiled"] = gorm.Expr("NULL")
		} else {
			docRaw, err := json.Marshal(res.Document)
			if err != nil {
				return nil, err
			}
			compRaw, err := json.Marshal(res.Compiled)
			if err != nil {
				return nil, err
			}
			updates["status"] = "ready"
			updates["clarification"] = datatypes.JSON([]byte("[]"))
			updates["dsl"] = datatypes.JSON(docRaw)
			updates["compiled"] = datatypes.JSON(compRaw)
		}
		if p.HasConfig {
			if !jsonIsObject(p.BacktestConfigDraft) {
				return nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "backtest_config_draft 须为对象")
			}
			updates["backtest_config_draft"] = datatypes.JSON(p.BacktestConfigDraft)
		}
	} else {
		doc, err := strategy.ParseDSL(p.DSL)
		if err != nil {
			return nil, err
		}
		c, err := strategy.Compile(doc)
		if err != nil {
			return nil, err
		}
		raw, _ := json.Marshal(doc)
		cr, _ := json.Marshal(c)
		updates["dsl"] = datatypes.JSON(raw)
		updates["compiled"] = datatypes.JSON(cr)
		updates["status"] = "ready"
	}

	res := h.DB.WithContext(ctx).Model(&model.Draft{}).
		Where("id = ? AND owner_id = ? AND revision = ?", id, uid, p.Revision).
		Updates(mergeUpdates(updates, map[string]any{"revision": gorm.Expr("revision + 1")}))
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, finance.NewError(409, "conflict", "REVISION_CONFLICT", "草稿版本冲突")
	}
	return h.GetDraft(ctx, id)
}

func mergeUpdates(base map[string]any, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func jsonIsObject(raw json.RawMessage) bool {
	var obj map[string]any
	return json.Unmarshal(raw, &obj) == nil && obj != nil
}

// fieldSourcesAfterEdit keeps per-section provenance: sections changed by this
// edit become "user"; untouched sections keep their previous source.
func fieldSourcesAfterEdit(current model.Draft, newState []byte) []byte {
	return fieldSourcesLabeled(current, newState, "user")
}

// fieldSourcesLabeled marks sections changed by this write with the given source
// label ("user" for window edits, "ai_suggestion" for generation output).
func fieldSourcesLabeled(current model.Draft, newState []byte, label string) []byte {
	prev := map[string]json.RawMessage{}
	_ = json.Unmarshal(current.EditorState, &prev)
	var next map[string]json.RawMessage
	_ = json.Unmarshal(newState, &next)
	sources := map[string]string{}
	_ = json.Unmarshal(current.FieldSources, &sources)
	sections := []string{"name", "signal_period", "price_basis", "indicators", "entry", "exit", "position", "risk", "execution"}
	for _, s := range sections {
		old, hadOld := prev[s]
		now, hasNew := next[s]
		if !hasNew {
			continue
		}
		if !hadOld || canonSection(old) != canonSection(now) {
			sources[s] = label
		} else if sources[s] == "" {
			sources[s] = "system_default"
		}
	}
	if v := sources["instrument_id"]; v == "" {
		sources["instrument_id"] = label
	}
	raw, _ := json.Marshal(sources)
	return raw
}

// canonSection normalizes a JSON section so key order never looks like a change.
func canonSection(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// draftView is the owner-scoped draft DTO (§12.4.6): editor state, field
// sources, backtest config draft, missing fields and market origin with the
// origin item's current listing status.
func draftView(d model.Draft) map[string]any {
	missing := []string{}
	var clar map[string]any
	if json.Unmarshal(d.Clarification, &clar) == nil {
		if list, ok := clar["missing_fields"].([]any); ok {
			for _, m := range list {
				if s, ok := m.(string); ok {
					missing = append(missing, s)
				}
			}
		}
	}
	return map[string]any{
		"draft_id": d.ID, "revision": d.Revision, "status": d.Status, "text": d.Text,
		"instrument_id": d.InstrumentID, "dsl": json.RawMessage(orEmptyJSON(d.DSL, "null")),
		"assumptions": json.RawMessage(orEmptyJSON(d.Assumptions, "[]")), "compiled": json.RawMessage(orEmptyJSON(d.Compiled, "null")),
		"clarification":         json.RawMessage(orEmptyJSON(d.Clarification, "[]")),
		"editor_schema_version": d.EditorSchemaVersion,
		"editor_state":          json.RawMessage(orEmptyJSON(d.EditorState, "null")),
		"field_sources":         json.RawMessage(orEmptyJSON(d.FieldSources, "{}")),
		"backtest_config_draft": json.RawMessage(orEmptyJSON(d.BacktestConfigDraft, "{}")),
		"missing_fields":        missing,
		"origin":                originView(d),
	}
}

func orEmptyJSON(raw datatypes.JSON, fallback string) []byte {
	if len(raw) == 0 {
		return []byte(fallback)
	}
	return raw
}

func originView(d model.Draft) map[string]any {
	if d.OriginMarketItemID == nil || d.OriginMarketVersionID == nil {
		return nil
	}
	out := map[string]any{
		"market_item_id":    *d.OriginMarketItemID,
		"market_version_id": *d.OriginMarketVersionID,
	}
	return out
}

// originWithStatus fills the origin item's current listing status.
func (h *Hub) originWithStatus(ctx context.Context, d model.Draft, out map[string]any) {
	origin, _ := out["origin"].(map[string]any)
	if origin == nil {
		return
	}
	itemID, _ := origin["market_item_id"].(string)
	var item model.MarketItem
	if err := h.DB.WithContext(ctx).Where("id = ?", itemID).Take(&item).Error; err == nil {
		origin["market_status"] = item.Status
	}
}

// GetDraft returns the owner-scoped draft view with origin status.
func (h *Hub) GetDraftView(ctx context.Context, id string) (map[string]any, error) {
	uid := finance.UserIDFrom(ctx)
	var d model.Draft
	if err := h.DB.WithContext(ctx).Where("id = ? AND owner_id = ?", id, uid).Take(&d).Error; err != nil {
		return nil, finance.NewError(404, "not_found", "NOT_FOUND", "草稿不存在")
	}
	view := draftView(d)
	h.originWithStatus(ctx, d, view)
	return view, nil
}
