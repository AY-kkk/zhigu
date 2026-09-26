package strategy_market

import (
	"context"
	"encoding/json"
	"strings"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"zhigu/server/httpx"
	model "zhigu/server/model/strategy"
	"zhigu/server/service/finance"
)

// configWhitelist is the only backtest config subset copied into public evidence;
// private inputs never leave the owner's run.
var configWhitelist = []string{
	"instrument_id", "initial_cash", "currency", "start", "end",
	"fee_schedule_id", "commission_config", "slippage_bps", "participation_cap",
	"benchmark", "data_snapshot_id",
}

// ImportEvidence snapshots the admin's own succeeded verification backtest into
// a pending evidence record bound to the market version's validated DSL and
// instrument (§12.6). source_run_id stays private (audit only).
func (s *Service) ImportEvidence(ctx context.Context, uid uint, idemKey, itemID, versionID string, req EvidenceImportReq) (map[string]any, bool, error) {
	key, err := requireIdemKey(idemKey)
	if err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(req.SourceRunID) == "" || strings.TrimSpace(req.RightsNote) == "" {
		return nil, false, finance.NewError(422, "validation", "STRATEGY_INVALID", "缺少 source_run_id 或 rights_note")
	}
	hash, _ := canonicalHash(map[string]any{"op": OpEvidenceImport, "item_id": itemID, "version_id": versionID, "revision": req.Revision, "source_run_id": req.SourceRunID, "rights_note": req.RightsNote})
	var out map[string]any
	var replayed bool
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, resp, rep, ierr := s.idempotent(ctx, tx, uid, OpEvidenceImport, key, hash, func(tx *gorm.DB) (string, any, error) {
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
			rep := parseValidationReport(ver.ValidationReport)
			if ver.ValidationStatus != ValidationPassed {
				return "", nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "版本未通过规则校验，不能导入证据")
			}

			var run model.BacktestRun
			// 仅允许管理员自己持有的 succeeded 回测记录。
			if err := tx.Where("id = ? AND owner_id = ? AND status = 'succeeded'", req.SourceRunID, uid).Take(&run).Error; err != nil {
				return "", nil, finance.NewError(404, "not_found", "NOT_FOUND", "没有可导入的成功回测记录")
			}
			var saved model.Version
			if err := tx.Where("id = ?", run.StrategyVersionID).Take(&saved).Error; err != nil {
				return "", nil, finance.NewError(404, "not_found", "NOT_FOUND", "回测绑定的策略版本不存在")
			}
			dslHash, err := canonicalHash(json.RawMessage(saved.DSL))
			if err != nil || dslHash != rep.DSLHash {
				return "", nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "回测规则与市场版本绑定 DSL 不一致")
			}
			resultHash := ""
			if run.ResultHash != nil {
				resultHash = *run.ResultHash
			}
			cfgMap := map[string]any{}
			_ = json.Unmarshal(run.Config, &cfgMap)
			runInst, _ := cfgMap["instrument_id"].(string)
			if runInst == "" || runInst != rep.InstrumentID {
				return "", nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "回测标的与市场版本验证标的不同")
			}

			cfg := map[string]any{}
			for _, k := range configWhitelist {
				if v, ok := cfgMap[k]; ok {
					cfg[k] = v
				}
			}
			manifest := map[string]any{}
			_ = json.Unmarshal(run.Manifest, &manifest)
			if manifest == nil {
				manifest = map[string]any{}
			}
			manifest["rights_note"] = req.RightsNote

			equity, trades := s.extractLedger(tx, run.ID)
			limitations := json.RawMessage([]byte("[]"))
			var res model.Result
			if err := tx.Where("run_id = ?", run.ID).Take(&res).Error; err == nil && len(res.Assumptions) > 0 {
				limitations = json.RawMessage(res.Assumptions)
			}
			metrics := json.RawMessage(run.ResultSummary)
			if len(metrics) == 0 {
				metrics = json.RawMessage([]byte("{}"))
			}
			evidenceID := httpx.NewID("sme")
			evidenceHash, err := canonicalHash(map[string]any{
				"bound_dsl_hash": rep.DSLHash, "validation_instrument_id": rep.InstrumentID,
				"config": cfg, "manifest": manifest, "metrics": json.RawMessage(metrics),
				"equity": equity, "trades": trades, "limitations": json.RawMessage(limitations),
				"result_hash": resultHash,
			})
			if err != nil {
				return "", nil, err
			}
			now := nowUTC()
			cfgRaw, _ := json.Marshal(cfg)
			manRaw, _ := json.Marshal(manifest)
			eqRaw, _ := json.Marshal(equity)
			trRaw, _ := json.Marshal(trades)
			row := model.MarketEvidence{
				ID: evidenceID, ItemID: itemID, MarketVersionID: ver.ID,
				Status: EvidencePending, ValidationInstrumentID: rep.InstrumentID,
				BoundDSLHash: rep.DSLHash,
				Config:       datatypes.JSON(cfgRaw), Manifest: datatypes.JSON(manRaw),
				Metrics: datatypes.JSON(metrics), Equity: datatypes.JSON(eqRaw),
				Trades: datatypes.JSON(trRaw), Limitations: datatypes.JSON(limitations),
				ResultHash: resultHash, EvidenceHash: evidenceHash,
				CreatedBy: uid, CreatedAt: now,
			}
			if err := tx.Create(&row).Error; err != nil {
				return "", nil, err
			}
			after := item.Revision + 1
			if err := tx.Model(&model.MarketItem{}).Where("id = ?", itemID).Updates(map[string]any{"revision": after, "updated_at": now}).Error; err != nil {
				return "", nil, err
			}
			if err := s.audit(tx, uid, "import_evidence", key, itemID, &ver.ID, item.Revision, after,
				"source_run_id="+req.SourceRunID,
				map[string]any{"evidence_id": evidenceID, "evidence_hash": evidenceHash, "result_hash": resultHash}); err != nil {
				return "", nil, err
			}
			return evidenceID, map[string]any{
				"evidence_id": evidenceID, "item_id": itemID, "version_id": ver.ID,
				"revision": after, "status": EvidencePending,
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

func (s *Service) extractLedger(tx *gorm.DB, runID string) (equity []map[string]any, trades []map[string]any) {
	var rows []model.EquityRow
	_ = tx.Where("run_id = ?", runID).Order("trade_date asc").Find(&rows).Error
	equity = make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		equity = append(equity, map[string]any{
			"date": r.TradeDate, "equity": r.Equity, "cash": r.Cash,
			"position_qty": r.PositionQty, "market_value": r.MarketValue, "drawdown": r.Drawdown,
		})
	}
	var orders []model.Order
	_ = tx.Where("run_id = ?", runID).Order("created_at asc").Find(&orders).Error
	sideOf := map[string]model.Order{}
	for _, o := range orders {
		sideOf[o.ID] = o
	}
	var fills []model.Fill
	_ = tx.Where("run_id = ?", runID).Order("fill_date asc, id asc").Find(&fills).Error
	trades = make([]map[string]any, 0, len(fills))
	for _, f := range fills {
		o := sideOf[f.OrderID]
		trades = append(trades, map[string]any{
			"date": f.FillDate, "order_id": f.OrderID, "side": o.Side, "status": o.Status,
			"qty": f.Qty, "price": f.Price, "fees": json.RawMessage(f.Fees),
			"cash_delta": f.CashDelta, "reason": o.Reason,
		})
	}
	return equity, trades
}

// ReviewEvidence records an approve/reject/revoke decision. Evidence content is
// never mutated; only status/review columns change and the audit trail grows.
func (s *Service) ReviewEvidence(ctx context.Context, uid uint, idemKey, itemID, evidenceID string, req EvidenceReviewReq) (map[string]any, bool, error) {
	key, err := requireIdemKey(idemKey)
	if err != nil {
		return nil, false, err
	}
	var action string
	switch req.Decision {
	case "approve":
		action = "approve_evidence"
	case "reject":
		action = "reject_evidence"
	case "revoke":
		action = "revoke_evidence"
	default:
		return nil, false, finance.NewError(400, "validation", "INVALID_PARAM", "decision 仅支持 approve/reject/revoke")
	}
	hash, _ := canonicalHash(map[string]any{"op": OpEvidenceReview, "item_id": itemID, "evidence_id": evidenceID, "revision": req.Revision, "decision": req.Decision, "reason": req.Reason})
	var out map[string]any
	var replayed bool
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, resp, rep, ierr := s.idempotent(ctx, tx, uid, OpEvidenceReview, key, hash, func(tx *gorm.DB) (string, any, error) {
			item, err := itemLocked(tx, itemID)
			if err != nil {
				return "", nil, err
			}
			if item.Revision != req.Revision {
				return "", nil, finance.NewError(409, "conflict", "REVISION_CONFLICT", "条目版本冲突")
			}
			var ev model.MarketEvidence
			if err := tx.Where("id = ? AND item_id = ?", evidenceID, itemID).Take(&ev).Error; err != nil {
				return "", nil, finance.NewError(404, "not_found", "NOT_FOUND", "证据不存在")
			}
			statusAfter := map[string]string{"approve": EvidenceApproved, "reject": EvidenceRejected, "revoke": EvidenceRevoked}[req.Decision]
			if req.Decision == "revoke" {
				if ev.Status != EvidenceApproved {
					return "", nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "只有已批准证据可撤销")
				}
			} else if ev.Status != EvidencePending {
				return "", nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "只有待审核证据可批准或驳回")
			}
			if req.Decision == "approve" {
				if err := requireDisclosable(ev); err != nil {
					return "", nil, err
				}
			}
			now := nowUTC()
			if err := tx.Model(&model.MarketEvidence{}).Where("id = ?", ev.ID).Updates(map[string]any{
				"status": statusAfter, "reviewed_by": uid, "reviewed_at": now, "review_note": req.Reason,
			}).Error; err != nil {
				return "", nil, err
			}
			after := item.Revision + 1
			if err := tx.Model(&model.MarketItem{}).Where("id = ?", itemID).Updates(map[string]any{"revision": after, "updated_at": now}).Error; err != nil {
				return "", nil, err
			}
			vid := ev.MarketVersionID
			if err := s.audit(tx, uid, action, key, itemID, &vid, item.Revision, after, req.Reason, map[string]any{"evidence_id": ev.ID, "decision": req.Decision}); err != nil {
				return "", nil, err
			}
			return evidenceID, map[string]any{"evidence_id": ev.ID, "status": statusAfter, "revision": after}, nil
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

// requireDisclosable enforces the approve gate: complete manifest, result hash
// and disclosure-rights note must all be present.
func requireDisclosable(ev model.MarketEvidence) error {
	if strings.TrimSpace(ev.ResultHash) == "" {
		return finance.NewError(422, "validation", "STRATEGY_INVALID", "缺少结果 hash，不能批准")
	}
	var manifest map[string]any
	if err := json.Unmarshal(ev.Manifest, &manifest); err != nil || len(manifest) == 0 {
		return finance.NewError(422, "validation", "STRATEGY_INVALID", "缺少完整 manifest，不能批准")
	}
	note, _ := manifest["rights_note"].(string)
	if strings.TrimSpace(note) == "" {
		return finance.NewError(422, "validation", "STRATEGY_INVALID", "缺少披露权限记录，不能批准")
	}
	return nil
}
