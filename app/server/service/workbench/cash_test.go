package workbench

import (
	"testing"

	"github.com/shopspring/decimal"

	"zhigu/server/service/backtest"
	"zhigu/server/service/indicators"
	"zhigu/server/service/market"
)

func TestRaiseCashForOneLot(t *testing.T) {
	cfg := backtest.Config{InitialCash: "100000", Start: "2020-01-02"}
	bars := []indicators.Bar{{Time: "2020-01-02", Close: decimal.RequireFromString("1252.57")}}
	note := raiseCashForOneLot(&cfg, bars, 100)
	if note == "" {
		t.Fatal("want raise note")
	}
	if market.MustDec(cfg.InitialCash).LessThan(decimal.RequireFromString("250514")) {
		t.Fatalf("cash %s", cfg.InitialCash)
	}
	keep := backtest.Config{InitialCash: "100000"}
	cheap := []indicators.Bar{{Time: "2020-01-02", Close: decimal.RequireFromString("430")}}
	if n := raiseCashForOneLot(&keep, cheap, 100); n != "" {
		t.Fatalf("should keep cash: %s %s", n, keep.InitialCash)
	}
}
