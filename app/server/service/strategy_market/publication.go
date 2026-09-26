package strategy_market

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"zhigu/server/httpx"
	model "zhigu/server/model/strategy"
	"zhigu/server/service/finance"
	"zhigu/server/service/strategy"
)

// ---------- admin: create / version / validate / publish / withdraw ----------

func requireIdemKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", finance.NewError(400, "validation", "INVALID_PARAM", "缺少 Idempotency-Key")
	}
	return key, nil
}

// validateVersionContent strictly checks the immutable content fields and
// returns their canonical hash (§12.2: 名称1～80、摘要1～160、来源白名单等).
func validateVersionContent(vc VersionContent) (string, map[string]SourceEntry, error) {
	name := strings.TrimSpace(vc.Name)
	if n := utf8.RuneCountInString(name); n < 1 || n > 80 {
		return "", nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "名称须为 1～80 字")
	}
	summary := strings.TrimSpace(vc.Summary)
	if n := utf8.RuneCountInString(summary); n < 1 || n > 160 {
		return "", nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "摘要须为 1～160 字")
	}
	if strings.TrimSpace(vc.Category) == "" {
		return "", nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "缺少 category")
	}
	if strings.TrimSpace(vc.SignalPeriod) == "" {
		return "", nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "缺少 signal_period")
	}
	var tags, markets []string
	if err := json.Unmarshal(vc.Tags, &tags); err != nil {
		return "", nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "tags 必须是字符串数组")
	}
	if err := json.Unmarshal(vc.Markets, &markets); err != nil || len(markets) == 0 {
		return "", nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "markets 必须是非空数组")
	}
	for _, m := range markets {
		if m != "A" && m != "HK" {
			return "", nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "markets 仅支持 A 或 HK")
		}
	}
	sources, err := parseSources(vc.Sources)
	if err != nil {
		return "", nil, err
	}
	if strings.TrimSpace(vc.EditorSchemaVersion) != strategy.EditorSchemaVersion {
		return "", nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "editor_schema_version 必须为 "+strategy.EditorSchemaVersion)
	}
	if _, err := strategy.ParseMarketTemplate(vc.RuleTemplate); err != nil {
		return "", nil, err
	}
	if err := strictDecode(vc.BacktestDefaults, &BacktestDefaults{}); err != nil {
		return "", nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "backtest_defaults 含未知或非法字段")
	}
	hash, err := canonicalHash(map[string]any{
		"name": vc.Name, "summary": vc.Summary, "category": vc.Category,
		"tags": json.RawMessage(vc.Tags), "markets": json.RawMessage(vc.Markets),
		"signal_period": vc.SignalPeriod, "description": vc.Description,
		"hypothesis": vc.Hypothesis, "failure_cases": vc.FailureCases,
		"sources": json.RawMessage(vc.Sources), "rights_note": vc.RightsNote,
		"editor_schema_version": vc.EditorSchemaVersion,
		"rule_template":         json.RawMessage(vc.RuleTemplate),
		"backtest_defaults":     json.RawMessage(vc.BacktestDefaults),
	})
	if err != nil {
		return "", nil, err
	}
	return hash, sources, nil
}

func parseSources(raw json.RawMessage) (map[string]SourceEntry, error) {
	var entries []SourceEntry
	if err := strictDecode(raw, &entries); err != nil || len(entries) == 0 {
		return nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "sources 必须是非空严格结构数组")
	}
	out := make(map[string]SourceEntry, len(entries))
	for i, e := range entries {
		e.Title = strings.TrimSpace(e.Title)
		if e.Title == "" {
			return nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "sources 缺少 title")
		}
		if e.URL == "" && e.RecordRef == "" {
			return nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "sources 缺少 URL 或内部记录号")
		}
		if e.URL != "" && !strings.HasPrefix(e.URL, "http://") && !strings.HasPrefix(e.URL, "https://") {
			return nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "sources URL 仅允许 http/https")
		}
		if strings.TrimSpace(e.CollectedAt) == "" || len(e.CollectedAt) != 10 {
			return nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "sources 缺少整理日期")
		}
		if strings.TrimSpace(e.Adaptation) == "" {
			return nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "sources 缺少原版／改编说明")
		}
		out[strconvItoa(i)] = e
	}
	return out, nil
}

func strictDecode(raw json.RawMessage, dst any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return finance.NewError(422, "validation", "STRATEGY_INVALID", "缺少 JSON 内容")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func newVersionRow(itemID string, versionNo int, uid uint, vc VersionContent, contentHash string) model.MarketVersion {
	now := nowUTC()
	return model.MarketVersion{
		ID: httpx.NewID("smv"), ItemID: itemID, VersionNo: versionNo,
		Name: vc.Name, Summary: vc.Summary, Category: vc.Category,
		Tags: datatypes.JSON(vc.Tags), Markets: datatypes.JSON(vc.Markets),
		SignalPeriod: vc.SignalPeriod, Description: vc.Description,
		Hypothesis: vc.Hypothesis, FailureCases: vc.FailureCases,
		Sources: datatypes.JSON(vc.Sources), RightsNote: vc.RightsNote,
		EditorSchemaVersion: vc.EditorSchemaVersion,
		RuleTemplate:        datatypes.JSON(vc.RuleTemplate),
		BacktestDefaults:    datatypes.JSON(vc.BacktestDefaults),
		ContentHash:         contentHash, ValidationStatus: ValidationPending,
		ValidationReport: datatypes.JSON([]byte("{}")),
		CreatedBy:        uid, CreatedAt: now,
	}
}

// CreateItem creates a draft market item with its first immutable version.
func (s *Service) CreateItem(ctx context.Context, uid uint, idemKey string, req CreateItemReq) (map[string]any, bool, error) {
	key, err := requireIdemKey(idemKey)
	if err != nil {
		return nil, false, err
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" || utf8.RuneCountInString(slug) > 80 {
		return nil, false, finance.NewError(422, "validation", "STRATEGY_INVALID", "slug 须为 1～80 字")
	}
	contentHash, _, err := validateVersionContent(req.Version)
	if err != nil {
		return nil, false, err
	}
	hash, _ := canonicalHash(map[string]any{"op": OpCreateItem, "slug": slug, "content_hash": contentHash})
	var out map[string]any
	var replayed bool
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, resp, rep, ierr := s.idempotent(ctx, tx, uid, OpCreateItem, key, hash, func(tx *gorm.DB) (string, any, error) {
			itemID := httpx.NewID("smi")
			now := nowUTC()
			item := model.MarketItem{
				ID: itemID, Slug: slug, Status: ItemDraft, Revision: 1,
				CreatedBy: uid, CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Create(&item).Error; err != nil {
				return "", nil, finance.NewError(400, "validation", "INVALID_PARAM", "slug 已存在")
			}
			ver := newVersionRow(itemID, 1, uid, req.Version, contentHash)
			if err := tx.Create(&ver).Error; err != nil {
				return "", nil, err
			}
			if err := s.audit(tx, uid, "create_version", key, itemID, &ver.ID, 0, 1, "", map[string]any{"version_id": ver.ID, "content_hash": contentHash}); err != nil {
				return "", nil, err
			}
			return itemID, map[string]any{"item_id": itemID, "version_id": ver.ID, "revision": item.Revision}, nil
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
	return out, replayed, nil
}

// CreateVersion appends a new immutable version (never mutates old ones).
func (s *Service) CreateVersion(ctx context.Context, uid uint, idemKey, itemID string, req CreateVersionReq) (map[string]any, bool, error) {
	key, err := requireIdemKey(idemKey)
	if err != nil {
		return nil, false, err
	}
	contentHash, _, err := validateVersionContent(req.Version)
	if err != nil {
		return nil, false, err
	}
	hash, _ := canonicalHash(map[string]any{"op": OpCreateVersion, "item_id": itemID, "revision": req.Revision, "content_hash": contentHash})
	var out map[string]any
	var replayed bool
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, resp, rep, ierr := s.idempotent(ctx, tx, uid, OpCreateVersion, key, hash, func(tx *gorm.DB) (string, any, error) {
			item, err := itemLocked(tx, itemID)
			if err != nil {
				return "", nil, err
			}
			if item.Status != ItemDraft && item.Status != ItemPublished {
				return "", nil, errItemState(item)
			}
			if item.Revision != req.Revision {
				return "", nil, finance.NewError(409, "conflict", "REVISION_CONFLICT", "条目版本冲突")
			}
			var maxNo int
			if err := tx.Raw(`SELECT COALESCE(MAX(version_no), 0) FROM finance_strategy_market_versions WHERE item_id = ?`, itemID).Scan(&maxNo).Error; err != nil {
				return "", nil, err
			}
			ver := newVersionRow(itemID, maxNo+1, uid, req.Version, contentHash)
			if err := tx.Create(&ver).Error; err != nil {
				return "", nil, err
			}
			after := item.Revision + 1
			if err := tx.Model(&model.MarketItem{}).Where("id = ?", itemID).Updates(map[string]any{"revision": after, "updated_at": nowUTC()}).Error; err != nil {
				return "", nil, err
			}
			if err := s.audit(tx, uid, "create_version", key, itemID, &ver.ID, item.Revision, after, "", map[string]any{"version_id": ver.ID, "content_hash": contentHash}); err != nil {
				return "", nil, err
			}
			return itemID, map[string]any{"item_id": itemID, "version_id": ver.ID, "revision": after}, nil
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
	return out, replayed, err
}

// Validate binds the rule to a real catalog instrument, compiles it with the
// server compiler and records the report + hashes. No backtest runs here.
func (s *Service) Validate(ctx context.Context, uid uint, idemKey, itemID, versionID string, req ValidateReq) (map[string]any, bool, error) {
	key, err := requireIdemKey(idemKey)
	if err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(req.ValidationInstrumentID) == "" {
		return nil, false, finance.NewError(422, "validation", "STRATEGY_INVALID", "缺少 validation_instrument_id")
	}
	hash, _ := canonicalHash(map[string]any{"op": OpValidate, "item_id": itemID, "version_id": versionID, "revision": req.Revision, "instrument_id": req.ValidationInstrumentID})
	var out map[string]any
	var replayed bool
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, resp, rep, ierr := s.idempotent(ctx, tx, uid, OpValidate, key, hash, func(tx *gorm.DB) (string, any, error) {
			item, err := itemLocked(tx, itemID)
			if err != nil {
				return "", nil, err
			}
			if item.Status == ItemWithdrawn {
				return "", nil, errItemState(item)
			}
			if item.Revision != req.Revision {
				return "", nil, finance.NewError(409, "conflict", "REVISION_CONFLICT", "条目版本冲突")
			}
			ver, err := versionOf(tx, itemID, versionID)
			if err != nil {
				return "", nil, err
			}
			status, report, rerr := s.buildValidationReport(ctx, ver, req.ValidationInstrumentID)
			if rerr != nil {
				return "", nil, rerr
			}
			now := nowUTC()
			updates := map[string]any{"validation_status": status, "validation_report": report}
			if status == ValidationPassed {
				updates["validated_at"] = now
			}
			if err := tx.Model(&model.MarketVersion{}).Where("id = ?", ver.ID).Updates(updates).Error; err != nil {
				return "", nil, err
			}
			after := item.Revision + 1
			if err := tx.Model(&model.MarketItem{}).Where("id = ?", itemID).Updates(map[string]any{"revision": after, "updated_at": now}).Error; err != nil {
				return "", nil, err
			}
			if err := s.audit(tx, uid, "validate", key, itemID, &ver.ID, item.Revision, after, "", map[string]any{"version_id": ver.ID, "instrument_id": req.ValidationInstrumentID, "validation_status": status}); err != nil {
				return "", nil, err
			}
			return itemID, map[string]any{
				"item_id": itemID, "version_id": ver.ID, "revision": after,
				"validation_status": status, "validation_report": json.RawMessage(report),
			}, nil
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
	return out, replayed, err
}

// buildValidationReport compiles the template against a real instrument and
// returns (status, report JSON). Request-level errors are returned as error;
// content-level outcomes are recorded as passed/failed reports.
func (s *Service) buildValidationReport(ctx context.Context, ver model.MarketVersion, instrumentID string) (string, datatypes.JSON, error) {
	inst, err := s.Lookup.Instrument(ctx, instrumentID)
	if err != nil {
		return "", nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "验证标的不在受支持目录内")
	}
	tpl, err := strategy.ParseMarketTemplate(ver.RuleTemplate)
	if err != nil {
		return "", nil, err
	}
	group := marketGroup(inst.Exchange)
	allowed := false
	for _, m := range tpl.InstrumentBinding.Markets {
		if m == group {
			allowed = true
		}
	}
	fail := func(reason string) (string, datatypes.JSON, error) {
		rep, _ := json.Marshal(map[string]any{"instrument_id": instrumentID, "error": reason})
		return ValidationFailed, rep, nil
	}
	if !allowed {
		return fail("标的市场不在 instrument_binding.markets 内")
	}
	if strings.TrimSpace(ver.SignalPeriod) != strings.TrimSpace(tpl.EditorState.SignalPeriod) {
		return fail("signal_period 与规则模板不一致")
	}
	if strings.TrimSpace(ver.RightsNote) == "" {
		return fail("缺少 rights_note")
	}
	if _, err := parseSources(json.RawMessage(ver.Sources)); err != nil {
		return fail("来源信息不完整")
	}
	res, err := strategy.CompileEditor(tpl.EditorState, instrumentID)
	if err != nil {
		return fail(finance.ErrorCode(err) + ": " + finance.ErrorMessage(err))
	}
	if res.Status != "ready" {
		rep, _ := json.Marshal(map[string]any{"instrument_id": instrumentID, "missing_fields": res.Missing})
		return ValidationFailed, rep, nil
	}
	dslHash, err := canonicalHash(res.Document)
	if err != nil {
		return "", nil, err
	}
	rep, err := json.Marshal(map[string]any{
		"instrument_id":     instrumentID,
		"dsl_hash":          dslHash,
		"compiler_version":  res.Compiled.CompilerVersion,
		"warmup_bars":       res.Compiled.WarmupBars,
		"required_data":     res.Compiled.RequiredData,
		"continuity_checks": res.Continuity,
	})
	if err != nil {
		return "", nil, err
	}
	return ValidationPassed, rep, nil
}

// Publish atomically switches the current pointer to a validated version.
func (s *Service) Publish(ctx context.Context, uid uint, idemKey, itemID string, req PublishReq) (map[string]any, bool, error) {
	key, err := requireIdemKey(idemKey)
	if err != nil {
		return nil, false, err
	}
	hash, _ := canonicalHash(map[string]any{"op": OpPublish, "item_id": itemID, "revision": req.Revision, "market_version_id": req.MarketVersionID, "reason": req.Reason})
	var out map[string]any
	var replayed bool
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, resp, rep, ierr := s.idempotent(ctx, tx, uid, OpPublish, key, hash, func(tx *gorm.DB) (string, any, error) {
			item, err := itemLocked(tx, itemID)
			if err != nil {
				return "", nil, err
			}
			if item.Status == ItemWithdrawn {
				return "", nil, errItemState(item)
			}
			if item.Revision != req.Revision {
				return "", nil, finance.NewError(409, "conflict", "REVISION_CONFLICT", "条目版本冲突")
			}
			ver, err := versionOf(tx, itemID, req.MarketVersionID)
			if err != nil {
				return "", nil, err
			}
			if _, err := parseSources(json.RawMessage(ver.Sources)); err != nil || strings.TrimSpace(ver.RightsNote) == "" {
				return "", nil, finance.NewError(422, "validation", "MARKET_NOT_COPYABLE", "来源／权限信息不完整")
			}
			recomputed, _, err := versionContentHash(ver)
			if err != nil || recomputed != ver.ContentHash {
				return "", nil, finance.NewError(409, "conflict", "MARKET_VERSION_CHANGED", "内容 hash 不匹配")
			}
			rep := parseValidationReport(ver.ValidationReport)
			if ver.ValidationStatus != ValidationPassed || !EngineSupported(rep.CompilerVersion) {
				return "", nil, finance.NewError(422, "validation", "MARKET_NOT_COPYABLE", "版本未通过规则校验或引擎不支持")
			}
			now := nowUTC()
			updates := map[string]any{
				"status": ItemPublished, "current_version_id": ver.ID,
				"revision": item.Revision + 1, "updated_at": now,
			}
			if item.PublishedAt == nil {
				updates["published_at"] = now
			}
			if err := tx.Model(&model.MarketItem{}).Where("id = ?", itemID).Updates(updates).Error; err != nil {
				return "", nil, err
			}
			if ver.PublishedAt == nil {
				if err := tx.Model(&model.MarketVersion{}).Where("id = ?", ver.ID).Update("published_at", now).Error; err != nil {
					return "", nil, err
				}
			}
			if err := s.audit(tx, uid, "publish", key, itemID, &ver.ID, item.Revision, item.Revision+1, req.Reason, map[string]any{"version_id": ver.ID, "content_hash": ver.ContentHash}); err != nil {
				return "", nil, err
			}
			return itemID, map[string]any{"item_id": itemID, "market_version_id": ver.ID, "revision": item.Revision + 1, "status": ItemPublished}, nil
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
	return out, replayed, err
}

// Withdraw stops new copies; existing copies and history remain.
func (s *Service) Withdraw(ctx context.Context, uid uint, idemKey, itemID string, req WithdrawReq) (map[string]any, bool, error) {
	key, err := requireIdemKey(idemKey)
	if err != nil {
		return nil, false, err
	}
	hash, _ := canonicalHash(map[string]any{"op": OpWithdraw, "item_id": itemID, "revision": req.Revision, "reason": req.Reason})
	var out map[string]any
	var replayed bool
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, resp, rep, ierr := s.idempotent(ctx, tx, uid, OpWithdraw, key, hash, func(tx *gorm.DB) (string, any, error) {
			item, err := itemLocked(tx, itemID)
			if err != nil {
				return "", nil, err
			}
			if item.Status != ItemPublished {
				return "", nil, errItemState(item)
			}
			if item.Revision != req.Revision {
				return "", nil, finance.NewError(409, "conflict", "REVISION_CONFLICT", "条目版本冲突")
			}
			now := nowUTC()
			if err := tx.Model(&model.MarketItem{}).Where("id = ?", itemID).Updates(map[string]any{
				"status": ItemWithdrawn, "withdrawn_at": now, "revision": item.Revision + 1, "updated_at": now,
			}).Error; err != nil {
				return "", nil, err
			}
			if err := s.audit(tx, uid, "withdraw", key, itemID, item.CurrentVersionID, item.Revision, item.Revision+1, req.Reason, map[string]any{"status": ItemWithdrawn}); err != nil {
				return "", nil, err
			}
			return itemID, map[string]any{"item_id": itemID, "revision": item.Revision + 1, "status": ItemWithdrawn}, nil
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
	return out, replayed, err
}

func versionOf(tx *gorm.DB, itemID, versionID string) (model.MarketVersion, error) {
	var ver model.MarketVersion
	if err := tx.Where("id = ? AND item_id = ?", versionID, itemID).Take(&ver).Error; err != nil {
		return ver, finance.NewError(404, "not_found", "NOT_FOUND", "市场版本不存在")
	}
	return ver, nil
}

// versionContentHash recomputes the immutable content hash from stored columns.
func versionContentHash(ver model.MarketVersion) (string, map[string]SourceEntry, error) {
	return validateVersionContent(VersionContent{
		Name: ver.Name, Summary: ver.Summary, Category: ver.Category,
		Tags: json.RawMessage(ver.Tags), Markets: json.RawMessage(ver.Markets), SignalPeriod: ver.SignalPeriod,
		Description: ver.Description, Hypothesis: ver.Hypothesis, FailureCases: ver.FailureCases,
		Sources: json.RawMessage(ver.Sources), RightsNote: ver.RightsNote,
		EditorSchemaVersion: ver.EditorSchemaVersion,
		RuleTemplate:        json.RawMessage(ver.RuleTemplate), BacktestDefaults: json.RawMessage(ver.BacktestDefaults),
	})
}

// marketGroup maps a catalog exchange to the A/HK binding group.
func marketGroup(exchange string) string {
	if strings.EqualFold(exchange, "HKEX") {
		return "HK"
	}
	return "A"
}

func strconvItoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}
