package finance

import (
	"context"
	"time"

	"gorm.io/datatypes"

	modelfinance "zhigu/server/model/finance"
)

func (s *ResearchService) PurgeRetention(ctx context.Context) error {
	now := s.Clock.Now()
	dayAgo := now.Add(-24 * time.Hour)
	monthAgo := now.Add(-30 * 24 * time.Hour)
	emptyJSON := datatypes.JSON([]byte("{}"))
	emptyArr := datatypes.JSON([]byte("[]"))
	deleted := s.DB.WithContext(ctx).Model(&modelfinance.ResearchRun{}).
		Where("deleted_at IS NOT NULL AND deleted_at < ?", dayAgo)
	_ = s.DB.WithContext(ctx).Model(&modelfinance.Evidence{}).
		Where("run_id IN (?)", s.DB.Model(&modelfinance.ResearchRun{}).Select("id").Where("deleted_at IS NOT NULL AND deleted_at < ?", dayAgo)).
		Updates(map[string]any{"text": "", "metrics": emptyArr, "updated_at": now})
	_ = s.DB.WithContext(ctx).Model(&modelfinance.Question{}).
		Where("run_id IN (?)", s.DB.Model(&modelfinance.ResearchRun{}).Select("id").Where("deleted_at IS NOT NULL AND deleted_at < ?", dayAgo)).
		Updates(map[string]any{"body": "", "response": emptyJSON, "updated_at": now})
	_ = s.DB.WithContext(ctx).Model(&modelfinance.Report{}).
		Where("run_id IN (?)", s.DB.Model(&modelfinance.ResearchRun{}).Select("id").Where("deleted_at IS NOT NULL AND deleted_at < ?", dayAgo)).
		Updates(map[string]any{"body": emptyJSON, "updated_at": now})
	_ = s.DB.WithContext(ctx).Model(&modelfinance.ResearchTask{}).
		Where("run_id IN (?)", s.DB.Model(&modelfinance.ResearchRun{}).Select("id").Where("deleted_at IS NOT NULL AND deleted_at < ?", dayAgo)).
		Updates(map[string]any{"result": emptyJSON, "updated_at": now})
	_ = s.DB.WithContext(ctx).Model(&modelfinance.DataRecord{}).
		Where("grant_id IN (?)", s.DB.Model(&modelfinance.ToolGrant{}).Select("id").Where("run_id IN (?)",
			s.DB.Model(&modelfinance.ResearchRun{}).Select("id").Where("deleted_at IS NOT NULL AND deleted_at < ?", dayAgo))).
		Updates(map[string]any{"payload": emptyArr})
	_ = s.DB.WithContext(ctx).Model(&modelfinance.ClaimDraft{}).
		Where("confirmed_run_id IN (?)", s.DB.Model(&modelfinance.ResearchRun{}).Select("id").Where("deleted_at IS NOT NULL AND deleted_at < ?", dayAgo)).
		Updates(map[string]any{"text": "", "items": emptyArr, "updated_at": now})
	_ = deleted.Updates(map[string]any{"claim_snapshot": emptyArr, "updated_at": now})
	var expiredTasks []modelfinance.ResearchTask
	_ = s.DB.WithContext(ctx).Where("run_id IN (?)", s.DB.Model(&modelfinance.ResearchRun{}).Select("id").Where("deleted_at IS NOT NULL AND deleted_at < ?", dayAgo)).
		Find(&expiredTasks)
	for _, task := range expiredTasks {
		_ = s.purgeExecutorCopy(ctx, task.ID)
	}
	_ = s.DB.WithContext(ctx).Model(&modelfinance.ResearchRun{}).
		Where("deleted_at IS NULL AND created_at < ?", monthAgo).
		Updates(map[string]any{"deleted_at": now, "updated_at": now})
	return nil
}
