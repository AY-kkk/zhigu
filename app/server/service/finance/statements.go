package finance

type statementKind string

const (
	sheetIncome  statementKind = "income"
	sheetBalance statementKind = "balance"
	sheetCash    statementKind = "cash"

	MetricOperateProfit = "operating_profit"
	MetricNetIncome     = "net_income"
	MetricTotalAssets   = "total_assets"
	MetricTotalLiab     = "total_liabilities"
	MetricEquityParent  = "equity_parent"
	MetricICF           = "investing_cash_flow_net"
	MetricFCF           = "financing_cash_flow_net"
	MetricCashEnd       = "cash_and_equivalents"

	emIncome  = "RPT_F10_FINANCE_GINCOME"
	emBalance = "RPT_F10_FINANCE_GBALANCE"
	emCash    = "RPT_F10_FINANCE_GCASHFLOW"
	hkSummary = "RPT_CUSTOM_HKSK_APPFN_CASHFLOW_SUMMARY"
	hkIncome  = "RPT_HKF10_FN_INCOME_PC"
	hkBalance = "RPT_HKF10_FN_BALANCE_PC"
	hkCash    = "RPT_HKF10_FN_CASHFLOW_PC"
)

type MetricDef struct {
	Name    string
	Sheet   statementKind
	AField  string
	HKNames []string
	HKCodes []string
}

var MetricDefs = []MetricDef{
	{MetricRevenue, sheetIncome, "TOTAL_OPERATE_INCOME", []string{"营运收入", "营业额"}, []string{"004001999", "004001001"}},
	{MetricOperateProfit, sheetIncome, "OPERATE_PROFIT", []string{"经营溢利"}, []string{"004010999"}},
	{MetricNetIncome, sheetIncome, "NETPROFIT", []string{"除税后溢利", "持续经营业务税后利润"}, []string{"004012999", "004012002"}},
	{MetricNetParent, sheetIncome, "PARENT_NETPROFIT", []string{"股东应占溢利"}, []string{"004025002"}},
	{MetricTotalAssets, sheetBalance, "TOTAL_ASSETS", []string{"总资产"}, []string{"004009999"}},
	{MetricTotalLiab, sheetBalance, "TOTAL_LIABILITIES", []string{"总负债"}, []string{"004025999"}},
	{MetricEquityParent, sheetBalance, "TOTAL_PARENT_EQUITY", []string{"股东权益"}, []string{"004030999"}},
	{MetricOCF, sheetCash, "NETCASH_OPERATE", []string{"经营业务现金净额"}, []string{"003999"}},
	{MetricICF, sheetCash, "NETCASH_INVEST", []string{"投资业务现金净额"}, []string{"005999"}},
	{MetricFCF, sheetCash, "NETCASH_FINANCE", []string{"融资业务现金净额"}, []string{"007999"}},
	{MetricCashEnd, sheetCash, "END_CASH", []string{"期末现金"}, []string{"011999"}},
}

var metricByName = map[string]MetricDef{}

func init() {
	FrozenMetrics = make([]string, 0, len(MetricDefs))
	for _, def := range MetricDefs {
		metricByName[def.Name] = def
		FrozenMetrics = append(FrozenMetrics, def.Name)
	}
}

func metricDef(name string) (MetricDef, bool) {
	def, ok := metricByName[name]
	return def, ok
}

func sheetsNeeded(metrics []string) map[statementKind]struct{} {
	out := map[statementKind]struct{}{}
	for _, name := range metrics {
		if def, ok := metricDef(name); ok {
			out[def.Sheet] = struct{}{}
		}
	}
	return out
}
