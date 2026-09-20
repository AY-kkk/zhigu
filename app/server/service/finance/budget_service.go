package finance

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	modelfinance "zhigu/server/model/finance"
)

type DBBudget struct {
	DB *gorm.DB
	mu sync.Mutex
}

func NewDBBudget(db *gorm.DB) *DBBudget { return &DBBudget{DB: db} }

func (b *DBBudget) Reserve(ctx context.Context, request BudgetRequest) (Reservation, error) {
	if request.RequestID == "" {
		return Reservation{}, NewError(400, "validation", "MISSING_REQUEST_ID", "缺少 request_id")
	}
	var out Reservation
	err := b.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("singleton_id = ?", "global").Take(&modelfinance.Scheduler{}).Error; err != nil {
			return err
		}
		var existing modelfinance.UsageLedger
		q := tx.Where("request_id = ?", request.RequestID).Take(&existing)
		if q.Error == nil {
			out = Reservation{ID: existing.ID, RequestID: existing.RequestID, Status: "reserved", ReservedInput: request.ReservedInput, ReservedOutput: request.ReservedOutput}
			return nil
		}
		if q.Error != nil && !errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return q.Error
		}
		snap := DefaultBudgetSnapshot()
		if request.RunID != "" {
			var run modelfinance.ResearchRun
			if err := tx.Where("id = ?", request.RunID).Take(&run).Error; err == nil {
				snap = ParseBudgetSnapshot(run.BudgetSnapshot)
			}
		}
		runMax := snap.MaxModelCalls
		roleMax := snap.MaxModelCallsPerRole
		if request.Kind == "tool" {
			runMax = snap.MaxToolCalls
			roleMax = snap.MaxToolCallsPerRole
		}
		scope := tx.Model(&modelfinance.UsageLedger{}).Where("kind = ? AND status IN ?", request.Kind, []string{"reserved", "succeeded", "unknown"})
		if request.RunID != "" {
			scope = scope.Where("run_id = ?", request.RunID)
		} else if request.ParseID != "" {
			scope = scope.Where("parse_id = ?", request.ParseID)
		}
		var used int64
		if err := scope.Count(&used).Error; err != nil {
			return err
		}
		if used >= int64(runMax) {
			return NewError(429, "budget", "BUDGET_EXCEEDED", "预算不足")
		}
		if request.TaskID != "" {
			var roleUsed int64
			roleScope := tx.Model(&modelfinance.UsageLedger{}).Where("kind = ? AND task_id = ? AND status IN ?", request.Kind, request.TaskID, []string{"reserved", "succeeded", "unknown"})
			if err := roleScope.Count(&roleUsed).Error; err != nil {
				return err
			}
			if roleUsed >= int64(roleMax) {
				return NewError(429, "budget", "BUDGET_EXCEEDED", "角色预算不足")
			}
		}
		now := time.Now().UTC()
		row := modelfinance.UsageLedger{
			ID:             uuid.NewString(),
			OwnerID:        request.OwnerID,
			RequestID:      request.RequestID,
			Purpose:        request.Purpose,
			Kind:           request.Kind,
			Status:         "reserved",
			ReservedTokens: request.ReservedInput + request.ReservedOutput,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if request.RunID != "" {
			row.RunID = &request.RunID
		}
		if request.TaskID != "" {
			row.TaskID = &request.TaskID
		}
		if request.ParseID != "" {
			row.ParseID = &request.ParseID
		}
		if request.QuestionID != "" {
			row.QuestionID = &request.QuestionID
		}
		if err := tx.Create(&row).Error; err != nil {
			if isUnique(err) {
				var again modelfinance.UsageLedger
				if tx.Where("request_id = ?", request.RequestID).Take(&again).Error == nil {
					out = Reservation{ID: again.ID, RequestID: again.RequestID, Status: "reserved"}
					return nil
				}
			}
			return err
		}
		out = Reservation{ID: row.ID, RequestID: row.RequestID, Status: "reserved", ReservedInput: request.ReservedInput, ReservedOutput: request.ReservedOutput}
		return nil
	})
	return out, err
}

func (b *DBBudget) Reconcile(ctx context.Context, reservationID string, usage ObservedUsage) error {
	return b.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row modelfinance.UsageLedger
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", reservationID).Take(&row).Error; err != nil {
			return NewError(404, "not_found", "RESERVATION_NOT_FOUND", "预占不存在")
		}
		now := time.Now().UTC()
		row.UpdatedAt = now
		if usage.Unknown {
			row.Status = "unknown"
		} else {
			row.Status = "succeeded"
			if usage.ActualInput != nil && usage.ActualOutput != nil {
				total := *usage.ActualInput + *usage.ActualOutput
				row.ActualTokens = &total
			}
		}
		if usage.UpstreamRequestID != "" {
			row.UpstreamRequestID = &usage.UpstreamRequestID
		}
		return tx.Save(&row).Error
	})
}

func (b *DBBudget) Ledger(requestID string) *modelfinance.UsageLedger {
	var row modelfinance.UsageLedger
	if err := b.DB.Where("request_id = ?", requestID).Take(&row).Error; err != nil {
		return nil
	}
	return &row
}

func DefaultPublicConfig() datatypes.JSON {
	raw, _ := json.Marshal(map[string]any{
		"protocol": "openai-chat-completions",
		"base_url": "https://api.openai.com/v1",
		"model":    "fixture-model",
		"purpose":  []string{"parser", "synthesizer", "verifier", "research"},
	})
	return datatypes.JSON(raw)
}
