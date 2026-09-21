package market

import (
	"context"
	"strings"
)

func ListCninfoCatalog(ctx context.Context, httpc *HTTP) ([]SeedInstrument, error) {
	base, err := FetchCninfoBaseline(ctx, httpc)
	if err != nil {
		return nil, err
	}
	var out []SeedInstrument
	for mkt, rows := range base {
		for _, row := range rows {
			inst := SeedInstrument{
				InstrumentID: row.ID, SecurityID: "sec_" + row.Code, Exchange: row.Exchange,
				Board: BoardOf(row.Exchange, row.Code), AssetType: "stock", Code: row.Code,
				Name: row.Name, Currency: "CNY", Status: "listed", Lot: 100,
			}
			if mkt == "HKEX" {
				inst.Currency = "HKD"
				inst.Lot = 0
				if len(row.Code) == 5 && strings.HasPrefix(row.Code, "8") {
					inst.Currency = "CNY"
				}
			}
			out = append(out, inst)
		}
	}
	if len(out) < 5000 {
		return nil, jsonError{"巨潮目录覆盖不足"}
	}
	return out, nil
}
