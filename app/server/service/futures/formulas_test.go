package futures

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestFuturesFormulasUseDecimalAndRejectIncomparableInputs(t *testing.T) {
	basis, err := Calculate(FormulaBasis, []FormulaInput{{Role: "spot", ContractKind: "spot", Value: decimal.RequireFromString("80000"), Unit: "CNY/t", Currency: "CNY", CaliberID: "standard", TradingDay: "2026-10-05"}, {Role: "futures", ContractKind: "actual", Value: decimal.RequireFromString("79000"), Unit: "CNY/t", Currency: "CNY", CaliberID: "standard", TradingDay: "2026-10-05"}})
	if err != nil || !basis.Value.Equal(decimal.RequireFromString("1000")) {
		t.Fatalf("basis=%v err=%v", basis, err)
	}
	rate, err := Calculate(FormulaBasisRate, []FormulaInput{{Role: "spot", ContractKind: "spot", Value: decimal.RequireFromString("80000"), Unit: "CNY/t", Currency: "CNY", CaliberID: "standard", TradingDay: "2026-10-05"}, {Role: "futures", ContractKind: "actual", Value: decimal.RequireFromString("79000"), Unit: "CNY/t", Currency: "CNY", CaliberID: "standard", TradingDay: "2026-10-05"}})
	if err != nil || !rate.Value.Equal(decimal.RequireFromString("1.25")) {
		t.Fatalf("rate=%v err=%v", rate, err)
	}
	spread, err := Calculate(FormulaCalendarSpread, []FormulaInput{{Role: "near", ContractKind: "actual", ExpiryAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), Value: decimal.RequireFromString("79000"), Unit: "CNY/t", Currency: "CNY", CaliberID: "standard", TradingDay: "2026-10-05"}, {Role: "far", ContractKind: "actual", ExpiryAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), Value: decimal.RequireFromString("79500"), Unit: "CNY/t", Currency: "CNY", CaliberID: "standard", TradingDay: "2026-10-05"}})
	if err != nil || !spread.Value.Equal(decimal.RequireFromString("-500")) {
		t.Fatalf("spread=%v err=%v", spread, err)
	}
	if _, err := Calculate(FormulaBasis, []FormulaInput{{Role: "spot", ContractKind: "spot", Value: decimal.RequireFromString("80000"), Unit: "CNY/t", Currency: "CNY", CaliberID: "standard", TradingDay: "2026-10-05"}, {Role: "futures", ContractKind: "actual", Value: decimal.RequireFromString("79000"), Unit: "USD/t", Currency: "USD", CaliberID: "standard", TradingDay: "2026-10-05"}}); err == nil {
		t.Fatal("different units must be rejected")
	}
	if _, err := Calculate(FormulaBasis, []FormulaInput{{Role: "spot", ContractKind: "spot", Value: decimal.RequireFromString("80000"), Unit: "CNY/t", Currency: "CNY", CaliberID: "standard", TradingDay: "2026-10-05"}, {Role: "futures", ContractKind: "actual", Value: decimal.RequireFromString("79000"), Unit: "CNY/t", Currency: "CNY", CaliberID: "standard", TradingDay: "2026-10-04"}}); err == nil {
		t.Fatal("different dates must be rejected")
	}
}

func TestFormulaRolesMakeInputOrderAndTradingDayStable(t *testing.T) {
	basis, err := Calculate(FormulaBasis, []FormulaInput{
		{RecordID: "late-futures", Role: "futures", ContractKind: "actual", ExpiryAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), Value: decimal.RequireFromString("79500"), Unit: "CNY/t", Currency: "CNY", CaliberID: "standard", TradingDay: "2026-10-05"},
		{RecordID: "early-spot", Role: "spot", ContractKind: "spot", Value: decimal.RequireFromString("80000"), Unit: "CNY/t", Currency: "CNY", CaliberID: "standard", TradingDay: "2026-10-05"},
	})
	if err != nil || !basis.Value.Equal(decimal.RequireFromString("500")) || basis.InputRecordIDs[0] != "early-spot" {
		t.Fatalf("role ordering failed: %+v err=%v", basis, err)
	}
	_, err = Calculate(FormulaBasis, []FormulaInput{
		{RecordID: "spot", Role: "spot", ContractKind: "spot", Value: decimal.RequireFromString("80000"), Unit: "CNY/t", Currency: "CNY", CaliberID: "standard", TradingDay: "2026-10-05"},
		{RecordID: "futures", Role: "futures", ContractKind: "actual", ExpiryAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), Value: decimal.RequireFromString("79500"), Unit: "CNY/t", Currency: "CNY", CaliberID: "standard", TradingDay: "2026-10-06"},
	})
	if err == nil {
		t.Fatal("different trading days must be rejected")
	}
}
