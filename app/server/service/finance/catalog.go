package finance

import (
	"strings"
	"unicode"
)

type ListedInstrument struct {
	ID       string `json:"instrument_id"`
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Industry string `json:"industry,omitempty"`
	Market   string `json:"market,omitempty"`
	OrgID    string `json:"-"`
	Column   string `json:"-"`
	Plate    string `json:"-"`
	Pinyin   string `json:"-"`
}

var FrozenLiveInstruments = []ListedInstrument{
	{ID: "600519.SH", Symbol: "600519", Name: "贵州茅台", Industry: "白酒", Market: MarketA, OrgID: "gssh0600519", Column: "sse", Plate: "sh"},
	{ID: "300750.SZ", Symbol: "300750", Name: "宁德时代", Industry: "制造/电池", Market: MarketA, OrgID: "GD165627", Column: "szse", Plate: "sz"},
	{ID: "000333.SZ", Symbol: "000333", Name: "美的集团", Industry: "家电/制造", Market: MarketA, OrgID: "9900005965", Column: "szse", Plate: "sz"},
}

var FrozenMetrics []string

const (
	MarketA         = "A"
	MarketHK        = "HK"
	CatalogLimit    = 20
	cninfoAListURL  = "http://www.cninfo.com.cn/new/data/szse_stock.json"
	cninfoHKListURL = "http://www.cninfo.com.cn/new/data/hke_stock.json"
)

func DemoInstrument() ListedInstrument {
	return ListedInstrument{ID: InstrumentDemo, Symbol: "DEMO:COMPANY", Name: "演示公司", Industry: "fixture", Market: MarketA}
}

func CatalogForMode(mode string) []ListedInstrument {
	if mode == ModeLive {
		out := make([]ListedInstrument, len(FrozenLiveInstruments))
		copy(out, FrozenLiveInstruments)
		return out
	}
	return []ListedInstrument{DemoInstrument()}
}

func LookupInstrument(id string) (ListedInstrument, bool) {
	id = NormalizeInstrumentID(id)
	if id == InstrumentDemo {
		return DemoInstrument(), true
	}
	for _, row := range FrozenLiveInstruments {
		if row.ID == id {
			return row, true
		}
	}
	return liveLookup(id)
}

func CoveredInstrument(id string) bool {
	return CoveredInstrumentMode(id, DataMode())
}

func CoveredInstrumentMode(id, mode string) bool {
	id = NormalizeInstrumentID(id)
	if mode != ModeLive {
		return id == InstrumentDemo
	}
	if id == InstrumentDemo {
		return false
	}
	if _, ok := LookupInstrument(id); ok {
		return true
	}
	_ = EnsureLiveCatalog(backgroundCatalogCtx())
	_, ok := LookupInstrument(id)
	return ok
}

func SecurityCode(instrumentID string) string {
	row, ok := LookupInstrument(instrumentID)
	if !ok {
		return ""
	}
	return row.Symbol
}

func LiveInstrumentIDs() []string {
	out := make([]string, 0, len(FrozenLiveInstruments))
	for _, row := range FrozenLiveInstruments {
		out = append(out, row.ID)
	}
	return out
}

func SearchCatalog(mode, q, market string, limit int) []ListedInstrument {
	if limit <= 0 || limit > CatalogLimit {
		limit = CatalogLimit
	}
	q = strings.TrimSpace(q)
	market = strings.ToUpper(strings.TrimSpace(market))
	if mode != ModeLive {
		demo := DemoInstrument()
		if q == "" || catalogTextHit(demo, q) {
			return []ListedInstrument{demo}
		}
		return nil
	}
	if q == "" {
		return CatalogForMode(ModeLive)
	}
	_ = EnsureLiveCatalog(backgroundCatalogCtx())
	if exact, ok := LookupInstrument(q); ok {
		if market == "" || exact.Market == market {
			return []ListedInstrument{exact}
		}
	}
	return liveSearch(q, market, limit)
}

func MatchInstrumentsFromText(text string, limit int) []ListedInstrument {
	if limit <= 0 || limit > 8 {
		limit = 8
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []ListedInstrument
	add := func(row ListedInstrument) {
		if _, ok := seen[row.ID]; ok {
			return
		}
		seen[row.ID] = struct{}{}
		out = append(out, row)
	}
	upper := strings.ToUpper(text)
	for _, cand := range extractInstrumentTokens(upper) {
		if row, ok := LookupInstrument(cand); ok && row.ID != InstrumentDemo {
			add(row)
		}
	}
	if len(out) >= limit {
		return out[:limit]
	}
	for _, row := range catalogSearchPool() {
		if len(out) >= limit {
			break
		}
		name := strings.TrimSpace(row.Name)
		if runeLen(name) < 3 {
			continue
		}
		if strings.Contains(text, name) {
			add(row)
		}
	}
	if len(out) > limit {
		return out[:limit]
	}
	return out
}

func NormalizeInstrumentID(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, " ", "")
	if s == "" || s == InstrumentDemo {
		return s
	}
	if strings.HasSuffix(s, ".HK") {
		return padHK(strings.TrimSuffix(s, ".HK")) + ".HK"
	}
	if strings.HasSuffix(s, ".SH") || strings.HasSuffix(s, ".SZ") || strings.HasSuffix(s, ".BJ") {
		return s
	}
	if !isDigits(s) {
		return strings.TrimSpace(raw)
	}
	if len(s) == 6 {
		id, _, _, ok := aShareListing(s)
		if ok {
			return id
		}
		return s
	}
	if len(s) <= 5 {
		return padHK(s) + ".HK"
	}
	return s
}

func aShareListing(code string) (id, column, plate string, ok bool) {
	if len(code) != 6 || !isDigits(code) {
		return "", "", "", false
	}
	if strings.HasPrefix(code, "200") || strings.HasPrefix(code, "900") {
		return "", "", "", false
	}
	switch code[0] {
	case '6':
		return code + ".SH", "sse", "sh", true
	case '0', '3':
		return code + ".SZ", "szse", "sz", true
	case '4', '8':
		return code + ".BJ", "third", "bj", true
	}
	if strings.HasPrefix(code, "92") {
		return code + ".BJ", "third", "bj", true
	}
	return "", "", "", false
}

func padHK(code string) string {
	code = strings.TrimSpace(code)
	if len(code) >= 5 {
		return code
	}
	return strings.Repeat("0", 5-len(code)) + code
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func extractInstrumentTokens(upper string) []string {
	var out []string
	for _, field := range strings.FieldsFunc(upper, func(r rune) bool {
		return unicode.IsSpace(r) || r == ',' || r == ';' || r == '/' || r == '|' || r == '(' || r == ')'
	}) {
		field = strings.Trim(field, "。，、：:")
		if field == "" {
			continue
		}
		out = append(out, field)
	}
	return out
}

func catalogTextHit(row ListedInstrument, q string) bool {
	q = strings.TrimSpace(q)
	if q == "" {
		return true
	}
	upper := strings.ToUpper(q)
	if strings.EqualFold(row.ID, NormalizeInstrumentID(q)) {
		return true
	}
	if strings.Contains(strings.ToUpper(row.ID), upper) || strings.Contains(strings.ToUpper(row.Symbol), upper) {
		return true
	}
	if strings.Contains(row.Name, q) {
		return true
	}
	if row.Pinyin != "" && strings.HasPrefix(row.Pinyin, strings.ToLower(q)) {
		return true
	}
	return false
}

func toPublicInstruments(rows []ListedInstrument) []Instrument {
	out := make([]Instrument, 0, len(rows))
	for _, row := range rows {
		out = append(out, Instrument{ID: row.ID, Symbol: row.Symbol, Name: row.Name, Market: row.Market})
	}
	return out
}

func runeLen(s string) int { return len([]rune(s)) }
