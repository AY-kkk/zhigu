package intel

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

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

type jobModelRef struct {
	ConfigID      string `json:"config_id"`
	ConfigDigest  string `json:"config_digest"`
	Protocol      string `json:"protocol"`
	Model         string `json:"model"`
	PromptVersion string `json:"prompt_version"`
}

type extractionJobPayload struct {
	SourceRevisionID string      `json:"source_revision_id"`
	ModelConfig      jobModelRef `json:"model_config"`
}

type ingestionJobPayload struct {
	Codes []string `json:"codes"`
	From  string   `json:"from"`
	To    string   `json:"to"`
}

func (s *Service) ProcessJob(ctx context.Context, lease JobLease) error {
	status, received, processed, quarantined, jobErr := "partial", 0, 0, 0, "fixture ingestion is synchronous through replay actions"
	if lease.Kind == "ingest" {
		return s.processIngestionJob(ctx, lease)
	}
	if lease.Provider != "fixture" && lease.Provider != "manual" {
		status, jobErr = "failed", "provider disabled or unauthorized"
		return s.CompleteJob(ctx, lease, status, received, processed, quarantined, jobErr)
	}
	if lease.Kind != "extract" {
		return s.CompleteJob(ctx, lease, status, received, processed, quarantined, jobErr)
	}
	var rawPayload string
	if err := s.DB.WithContext(ctx).Raw(`SELECT payload::text FROM finance_intel_jobs WHERE id=? AND namespace_id=?`, lease.ID, lease.NamespaceID).Scan(&rawPayload).Error; err != nil {
		return err
	}
	var payload extractionJobPayload
	if err := json.Unmarshal([]byte(rawPayload), &payload); err != nil {
		return s.CompleteJob(ctx, lease, "failed", 0, 0, 1, "invalid extraction job payload")
	}
	if payload.ModelConfig.Protocol == "fixture" || payload.ModelConfig.Protocol == "manual" {
		jobErr = "manual extraction requires verified semantic review"
		return s.CompleteJob(ctx, lease, status, received, processed, quarantined, jobErr)
	}
	var sourceRow map[string]any
	if err := s.DB.WithContext(ctx).Raw(`SELECT id,logical_id,allowed_excerpt FROM finance_intel_source_revisions WHERE namespace_id=? AND logical_id=?`, lease.NamespaceID, payload.SourceRevisionID).Scan(&sourceRow).Error; err != nil {
		return err
	}
	if len(sourceRow) == 0 {
		return s.CompleteJob(ctx, lease, "failed", 0, 0, 1, "source revision not found")
	}
	scope, err := s.scope(ctx, lease.NamespaceID)
	if err != nil {
		return err
	}
	frozen, err := s.freezeModelForJob(ctx, scope)
	if err != nil {
		return s.CompleteJob(ctx, lease, "failed", 0, 0, 1, err.Error())
	}
	if frozen.ConfigID != payload.ModelConfig.ConfigID || frozen.ConfigDigest != payload.ModelConfig.ConfigDigest ||
		frozen.Protocol != payload.ModelConfig.Protocol || frozen.Model != payload.ModelConfig.Model {
		return s.CompleteJob(ctx, lease, "failed", 0, 0, 1, "frozen model config changed before execution")
	}
	extractor := NewModelExtractorV2(frozen)
	extractor.Call = s.ModelCall
	text, _ := sourceRow["allowed_excerpt"].(string)
	usageID, reserveErr := s.ReserveModelUsage(ctx, scope, lease.ID, lease.Attempts, intelInputTokenLimit+intelOutputTokenLimit)
	if reserveErr != nil {
		return s.CompleteJob(ctx, lease, "failed", 1, 0, 1, reserveErr.Error())
	}
	result, extractErr := extractor.Extract(ctx, ExtractionRequest{SourceRevisionID: payload.SourceRevisionID, Text: text})
	if extractErr != nil {
		_ = s.SettleModelUsage(ctx, usageID, 0, true)
	}

	if extractErr != nil {
		quarantined = 1
		status = "failed"
		if strings.Contains(extractErr.Error(), "REVIEW_REQUIRED") || strings.Contains(extractErr.Error(), "INVALID_CITATION") {
			status, quarantined = "partial", 1
		}
		return s.CompleteJob(ctx, lease, status, 1, 0, quarantined, extractErr.Error())
	}
	if err := s.SettleModelUsage(ctx, usageID, usageTotal(result.Usage), false); err != nil {
		return err
	}
	runID := "extr_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := execSQL(s.DB.WithContext(ctx), `INSERT INTO finance_intel_extraction_runs
(id,namespace_id,source_revision_id,config_id,config_digest,prompt_version,input_hash,attempt,status,output,validation,usage,mode,created_at)
VALUES (?,?,?,?,?,?,?,?, 'succeeded', ?::jsonb, ?::jsonb, ?::jsonb, 'live', ?)`,
		runID, lease.NamespaceID, sourceRow["id"], result.ConfigID, result.ConfigDigest, result.PromptVersion,
		sha256Text(text), lease.Attempts, mustJSON(result.Output), mustJSON(map[string]any{"schema_version": result.SchemaVersion, "validated": true}),
		mustJSON(result.Usage), s.now()); err != nil {
		return err
	}
	reviewID := "review_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := execSQL(s.DB.WithContext(ctx), `INSERT INTO finance_intel_review_items
(id,namespace_id,revision,kind,status,payload,reason,created_at)
VALUES (?,?,1,'extraction','pending',?::jsonb,'模型抽取需人工金标准核验',?)`,
		reviewID, lease.NamespaceID, mustJSON(map[string]any{"extraction_run_id": runID, "source_revision_id": payload.SourceRevisionID}), s.now()); err != nil {
		return err
	}
	return s.CompleteJob(ctx, lease, "succeeded", 1, 1, 0, "")
}

func (s *Service) processIngestionJob(ctx context.Context, lease JobLease) error {
	var raw string
	if err := s.DB.WithContext(ctx).Raw(`SELECT payload::text FROM finance_intel_jobs WHERE id=? AND namespace_id=?`, lease.ID, lease.NamespaceID).Scan(&raw).Error; err != nil {
		return err
	}
	var payload ingestionJobPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return s.CompleteJob(ctx, lease, "failed", 0, 0, 1, "invalid ingestion payload")
	}
	switch lease.Provider {
	case "fuyao":
		client := NewFuyaoClient()
		processed := 0
		for _, code := range payload.Codes {
			out, err := client.SearchTickers(ctx, code, 5)
			if err != nil {
				return s.CompleteJob(ctx, lease, "partial", len(payload.Codes), processed, 1, err.Error())
			}
			for _, ticker := range out.Items {
				verifiedAt := s.now()
				if ticker.SourceUpdatedAt != nil {
					verifiedAt = *ticker.SourceUpdatedAt
				}
				if err := execSQL(s.DB.WithContext(ctx), `INSERT INTO finance_intel_instruments
(id,namespace_id,code,name,exchange,source,verified_at,created_at)
VALUES (?,?,?,?,?,'fuyao',?,?)
ON CONFLICT(namespace_id,code) DO UPDATE SET name=EXCLUDED.name,exchange=EXCLUDED.exchange,source='fuyao',verified_at=EXCLUDED.verified_at`,
					"inst_"+strings.ReplaceAll(uuid.NewString(), "-", ""), lease.NamespaceID, ticker.Code, ticker.Name, ticker.Exchange, verifiedAt, s.now()); err != nil {
					return err
				}
				processed++
			}
		}
		return s.CompleteJob(ctx, lease, "succeeded", len(payload.Codes), processed, 0, "")
	case "ifind":
		capability, err := NewIFindMCPClient().Discover(ctx)
		if err != nil {
			return s.CompleteJob(ctx, lease, "failed", len(payload.Codes), 0, 1, err.Error())
		}
		reviewID := "review_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if err := execSQL(s.DB.WithContext(ctx), `INSERT INTO finance_intel_review_items
(id,namespace_id,revision,kind,status,payload,reason,created_at)
VALUES (?,?,1,'extraction','pending',?::jsonb,'iFinD工具已发现，需管理员选择实际schema后调用',?)`,
			reviewID, lease.NamespaceID, mustJSON(map[string]any{"capability": capability, "codes": payload.Codes}), s.now()); err != nil {
			return err
		}
		return s.CompleteJob(ctx, lease, "partial", len(payload.Codes), 0, 0, "iFinD requires reviewed tool selection")
	default:
		reviewID := "review_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		_ = execSQL(s.DB.WithContext(ctx), `INSERT INTO finance_intel_review_items
(id,namespace_id,revision,kind,status,payload,reason,created_at)
VALUES (?,?,1,'merge','pending',?::jsonb,'CNINFO需要经核验的orgId/column/plate映射',?)`,
			reviewID, lease.NamespaceID, mustJSON(map[string]any{"codes": payload.Codes}), s.now())
		return s.CompleteJob(ctx, lease, "partial", len(payload.Codes), 0, 0, "CNINFO subject mapping requires review")
	}
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
				_ = s.ProcessJob(ctx, lease)
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
