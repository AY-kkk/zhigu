package intel

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const defaultIntelDailyTokenLimit int64 = 200000

func intelDailyTokenLimit() int64 {
	raw := strings.TrimSpace(os.Getenv("ZHIGU_INTEL_DAILY_TOKEN_LIMIT"))
	if raw == "" {
		return defaultIntelDailyTokenLimit
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return defaultIntelDailyTokenLimit
	}
	return value
}

func usageTotal(usage map[string]any) int64 {
	for _, key := range []string{"total_tokens", "input_tokens", "output_tokens", "prompt_tokens", "completion_tokens"} {
		if value, ok := usage[key].(float64); ok && value >= 0 {
			return int64(value)
		}
	}
	return 0
}

func (s *Service) ReserveModelUsage(ctx context.Context, scope Scope, jobID string, attempt int, reserved int64) (string, error) {
	if reserved <= 0 {
		reserved = intelInputTokenLimit + intelOutputTokenLimit
	}
	var usageID string
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := execSQL(tx, `SELECT pg_advisory_xact_lock(hashtext(?))`, "intel-model-budget:"+scope.NamespaceID); err != nil {
			return err
		}
		var running int
		if err := tx.Raw(`SELECT count(*) FROM finance_intel_jobs WHERE namespace_id=? AND kind='extract' AND status='running' AND id<>?`, scope.NamespaceID, jobID).Scan(&running).Error; err != nil {
			return err
		}
		if running >= 2 {
			return newError(429, "MODEL_UNAVAILABLE", "模型并发已达到2")
		}
		start := time.Now().UTC()
		start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
		var used int64
		if err := tx.Raw(`SELECT COALESCE(SUM(reserved_tokens),0) FROM finance_intel_usage WHERE namespace_id=? AND created_at>=? AND created_at<?`, scope.NamespaceID, start, start.Add(24*time.Hour)).Scan(&used).Error; err != nil {
			return err
		}
		if used+reserved > intelDailyTokenLimit() {
			return newError(429, "MODEL_BUDGET_EXCEEDED", "Intel 模型每日预算不足")
		}
		usageID = "usage_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		return execSQL(tx, `INSERT INTO finance_intel_usage(id,namespace_id,job_id,attempt,reserved_tokens,status,created_at)
VALUES (?,?,?,?,?,'reserved',?) ON CONFLICT(namespace_id,job_id,attempt) DO NOTHING`, usageID, scope.NamespaceID, jobID, attempt, reserved, s.now())
	})
	if err != nil {
		return "", err
	}
	var existing string
	_ = s.DB.WithContext(ctx).Raw(`SELECT id FROM finance_intel_usage WHERE namespace_id=? AND job_id=? AND attempt=?`, scope.NamespaceID, jobID, attempt).Scan(&existing).Error
	if existing != "" {
		usageID = existing
	}
	return usageID, nil
}

func (s *Service) SettleModelUsage(ctx context.Context, usageID string, actual int64, unknown bool) error {
	status := "settled"
	if unknown {
		status = "usage_unknown"
	}
	return s.DB.WithContext(ctx).Exec(`UPDATE finance_intel_usage SET actual_tokens=?,status=?,settled_at=? WHERE id=?`, actual, status, s.now(), usageID).Error
}
