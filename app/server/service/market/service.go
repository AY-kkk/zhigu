package market

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	model "zhigu/server/model/market"
	"zhigu/server/service/finance"
	"zhigu/server/service/indicators"
)

type Service struct {
	DB      *gorm.DB
	Adapter Adapter
	HTTP    *HTTP
	group   singleflight.Group
	mode    string
}

func NewService(db *gorm.DB) *Service {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("ZHIGU_MARKET_MODE")))
	if mode == "" {
		mode = "fixture"
	}
	timeout := 8 * time.Second
	if mode == "live" {
		timeout = 25 * time.Second
	}
	httpClient := NewHTTP(timeout)
	var ad Adapter
	if mode == "live" {
		ad = &EastMoney{HTTP: httpClient}
	}
	return &Service{DB: db, Adapter: ad, HTTP: httpClient, mode: mode}
}

func (s *Service) Mode() string { return s.mode }

func (s *Service) EnsureFixture(ctx context.Context) error {
	var n int64
	if err := s.DB.WithContext(ctx).Model(&model.Instrument{}).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	return s.publishCatalog(ctx, "fixture", FixtureUniverse(), "fresh")
}

func (s *Service) publishCatalog(ctx context.Context, source string, rows []SeedInstrument, freshness string) error {
	return s.publishCatalogMeta(ctx, source, rows, freshness, map[string]any{"count": len(rows)})
}

func (s *Service) publishCatalogMeta(ctx context.Context, source string, rows []SeedInstrument, freshness string, extra map[string]any) error {
	ver := "cat_" + uuid.NewString()
	now := time.Now().UTC()
	asOf := now.Format("2006-01-02")
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, row := range rows {
			full, abbr := NameIndex(row.Name)
			aliases, _ := json.Marshal(row.Aliases)
			inst := model.Instrument{
				InstrumentID: row.InstrumentID, SecurityID: row.SecurityID, Exchange: row.Exchange, Board: row.Board,
				AssetType: row.AssetType, Code: row.Code, Name: row.Name, NameEN: row.NameEN, Aliases: datatypes.JSON(aliases),
				Pinyin: full, PinyinAbbr: abbr, Currency: row.Currency, CalendarID: CalendarID(row.Exchange),
				TradingStatus: row.Status, LotSize: row.Lot, LotRuleID: "lot_" + row.Exchange, TickRuleID: "tick_" + row.Exchange,
				ValidFrom: "1990-01-01", CatalogVersion: ver, SourcePayload: datatypes.JSON([]byte("{}")), CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&inst).Error; err != nil {
				return err
			}
			names := append([]string{row.Name, row.NameEN, row.Code, row.InstrumentID, abbr}, row.Aliases...)
			for _, a := range names {
				a = strings.TrimSpace(a)
				if a == "" {
					continue
				}
				al := model.Alias{Alias: a, Normalized: strings.ToLower(NormalizeQuery(a)), InstrumentID: row.InstrumentID, Kind: "alias", ValidFrom: "1990-01-01", CatalogVersion: ver}
				_ = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&al).Error
			}
		}
		body, _ := json.Marshal(extra)
		if len(body) == 0 {
			body = []byte("{}")
		}
		rel := model.CatalogRelease{
			CatalogVersion: ver, CatalogAsOf: asOf, FreshnessStatus: freshness, SourceID: source,
			SourceContractVersion: SourceContractVersion, InstrumentCount: len(rows), Payload: datatypes.JSON(body), PublishedAt: now,
		}
		if err := tx.Create(&rel).Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&model.Pointer{Name: "catalog", Value: ver, UpdatedAt: now}).Error
	})
}

func (s *Service) CatalogMeta(ctx context.Context) (map[string]any, error) {
	var p model.Pointer
	if err := s.DB.WithContext(ctx).Where("name = ?", "catalog").Take(&p).Error; err != nil {
		return map[string]any{"catalog_version": nil, "freshness_status": "missing"}, nil
	}
	var rel model.CatalogRelease
	_ = s.DB.WithContext(ctx).Where("catalog_version = ?", p.Value).Take(&rel).Error
	return map[string]any{
		"catalog_version": rel.CatalogVersion, "catalog_as_of": rel.CatalogAsOf,
		"freshness_status": rel.FreshnessStatus, "instrument_count": rel.InstrumentCount,
		"source_id": rel.SourceID, "source_contract_version": rel.SourceContractVersion, "intraday": "时效未验证",
	}, nil
}

type SearchResult struct {
	Items          []InstrumentView `json:"items"`
	NextCursor     *string          `json:"next_cursor"`
	CatalogVersion string           `json:"catalog_version"`
	CatalogAsOf    string           `json:"catalog_as_of"`
	Freshness      string           `json:"freshness_status"`
}

func (s *Service) ensureCatalog(ctx context.Context) error {
	if s.mode != "live" {
		return s.EnsureFixture(ctx)
	}
	meta, _ := s.CatalogMeta(ctx)
	src, _ := meta["source_id"].(string)
	n, _ := meta["instrument_count"].(int)
	if n >= 1000 && src != "" && src != "fixture" {
		return nil
	}
	return s.SyncLive(ctx)
}

func (s *Service) Search(ctx context.Context, q, market, exchange, cursor string, limit int) (SearchResult, error) {
	if err := s.ensureCatalog(ctx); err != nil {
		return SearchResult{}, err
	}
	if limit <= 0 {
		limit = 20
	} else if limit > 80 {
		limit = 80
	}
	meta, _ := s.CatalogMeta(ctx)
	ver, _ := meta["catalog_version"].(string)
	asOf, _ := meta["catalog_as_of"].(string)
	fresh, _ := meta["freshness_status"].(string)
	db := s.DB.WithContext(ctx).Model(&model.Instrument{})
	if ver != "" {
		db = db.Where("catalog_version = ?", ver)
	}
	if cursor != "" {
		if raw, err := base64.RawURLEncoding.DecodeString(cursor); err == nil && len(raw) > 0 {
			db = db.Where("instrument_id > ?", string(raw))
		}
	}
	q = strings.TrimSpace(q)
	if exchange != "" {
		db = db.Where("exchange = ?", exchange)
	}
	if market != "" {
		switch strings.ToUpper(market) {
		case "A", "CN":
			db = db.Where("exchange IN ?", []string{"SSE", "SZSE", "BSE"})
		case "HK", "HKEX":
			db = db.Where("exchange = ?", "HKEX")
		}
	}
	if q != "" {
		up := NormalizeQuery(q)
		like := "%" + strings.ToLower(q) + "%"
		hk := HKCandidates(q)
		ors := []string{
			"instrument_id = ?", "code = ?", "upper(name) = ?", "lower(pinyin_abbr) = ?",
			"instrument_id ILIKE ?", "name ILIKE ?", "name_en ILIKE ?", "pinyin ILIKE ?", "pinyin_abbr ILIKE ?",
		}
		args := []any{up, strings.TrimSuffix(strings.TrimSuffix(up, ".SH"), ".SZ"), q, strings.ToLower(q), like, like, like, like, like}
		if len(hk) > 0 {
			ors = append(ors, "instrument_id = ?")
			args = append(args, hk[0])
		}
		db = db.Where(strings.Join(ors, " OR "), args...)
	}
	var rows []model.Instrument
	if err := db.Order("instrument_id ASC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return SearchResult{}, err
	}
	exact := []model.Instrument{}
	rest := []model.Instrument{}
	want := strings.ToUpper(strings.TrimSpace(q))
	for _, r := range rows {
		if q != "" && cursor == "" && (r.InstrumentID == want || r.Code == want || containsHK(r.InstrumentID, q)) {
			exact = append(exact, r)
		} else {
			rest = append(rest, r)
		}
	}
	if cursor == "" {
		rankNameHits(q, rest)
		rows = append(exact, rest...)
	}
	var next *string
	if len(rows) > limit {
		last := rows[limit-1].InstrumentID
		c := base64.RawURLEncoding.EncodeToString([]byte(last))
		next = &c
		rows = rows[:limit]
	}
	items := make([]InstrumentView, 0, len(rows))
	for _, r := range rows {
		items = append(items, toView(r))
	}
	s.attachQuotes(ctx, items)
	return SearchResult{Items: items, NextCursor: next, CatalogVersion: ver, CatalogAsOf: asOf, Freshness: fresh}, nil
}

func listingKey(r model.Instrument) string {
	if r.ListingDate != nil {
		return *r.ListingDate
	}
	return ""
}

func rankNameHits(q string, rest []model.Instrument) {
	want := strings.TrimSpace(q)
	if want == "" {
		return
	}
	sort.SliceStable(rest, func(i, j int) bool {
		ni, nj := rest[i].Name == want, rest[j].Name == want
		if ni != nj {
			return ni
		}
		di := rest[i].DelistingDate != nil && *rest[i].DelistingDate != ""
		dj := rest[j].DelistingDate != nil && *rest[j].DelistingDate != ""
		if di != dj {
			return !di
		}
		if li, lj := listingKey(rest[i]), listingKey(rest[j]); li != lj {
			return li > lj
		}
		// 北交所 2025 年后代码迁到 92 开头，旧 43/83/87 与新代码同名时优先新代码。
		if rest[i].Exchange == "BSE" && rest[j].Exchange == "BSE" {
			ci, cj := strings.HasPrefix(rest[i].Code, "92"), strings.HasPrefix(rest[j].Code, "92")
			if ci != cj {
				return ci
			}
		}
		return false
	})
}

func (s *Service) attachQuotes(ctx context.Context, items []InstrumentView) {
	if s.mode != "live" || len(items) == 0 {
		return
	}
	em, ok := s.Adapter.(*EastMoney)
	if !ok || em == nil {
		return
	}
	seeds := make([]SeedInstrument, 0, len(items))
	for _, it := range items {
		seeds = append(seeds, SeedInstrument{InstrumentID: it.InstrumentID, Exchange: it.Exchange, Code: it.Code, Name: it.Name})
	}
	got, err := em.LastQuotes(ctx, seeds)
	if err != nil || got == nil {
		got = map[string]QuoteSnapshot{}
	}
	missing := applyLiveQuotes(items, got)
	for id, q := range s.dailyCloseQuotes(ctx, missing) {
		for i := range items {
			if items[i].InstrumentID != id || items[i].Last != "" {
				continue
			}
			stampQuote(&items[i], q)
		}
	}
	s.markStaleQuotes(ctx, items)
}

func applyLiveQuotes(items []InstrumentView, live map[string]QuoteSnapshot) []string {
	missing := make([]string, 0)
	for i := range items {
		q, ok := live[items[i].InstrumentID]
		if ok && q.Last != "" {
			stampQuote(&items[i], q)
			continue
		}
		missing = append(missing, items[i].InstrumentID)
	}
	return missing
}

func stampQuote(item *InstrumentView, q QuoteSnapshot) {
	item.Last, item.Change, item.ChangePct = q.Last, q.Change, q.ChangePct
	item.QuoteBasis = "last"
	item.QuoteAsOf = ""
	if q.Quality == nil {
		return
	}
	fs, _ := q.Quality["freshness_status"].(string)
	if fs != "daily_close" {
		return
	}
	item.QuoteBasis = "daily_close"
	item.QuoteAsOf = DayKey(q.MarketTime)
}

func (s *Service) markStaleQuotes(ctx context.Context, items []InstrumentView) {
	now := time.Now().UTC()
	for i := range items {
		if items[i].Last == "" {
			continue
		}
		q, ok := s.cachedClose(ctx, model.Instrument{InstrumentID: items[i].InstrumentID, Name: items[i].Name, Exchange: items[i].Exchange})
		if !ok {
			continue
		}
		session := LastCompleteSession(CalendarID(items[i].Exchange), now)
		day, stale := staleQuoteDate(DayKey(q.MarketTime), session, 15)
		if !stale {
			continue
		}
		items[i].QuoteBasis = "daily_close"
		items[i].QuoteAsOf = day
	}
}

func staleQuoteDate(barDay, sessionDay string, gapDays int) (string, bool) {
	if barDay == "" || sessionDay == "" || barDay >= sessionDay {
		return "", false
	}
	bar, err1 := time.Parse("2006-01-02", barDay)
	session, err2 := time.Parse("2006-01-02", sessionDay)
	if err1 != nil || err2 != nil {
		return "", false
	}
	if session.Sub(bar) < time.Duration(gapDays)*24*time.Hour {
		return "", false
	}
	return barDay, true
}

func (s *Service) dailyCloseQuotes(ctx context.Context, ids []string) map[string]QuoteSnapshot {
	out := map[string]QuoteSnapshot{}
	if len(ids) == 0 {
		return out
	}
	var rows []model.Instrument
	if err := s.DB.WithContext(ctx).Where("instrument_id IN ?", ids).Find(&rows).Error; err != nil {
		return out
	}
	cold := make([]model.Instrument, 0)
	for _, row := range rows {
		if q, ok := s.cachedClose(ctx, row); ok {
			out[row.InstrumentID] = q
			continue
		}
		cold = append(cold, row)
	}
	if len(cold) > 8 {
		cold = cold[:8]
	}
	for _, row := range cold {
		pack, err := s.loadBars(ctx, row, "1d", "raw")
		if err != nil || len(pack.Bars) == 0 {
			continue
		}
		last := pack.Bars[len(pack.Bars)-1]
		out[row.InstrumentID] = closeQuote(row, last.Close, last.Volume, last.Time, last.IsFinal, pack.Source)
	}
	return out
}

func (s *Service) cachedClose(ctx context.Context, row model.Instrument) (QuoteSnapshot, bool) {
	var snap model.Snapshot
	err := s.DB.WithContext(ctx).Where("instrument_id = ? AND period = ? AND adjust = ?", row.InstrumentID, "1d", "raw").
		Order("created_at desc").Take(&snap).Error
	if err != nil || snap.ID == "" {
		return QuoteSnapshot{}, false
	}
	var bar model.BarRow
	err = s.DB.WithContext(ctx).Where("snapshot_id = ?", snap.ID).Order("trade_date desc").Take(&bar).Error
	if err != nil || bar.Close == "" {
		return QuoteSnapshot{}, false
	}
	return closeQuote(row, bar.Close, bar.Volume, bar.TradeDate, bar.IsFinal, snap.SourceID), true
}

func closeQuote(row model.Instrument, last, volume, day string, final bool, source string) QuoteSnapshot {
	return QuoteSnapshot{
		InstrumentID: row.InstrumentID, Name: row.Name, Exchange: row.Exchange,
		Last: last, Volume: volume, MarketTime: day, IsFinal: final, SourceID: source,
		Quality: map[string]any{"status": "unverified", "freshness_status": "daily_close"},
	}
}

func containsHK(id, q string) bool {
	for _, c := range HKCandidates(q) {
		if c == id {
			return true
		}
	}
	return false
}

func PreferInstrument(q, text string, items []InstrumentView) string {
	wantHK := strings.Contains(text, "港")
	cand := make([]InstrumentView, 0, len(items))
	for _, it := range items {
		if it.AssetType != "" && it.AssetType != "stock" {
			continue
		}
		if it.DelistingDate != nil && *it.DelistingDate != "" {
			continue
		}
		if it.TradingStatus == "delisted" {
			continue
		}
		if it.Name == q || strings.Contains(text, it.Name) || (q != "" && strings.HasPrefix(it.Name, q)) {
			cand = append(cand, it)
		}
	}
	if len(cand) == 0 {
		if len(items) == 1 && (items[0].AssetType == "" || items[0].AssetType == "stock") {
			return items[0].InstrumentID
		}
		return ""
	}
	best := cand[0]
	for _, it := range cand[1:] {
		if preferName(q, wantHK, it, best) {
			best = it
		}
	}
	return best.InstrumentID
}

func preferName(q string, wantHK bool, a, b InstrumentView) bool {
	aHK, bHK := a.Exchange == "HKEX", b.Exchange == "HKEX"
	if aHK != bHK {
		if wantHK {
			return aHK
		}
		return !aHK
	}
	ar, br := len([]rune(a.Name)), len([]rune(b.Name))
	if ar != br {
		return ar < br
	}
	return strings.HasPrefix(a.Name, q) && !strings.HasPrefix(b.Name, q)
}

func toView(r model.Instrument) InstrumentView {
	v := InstrumentView{
		InstrumentID: r.InstrumentID, SecurityID: r.SecurityID, Name: r.Name, Code: r.Code,
		Exchange: r.Exchange, Board: r.Board, AssetType: r.AssetType, Currency: r.Currency,
		CalendarID: r.CalendarID, TradingStatus: r.TradingStatus, LotSize: r.LotSize,
		ListingDate: r.ListingDate, DelistingDate: r.DelistingDate, CatalogVersion: r.CatalogVersion,
	}
	_ = json.Unmarshal(r.Aliases, &v.Aliases)
	if r.AssetType != "stock" {
		v.Unsupported = "该证券类型不是股票，策略与股票回测不可用"
	}
	if v.LotSize <= 0 {
		v.LotSize = RuleFor(r.Exchange, r.Board, time.Now().Format("2006-01-02")).LotSize
	}
	return v
}

func (s *Service) GetInstrument(ctx context.Context, id string) (map[string]any, error) {
	if err := s.ensureCatalog(ctx); err != nil {
		return nil, err
	}
	var row model.Instrument
	if err := s.DB.WithContext(ctx).Where("instrument_id = ?", id).Take(&row).Error; err != nil {
		return nil, finance.NewError(404, "not_found", "NOT_FOUND", "标的不存在")
	}
	view := toView(row)
	rule := RuleFor(row.Exchange, row.Board, time.Now().Format("2006-01-02"))
	if view.LotSize > 0 {
		rule.LotSize = view.LotSize
	}
	cover := s.coverage(ctx, id)
	if q, err := s.GetQuote(ctx, id); err == nil {
		view.Last, view.Change, view.ChangePct = q.Last, q.Change, q.ChangePct
	}
	return map[string]any{
		"instrument":         view,
		"rule":               rule,
		"periods":            []string{"1d", "1w", "1mo"},
		"adjust":             []string{"raw", "qfq", "hfq"},
		"coverage":           cover,
		"intraday":           map[string]any{"status": "unverified", "label": "最新价延迟未知"},
		"backtest_available": view.AssetType == "stock" && view.LotSize > 0,
		"reason":             view.Unsupported,
	}, nil
}

func (s *Service) GetQuote(ctx context.Context, id string) (QuoteSnapshot, error) {
	if err := s.ensureCatalog(ctx); err != nil {
		return QuoteSnapshot{}, err
	}
	var row model.Instrument
	if err := s.DB.WithContext(ctx).Where("instrument_id = ?", id).Take(&row).Error; err != nil {
		return QuoteSnapshot{}, finance.NewError(404, "not_found", "NOT_FOUND", "标的不存在")
	}
	seed := SeedInstrument{InstrumentID: row.InstrumentID, Exchange: row.Exchange, Code: row.Code, Name: row.Name, Board: row.Board, AssetType: row.AssetType}
	if s.mode == "live" {
		if em, ok := s.Adapter.(*EastMoney); ok && em != nil {
			if q, err := em.LastQuote(ctx, seed); err == nil {
				return q, nil
			}
		}
	}
	pack, err := s.loadBars(ctx, row, "1d", "raw")
	if err != nil || len(pack.Bars) == 0 {
		return QuoteSnapshot{}, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "最新价不可用")
	}
	last := pack.Bars[len(pack.Bars)-1]
	return QuoteSnapshot{
		InstrumentID: row.InstrumentID, Name: row.Name, Exchange: row.Exchange,
		Last: last.Close, Open: last.Open, High: last.High, Low: last.Low, PrevClose: last.Close,
		Volume: last.Volume, ObservedAt: time.Now().UTC().Format(time.RFC3339), MarketTime: last.Time,
		IsFinal: last.IsFinal, SourceID: pack.Source,
		Quality: map[string]any{"status": "unverified", "freshness_status": "daily_close", "warnings": []string{"无盘中最新价，展示最近完整日收盘"}},
	}, nil
}

func (s *Service) GetQuotes(ctx context.Context, ids []string) ([]QuoteSnapshot, error) {
	if err := s.ensureCatalog(ctx); err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	want := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		want = append(want, id)
		if len(want) >= 80 {
			break
		}
	}
	if len(want) == 0 {
		return []QuoteSnapshot{}, nil
	}
	var rows []model.Instrument
	if err := s.DB.WithContext(ctx).Where("instrument_id IN ?", want).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]QuoteSnapshot, 0, len(want))
	if s.mode == "live" {
		if em, ok := s.Adapter.(*EastMoney); ok && em != nil {
			seeds := make([]SeedInstrument, 0, len(rows))
			for _, r := range rows {
				seeds = append(seeds, SeedInstrument{InstrumentID: r.InstrumentID, Exchange: r.Exchange, Code: r.Code, Name: r.Name, Board: r.Board, AssetType: r.AssetType})
			}
			got, err := em.LastQuotes(ctx, seeds)
			if err == nil {
				missing := make([]string, 0)
				have := map[string]QuoteSnapshot{}
				for _, id := range want {
					if q, ok := got[id]; ok && q.Last != "" {
						have[id] = q
					} else {
						missing = append(missing, id)
					}
				}
				for id, q := range s.dailyCloseQuotes(ctx, missing) {
					have[id] = q
				}
				for _, id := range want {
					if q, ok := have[id]; ok {
						out = append(out, q)
					}
				}
				return out, nil
			}
		}
	}
	for _, id := range want {
		if q, err := s.GetQuote(ctx, id); err == nil {
			out = append(out, q)
		}
	}
	return out, nil
}

func (s *Service) coverage(ctx context.Context, id string) map[string]any {
	var snap model.Snapshot
	err := s.DB.WithContext(ctx).Where("instrument_id = ? AND period = ? AND adjust = ?", id, "1d", "raw").Order("created_at desc").Take(&snap).Error
	if err != nil {
		return map[string]any{"status": "unknown", "supported_periods": []string{"1d", "1w", "1mo"}}
	}
	return map[string]any{
		"status": "ready", "earliest_date": snap.EarliestDate, "latest_complete_date": snap.LatestCompleteDate,
		"supported_periods": []string{"1d", "1w", "1mo"}, "data_snapshot_id": snap.ID,
	}
}

type OHLCV struct {
	InstrumentID   string         `json:"instrument_id"`
	Name           string         `json:"name"`
	Exchange       string         `json:"exchange"`
	Currency       string         `json:"currency"`
	Period         string         `json:"period"`
	Adjust         string         `json:"adjust"`
	DataSnapshotID string         `json:"data_snapshot_id"`
	Source         map[string]any `json:"source"`
	Quality        map[string]any `json:"quality"`
	Bars           []QuoteBar     `json:"bars"`
	HasMore        bool           `json:"has_more"`
	NextCursor     *string        `json:"next_cursor"`
}

func (s *Service) GetOHLCV(ctx context.Context, id, period, adjust, start, end, cursor string, limit int) (OHLCV, error) {
	if err := s.ensureCatalog(ctx); err != nil {
		return OHLCV{}, err
	}
	var inst model.Instrument
	if err := s.DB.WithContext(ctx).Where("instrument_id = ?", id).Take(&inst).Error; err != nil {
		return OHLCV{}, finance.NewError(404, "not_found", "NOT_FOUND", "标的不存在")
	}
	if period == "" {
		period = "1d"
	}
	if adjust == "" {
		adjust = "raw"
	}
	if period != "1d" && period != "1w" && period != "1mo" {
		return OHLCV{}, finance.NewError(400, "validation", "INVALID_PARAM", "不支持的周期")
	}
	if adjust != "raw" && adjust != "qfq" && adjust != "hfq" {
		return OHLCV{}, finance.NewError(400, "validation", "INVALID_PARAM", "不支持的复权")
	}
	if limit <= 0 {
		limit = 250
	}
	if limit > 2000 {
		return OHLCV{}, finance.NewError(400, "validation", "RANGE_TOO_LARGE", "单次最多 2000 根")
	}
	key := id + "|" + period + "|" + adjust
	v, err, _ := s.group.Do(key, func() (any, error) {
		return s.loadBars(ctx, inst, period, adjust)
	})
	if err != nil {
		return OHLCV{}, err
	}
	pack := v.(barPack)
	bars := normalizeBars(pack.Bars)
	if start != "" || end != "" {
		filtered := make([]QuoteBar, 0, len(bars))
		for _, b := range bars {
			if start != "" && b.Time < start {
				continue
			}
			if end != "" && b.Time > end {
				continue
			}
			filtered = append(filtered, b)
		}
		bars = filtered
	}
	if cursor != "" {
		if raw, err := base64.RawURLEncoding.DecodeString(cursor); err == nil {
			cut := string(raw)
			trimmed := make([]QuoteBar, 0, len(bars))
			for _, b := range bars {
				if b.Time < cut {
					trimmed = append(trimmed, b)
				}
			}
			bars = trimmed
		}
	}
	hasMore := false
	var next *string
	// 展示窗口含最新时叠加延迟行情的当日未完成 bar；快照本身仍只按完整日判定新鲜度。
	if s.mode == "live" && end == "" && cursor == "" {
		if em, ok := s.Adapter.(*EastMoney); ok && em != nil {
			seed := SeedInstrument{InstrumentID: inst.InstrumentID, Exchange: inst.Exchange, Code: inst.Code, Board: inst.Board, AssetType: inst.AssetType}
			if q, qerr := em.LastQuote(ctx, seed); qerr == nil {
				bars = applyIntrabar(bars, q, CalendarID(inst.Exchange), time.Now().UTC())
			}
		}
	}
	if len(bars) > limit {
		hasMore = true
		startIdx := len(bars) - limit
		enc := base64.RawURLEncoding.EncodeToString([]byte(bars[startIdx].Time))
		next = &enc
		bars = bars[startIdx:]
	}
	last := ""
	if pack.Latest != nil {
		last = *pack.Latest
	}
	status := "complete"
	if len(bars) == 0 {
		status = "empty"
	}
	fresh := "fixture"
	if s.mode == "live" {
		fresh = "unverified"
		want := LastCompleteSession(CalendarID(inst.Exchange), time.Now().UTC())
		if last != "" && last >= want {
			fresh = "daily_complete"
		}
	}
	return OHLCV{
		InstrumentID: inst.InstrumentID, Name: inst.Name, Exchange: inst.Exchange, Currency: inst.Currency,
		Period: period, Adjust: adjust, DataSnapshotID: pack.SnapshotID,
		Source:  map[string]any{"source_id": pack.Source, "source_contract_version": SourceContractVersion, "retrieved_at": time.Now().UTC().Format(time.RFC3339)},
		Quality: map[string]any{"status": status, "freshness_status": fresh, "last_complete_date": last, "missing_dates": []string{}, "warnings": []string{"盘中时效未验证"}, "is_final": len(bars) == 0 || bars[len(bars)-1].IsFinal},
		Bars:    bars, HasMore: hasMore, NextCursor: next,
	}, nil
}

type barPack struct {
	SnapshotID string
	Source     string
	Bars       []QuoteBar
	Latest     *string
}

func (s *Service) loadBars(ctx context.Context, inst model.Instrument, period, adjust string) (barPack, error) {
	var snap model.Snapshot
	err := s.DB.WithContext(ctx).Where("instrument_id = ? AND period = ? AND adjust = ?", inst.InstrumentID, period, adjust).
		Order("created_at desc").Take(&snap).Error
	if err == nil && s.snapshotFresh(inst, snap) {
		var rows []model.BarRow
		if err := s.DB.WithContext(ctx).Where("snapshot_id = ?", snap.ID).Order("trade_date asc").Find(&rows).Error; err != nil {
			return barPack{}, err
		}
		bars := make([]QuoteBar, 0, len(rows))
		for _, r := range rows {
			b := QuoteBar{Time: DayKey(r.TradeDate), Open: r.Open, High: r.High, Low: r.Low, Close: r.Close, Volume: r.Volume, IsFinal: r.IsFinal}
			if r.PeriodStart != nil {
				b.PeriodStart = *r.PeriodStart
			}
			if r.PeriodEnd != nil {
				b.PeriodEnd = *r.PeriodEnd
			}
			bars = append(bars, b)
		}
		return barPack{SnapshotID: snap.ID, Source: snap.SourceID, Bars: bars, Latest: snap.LatestCompleteDate}, nil
	}
	var daily []QuoteBar
	if s.mode == "live" {
		if s.Adapter == nil {
			return barPack{}, finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "live 行情适配器未配置")
		}
		seed := SeedInstrument{InstrumentID: inst.InstrumentID, Exchange: inst.Exchange, Code: inst.Code, Board: inst.Board, AssetType: inst.AssetType}
		got, err := s.Adapter.Bars(ctx, seed, "1d", adjust, "", "", 2000)
		if err != nil {
			return barPack{}, err
		}
		daily = got
	} else {
		daily = SyntheticBars(inst.InstrumentID, 400, PreviewDate())
	}
	use := daily
	if period != "1d" {
		use = aggregate(daily, period)
	}
	id := "snap_" + uuid.NewString()
	now := time.Now().UTC()
	var earliest, latest *string
	if len(use) > 0 {
		e := use[0].Time
		earliest = &e
		latest = finalCompleteDate(use)
	}
	hash := finance.SHA256Text(inst.InstrumentID + period + adjust + strconv.Itoa(len(use)))
	src := s.mode
	if s.Adapter != nil && s.mode == "live" {
		src = s.Adapter.ID()
	}
	row := model.Snapshot{
		ID: id, InstrumentID: inst.InstrumentID, Period: period, Adjust: adjust,
		SourceID: src, SourceContractVersion: SourceContractVersion, Quality: datatypes.JSON([]byte(`{"status":"complete"}`)),
		BarsHash: hash, Immutable: true, CreatedAt: now, EarliestDate: earliest, LatestCompleteDate: latest,
	}
	if err := s.DB.WithContext(ctx).Create(&row).Error; err != nil {
		return barPack{}, err
	}
	batch := make([]model.BarRow, 0, len(use))
	for _, b := range use {
		br := model.BarRow{SnapshotID: id, InstrumentID: inst.InstrumentID, Period: period, TradeDate: b.Time, Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume, IsFinal: b.IsFinal}
		if b.PeriodStart != "" {
			ps := b.PeriodStart
			br.PeriodStart = &ps
		}
		if b.PeriodEnd != "" {
			pe := b.PeriodEnd
			br.PeriodEnd = &pe
		}
		batch = append(batch, br)
	}
	if len(batch) > 0 {
		if err := s.DB.WithContext(ctx).CreateInBatches(batch, 400).Error; err != nil {
			return barPack{}, err
		}
	}
	return barPack{SnapshotID: id, Source: src, Bars: use, Latest: latest}, nil
}

func normalizeBars(bars []QuoteBar) []QuoteBar {
	out := make([]QuoteBar, 0, len(bars))
	for _, b := range bars {
		b.Time = DayKey(b.Time)
		if b.PeriodStart != "" {
			b.PeriodStart = DayKey(b.PeriodStart)
		}
		if b.PeriodEnd != "" {
			b.PeriodEnd = DayKey(b.PeriodEnd)
		}
		if b.Time == "" {
			continue
		}
		out = append(out, b)
	}
	return out
}

func (s *Service) snapshotFresh(inst model.Instrument, snap model.Snapshot) bool {
	if snap.LatestCompleteDate == nil || *snap.LatestCompleteDate == "" {
		return false
	}
	if s.mode != "live" {
		return true
	}
	want := LastCompleteSession(CalendarID(inst.Exchange), time.Now().UTC())
	return *snap.LatestCompleteDate >= want
}

func aggregate(daily []QuoteBar, period string) []QuoteBar {
	if len(daily) == 0 {
		return nil
	}
	type bucket struct {
		key  string
		bars []QuoteBar
	}
	var buckets []bucket
	curKey := ""
	for _, b := range daily {
		t, err := time.Parse("2006-01-02", b.Time)
		if err != nil {
			continue
		}
		key := ""
		if period == "1w" {
			y, w := t.ISOWeek()
			key = strconv.Itoa(y) + "-W" + strconv.Itoa(w)
		} else {
			key = t.Format("2006-01")
		}
		if key != curKey {
			buckets = append(buckets, bucket{key: key})
			curKey = key
		}
		buckets[len(buckets)-1].bars = append(buckets[len(buckets)-1].bars, b)
	}
	out := make([]QuoteBar, 0, len(buckets))
	for _, bk := range buckets {
		if len(bk.bars) == 0 {
			continue
		}
		first, last := bk.bars[0], bk.bars[len(bk.bars)-1]
		high, low := MustDec(first.High), MustDec(first.Low)
		vol := MustDec("0")
		for _, b := range bk.bars {
			if MustDec(b.High).GreaterThan(high) {
				high = MustDec(b.High)
			}
			if MustDec(b.Low).LessThan(low) {
				low = MustDec(b.Low)
			}
			vol = vol.Add(MustDec(b.Volume))
		}
		out = append(out, QuoteBar{
			Time: last.Time, Open: first.Open, High: high.String(), Low: low.String(), Close: last.Close,
			Volume: vol.String(), IsFinal: last.IsFinal, PeriodStart: first.Time, PeriodEnd: last.Time,
		})
	}
	return out
}

func (s *Service) IndicatorBars(ctx context.Context, id, period, adjust string) ([]indicators.Bar, string, error) {
	out, err := s.GetOHLCV(ctx, id, period, adjust, "", "", "", 2000)
	if err != nil {
		return nil, "", err
	}
	bars := make([]indicators.Bar, 0, len(out.Bars))
	for _, b := range out.Bars {
		if !b.IsFinal {
			continue
		}
		bars = append(bars, indicators.Bar{
			Time: b.Time, Open: MustDec(b.Open), High: MustDec(b.High), Low: MustDec(b.Low),
			Close: MustDec(b.Close), Volume: MustDec(b.Volume),
		})
	}
	return bars, out.DataSnapshotID, nil
}

func (s *Service) Actions(ctx context.Context, id string) ([]CorporateAction, error) {
	var rows []model.Action
	if err := s.DB.WithContext(ctx).Where("instrument_id = ?", id).Order("effective_at asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return actionsFromRows(rows), nil
	}
	if s.mode != "live" || s.Adapter == nil {
		return nil, nil
	}
	var inst model.Instrument
	if err := s.DB.WithContext(ctx).Where("instrument_id = ?", id).Take(&inst).Error; err != nil {
		return nil, finance.NewError(404, "not_found", "NOT_FOUND", "标的不存在")
	}
	got, err := s.Adapter.Actions(ctx, SeedInstrument{InstrumentID: inst.InstrumentID, Exchange: inst.Exchange, Code: inst.Code})
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for i, a := range got {
		row := model.Action{
			ID: fmt.Sprintf("act_%s_%d", id, i), InstrumentID: id, Kind: a.Kind, EffectiveAt: a.EffectiveAt,
			AvailableAt: a.AvailableAt, RecordDate: a.RecordDate, PayDate: a.PayDate,
			CashAmount: ptrIf(a.CashAmount), Ratio: ptrIf(a.Ratio),
			SourceID: s.Adapter.ID(), Version: AdjustmentVersion, EvidenceLevel: a.EvidenceLevel,
			Payload: datatypes.JSON([]byte("{}")), CreatedAt: now,
		}
		_ = s.DB.WithContext(ctx).Create(&row).Error
	}
	return got, nil
}

func actionsFromRows(rows []model.Action) []CorporateAction {
	out := make([]CorporateAction, 0, len(rows))
	for _, r := range rows {
		a := CorporateAction{Kind: r.Kind, EffectiveAt: r.EffectiveAt, AvailableAt: r.AvailableAt, RecordDate: r.RecordDate, PayDate: r.PayDate, EvidenceLevel: r.EvidenceLevel}
		if r.CashAmount != nil {
			a.CashAmount = *r.CashAmount
		}
		if r.Ratio != nil {
			a.Ratio = *r.Ratio
		}
		out = append(out, a)
	}
	return out
}

func (s *Service) Instrument(ctx context.Context, id string) (InstrumentView, error) {
	var row model.Instrument
	if err := s.ensureCatalog(ctx); err != nil {
		return InstrumentView{}, err
	}
	if err := s.DB.WithContext(ctx).Where("instrument_id = ?", id).Take(&row).Error; err != nil {
		return InstrumentView{}, finance.NewError(404, "not_found", "NOT_FOUND", "标的不存在")
	}
	return toView(row), nil
}

func (s *Service) SyncLive(ctx context.Context) error {
	if s.mode != "live" {
		return s.EnsureFixture(ctx)
	}
	if s.HTTP == nil {
		return finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", "live HTTP 未配置")
	}
	all, err := ListCninfoCatalog(ctx, s.HTTP)
	if err != nil {
		return finance.NewError(503, "unavailable", "DATA_UNAVAILABLE", err.Error())
	}
	counts := map[string]int{"cninfo": len(all)}
	off := FetchOfficialLists(ctx, s.HTTP)
	for mkt, rows := range off.ByMarket {
		counts["official_"+mkt] = len(rows)
	}
	for mkt := range off.Errors {
		counts["official_"+mkt+"_error"] = 1
	}
	all = MergeOfficialCatalog(all, off)
	if s.Adapter != nil {
		for _, k := range []string{"SSE_ETF"} {
			rows, lerr := s.Adapter.List(ctx, k)
			if lerr != nil {
				counts[k+"_error"] = 1
				continue
			}
			counts[k] = len(rows)
			all = append(all, rows...)
		}
	}
	all = dedupeSeeds(all)
	meta := map[string]any{"boards": counts, "count": len(all), "official_sources": off.Sources, "official_errors": off.Errors}
	return s.publishCatalogMeta(ctx, "cninfo_list+exchange_official+eastmoney_kline", all, "fresh", meta)
}

func dedupeSeeds(in []SeedInstrument) []SeedInstrument {
	seen := map[string]struct{}{}
	out := make([]SeedInstrument, 0, len(in))
	for _, row := range in {
		if _, ok := seen[row.InstrumentID]; ok {
			continue
		}
		seen[row.InstrumentID] = struct{}{}
		out = append(out, row)
	}
	return out
}
