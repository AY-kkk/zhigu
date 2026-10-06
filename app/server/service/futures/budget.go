package futures

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AccountScope string

const (
	UserAccount   AccountScope = "user"
	ModuleAccount AccountScope = "module"
)

var ErrQuotaExceeded = errors.New("futures: quota exceeded")

type Reservation struct {
	AttemptID     string
	OwnerID       uint
	RunID         string
	BudgetDay     time.Time
	AmountCNY     decimal.Decimal
	Tokens        int
	Kind          string
	MaxModelCalls int
	MaxToolCalls  int
	MaxTokens     int
}

type BudgetAccount struct {
	OwnerID        uint            `gorm:"column:owner_id;primaryKey"`
	Scope          AccountScope    `gorm:"column:scope;primaryKey"`
	BudgetDate     time.Time       `gorm:"column:budget_date;primaryKey"`
	LimitCNY       decimal.Decimal `gorm:"column:limit_cny"`
	ReservedCNY    decimal.Decimal `gorm:"column:reserved_cny"`
	SpentCNY       decimal.Decimal `gorm:"column:spent_cny"`
	ModelCalls     int             `gorm:"column:model_calls"`
	ToolCalls      int             `gorm:"column:tool_calls"`
	Tokens         int             `gorm:"column:tokens"`
	ModelCallLimit int             `gorm:"column:model_call_limit"`
	ToolCallLimit  int             `gorm:"column:tool_call_limit"`
	TokenLimit     int             `gorm:"column:token_limit"`
}

func (BudgetAccount) TableName() string { return "futures_budget_accounts" }

type BudgetReservation struct {
	AttemptID string          `gorm:"column:attempt_id;primaryKey"`
	OwnerID   uint            `gorm:"column:owner_id"`
	RunID     string          `gorm:"column:run_id"`
	BudgetDay time.Time       `gorm:"column:budget_date"`
	AmountCNY decimal.Decimal `gorm:"column:amount_cny"`
	Tokens    int             `gorm:"column:tokens"`
	Kind      string          `gorm:"column:kind"`
	State     string          `gorm:"column:state"`
	CreatedAt time.Time       `gorm:"column:created_at"`
}

func (BudgetReservation) TableName() string { return "futures_budget_reservations" }

type BudgetUsage struct {
	AttemptID    string    `gorm:"column:attempt_id;primaryKey"`
	OwnerID      uint      `gorm:"column:owner_id"`
	RunID        string    `gorm:"column:run_id"`
	InputTokens  *int      `gorm:"column:input_tokens"`
	OutputTokens *int      `gorm:"column:output_tokens"`
	UsageState   string    `gorm:"column:usage_state"`
	RecordedAt   time.Time `gorm:"column:recorded_at"`
}

func (BudgetUsage) TableName() string { return "futures_usage" }

type DBBudget struct{ DB *gorm.DB }

func NewDBBudget(db *gorm.DB) *DBBudget { return &DBBudget{DB: db} }

func (b *DBBudget) SeedAccount(ctx context.Context, scope AccountScope, ownerID uint, day time.Time, limit decimal.Decimal, modelCalls, toolCalls, tokens int) error {
	account := BudgetAccount{OwnerID: ownerID, Scope: scope, BudgetDate: day, LimitCNY: limit,
		ReservedCNY: decimal.Zero, SpentCNY: decimal.Zero,
		ModelCallLimit: modelCalls, ToolCallLimit: toolCalls, TokenLimit: tokens}
	return b.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(account).Error
}

func (b *DBBudget) Account(ctx context.Context, scope AccountScope, ownerID uint, day time.Time) (BudgetAccount, error) {
	var account BudgetAccount
	err := b.DB.WithContext(ctx).Where(`scope=? AND owner_id=? AND budget_date=?`, scope, ownerID, day).First(&account).Error
	return account, err
}

func (b *DBBudget) Reserve(ctx context.Context, request Reservation) (BudgetReservation, error) {
	out := BudgetReservation{}
	err := b.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		day := request.BudgetDay.UTC()
		userAccount, err := lockBudgetAccount(tx, UserAccount, request.OwnerID, day)
		if err != nil {
			return err
		}
		moduleAccount, err := lockBudgetAccount(tx, ModuleAccount, 0, day)
		if err != nil {
			return err
		}
		if userAccount.ReservedCNY.Add(userAccount.SpentCNY).Add(request.AmountCNY).GreaterThan(userAccount.LimitCNY) ||
			moduleAccount.ReservedCNY.Add(moduleAccount.SpentCNY).Add(request.AmountCNY).GreaterThan(moduleAccount.LimitCNY) ||
			(userAccount.TokenLimit > 0 && userAccount.Tokens+request.Tokens > userAccount.TokenLimit) ||
			(moduleAccount.TokenLimit > 0 && moduleAccount.Tokens+request.Tokens > moduleAccount.TokenLimit) {
			return ErrQuotaExceeded
		}
		if request.Kind == "model" && (userAccount.ModelCalls+1 > userAccount.ModelCallLimit || moduleAccount.ModelCalls+1 > moduleAccount.ModelCallLimit) {
			return ErrQuotaExceeded
		}
		if request.Kind == "tool" && (userAccount.ToolCalls+1 > userAccount.ToolCallLimit || moduleAccount.ToolCalls+1 > moduleAccount.ToolCallLimit) {
			return ErrQuotaExceeded
		}
		if err := addBudgetUsage(tx, UserAccount, request.OwnerID, day, request); err != nil {
			return err
		}
		if err := addBudgetUsage(tx, ModuleAccount, 0, day, request); err != nil {
			return err
		}
		out = BudgetReservation{AttemptID: request.AttemptID, OwnerID: request.OwnerID, RunID: request.RunID,
			BudgetDay: day, AmountCNY: request.AmountCNY, Tokens: request.Tokens, Kind: request.Kind, State: "reserved", CreatedAt: time.Now().UTC()}
		return tx.Create(out).Error
	})
	return out, err
}

func enforceRunLimits(tx *gorm.DB, request Reservation) error {
	if request.Kind != "model" && request.Kind != "tool" {
		return ErrInvalidInput
	}
	if (request.Kind == "model" && request.MaxModelCalls > 0) || (request.Kind == "tool" && request.MaxToolCalls > 0) {
		var calls int64
		if err := tx.Raw(`SELECT count(*) FROM futures_budget_reservations WHERE run_id=? AND kind=?`, request.RunID, request.Kind).Scan(&calls).Error; err != nil {
			return err
		}
		limit := request.MaxModelCalls
		if request.Kind == "tool" {
			limit = request.MaxToolCalls
		}
		if calls >= int64(limit) {
			return ErrQuotaExceeded
		}
	}
	if request.MaxTokens > 0 {
		var tokens int64
		if err := tx.Raw(`SELECT coalesce(sum(tokens),0) FROM futures_budget_reservations WHERE run_id=?`, request.RunID).Scan(&tokens).Error; err != nil {
			return err
		}
		if tokens+int64(request.Tokens) > int64(request.MaxTokens) {
			return ErrQuotaExceeded
		}
	}
	return nil
}

func lockBudgetAccount(tx *gorm.DB, scope AccountScope, ownerID uint, day time.Time) (BudgetAccount, error) {
	var account BudgetAccount
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where(`scope=? AND owner_id=? AND budget_date=?`, scope, ownerID, day).First(&account).Error
	return account, err
}

func addBudgetUsage(tx *gorm.DB, scope AccountScope, ownerID uint, day time.Time, request Reservation) error {
	updates := map[string]any{
		`reserved_cny`: gorm.Expr(`reserved_cny + ?`, request.AmountCNY),
		`tokens`:       gorm.Expr(`tokens + ?`, request.Tokens),
	}
	if request.Kind == "model" {
		updates[`model_calls`] = gorm.Expr(`model_calls + 1`)
	}
	if request.Kind == "tool" {
		updates[`tool_calls`] = gorm.Expr(`tool_calls + 1`)
	}
	return tx.Model(&BudgetAccount{}).Where(`scope=? AND owner_id=? AND budget_date=?`, scope, ownerID, day).Updates(updates).Error
}

func (b *DBBudget) SettleUnknown(ctx context.Context, ownerID uint, attemptID string, amount decimal.Decimal, tokens int) error {
	return b.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var reservation BudgetReservation
		if err := tx.Where(`attempt_id=? AND owner_id=?`, attemptID, ownerID).First(&reservation).Error; err != nil {
			return err
		}
		res := tx.Model(&BudgetReservation{}).
			Where(`attempt_id=? AND owner_id=? AND state='reserved'`, attemptID, ownerID).
			Update(`state`, `unknown`)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrNotFound
		}
		return tx.Create(BudgetUsage{AttemptID: attemptID, OwnerID: ownerID, RunID: reservation.RunID, UsageState: "unknown", RecordedAt: time.Now().UTC()}).Error
	})
}

func (b *DBBudget) SettleKnown(ctx context.Context, ownerID uint, attemptID string, amount decimal.Decimal, tokens int) error {
	return b.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var reservation BudgetReservation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(`attempt_id=? AND owner_id=?`, attemptID, ownerID).First(&reservation).Error; err != nil {
			return err
		}
		if reservation.State != "reserved" {
			return ErrRevisionConflict
		}
		if err := tx.Model(&BudgetReservation{}).Where(`attempt_id=?`, attemptID).Update(`state`, `settled`).Error; err != nil {
			return err
		}
		for _, scope := range []AccountScope{UserAccount, ModuleAccount} {
			id := ownerID
			if scope == ModuleAccount {
				id = 0
			}
			if err := tx.Model(&BudgetAccount{}).
				Where(`scope=? AND owner_id=? AND budget_date=?`, scope, id, reservation.BudgetDay).
				Updates(map[string]any{
					`reserved_cny`: gorm.Expr(`reserved_cny - ?`, reservation.AmountCNY),
					`spent_cny`:    gorm.Expr(`spent_cny + ?`, amount),
					`tokens`:       gorm.Expr(`tokens + ?`, tokens-reservation.Tokens),
				}).Error; err != nil {
				return err
			}
		}
		return tx.Create(BudgetUsage{AttemptID: attemptID, OwnerID: ownerID, RunID: reservation.RunID, UsageState: "known", RecordedAt: time.Now().UTC()}).Error
	})
}
