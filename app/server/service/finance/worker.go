package finance

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"

	modelfinance "zhigu/server/model/finance"
)

func (s *ResearchService) StartWorker(ctx context.Context) {
	WarmLiveCatalog()
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.Tick(ctx, true)
			}
		}
	}()
}

func (s *ResearchService) tick(ctx context.Context) {
	s.Tick(ctx, false)
}

func (s *ResearchService) Tick(ctx context.Context, async bool) {
	now := s.Clock.Now()
	s.failQueueTimeouts(ctx, now)
	s.reapExpiredLeases(ctx, now)
	s.convergeCancels(ctx)
	_ = s.PurgeRetention(ctx)
	var runs []modelfinance.ResearchRun
	if err := s.DB.WithContext(ctx).
		Where("status = ? AND deleted_at IS NULL", StatusQueued).
		Where("lease_until IS NULL OR lease_until < ?", now).
		Order("created_at ASC").Limit(GlobalActiveMax).Find(&runs).Error; err != nil {
		return
	}
	for _, run := range runs {
		owner := "zhigu-worker"
		until := now.Add(LeaseTTL)
		res := s.DB.WithContext(ctx).Model(&modelfinance.ResearchRun{}).
			Where("id = ? AND status = ?", run.ID, StatusQueued).
			Updates(map[string]any{"lease_owner": owner, "lease_until": until, "updated_at": now})
		if res.RowsAffected == 0 {
			continue
		}
		id := run.ID
		runFn := func() {
			orch := NewOrchestrator(s, NewEvidenceService(s.DB))
			if err := orch.Run(ctx, id); err != nil {
				log.Printf("eino run %s: %v", id, err)
			}
		}
		if async {
			go runFn()
		} else {
			runFn()
		}
	}
}

func (s *ResearchService) failQueueTimeouts(ctx context.Context, now time.Time) {
	var runs []modelfinance.ResearchRun
	if err := s.DB.WithContext(ctx).Where("status = ? AND deleted_at IS NULL", StatusQueued).Find(&runs).Error; err != nil {
		return
	}
	for _, run := range runs {
		_, queueTimeout := s.snapshotTimeouts(run)
		if run.CreatedAt.Add(queueTimeout).After(now) {
			continue
		}
		_ = s.DB.WithContext(ctx).Model(&modelfinance.ResearchRun{}).
			Where("id = ? AND status = ?", run.ID, StatusQueued).
			Updates(map[string]any{"status": StatusFailed, "stage": "queue_timeout", "updated_at": now, "lease_owner": nil, "lease_until": nil, "version": gorm.Expr("version + 1")})
		s.revokeTokens(ctx, run.ID)
	}
}

func (s *ResearchService) reapExpiredLeases(ctx context.Context, now time.Time) {
	var runs []modelfinance.ResearchRun
	if err := s.DB.WithContext(ctx).Where("status IN ? AND lease_until IS NOT NULL AND lease_until < ? AND deleted_at IS NULL",
		[]string{StatusResearching, StatusVerifying}, now).Find(&runs).Error; err != nil {
		return
	}
	for _, run := range runs {
		res := s.DB.WithContext(ctx).Model(&modelfinance.ResearchRun{}).
			Where("id = ? AND version = ?", run.ID, run.Version).
			Updates(map[string]any{"status": StatusIncomplete, "stage": "lease_lost", "updated_at": now, "lease_owner": nil, "lease_until": nil, "version": run.Version + 1})
		if res.RowsAffected == 0 {
			continue
		}
		s.revokeTokens(ctx, run.ID)
	}
}

func (s *ResearchService) renewLease(ctx context.Context, runID string) error {
	now := s.Clock.Now()
	until := now.Add(LeaseTTL)
	res := s.DB.WithContext(ctx).Model(&modelfinance.ResearchRun{}).
		Where("id = ? AND lease_owner IS NOT NULL AND status IN ?", runID, []string{StatusResearching, StatusVerifying}).
		Updates(map[string]any{"lease_until": until, "updated_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return NewError(409, "conflict", "LEASE_LOST", "租约已丢失或任务已结束")
	}
	return nil
}
