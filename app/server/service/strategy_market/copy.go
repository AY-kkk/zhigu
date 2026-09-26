package strategy_market

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"zhigu/server/httpx"
	model "zhigu/server/model/strategy"
	"zhigu/server/service/finance"
	"zhigu/server/service/strategy"
)

// Copy atomically creates an owner-bound private draft from the current
// published market version (§12.5 复制事务). Concurrent same-key requests create
// exactly one draft; replay verifies the draft is still visible or returns 410.
func (s *Service) Copy(ctx context.Context, uid uint, idemKey, itemID string, req CopyReq) (map[string]any, bool, error) {
	key, err := requireIdemKey(idemKey)
	if err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(req.MarketVersionID) == "" {
		return nil, false, finance.NewError(422, "validation", "STRATEGY_INVALID", "缺少 market_version_id")
	}
	ov := CopyOverrides{}
	if req.Overrides != nil {
		ov = *req.Overrides
	}
	if err := s.validateOverrides(ctx, ov); err != nil {
		return nil, false, err
	}
	hash, _ := canonicalHash(map[string]any{"op": OpCopy, "item_id": itemID, "market_version_id": req.MarketVersionID, "overrides": ov})
	var out map[string]any
	var replayed bool
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, resp, rep, ierr := s.idempotent(ctx, tx, uid, OpCopy, key, hash, func(tx *gorm.DB) (string, any, error) {
			return s.createCopy(ctx, tx, uid, itemID, req, ov)
		})
		if ierr != nil {
			return ierr
		}
		replayed = rep
		if b, ok := resp.(json.RawMessage); ok {
			return json.Unmarshal(b, &out)
		}
		out, _ = resp.(map[string]any)
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if replayed {
		// The stored response is only replayed while the copy is still visible.
		draftID, _ := out["draft_id"].(string)
		var d model.Draft
		if err := s.DB.WithContext(ctx).Where("id = ? AND owner_id = ?", draftID, uid).Take(&d).Error; err != nil {
			return nil, true, finance.NewError(410, "gone", "COPY_TARGET_GONE", "副本草稿已删除")
		}
	}
	return out, replayed, nil
}

func (s *Service) validateOverrides(ctx context.Context, ov CopyOverrides) error {
	if ov.InitialCash != "" {
		if !positiveDecimal(ov.InitialCash) {
			return finance.NewError(422, "validation", "STRATEGY_INVALID", "initial_cash 须为正十进制字符串")
		}
	}
	if ov.Start != "" && len(ov.Start) != 10 {
		return finance.NewError(422, "validation", "STRATEGY_INVALID", "start 须为 ISO 日期")
	}
	if ov.End != "" && len(ov.End) != 10 {
		return finance.NewError(422, "validation", "STRATEGY_INVALID", "end 须为 ISO 日期")
	}
	if ov.Start != "" && ov.End != "" && ov.Start > ov.End {
		return finance.NewError(422, "validation", "STRATEGY_INVALID", "start 不得晚于 end")
	}
	if ov.Currency != "" && ov.InstrumentID != "" {
		inst, err := s.Lookup.Instrument(ctx, ov.InstrumentID)
		if err != nil {
			return finance.NewError(422, "validation", "STRATEGY_INVALID", "标的不在受支持目录内")
		}
		// 单次运行只用标的柜台币种：显式 currency 不得与柜台币种冲突。
		if !strings.EqualFold(inst.Currency, ov.Currency) {
			return finance.NewError(422, "validation", "STRATEGY_INVALID", "currency 必须与标的柜台币种一致")
		}
	}
	return nil
}

func positiveDecimal(s string) bool {
	d, err := decimal.NewFromString(strings.TrimSpace(s))
	return err == nil && d.GreaterThan(decimal.Zero)
}

func (s *Service) createCopy(ctx context.Context, tx *gorm.DB, uid uint, itemID string, req CopyReq, ov CopyOverrides) (string, any, error) {
	item, err := itemLocked(tx, itemID)
	if err != nil {
		return "", nil, err
	}
	if item.Status != ItemPublished {
		return "", nil, errItemState(item)
	}
	if req.MarketVersionID != itemIDOr(item.CurrentVersionID) {
		return "", nil, finance.NewError(409, "conflict", "MARKET_VERSION_CHANGED", "请求版本不是当前版本")
	}
	ver, err := versionOf(tx, itemID, req.MarketVersionID)
	if err != nil {
		return "", nil, err
	}
	rep := parseValidationReport(ver.ValidationReport)
	if ver.ValidationStatus != ValidationPassed || !EngineSupported(rep.CompilerVersion) {
		return "", nil, finance.NewError(422, "validation", "MARKET_NOT_COPYABLE", "版本未通过规则校验或引擎不支持")
	}
	tpl, err := strategy.ParseMarketTemplate(ver.RuleTemplate)
	if err != nil {
		return "", nil, err
	}

	state := tpl.EditorState
	sources := map[string]string{
		"name": "market_default", "signal_period": "market_default", "price_basis": "market_default",
		"indicators": "market_default", "entry": "market_default", "exit": "market_default",
		"position": "market_default", "risk": "market_default", "execution": "market_default",
		"backtest_config": "market_default",
	}
	if ov.InstrumentID != "" {
		inst := ov.InstrumentID
		state.InstrumentID = &inst
		sources["instrument_id"] = "user"
	} else {
		sources["instrument_id"] = "market_default"
	}

	result, err := strategy.CompileEditor(state, ov.InstrumentID)
	if err != nil {
		return "", nil, err
	}

	now := nowUTC()
	draftID := httpx.NewID("sdr")
	draft := model.Draft{
		ID: draftID, OwnerID: uid, Text: "",
		Status: "ready", Revision: 1,
		Assumptions: datatypes.JSON([]byte("[]")), CapabilityErrors: datatypes.JSON([]byte("[]")),
		Clarification:         datatypes.JSON([]byte("[]")),
		EditorSchemaVersion:   strPtr(strategy.EditorSchemaVersion),
		OriginMarketItemID:    strPtr(itemID),
		OriginMarketVersionID: strPtr(ver.ID),
		CreatedAt:             now, UpdatedAt: now,
	}
	if state.InstrumentID != nil {
		draft.InstrumentID = state.InstrumentID
	}
	stateRaw, err := json.Marshal(state)
	if err != nil {
		return "", nil, err
	}
	draft.EditorState = datatypes.JSON(stateRaw)

	cfg := buildConfigDraft(ver, ov, sources)
	cfgRaw, _ := json.Marshal(cfg)
	draft.BacktestConfigDraft = datatypes.JSON(cfgRaw)
	srcRaw, _ := json.Marshal(sources)
	draft.FieldSources = datatypes.JSON(srcRaw)

	if result.Status != "ready" {
		draft.Status = "needs_clarification"
		clar, _ := json.Marshal(map[string]any{"missing_fields": result.Missing})
		draft.Clarification = datatypes.JSON(clar)
	} else {
		docRaw, err := json.Marshal(result.Document)
		if err != nil {
			return "", nil, err
		}
		compRaw, err := json.Marshal(result.Compiled)
		if err != nil {
			return "", nil, err
		}
		draft.DSL = datatypes.JSON(docRaw)
		draft.Compiled = datatypes.JSON(compRaw)
	}
	if err := tx.Create(&draft).Error; err != nil {
		return "", nil, err
	}
	resp := map[string]any{
		"draft_id": draftID, "revision": draft.Revision, "status": draft.Status,
		"origin": map[string]any{
			"market_item_id": itemID, "market_version_id": ver.ID,
			"version_no": ver.VersionNo, "market_status": item.Status,
		},
		"next_path": "/app/strategies?draft_id=" + draftID,
	}
	return draftID, resp, nil
}

func buildConfigDraft(ver model.MarketVersion, ov CopyOverrides, sources map[string]string) map[string]any {
	cfg := map[string]any{}
	var defs BacktestDefaults
	_ = json.Unmarshal(ver.BacktestDefaults, &defs)
	put := func(key, val string, src string) {
		if val == "" {
			return
		}
		cfg[key] = val
		sources["backtest_config."+key] = src
	}
	src := "market_default"
	putStr := func(key string, v *string) {
		if v != nil {
			put(key, *v, src)
		}
	}
	putStr("initial_cash", defs.InitialCash)
	putStr("currency", defs.Currency)
	putStr("start", defs.Start)
	putStr("end", defs.End)
	putStr("slippage_bps", defs.SlippageBps)
	putStr("participation_cap", defs.ParticipationCap)
	putStr("commission_config", defs.CommissionConfig)
	putStr("fee_schedule_id", defs.FeeScheduleID)
	putStr("benchmark", defs.Benchmark)
	src = "user"
	put("instrument_id", ov.InstrumentID, src)
	put("initial_cash", ov.InitialCash, src)
	put("currency", ov.Currency, src)
	put("start", ov.Start, src)
	put("end", ov.End, src)
	return cfg
}

func strPtr(s string) *string { return &s }

func itemIDOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
