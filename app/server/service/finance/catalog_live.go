package finance

import (
	"context"
	"encoding/json"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

type cninfoStockFile struct {
	StockList []cninfoStock `json:"stockList"`
}

type cninfoStock struct {
	Code     string `json:"code"`
	Pinyin   string `json:"pinyin"`
	Category string `json:"category"`
	OrgID    string `json:"orgId"`
	Name     string `json:"zwjc"`
}

type liveCatalogStore struct {
	mu      sync.RWMutex
	loaded  time.Time
	byID    map[string]ListedInstrument
	rows    []ListedInstrument
	loading sync.Mutex
}

var liveCatalog liveCatalogStore

func backgroundCatalogCtx() context.Context {
	return context.Background()
}

func WarmLiveCatalog() {
	if DataMode() != ModeLive {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if err := EnsureLiveCatalog(ctx); err != nil {
			log.Printf("live catalog: %v", err)
			return
		}
		log.Printf("live catalog ready n=%d", CatalogSize())
	}()
}

func CatalogSize() int {
	liveCatalog.mu.RLock()
	defer liveCatalog.mu.RUnlock()
	return len(liveCatalog.rows)
}

func EnsureLiveCatalog(ctx context.Context) error {
	liveCatalog.mu.RLock()
	fresh := !liveCatalog.loaded.IsZero() && time.Since(liveCatalog.loaded) < 12*time.Hour && len(liveCatalog.rows) > 0
	liveCatalog.mu.RUnlock()
	if fresh {
		return nil
	}
	liveCatalog.loading.Lock()
	defer liveCatalog.loading.Unlock()
	liveCatalog.mu.RLock()
	fresh = !liveCatalog.loaded.IsZero() && time.Since(liveCatalog.loaded) < 12*time.Hour && len(liveCatalog.rows) > 0
	liveCatalog.mu.RUnlock()
	if fresh {
		return nil
	}
	if ctx == nil {
		ctx = backgroundCatalogCtx()
	}
	rows, err := fetchCninfoCatalog(ctx)
	if err != nil {
		return err
	}
	liveCatalog.replace(rows)
	return nil
}

func (c *liveCatalogStore) replace(rows []ListedInstrument) {
	byID := make(map[string]ListedInstrument, len(rows)+len(FrozenLiveInstruments))
	merged := make([]ListedInstrument, 0, len(rows)+len(FrozenLiveInstruments))
	add := func(row ListedInstrument) {
		if _, ok := byID[row.ID]; ok {
			return
		}
		byID[row.ID] = row
		merged = append(merged, row)
	}
	for _, row := range FrozenLiveInstruments {
		add(row)
	}
	for _, row := range rows {
		add(row)
	}
	c.mu.Lock()
	c.byID = byID
	c.rows = merged
	c.loaded = time.Now()
	c.mu.Unlock()
}

func liveLookup(id string) (ListedInstrument, bool) {
	liveCatalog.mu.RLock()
	defer liveCatalog.mu.RUnlock()
	row, ok := liveCatalog.byID[id]
	return row, ok
}

func catalogSearchPool() []ListedInstrument {
	liveCatalog.mu.RLock()
	defer liveCatalog.mu.RUnlock()
	if len(liveCatalog.rows) > 0 {
		out := make([]ListedInstrument, len(liveCatalog.rows))
		copy(out, liveCatalog.rows)
		return out
	}
	out := make([]ListedInstrument, len(FrozenLiveInstruments))
	copy(out, FrozenLiveInstruments)
	return out
}

func liveSearch(q, market string, limit int) []ListedInstrument {
	pool := catalogSearchPool()
	type scored struct {
		row   ListedInstrument
		score int
	}
	var hits []scored
	norm := NormalizeInstrumentID(q)
	lower := strings.ToLower(q)
	for _, row := range pool {
		if market != "" && row.Market != market {
			continue
		}
		score := 0
		switch {
		case row.ID == norm:
			score = 100
		case strings.EqualFold(row.Symbol, strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(norm, ".HK"), ".SH"), ".SZ")) || row.Symbol == q:
			score = 90
		case row.Name == q:
			score = 80
		case strings.Contains(row.Name, q):
			score = 50 + runeLen(q)
		case row.Pinyin != "" && strings.HasPrefix(row.Pinyin, lower):
			score = 40
		case strings.HasPrefix(row.Symbol, q) || strings.HasPrefix(row.ID, strings.ToUpper(q)):
			score = 30
		default:
			if catalogTextHit(row, q) {
				score = 10
			}
		}
		if score > 0 {
			hits = append(hits, scored{row: row, score: score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].row.ID < hits[j].row.ID
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]ListedInstrument, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.row)
	}
	return out
}

func fetchCninfoCatalog(ctx context.Context) ([]ListedInstrument, error) {
	httpc := NewCountedHTTP(4, 30*time.Second)
	aBody, _, err := httpc.Get(ctx, cninfoAListURL)
	if err != nil {
		return nil, err
	}
	var aFile cninfoStockFile
	if err := json.Unmarshal(aBody, &aFile); err != nil {
		return nil, NewError(503, "unavailable", "DATA_UNAVAILABLE", "A股目录无法解析")
	}
	var hkFile cninfoStockFile
	if hkBody, _, hkErr := httpc.Get(ctx, cninfoHKListURL); hkErr == nil {
		_ = json.Unmarshal(hkBody, &hkFile)
	}
	rows := make([]ListedInstrument, 0, len(aFile.StockList)+len(hkFile.StockList))
	for _, row := range aFile.StockList {
		if inst, ok := listedFromCninfo(row, MarketA); ok {
			rows = append(rows, inst)
		}
	}
	for _, row := range hkFile.StockList {
		if inst, ok := listedFromCninfo(row, MarketHK); ok {
			rows = append(rows, inst)
		}
	}
	if len(rows) < 100 {
		return nil, NewError(503, "unavailable", "DATA_UNAVAILABLE", "证券目录覆盖不足")
	}
	return rows, nil
}

func listedFromCninfo(row cninfoStock, market string) (ListedInstrument, bool) {
	code := strings.TrimSpace(row.Code)
	name := strings.TrimSpace(row.Name)
	org := strings.TrimSpace(row.OrgID)
	if code == "" || name == "" || org == "" {
		return ListedInstrument{}, false
	}
	cat := strings.TrimSpace(row.Category)
	if market == MarketA {
		if cat == "B股" {
			return ListedInstrument{}, false
		}
		id, column, plate, ok := aShareListing(code)
		if !ok {
			return ListedInstrument{}, false
		}
		return ListedInstrument{
			ID: id, Symbol: code, Name: name, Market: MarketA,
			OrgID: org, Column: column, Plate: plate, Pinyin: strings.ToLower(strings.TrimSpace(row.Pinyin)),
		}, true
	}
	if cat != "" && cat != "港股" {
		return ListedInstrument{}, false
	}
	id := padHK(code) + ".HK"
	return ListedInstrument{
		ID: id, Symbol: padHK(code), Name: name, Market: MarketHK,
		OrgID: org, Column: "hke", Plate: "hke", Pinyin: strings.ToLower(strings.TrimSpace(row.Pinyin)),
	}, true
}

func ReplaceLiveCatalogForTest(rows []ListedInstrument) {
	liveCatalog.replace(rows)
}

func ResetLiveCatalogForTest() {
	liveCatalog.mu.Lock()
	liveCatalog.byID = nil
	liveCatalog.rows = nil
	liveCatalog.loaded = time.Time{}
	liveCatalog.mu.Unlock()
}
