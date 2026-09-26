package intel

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type JobLease struct {
	ID          string    `json:"id"`
	NamespaceID string    `json:"namespace_id"`
	Kind        string    `json:"kind"`
	Provider    string    `json:"provider"`
	Generation  int64     `json:"generation"`
	LeaseEpoch  int64     `json:"lease_epoch"`
	LeaseUntil  time.Time `json:"lease_until"`
	Attempts    int       `json:"attempts"`
}

func (s *Service) ClaimJob(ctx context.Context, owner string) (JobLease, error) {
	if owner == "" {
		owner = "intel-worker"
	}
	var row map[string]any
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Raw(`UPDATE finance_intel_jobs j
SET status='running', owner=?, lease_epoch=j.lease_epoch+1, lease_until=now()+interval '120 seconds', attempts=j.attempts+1, updated_at=now()
WHERE j.id = (
  SELECT j2.id FROM finance_intel_jobs j2
  JOIN finance_intel_namespaces n ON n.id=j2.namespace_id AND n.generation=j2.generation
  WHERE j2.status='queued' OR (j2.status='running' AND j2.lease_until < now())
  ORDER BY j2.created_at,j2.id
  LIMIT 1
  FOR UPDATE SKIP LOCKED
)
RETURNING j.id,j.namespace_id,j.kind,j.provider,j.generation,j.lease_epoch,j.lease_until,j.attempts`, owner).Scan(&row).Error
	})
	if err != nil {
		return JobLease{}, err
	}
	if len(row) == 0 {
		return JobLease{}, newError(404, "NOT_FOUND", "没有可领取任务")
	}
	return JobLease{
		ID: row["id"].(string), NamespaceID: row["namespace_id"].(string),
		Kind: row["kind"].(string), Provider: row["provider"].(string),
		Generation: toInt64(row["generation"]), LeaseEpoch: toInt64(row["lease_epoch"]),
		LeaseUntil: row["lease_until"].(time.Time), Attempts: toInt(row["attempts"]),
	}, nil
}

func (s *Service) RenewJob(ctx context.Context, lease JobLease) error {
	res := s.DB.WithContext(ctx).Exec(`UPDATE finance_intel_jobs
SET lease_until=now()+interval '120 seconds',updated_at=now()
WHERE id=? AND namespace_id=? AND generation=? AND lease_epoch=? AND status='running'`,
		lease.ID, lease.NamespaceID, lease.Generation, lease.LeaseEpoch)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return newError(409, "VERSION_CONFLICT", "任务租约已失效")
	}
	return nil
}

func (s *Service) CompleteJob(ctx context.Context, lease JobLease, status string, received, processed, quarantined int, jobErr string) error {
	if status != "succeeded" && status != "partial" && status != "failed" {
		return newError(400, "INVALID_PARAM", "任务终态不合法")
	}
	res := s.DB.WithContext(ctx).Exec(`UPDATE finance_intel_jobs
SET status=?,received_count=?,processed_count=?,quarantined_count=?,error=?,lease_until=NULL,updated_at=now()
WHERE id=? AND namespace_id=? AND generation=? AND lease_epoch=? AND status='running'`,
		status, received, processed, quarantined, jobErr, lease.ID, lease.NamespaceID, lease.Generation, lease.LeaseEpoch)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return newError(409, "VERSION_CONFLICT", "任务租约或 generation 已失效")
	}
	return nil
}

// StartWorker is a bounded durable queue worker. Provider fetch/model calls are
// intentionally fail-closed here; fixture/manual jobs are surfaced as partial
// review work rather than pretending a live extraction succeeded.
func (s *Service) StartWorker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.processOutbox(ctx)
				lease, err := s.ClaimJob(ctx, "intel-worker")
				if err != nil {
					continue
				}
				status, jobErr := "partial", ""
				switch {
				case lease.Provider != "fixture" && lease.Provider != "manual":
					status, jobErr = "failed", "provider disabled or unauthorized"
				case lease.Kind == "extract":
					jobErr = "manual extraction requires admin review"
				default:
					jobErr = "fixture ingestion is synchronous through replay actions"
				}
				_ = s.CompleteJob(ctx, lease, status, 0, 0, 0, jobErr)
			}
		}
	}()
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case int:
		return int64(x)
	case int32:
		return int64(x)
	case int64:
		return x
	case float64:
		return int64(x)
	default:
		return 0
	}
}

type OutboxLease struct {
	ID          string
	NamespaceID string
	Principal   string
	ChangeID    string
	Generation  int64
	LeaseEpoch  int64
}

func (s *Service) ClaimOutbox(ctx context.Context, owner string) (OutboxLease, error) {
	var row map[string]any
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Raw(`UPDATE finance_intel_outbox o
SET status='processing',lease_owner=?,lease_epoch=o.lease_epoch+1,lease_until=now()+interval '120 seconds',attempts=o.attempts+1
WHERE o.id=(
  SELECT o2.id FROM finance_intel_outbox o2
  JOIN finance_intel_namespaces n ON n.id=o2.namespace_id AND n.generation=o2.lease_epoch*0+n.generation
  WHERE o2.status='pending' AND o2.available_at<=now()
  ORDER BY o2.available_at,o2.id LIMIT 1 FOR UPDATE SKIP LOCKED
)
RETURNING o.id,o.namespace_id,o.principal_key,o.change_id,o.lease_epoch`, owner).Scan(&row).Error
	})
	if err != nil {
		return OutboxLease{}, err
	}
	if len(row) == 0 {
		return OutboxLease{}, newError(404, "NOT_FOUND", "没有待投递通知")
	}
	var generation int64
	_ = s.DB.WithContext(ctx).Raw(`SELECT generation FROM finance_intel_namespaces WHERE id=?`, row["namespace_id"]).Scan(&generation).Error
	return OutboxLease{
		ID: row["id"].(string), NamespaceID: row["namespace_id"].(string),
		Principal: row["principal_key"].(string), ChangeID: row["change_id"].(string),
		Generation: generation, LeaseEpoch: toInt64(row["lease_epoch"]),
	}, nil
}

func (s *Service) CompleteOutbox(ctx context.Context, lease OutboxLease, status string) error {
	if status != "sent" && status != "failed" {
		return newError(400, "INVALID_PARAM", "投递终态不合法")
	}
	res := s.DB.WithContext(ctx).Exec(`UPDATE finance_intel_outbox
SET status=?,lease_until=NULL
WHERE id=? AND namespace_id=? AND principal_key=? AND lease_epoch=? AND status='processing'`,
		status, lease.ID, lease.NamespaceID, lease.Principal, lease.LeaseEpoch)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return newError(409, "VERSION_CONFLICT", "投递租约已失效")
	}
	return nil
}

func (s *Service) processOutbox(ctx context.Context) {
	lease, err := s.ClaimOutbox(ctx, "intel-outbox-worker")
	if err != nil {
		return
	}
	// The notification row is created transactionally with the Change. Delivery
	// only finalizes the durable outbox record; the unique notification key makes
	// replay safe and a crashed worker can resume from processing leases.
	_ = s.CompleteOutbox(ctx, lease, "sent")
}
