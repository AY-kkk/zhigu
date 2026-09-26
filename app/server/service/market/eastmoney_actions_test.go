package market

import (
	"testing"

	model "zhigu/server/model/market"
)

func TestParseActionsEmptyPayloadIsNotError(t *testing.T) {
	got, err := parseActions([]byte(`{"version":null,"result":null,"success":false,"message":"返回数据为空","code":9201}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestParseActionsGarbage(t *testing.T) {
	if _, err := parseActions([]byte(`<!doctype html>`)); err == nil {
		t.Fatal("want parse error")
	}
}

func TestParseActionsCashDividend(t *testing.T) {
	raw := []byte(`{"success":true,"result":{"data":[{
		"IMPL_PLAN_PROFILE":"10派3.00元(含税,扣税后2.70元)",
		"EX_DIVIDEND_DATE":"2026-06-04 00:00:00",
		"EQUITY_RECORD_DATE":"2026-06-03 00:00:00",
		"PRETAX_BONUS_RMB":3
	}]}}`)
	got, err := parseActions(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != "cash_dividend" || got[0].EffectiveAt != "2026-06-04" || got[0].CashAmount != "0.3" {
		t.Fatalf("%+v", got)
	}
	if got[0].AvailableAt == nil || *got[0].AvailableAt != "2026-06-04" {
		t.Fatalf("available %+v", got[0].AvailableAt)
	}
}

func TestRankNameHitsPrefersNewerListing(t *testing.T) {
	old, neu := "2015-12-01", "2024-04-18"
	rest := []model.Instrument{
		{InstrumentID: "835185.BJ", Name: "贝特瑞", ListingDate: &old},
		{InstrumentID: "920185.BJ", Name: "贝特瑞", ListingDate: &neu},
	}
	rankNameHits("贝特瑞", rest)
	if rest[0].InstrumentID != "920185.BJ" {
		t.Fatalf("%s", rest[0].InstrumentID)
	}
	legacy := []model.Instrument{
		{InstrumentID: "835185.BJ", Exchange: "BSE", Code: "835185", Name: "贝特瑞"},
		{InstrumentID: "920185.BJ", Exchange: "BSE", Code: "920185", Name: "贝特瑞"},
	}
	rankNameHits("贝特瑞", legacy)
	if legacy[0].InstrumentID != "920185.BJ" {
		t.Fatalf("legacy order %s", legacy[0].InstrumentID)
	}
}
