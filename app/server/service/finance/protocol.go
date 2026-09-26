package finance

import (
	"os"
	"strings"
	"time"
)

const (
	ContractVersionHeader = "X-Zhigu-Contract-Version"
	ContractVersion       = "2"
	ModelProtocolHeader   = "X-Zhigu-Model-Protocol"
	ProductUserAgent      = "Zhigu/0.1 (private research prototype; contact: local-dev@zhigu.invalid)"

	ProtocolChatCompletions = "openai_chat_completions"
	ProtocolResponses       = "openai_responses"
	ProtocolChatLegacy      = "openai-chat-completions"

	SourcePolicyVersion = "source_eastmoney_cninfo_v2"
	DataVersion         = "eastmoney_hsf10_hkf10_cninfo_v1"
	ConnectorModeGoHTTP = "go_http"

	MetricRevenue   = "revenue"
	MetricNetParent = "net_income_parent"
	MetricOCF       = "operating_cash_flow_net"

	FinancialHTTPLimit = 6
	FilingHTTPLimit    = 6
	ToolTimeout        = 15 * time.Second
)

// ResearchScopePrefix is injected by Go in front of system prompts. Do not put it on ResearchTask.
const ResearchScopePrefix = "研究口径（服务端冻结）：\n市场=本次确认标的所属市场（A股或港股）\n记账币种=报表披露币种（A股默认CNY；港股以源字段为准）\n会计准则=A股中国企业会计准则；港股国际财务报告准则或香港财务报告准则\n报表=已披露年度利润表、资产负债表、现金流量表\n缺字段记不足，禁止当作0。\n"

func DataMode() string {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("ZHIGU_DATA_MODE")))
	if v == ModeLive {
		return ModeLive
	}
	return ModeFixture
}

func NormalizeProtocol(raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "", ProtocolChatCompletions, ProtocolChatLegacy:
		return ProtocolChatCompletions, nil
	case ProtocolResponses:
		return ProtocolResponses, nil
	default:
		return "", NewError(409, "conflict", "MODEL_PROTOCOL_MISMATCH", "协议未确认或不受支持")
	}
}

func ModelAlias() string { return "finance-research" }
