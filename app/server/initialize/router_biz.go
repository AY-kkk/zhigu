package initialize

// RegisterFinanceRouter is the GoSaaS router_biz.go drop-in name.
// The zhigu host wires routes in main.go to avoid an initialize↔finance import cycle.
func RegisterFinanceRouter() {}
