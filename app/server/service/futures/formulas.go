package futures

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

type Formula string

const (
	FormulaBasis           Formula = "basis"
	FormulaBasisRate       Formula = "basis_rate"
	FormulaCalendarSpread  Formula = "calendar_spread"
	FormulaInventoryChange Formula = "inventory_change"
	FormulaInventoryRate   Formula = "inventory_change_rate"
	FormulaCoverage        Formula = "coverage"
)

const FormulaVersion = "futures.formula.v1"

type FormulaInput struct {
	RecordID     string
	Role         string
	Value        decimal.Decimal
	Unit         string
	Currency     string
	CaliberID    string
	SeriesID     string
	ContractKind string
	TradingDay   string
	ExpiryAt     time.Time
	PeriodStart  string
	PeriodEnd    string
}

type FormulaResult struct {
	Formula        Formula
	FormulaVersion string
	InputRecordIDs []string
	Value          decimal.Decimal
	Unit           string
}

func Calculate(formula Formula, inputs []FormulaInput) (FormulaResult, error) {
	result := FormulaResult{Formula: formula, FormulaVersion: FormulaVersion}
	if err := comparable(inputs); err != nil {
		return FormulaResult{}, err
	}
	roles := map[string]FormulaInput{}
	for _, input := range inputs {
		if input.Role == "" {
			return FormulaResult{}, fmt.Errorf("%w: formula inputs need explicit roles", ErrInvalidInput)
		}
		if _, exists := roles[input.Role]; exists {
			return FormulaResult{}, fmt.Errorf("%w: duplicate formula role", ErrInvalidInput)
		}
		roles[input.Role] = input
	}
	appendRole := func(role string) {
		result.InputRecordIDs = append(result.InputRecordIDs, roles[role].RecordID)
	}
	switch formula {
	case FormulaBasis, FormulaBasisRate:
		spot, futures := roles["spot"], roles["futures"]
		if spot.Role == "" || futures.Role == "" || spot.ContractKind != "spot" || futures.ContractKind != "actual" {
			return FormulaResult{}, fmt.Errorf("%w: basis requires spot and actual futures", ErrInvalidInput)
		}
		appendRole("spot")
		appendRole("futures")
		result.Value = spot.Value.Sub(futures.Value)
		result.Unit = spot.Unit
		if formula == FormulaBasisRate {
			if !spot.Value.IsPositive() {
				return FormulaResult{}, fmt.Errorf("%w: basis_rate denominator must be positive", ErrInvalidInput)
			}
			result.Value = result.Value.Div(spot.Value).Mul(decimal.NewFromInt(100))
			result.Unit = "%"
		}
	case FormulaCalendarSpread:
		near, far := roles["near"], roles["far"]
		if near.Role == "" || far.Role == "" || near.ContractKind != "actual" || far.ContractKind != "actual" ||
			near.ExpiryAt.IsZero() || far.ExpiryAt.IsZero() || !near.ExpiryAt.Before(far.ExpiryAt) {
			return FormulaResult{}, fmt.Errorf("%w: calendar_spread requires ordered actual contracts", ErrInvalidInput)
		}
		appendRole("near")
		appendRole("far")
		result.Value, result.Unit = near.Value.Sub(far.Value), near.Unit
	case FormulaInventoryChange, FormulaInventoryRate:
		current, previous := roles["current"], roles["previous"]
		if current.Role == "" || previous.Role == "" || current.SeriesID == "" || current.SeriesID != previous.SeriesID ||
			current.PeriodStart == "" || previous.PeriodStart == "" || !periodAfter(current, previous) {
			return FormulaResult{}, fmt.Errorf("%w: inventory change requires consecutive series periods", ErrInvalidInput)
		}
		appendRole("current")
		appendRole("previous")
		result.Value, result.Unit = current.Value.Sub(previous.Value), current.Unit
		if formula == FormulaInventoryRate {
			if !previous.Value.IsPositive() {
				return FormulaResult{}, fmt.Errorf("%w: inventory rate denominator must be positive", ErrInvalidInput)
			}
			result.Value = result.Value.Div(previous.Value).Mul(decimal.NewFromInt(100))
			result.Unit = "%"
		}
	case FormulaCoverage:
		available, required := roles["available"], roles["required"]
		if available.Role == "" || required.Role == "" || available.PeriodStart != required.PeriodStart || available.PeriodEnd != required.PeriodEnd {
			return FormulaResult{}, fmt.Errorf("%w: coverage requires aligned periods", ErrInvalidInput)
		}
		appendRole("available")
		appendRole("required")
		if !required.Value.IsPositive() {
			return FormulaResult{}, fmt.Errorf("%w: coverage denominator must be positive", ErrInvalidInput)
		}
		result.Value = available.Value.Div(required.Value).Mul(decimal.NewFromInt(100))
		result.Unit = "%"
	default:
		return FormulaResult{}, fmt.Errorf("%w: unsupported formula", ErrInvalidInput)
	}
	return result, nil
}

func comparable(inputs []FormulaInput) error {
	if len(inputs) < 2 {
		return fmt.Errorf("%w: formula needs at least two records", ErrInvalidInput)
	}
	for _, input := range inputs {
		if input.Unit == "" || input.Currency == "" || input.CaliberID == "" || input.TradingDay == "" {
			return fmt.Errorf("%w: formula inputs need unit, currency, caliber, and trading day", ErrInvalidInput)
		}
		if input.Unit != inputs[0].Unit || input.Currency != inputs[0].Currency || input.CaliberID != inputs[0].CaliberID || input.TradingDay != inputs[0].TradingDay {
			return fmt.Errorf("%w: formula inputs are not comparable", ErrInvalidInput)
		}
	}
	return nil
}

func periodAfter(current, previous FormulaInput) bool {
	currentStart, err1 := time.Parse("2006-01-02", current.PeriodStart)
	previousStart, err2 := time.Parse("2006-01-02", previous.PeriodStart)
	return err1 == nil && err2 == nil && currentStart.After(previousStart)
}
