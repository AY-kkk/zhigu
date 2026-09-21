package market

import "testing"

func TestParseGtimgQuotesAShareAndHK(t *testing.T) {
	raw := []byte(`v_sz000001="51~x~000001~11.73~11.70~11.70";
v_hk00700="100~y~00700~430.000~419.000~424.600";`)
	got, err := parseGtimgQuotes(raw, map[string]SeedInstrument{
		"000001.SZ": {InstrumentID: "000001.SZ", Exchange: "SZSE", Code: "000001", Name: "平安银行"},
		"00700.HK":  {InstrumentID: "00700.HK", Exchange: "HKEX", Code: "00700", Name: "腾讯控股"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["000001.SZ"].Last != "11.73" || got["000001.SZ"].Change != "0.03" || got["00700.HK"].Last != "430.000" || got["00700.HK"].ChangePct != "2.63" {
		t.Fatalf("%+v", got)
	}
}

func TestParseGtimgBarsAShareVolumeLots(t *testing.T) {
	raw := []byte(`{"code":0,"msg":"","data":{"sh600519":{"qfqday":[["2026-09-21","1259.00","1252.57","1259.95","1250.80","25017"]]}}}`)
	inst := SeedInstrument{Exchange: "SSE", Code: "600519", InstrumentID: "600519.SH"}
	got, err := parseGtimgBars(raw, inst, "qfq", "", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Close != "1252.57" || got[0].High != "1259.95" || got[0].Volume != "2501700" {
		t.Fatalf("%+v", got)
	}
}

func TestParseGtimgBarsHKShares(t *testing.T) {
	raw := []byte(`{"code":0,"data":{"hk00700":{"day":[["2026-09-21","424.6","430.0","434.8","423.0","12345"]]}}}`)
	inst := SeedInstrument{Exchange: "HKEX", Code: "00700", InstrumentID: "00700.HK"}
	got, err := parseGtimgBars(raw, inst, "", "", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Open != "424.6" || got[0].Volume != "12345" {
		t.Fatalf("%+v", got)
	}
}

func TestParseSinaBarsUsesShares(t *testing.T) {
	raw := []byte(`[{"day":"2026-09-21","open":"19.810","high":"20.100","low":"19.630","close":"20.090","volume":"3202796"}]`)
	inst := SeedInstrument{Exchange: "BSE", Code: "920185", InstrumentID: "920185.BJ"}
	got, err := parseSinaBars(raw, inst, "", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].High != "20.100" || got[0].Volume != "3202796" {
		t.Fatalf("%+v", got)
	}
}
