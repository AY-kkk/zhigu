package finance

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	modelfinance "zhigu/server/model/finance"
)

var activeRunStatuses = []string{StatusQueued, StatusResearching, StatusVerifying, StatusCanceling}

type BudgetSnapshot struct {
	MaxModelCalls        int `json:"max_model_calls"`
	MaxToolCalls         int `json:"max_tool_calls"`
	MaxModelCallsPerRole int `json:"max_model_calls_per_role"`
	MaxToolCallsPerRole  int `json:"max_tool_calls_per_role"`
	RunTimeoutSeconds    int `json:"run_timeout_seconds"`
	QueueTimeoutSeconds  int `json:"queue_timeout_seconds"`
}

func DefaultBudgetSnapshot() BudgetSnapshot {
	return BudgetSnapshot{
		MaxModelCalls:        14,
		MaxToolCalls:         12,
		MaxModelCallsPerRole: 4,
		MaxToolCallsPerRole:  6,
		RunTimeoutSeconds:    180,
		QueueTimeoutSeconds:  30,
	}
}

func FreezeBudgetSnapshot(policy PolicyView) BudgetSnapshot {
	snap := DefaultBudgetSnapshot()
	if policy.MaxModelCalls > 0 {
		snap.MaxModelCalls = policy.MaxModelCalls
	}
	if policy.MaxToolCalls > 0 {
		snap.MaxToolCalls = policy.MaxToolCalls
	}
	if policy.RunTimeoutSeconds > 0 {
		snap.RunTimeoutSeconds = policy.RunTimeoutSeconds
	}
	if policy.QueueTimeoutSeconds > 0 {
		snap.QueueTimeoutSeconds = policy.QueueTimeoutSeconds
	}
	if snap.MaxModelCallsPerRole > snap.MaxModelCalls {
		snap.MaxModelCallsPerRole = snap.MaxModelCalls
	}
	if snap.MaxToolCallsPerRole > snap.MaxToolCalls {
		snap.MaxToolCallsPerRole = snap.MaxToolCalls
	}
	return snap
}

func ParseBudgetSnapshot(raw []byte) BudgetSnapshot {
	snap := DefaultBudgetSnapshot()
	if len(raw) == 0 {
		return snap
	}
	_ = json.Unmarshal(raw, &snap)
	if snap.MaxModelCalls <= 0 {
		snap.MaxModelCalls = 14
	}
	if snap.MaxToolCalls <= 0 {
		snap.MaxToolCalls = 12
	}
	if snap.MaxModelCallsPerRole <= 0 {
		snap.MaxModelCallsPerRole = 4
	}
	if snap.MaxToolCallsPerRole <= 0 {
		snap.MaxToolCallsPerRole = 6
	}
	if snap.MaxModelCallsPerRole > snap.MaxModelCalls {
		snap.MaxModelCallsPerRole = snap.MaxModelCalls
	}
	if snap.MaxToolCallsPerRole > snap.MaxToolCalls {
		snap.MaxToolCallsPerRole = snap.MaxToolCalls
	}
	return snap
}

func runIsCanceled(run modelfinance.ResearchRun) bool {
	return run.DeletedAt != nil || run.Status == StatusCanceling || run.Status == StatusCanceled
}

func runAllowsTools(run modelfinance.ResearchRun) bool {
	return runAllowsToolsAt(run, time.Now().UTC())
}

func runAllowsToolsAt(run modelfinance.ResearchRun, now time.Time) bool {
	if run.DeletedAt != nil {
		return false
	}
	switch run.Status {
	case StatusQueued, StatusResearching, StatusVerifying:
	default:
		return false
	}
	if run.DeadlineAt != nil && !now.Before(*run.DeadlineAt) {
		return false
	}
	return true
}

func (s *ResearchService) snapshotTimeouts(run modelfinance.ResearchRun) (runTimeout, queueTimeout time.Duration) {
	snap := ParseBudgetSnapshot(run.BudgetSnapshot)
	runTimeout = RunTimeout
	queueTimeout = QueueTimeout
	if snap.RunTimeoutSeconds > 0 {
		runTimeout = time.Duration(snap.RunTimeoutSeconds) * time.Second
	}
	if snap.QueueTimeoutSeconds > 0 {
		queueTimeout = time.Duration(snap.QueueTimeoutSeconds) * time.Second
	}
	return runTimeout, queueTimeout
}

func (s *ResearchService) casUpdateRun(tx *gorm.DB, runID string, expectedVersion int64, allowed []string, updates map[string]any) error {
	if tx == nil {
		tx = s.DB
	}
	now := s.Clock.Now()
	if _, ok := updates["updated_at"]; !ok {
		updates["updated_at"] = now
	}
	if _, ok := updates["version"]; !ok {
		updates["version"] = expectedVersion + 1
	}
	q := tx.Model(&modelfinance.ResearchRun{}).Where("id = ? AND version = ? AND deleted_at IS NULL", runID, expectedVersion)
	if len(allowed) > 0 {
		q = q.Where("status IN ?", allowed)
	}
	res := q.Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return NewError(409, "conflict", "RUN_CLOSED", "状态或版本已变更，拒绝写入")
	}
	return nil
}

func (s *ResearchService) finalizeCancel(ctx context.Context, runID string) error {
	var tasks []modelfinance.ResearchTask
	if err := s.DB.WithContext(ctx).Where("run_id = ?", runID).Find(&tasks).Error; err != nil {
		return err
	}
	acked := true
	for _, task := range tasks {
		if _, err := s.Client.Cancel(ctx, task.ID); err != nil {
			if ErrorCode(err) != "TASK_NOT_FOUND" {
				acked = false
			}
		}
	}
	s.revokeTokens(ctx, runID)
	run, err := s.loadRun(runID)
	if err != nil {
		return err
	}
	now := s.Clock.Now()
	leaseExpired := run.LeaseUntil == nil || !now.Before(*run.LeaseUntil)
	if !acked && !leaseExpired {
		return NewError(409, "conflict", "CANCEL_IN_PROGRESS", "等待执行器确认或租约过期")
	}
	res := s.DB.WithContext(ctx).Model(&modelfinance.ResearchRun{}).
		Where("id = ? AND status = ?", runID, StatusCanceling).
		Updates(map[string]any{
			"status": StatusCanceled, "stage": "canceled", "updated_at": now,
			"lease_owner": nil, "lease_until": nil, "version": gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return NewError(409, "conflict", "RUN_CLOSED", "状态已变更")
	}
	if run.Stage == "deleting" {
		_ = s.DB.WithContext(ctx).Model(&modelfinance.ResearchRun{}).Where("id = ?", runID).
			Update("deleted_at", now)
	}
	_ = s.DB.WithContext(ctx).Model(&modelfinance.ResearchTask{}).Where("run_id = ?", runID).
		Updates(map[string]any{"status": "canceled", "updated_at": now})
	return nil
}

func (s *ResearchService) convergeCancels(ctx context.Context) {
	var runs []modelfinance.ResearchRun
	if err := s.DB.WithContext(ctx).Where("status = ?", StatusCanceling).Find(&runs).Error; err != nil {
		return
	}
	for _, run := range runs {
		_ = s.finalizeCancel(ctx, run.ID)
	}
}

func (s *ResearchService) loadBudgetSnapshot(tx *gorm.DB, runID string) BudgetSnapshot {
	if tx == nil {
		tx = s.DB
	}
	var run modelfinance.ResearchRun
	if err := tx.Where("id = ?", runID).Take(&run).Error; err != nil {
		return DefaultBudgetSnapshot()
	}
	return ParseBudgetSnapshot(run.BudgetSnapshot)
}
