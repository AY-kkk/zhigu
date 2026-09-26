package workbench

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"zhigu/server/httpx"
	model "zhigu/server/model/strategy"
	"zhigu/server/service/finance"
	"zhigu/server/service/strategy"
)

// SaveStrategy persists the draft as a new immutable version in ONE transaction:
// draft and strategy rows are locked, revision and base_version_id are verified,
// and the idempotency key is reserved first so concurrent same-key requests
// produce exactly one version (§12.4.7). Invalid references leave no empty strategy.
func (h *Hub) SaveStrategy(ctx context.Context, idem, draftID string, revision int, strategyID, baseVersion string) (map[string]any, error) {
	uid := finance.UserIDFrom(ctx)
	if strings.TrimSpace(idem) == "" {
		return nil, finance.NewError(400, "validation", "INVALID_PARAM", "缺少 Idempotency-Key")
	}
	hash := finance.SHA256Text(draftID + "|" + strings.TrimSpace(strategyID) + "|" + itoa(revision) + "|" + strings.TrimSpace(baseVersion))
	var out map[string]any
	err := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 幂等占位先写：SAVEPOINT 防唯一冲突毒化事务。
		if err := tx.SavePoint("save_reserve").Error; err != nil {
			return err
		}
		rec := model.Idempotency{
			OwnerID: uid, Operation: "save_strategy", IdempotencyKey: idem,
			RequestHash: hash, CreatedAt: time.Now().UTC(),
		}
		if cerr := tx.Create(&rec).Error; cerr != nil {
			tx.RollbackTo("save_reserve")
			var existing model.Idempotency
			if err := tx.Where("owner_id = ? AND operation = ? AND idempotency_key = ?", uid, "save_strategy", idem).Take(&existing).Error; err != nil {
				return cerr
			}
			if existing.RequestHash != hash {
				return finance.NewError(409, "conflict", "IDEMPOTENCY_CONFLICT", "相同幂等键对应不同请求")
			}
			var saved model.Version
			if err := tx.Where("id = ?", existing.ObjectID).Take(&saved).Error; err != nil {
				return finance.NewError(404, "not_found", "NOT_FOUND", "策略版本不存在")
			}
			out = map[string]any{"strategy_id": saved.StrategyID, "version_id": saved.ID, "revision": saved.Revision}
			return nil
		}

		var d model.Draft
		if err := tx.Raw(`SELECT * FROM finance_strategy_drafts WHERE id = ? AND owner_id = ? FOR UPDATE`, draftID, uid).Scan(&d).Error; err != nil {
			return err
		}
		if d.ID == "" {
			return finance.NewError(404, "not_found", "NOT_FOUND", "草稿不存在")
		}
		if d.Revision != revision || len(d.DSL) == 0 {
			return finance.NewError(422, "validation", "STRATEGY_INVALID", "草稿未就绪或版本不匹配")
		}
		now := time.Now().UTC()
		if strings.TrimSpace(strategyID) == "" {
			strategyID = httpx.NewID("st")
			st := model.Strategy{ID: strategyID, OwnerID: uid, Name: "未命名策略", CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&st).Error; err != nil {
				return err
			}
		} else {
			var st model.Strategy
			if err := tx.Raw(`SELECT * FROM finance_strategies WHERE id = ? AND owner_id = ? AND deleted_at IS NULL FOR UPDATE`, strategyID, uid).Scan(&st).Error; err != nil {
				return err
			}
			if st.ID == "" {
				return finance.NewError(404, "not_found", "NOT_FOUND", "策略不存在")
			}
			current := ""
			if st.CurrentVersionID != nil {
				current = *st.CurrentVersionID
			}
			// R4：已有策略追加版本必须携带 base_version_id，缺省与过期都拒绝。
			if baseVersion == "" {
				return finance.NewError(422, "validation", "STRATEGY_INVALID", "向已有策略追加版本必须提供 base_version_id")
			}
			if current != baseVersion {
				return finance.NewError(409, "conflict", "REVISION_CONFLICT", "策略已被其他编辑保存，请先查看最新版本")
			}
		}
		var count int64
		if err := tx.Model(&model.Version{}).Where("strategy_id = ?", strategyID).Count(&count).Error; err != nil {
			return err
		}
		var doc strategy.Document
		_ = json.Unmarshal(d.DSL, &doc)
		compilerVersion := strategy.CompilerVersion
		if doc.SchemaVersion == strategy.SchemaVersionV2 {
			compilerVersion = strategy.CompilerVersionV2
		}
		fieldSources := d.FieldSources
		if len(fieldSources) == 0 {
			fieldSources = datatypes.JSON([]byte("{}"))
		}
		configDraft := d.BacktestConfigDraft
		if len(configDraft) == 0 {
			configDraft = datatypes.JSON([]byte("{}"))
		}
		vid := httpx.NewID("stv")
		ver := model.Version{
			ID: vid, StrategyID: strategyID, OwnerID: uid, Revision: int(count) + 1, Name: doc.Name,
			DSL: d.DSL, DSLHash: finance.SHA256Text(string(d.DSL)), Compiled: d.Compiled,
			CompilerVersion: compilerVersion, CreatedAt: now,
			EditorSchemaVersion: d.EditorSchemaVersion, EditorState: d.EditorState,
			FieldSources: fieldSources, BacktestConfigDraft: configDraft,
			OriginMarketItemID: d.OriginMarketItemID, OriginMarketVersionID: d.OriginMarketVersionID,
		}
		if err := tx.Create(&ver).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Strategy{}).Where("id = ?", strategyID).Updates(map[string]any{
			"current_version_id": vid, "name": doc.Name, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		snap, _ := json.Marshal(map[string]any{"strategy_id": strategyID, "version_id": vid, "revision": ver.Revision})
		if err := tx.Model(&model.Idempotency{}).
			Where("owner_id = ? AND operation = ? AND idempotency_key = ?", uid, "save_strategy", idem).
			Updates(map[string]any{"object_id": vid, "response_snapshot": datatypes.JSON(snap)}).Error; err != nil {
			return err
		}
		out = map[string]any{"strategy_id": strategyID, "version_id": vid, "revision": ver.Revision}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
