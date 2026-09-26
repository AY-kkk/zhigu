package finance

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	modelfinance "zhigu/server/model/finance"
)

type MemoryBudget struct {
	mu        sync.Mutex
	remaining map[string]int
	rows      map[string]*modelfinance.UsageLedger
	db        *gorm.DB
}

func NewMemoryBudget() *MemoryBudget {
	return &MemoryBudget{
		remaining: map[string]int{"model:run": 14, "tool:run": 12},
		rows:      map[string]*modelfinance.UsageLedger{},
	}
}

func (b *MemoryBudget) Attach(db *gorm.DB) { b.db = db }

func (b *MemoryBudget) Reserve(ctx context.Context, request BudgetRequest) (Reservation, error) {
	_ = ctx
	if request.RequestID == "" {
		return Reservation{}, NewError(400, "validation", "MISSING_REQUEST_ID", "缺少 request_id")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if existing, ok := b.rows[request.RequestID]; ok {
		return Reservation{ID: existing.ID, RequestID: existing.RequestID, Status: "reserved", ReservedInput: request.ReservedInput, ReservedOutput: request.ReservedOutput}, nil
	}
	key := request.Kind + ":" + request.RunID
	if request.RunID == "" {
		key = request.Kind + ":parse:" + request.ParseID
	}
	snap := DefaultBudgetSnapshot()
	if request.RunID != "" && b.db != nil {
		var run modelfinance.ResearchRun
		if err := b.db.Where("id = ?", request.RunID).Take(&run).Error; err == nil {
			snap = ParseBudgetSnapshot(run.BudgetSnapshot)
		}
	}
	limit := snap.MaxModelCalls
	roleLimit := snap.MaxModelCallsPerRole
	if request.Kind == "tool" {
		limit = snap.MaxToolCalls
		roleLimit = snap.MaxToolCallsPerRole
	}
	if _, ok := b.remaining[key]; !ok {
		b.remaining[key] = limit
	}
	if b.remaining[key] <= 0 {
		return Reservation{}, NewError(429, "budget", "BUDGET_EXCEEDED", "预算不足")
	}
	if request.TaskID != "" {
		roleKey := request.Kind + ":task:" + request.TaskID
		if _, ok := b.remaining[roleKey]; !ok {
			b.remaining[roleKey] = roleLimit
		}
		if b.remaining[roleKey] <= 0 {
			return Reservation{}, NewError(429, "budget", "BUDGET_EXCEEDED", "角色预算不足")
		}
		b.remaining[roleKey]--
	}
	b.remaining[key]--
	id := uuid.NewString()
	tokens := request.ReservedInput + request.ReservedOutput
	row := &modelfinance.UsageLedger{
		ID:             id,
		OwnerID:        request.OwnerID,
		RequestID:      request.RequestID,
		Purpose:        request.Purpose,
		Kind:           request.Kind,
		Status:         "reserved",
		ReservedTokens: tokens,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
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
	b.rows[request.RequestID] = row
	if b.db != nil {
		if err := b.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(row).Error; err != nil {
			return Reservation{}, err
		}
	}
	return Reservation{ID: id, RequestID: request.RequestID, Status: "reserved", ReservedInput: request.ReservedInput, ReservedOutput: request.ReservedOutput}, nil
}

func (b *MemoryBudget) Reconcile(ctx context.Context, reservationID string, usage ObservedUsage) error {
	_ = ctx
	b.mu.Lock()
	defer b.mu.Unlock()
	var row *modelfinance.UsageLedger
	for _, r := range b.rows {
		if r.ID == reservationID {
			row = r
			break
		}
	}
	if row == nil {
		return NewError(404, "not_found", "RESERVATION_NOT_FOUND", "预占不存在")
	}
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
	row.UpdatedAt = time.Now().UTC()
	if b.db != nil {
		return b.db.Save(row).Error
	}
	return nil
}

func (b *MemoryBudget) Ledger(requestID string) *modelfinance.UsageLedger {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.rows[requestID]
}
