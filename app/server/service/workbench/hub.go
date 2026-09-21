package workbench

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"zhigu/server/httpx"
	model "zhigu/server/model/strategy"
	"zhigu/server/service/backtest"
	"zhigu/server/service/finance"
	"zhigu/server/service/indicators"
	"zhigu/server/service/market"
	"zhigu/server/service/strategy"
)

type Hub struct {
	DB      *gorm.DB
	Market  *market.Service
	Config  *finance.ConfigService
	mu      sync.Mutex
	active  map[uint]int
	genN    map[uint][]time.Time
	genBusy map[uint]int
}

func NewHub(db *gorm.DB, mkt *market.Service) *Hub {
	return &Hub{DB: db, Market: mkt, active: map[uint]int{}, genN: map[uint][]time.Time{}, genBusy: map[uint]int{}}
}

func (h *Hub) UseConfig(cfg *finance.ConfigService) *Hub {
	h.Config = cfg
	return h
}

func (h *Hub) allowGenerate(uid uint) error {
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.genBusy == nil {
		h.genBusy = map[uint]int{}
	}
	if h.genN == nil {
		h.genN = map[uint][]time.Time{}
	}
	if h.genBusy[uid] >= 1 {
		return finance.NewError(429, "budget", "RATE_LIMITED", "策略生成并发为 1")
	}
	cut := now.Add(-time.Minute)
	kept := h.genN[uid][:0]
	for _, ts := range h.genN[uid] {
		if ts.After(cut) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= 10 {
		h.genN[uid] = kept
		return finance.NewError(429, "budget", "RATE_LIMITED", "策略生成每分钟最多 10 次")
	}
	h.genN[uid] = append(kept, now)
	h.genBusy[uid]++
	return nil
}

func (h *Hub) decGen(uid uint) {
	h.mu.Lock()
	if h.genBusy[uid] > 0 {
		h.genBusy[uid]--
	}
	h.mu.Unlock()
}

func (h *Hub) liveModel(ctx context.Context) (strategy.LiveInput, bool) {
	if h.Config == nil {
		return strategy.LiveInput{}, false
	}
	row, err := h.Config.Active(ctx, "model")
	if err != nil || len(row.SecretCiphertext) == 0 {
		return strategy.LiveInput{}, false
	}
	key, err := finance.DecryptSecret(row.SecretCiphertext)
	if err != nil || strings.TrimSpace(key) == "" {
		return strategy.LiveInput{}, false
	}
	var pub map[string]any
	_ = json.Unmarshal(row.PublicConfig, &pub)
	base, _ := pub["base_url"].(string)
	proto, _ := pub["protocol"].(string)
	modelName, _ := pub["model"].(string)
	if strings.TrimSpace(base) == "" {
		return strategy.LiveInput{}, false
	}
	return strategy.LiveInput{Protocol: proto, BaseURL: base, APIKey: key, Model: modelName, ConfigID: row.ID}, true
}

func (h *Hub) SyncCatalog(ctx context.Context) error {
	return h.Market.SyncLive(ctx)
}

func (h *Hub) Search(ctx context.Context, q, mkt, exchange, cursor string, limit int) (market.SearchResult, error) {
	return h.Market.Search(ctx, q, mkt, exchange, cursor, limit)
}

func (h *Hub) Instrument(ctx context.Context, id string) (map[string]any, error) {
	return h.Market.GetInstrument(ctx, id)
}

func (h *Hub) OHLCV(ctx context.Context, id, period, adjust, start, end, cursor string, limit int) (market.OHLCV, error) {
	return h.Market.GetOHLCV(ctx, id, period, adjust, start, end, cursor, limit)
}

func (h *Hub) Quote(ctx context.Context, id string) (market.QuoteSnapshot, error) {
	return h.Market.GetQuote(ctx, id)
}

func (h *Hub) Quotes(ctx context.Context, ids []string) ([]market.QuoteSnapshot, error) {
	return h.Market.GetQuotes(ctx, ids)
}

func (h *Hub) IndicatorRegistry() []indicators.Descriptor {
	return indicators.Registry()
}

func (h *Hub) IndicatorSeries(ctx context.Context, id, period, adjust string, specs []indicators.Spec) (map[string]any, error) {
	bars, snap, err := h.Market.IndicatorBars(ctx, id, period, adjust)
	if err != nil {
		return nil, err
	}
	series, err := indicators.ComputeMany(bars, specs)
	if err != nil {
		return nil, finance.NewError(400, "validation", "INVALID_PARAM", err.Error())
	}
	out := make([]map[string]any, 0, len(series))
	times := make([]string, len(bars))
	for i, b := range bars {
		times[i] = b.Time
	}
	for _, s := range series {
		fields := map[string]any{}
		for k, v := range s.Fields {
			fields[k] = indicators.Strings(v)
		}
		out = append(out, map[string]any{
			"id": s.ID, "type": s.Type, "params": s.Params, "fields": fields, "ready": s.Ready,
			"calculation_start": s.CalculationStart, "engine_version": s.EngineVersion,
		})
	}
	return map[string]any{
		"instrument_id": id, "period": period, "adjust": adjust, "data_snapshot_id": snap,
		"times": times, "series": out, "indicator_engine_version": indicators.EngineVersion,
	}, nil
}

func (h *Hub) GetWorkspace(ctx context.Context) (map[string]any, error) {
	uid := finance.UserIDFrom(ctx)
	var row model.Workspace
	if err := h.DB.WithContext(ctx).Where("owner_id = ?", uid).Take(&row).Error; err != nil {
		return map[string]any{"revision": 0, "watchlist": []any{}, "layout": map[string]any{}, "chart_indicators": []any{}}, nil
	}
	return map[string]any{"revision": row.Revision, "watchlist": json.RawMessage(row.Watchlist), "last_instrument_id": row.LastInstrumentID, "layout": json.RawMessage(row.Layout), "chart_indicators": json.RawMessage(row.ChartIndicators)}, nil
}

func (h *Hub) PutWorkspace(ctx context.Context, revision int, watchlist, layout, chart any, last *string) (map[string]any, error) {
	uid := finance.UserIDFrom(ctx)
	wJSON, _ := json.Marshal(watchlist)
	lJSON, _ := json.Marshal(layout)
	cJSON, _ := json.Marshal(chart)
	if wJSON == nil {
		wJSON = []byte("[]")
	}
	if lJSON == nil {
		lJSON = []byte("{}")
	}
	if cJSON == nil {
		cJSON = []byte("[]")
	}
	now := time.Now().UTC()
	var existing model.Workspace
	err := h.DB.WithContext(ctx).Where("owner_id = ?", uid).Take(&existing).Error
	if err == nil {
		if revision != existing.Revision {
			return nil, finance.NewError(409, "conflict", "REVISION_CONFLICT", "工作区版本冲突")
		}
		existing.Revision++
		existing.Watchlist, existing.Layout, existing.ChartIndicators = datatypes.JSON(wJSON), datatypes.JSON(lJSON), datatypes.JSON(cJSON)
		existing.LastInstrumentID, existing.UpdatedAt = last, now
		if err := h.DB.Save(&existing).Error; err != nil {
			return nil, err
		}
		return h.GetWorkspace(ctx)
	}
	row := model.Workspace{OwnerID: uid, Revision: 1, Watchlist: datatypes.JSON(wJSON), Layout: datatypes.JSON(lJSON), ChartIndicators: datatypes.JSON(cJSON), LastInstrumentID: last, UpdatedAt: now}
	if err := h.DB.Create(&row).Error; err != nil {
		return nil, err
	}
	return h.GetWorkspace(ctx)
}

type GenerateReq struct {
	Text         string `json:"text"`
	InstrumentID string `json:"instrument_id"`
	DraftID      string `json:"draft_id"`
	BaseRevision *int   `json:"base_revision"`
}

func (h *Hub) StartGenerate(ctx context.Context, idem string, req GenerateReq) (map[string]any, error) {
	uid := finance.UserIDFrom(ctx)
	if strings.TrimSpace(idem) == "" {
		return nil, finance.NewError(400, "validation", "INVALID_PARAM", "缺少 Idempotency-Key")
	}
	hash := finance.SHA256Text(req.Text + "|" + req.InstrumentID + "|" + req.DraftID)
	var existing model.Generation
	q := h.DB.WithContext(ctx).Where("owner_id = ? AND idempotency_key = ?", uid, idem).Take(&existing)
	if q.Error == nil {
		if existing.RequestHash != hash {
			return nil, finance.NewError(409, "conflict", "IDEMPOTENCY_CONFLICT", "相同幂等键对应不同请求")
		}
		return genAck(existing), nil
	}
	if err := h.allowGenerate(uid); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	draftID := req.DraftID
	if draftID == "" {
		draftID = httpx.NewID("sdr")
		d := model.Draft{ID: draftID, OwnerID: uid, Text: req.Text, Status: "queued", Revision: 1, Assumptions: datatypes.JSON([]byte("[]")), CapabilityErrors: datatypes.JSON([]byte("[]")), CreatedAt: now, UpdatedAt: now}
		if req.InstrumentID != "" {
			d.InstrumentID = &req.InstrumentID
		}
		if err := h.DB.Create(&d).Error; err != nil {
			h.decGen(uid)
			return nil, err
		}
	} else {
		var d model.Draft
		if err := h.DB.Where("id = ? AND owner_id = ?", draftID, uid).Take(&d).Error; err != nil {
			h.decGen(uid)
			return nil, finance.NewError(404, "not_found", "NOT_FOUND", "草稿不存在")
		}
		if req.BaseRevision != nil && *req.BaseRevision != d.Revision {
			h.decGen(uid)
			return nil, finance.NewError(409, "conflict", "REVISION_CONFLICT", "草稿版本冲突")
		}
	}
	gid := httpx.NewID("sgen")
	g := model.Generation{
		ID: gid, OwnerID: uid, DraftID: draftID, Status: "queued", Text: req.Text,
		IdempotencyKey: idem, RequestHash: hash, ExecutionEpoch: 1, CreatedAt: now, UpdatedAt: now,
	}
	if req.InstrumentID != "" {
		g.InstrumentID = &req.InstrumentID
	}
	g.BaseRevision = req.BaseRevision
	pv := strategy.PromptVersion
	g.PromptVersion = &pv
	if err := h.DB.Create(&g).Error; err != nil {
		h.decGen(uid)
		return nil, err
	}
	go func() {
		defer h.decGen(uid)
		h.runGenerate(gid)
	}()
	return genAck(g), nil
}

func genAck(g model.Generation) map[string]any {
	return map[string]any{"generation_id": g.ID, "draft_id": g.DraftID, "status": g.Status, "poll_url": "/api/finance/strategy-generations/" + g.ID}
}

func (h *Hub) runGenerate(id string) {
	var g model.Generation
	if err := h.DB.Where("id = ?", id).Take(&g).Error; err != nil {
		return
	}
	if g.Status == "canceled" {
		return
	}
	h.DB.Model(&g).Updates(map[string]any{"status": "generating", "updated_at": time.Now().UTC()})
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	inst := ""
	if g.InstrumentID != nil {
		inst = *g.InstrumentID
	}
	if inst == "" {
		inst = strategy.InferInstrumentID(g.Text)
	}
	if inst == "" {
		inst = h.resolveInstrument(ctx, g.Text)
	}
	out := strategy.GenerateFromText(strategy.GenerateInput{Text: g.Text, InstrumentID: inst})
	if live, ok := h.liveModel(ctx); ok {
		live.Text = g.Text
		live.InstrumentID = inst
		out = strategy.GenerateLive(ctx, live)
		if live.ConfigID != "" {
			cid := live.ConfigID
			g.ModelConfigVersion = &cid
		}
		proto := out.Source
		g.Protocol = &proto
	}
	h.DB.Model(&g).Updates(map[string]any{"status": "validating", "updated_at": time.Now().UTC()})
	var draft model.Draft
	if err := h.DB.Where("id = ?", g.DraftID).Take(&draft).Error; err != nil {
		return
	}
	now := time.Now().UTC()
	var latest model.Generation
	if err := h.DB.Where("id = ?", g.ID).Take(&latest).Error; err != nil {
		return
	}
	if latest.Status == "canceled" {
		return
	}
	draft.Status = out.Status
	draft.GenerationID = &g.ID
	draft.Revision++
	ass, _ := json.Marshal(out.Assumptions)
	draft.Assumptions = datatypes.JSON(ass)
	if out.Document != nil {
		raw, _ := json.Marshal(out.Document)
		draft.DSL = datatypes.JSON(raw)
		if c, err := strategy.Compile(*out.Document); err == nil {
			cr, _ := json.Marshal(c)
			draft.Compiled = datatypes.JSON(cr)
		}
	}
	if len(out.Questions) > 0 {
		q, _ := json.Marshal(out.Questions)
		draft.Clarification = datatypes.JSON(q)
	} else {
		draft.Clarification = datatypes.JSON([]byte("[]"))
	}
	if out.ErrorCode != "" {
		g.ErrorCode, g.ErrorMessage = &out.ErrorCode, &out.ErrorMessage
	}
	g.Status = out.Status
	src := out.Source
	if src == "" {
		src = "heuristic"
	}
	g.Protocol = &src
	if out.Status == "failed" && out.ErrorCode == "STRATEGY_UNSUPPORTED" {
		g.Status = "unsupported"
		draft.Status = "unsupported"
	}
	g.UpdatedAt, draft.UpdatedAt = now, now
	_ = h.DB.Save(&draft).Error
	_ = h.DB.Save(&g).Error
}

func (h *Hub) resolveInstrument(ctx context.Context, text string) string {
	if h.Market == nil {
		return ""
	}
	q := strategy.NameQuery(text)
	if q == "" || utf8.RuneCountInString(q) < 2 {
		return ""
	}
	res, err := h.Market.Search(ctx, q, "", "", "", 8)
	if err != nil {
		return ""
	}
	var cand []market.InstrumentView
	for _, it := range res.Items {
		if it.AssetType != "" && it.AssetType != "stock" {
			continue
		}
		if it.Name == q || strings.Contains(text, it.Name) {
			if it.DelistingDate != nil && *it.DelistingDate != "" {
				continue
			}
			if it.TradingStatus == "delisted" {
				continue
			}
			cand = append(cand, it)
		}
	}
	if len(cand) > 0 {
		return cand[0].InstrumentID
	}
	if len(res.Items) == 1 && (res.Items[0].AssetType == "" || res.Items[0].AssetType == "stock") {
		return res.Items[0].InstrumentID
	}
	return ""
}

func (h *Hub) GetGeneration(ctx context.Context, id string) (map[string]any, error) {
	uid := finance.UserIDFrom(ctx)
	var g model.Generation
	if err := h.DB.WithContext(ctx).Where("id = ? AND owner_id = ?", id, uid).Take(&g).Error; err != nil {
		return nil, finance.NewError(404, "not_found", "NOT_FOUND", "生成任务不存在")
	}
	var draft model.Draft
	_ = h.DB.Where("id = ?", g.DraftID).Take(&draft).Error
	return map[string]any{
		"generation_id": g.ID, "draft_id": g.DraftID, "status": g.Status, "draft_revision": draft.Revision,
		"dsl": json.RawMessage(draft.DSL), "assumptions": json.RawMessage(draft.Assumptions),
		"clarification": json.RawMessage(draft.Clarification), "compiled": json.RawMessage(draft.Compiled),
		"error_code": g.ErrorCode, "error_message": g.ErrorMessage, "prompt_version": g.PromptVersion,
		"source": g.Protocol, "model_config_version": g.ModelConfigVersion,
	}, nil
}

func (h *Hub) CancelGeneration(ctx context.Context, id string) error {
	uid := finance.UserIDFrom(ctx)
	var g model.Generation
	if err := h.DB.WithContext(ctx).Where("id = ? AND owner_id = ?", id, uid).Take(&g).Error; err != nil {
		return finance.NewError(404, "not_found", "NOT_FOUND", "生成任务不存在")
	}
	if g.Status == "ready" || g.Status == "failed" || g.Status == "unsupported" || g.Status == "canceled" || g.Status == "needs_clarification" {
		return nil
	}
	now := time.Now().UTC()
	return h.DB.Model(&g).Updates(map[string]any{"status": "canceled", "updated_at": now}).Error
}

func (h *Hub) GetDraft(ctx context.Context, id string) (map[string]any, error) {
	uid := finance.UserIDFrom(ctx)
	var d model.Draft
	if err := h.DB.WithContext(ctx).Where("id = ? AND owner_id = ?", id, uid).Take(&d).Error; err != nil {
		return nil, finance.NewError(404, "not_found", "NOT_FOUND", "草稿不存在")
	}
	return draftView(d), nil
}

func (h *Hub) ImportDraft(ctx context.Context, dsl json.RawMessage, inst, text string) (map[string]any, error) {
	uid := finance.UserIDFrom(ctx)
	doc, err := strategy.ParseDSL(dsl)
	if err != nil {
		return nil, err
	}
	if inst != "" {
		doc.InstrumentID = inst
	}
	c, err := strategy.Compile(doc)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(doc)
	cr, _ := json.Marshal(c)
	now := time.Now().UTC()
	id := httpx.NewID("sdr")
	d := model.Draft{
		ID: id, OwnerID: uid, Text: text, Status: "ready", Revision: 1,
		DSL: datatypes.JSON(raw), Compiled: datatypes.JSON(cr),
		Assumptions: datatypes.JSON([]byte("[]")), CapabilityErrors: datatypes.JSON([]byte("[]")),
		Clarification: datatypes.JSON([]byte("[]")), CreatedAt: now, UpdatedAt: now,
	}
	if doc.InstrumentID != "" {
		instID := doc.InstrumentID
		d.InstrumentID = &instID
	}
	if err := h.DB.Create(&d).Error; err != nil {
		return nil, err
	}
	return draftView(d), nil
}

func (h *Hub) PatchDraft(ctx context.Context, id string, revision int, dsl json.RawMessage) (map[string]any, error) {
	uid := finance.UserIDFrom(ctx)
	var d model.Draft
	if err := h.DB.WithContext(ctx).Where("id = ? AND owner_id = ?", id, uid).Take(&d).Error; err != nil {
		return nil, finance.NewError(404, "not_found", "NOT_FOUND", "草稿不存在")
	}
	if revision != d.Revision {
		return nil, finance.NewError(409, "conflict", "REVISION_CONFLICT", "草稿版本冲突")
	}
	doc, err := strategy.ParseDSL(dsl)
	if err != nil {
		return nil, err
	}
	c, err := strategy.Compile(doc)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(doc)
	cr, _ := json.Marshal(c)
	d.DSL, d.Compiled = datatypes.JSON(raw), datatypes.JSON(cr)
	d.Revision++
	d.Status = "ready"
	d.UpdatedAt = time.Now().UTC()
	if err := h.DB.Save(&d).Error; err != nil {
		return nil, err
	}
	return draftView(d), nil
}

func draftView(d model.Draft) map[string]any {
	return map[string]any{
		"draft_id": d.ID, "revision": d.Revision, "status": d.Status, "text": d.Text,
		"instrument_id": d.InstrumentID, "dsl": json.RawMessage(d.DSL),
		"assumptions": json.RawMessage(d.Assumptions), "compiled": json.RawMessage(d.Compiled),
		"clarification": json.RawMessage(d.Clarification),
	}
}

func (h *Hub) SaveStrategy(ctx context.Context, idem, draftID string, revision int, strategyID, baseVersion string) (map[string]any, error) {
	uid := finance.UserIDFrom(ctx)
	if idem == "" {
		return nil, finance.NewError(400, "validation", "INVALID_PARAM", "缺少 Idempotency-Key")
	}
	hash := finance.SHA256Text(draftID + "|" + strings.TrimSpace(strategyID) + "|" + itoa(revision))
	var rec model.Idempotency
	if err := h.DB.WithContext(ctx).Where("owner_id = ? AND operation = ? AND idempotency_key = ?", uid, "save_strategy", idem).Take(&rec).Error; err == nil {
		if rec.RequestHash != hash {
			return nil, finance.NewError(409, "conflict", "IDEMPOTENCY_CONFLICT", "相同幂等键对应不同请求")
		}
		var ver model.Version
		if err := h.DB.Where("id = ?", rec.ObjectID).Take(&ver).Error; err != nil {
			return nil, finance.NewError(404, "not_found", "NOT_FOUND", "策略版本不存在")
		}
		return map[string]any{"strategy_id": ver.StrategyID, "version_id": ver.ID, "revision": ver.Revision}, nil
	}
	var d model.Draft
	if err := h.DB.WithContext(ctx).Where("id = ? AND owner_id = ?", draftID, uid).Take(&d).Error; err != nil {
		return nil, finance.NewError(404, "not_found", "NOT_FOUND", "草稿不存在")
	}
	if d.Revision != revision || len(d.DSL) == 0 {
		return nil, finance.NewError(422, "validation", "STRATEGY_INVALID", "草稿未就绪或版本不匹配")
	}
	now := time.Now().UTC()
	if strategyID == "" {
		strategyID = httpx.NewID("st")
		st := model.Strategy{ID: strategyID, OwnerID: uid, Name: "未命名策略", CreatedAt: now, UpdatedAt: now}
		if err := h.DB.Create(&st).Error; err != nil {
			return nil, err
		}
	} else {
		var st model.Strategy
		if err := h.DB.Where("id = ? AND owner_id = ? AND deleted_at IS NULL", strategyID, uid).Take(&st).Error; err != nil {
			return nil, finance.NewError(404, "not_found", "NOT_FOUND", "策略不存在")
		}
	}
	var count int64
	h.DB.Model(&model.Version{}).Where("strategy_id = ?", strategyID).Count(&count)
	vid := httpx.NewID("stv")
	var name string
	_ = json.Unmarshal(d.DSL, &struct {
		Name *string `json:"name"`
	}{})
	var doc strategy.Document
	_ = json.Unmarshal(d.DSL, &doc)
	name = doc.Name
	ver := model.Version{
		ID: vid, StrategyID: strategyID, OwnerID: uid, Revision: int(count) + 1, Name: name,
		DSL: d.DSL, DSLHash: finance.SHA256Text(string(d.DSL)), Compiled: d.Compiled, CompilerVersion: strategy.CompilerVersion, CreatedAt: now,
	}
	if err := h.DB.Create(&ver).Error; err != nil {
		return nil, err
	}
	if err := h.DB.Model(&model.Strategy{}).Where("id = ?", strategyID).Updates(map[string]any{"current_version_id": vid, "name": name, "updated_at": now}).Error; err != nil {
		return nil, err
	}
	_ = h.DB.Create(&model.Idempotency{OwnerID: uid, Operation: "save_strategy", IdempotencyKey: idem, RequestHash: hash, ObjectID: vid, CreatedAt: now}).Error
	_ = baseVersion
	return map[string]any{"strategy_id": strategyID, "version_id": vid, "revision": ver.Revision}, nil
}

func itoa(v int) string {
	return strings.TrimSpace(strings.ReplaceAll(jsonNumber(v), "\n", ""))
}

func jsonNumber(v int) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (h *Hub) ListStrategies(ctx context.Context, limit int) (map[string]any, error) {
	uid := finance.UserIDFrom(ctx)
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var rows []model.Strategy
	if err := h.DB.WithContext(ctx).Where("owner_id = ? AND deleted_at IS NULL", uid).Order("updated_at desc").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		items = append(items, map[string]any{"strategy_id": r.ID, "name": r.Name, "current_version_id": r.CurrentVersionID, "updated_at": r.UpdatedAt})
	}
	return map[string]any{"items": items, "next_cursor": nil}, nil
}

func (h *Hub) GetStrategy(ctx context.Context, id, versionID string) (map[string]any, error) {
	uid := finance.UserIDFrom(ctx)
	var st model.Strategy
	if err := h.DB.WithContext(ctx).Where("id = ? AND owner_id = ? AND deleted_at IS NULL", id, uid).Take(&st).Error; err != nil {
		return nil, finance.NewError(404, "not_found", "NOT_FOUND", "策略不存在")
	}
	var vers []model.Version
	q := h.DB.Where("strategy_id = ?", id).Order("revision desc")
	if versionID != "" {
		q = h.DB.Where("strategy_id = ? AND id = ?", id, versionID)
	}
	_ = q.Find(&vers).Error
	var runs []model.BacktestRun
	_ = h.DB.Where("strategy_id = ? AND owner_id = ?", id, uid).Order("created_at desc").Limit(10).Find(&runs).Error
	return map[string]any{"strategy": st, "versions": vers, "recent_runs": runs}, nil
}

func (h *Hub) DeleteStrategy(ctx context.Context, id string) (map[string]any, error) {
	uid := finance.UserIDFrom(ctx)
	now := time.Now().UTC()
	res := h.DB.WithContext(ctx).Model(&model.Strategy{}).Where("id = ? AND owner_id = ? AND deleted_at IS NULL", id, uid).Updates(map[string]any{"deleted_at": now, "updated_at": now})
	if res.RowsAffected == 0 {
		return nil, finance.NewError(404, "not_found", "NOT_FOUND", "策略不存在")
	}
	h.DB.Model(&model.BacktestRun{}).Where("strategy_id = ? AND owner_id = ? AND status IN ?", id, uid, []string{"queued", "preparing_data", "running"}).
		Updates(map[string]any{"status": "canceled", "updated_at": now})
	return map[string]any{"strategy_id": id, "status": "deleting"}, nil
}

func (h *Hub) StartBacktest(ctx context.Context, idem string, cfg backtest.Config) (map[string]any, error) {
	uid := finance.UserIDFrom(ctx)
	if idem == "" {
		return nil, finance.NewError(400, "validation", "INVALID_PARAM", "缺少 Idempotency-Key")
	}
	raw, _ := json.Marshal(cfg)
	hash := finance.SHA256Text(string(raw))
	var existing model.BacktestRun
	if err := h.DB.WithContext(ctx).Where("owner_id = ? AND idempotency_key = ?", uid, idem).Take(&existing).Error; err == nil {
		if existing.RequestHash != hash {
			return nil, finance.NewError(409, "conflict", "IDEMPOTENCY_CONFLICT", "相同幂等键对应不同请求")
		}
		return btAck(existing), nil
	}
	var ver model.Version
	if err := h.DB.Where("id = ? AND owner_id = ?", cfg.StrategyVersionID, uid).Take(&ver).Error; err != nil {
		return nil, finance.NewError(404, "not_found", "NOT_FOUND", "策略版本不存在")
	}
	h.mu.Lock()
	if h.active[uid] >= 2 {
		h.mu.Unlock()
		return nil, finance.NewError(429, "budget", "RATE_LIMITED", "同时最多 2 个回测")
	}
	h.active[uid]++
	h.mu.Unlock()
	now := time.Now().UTC()
	id := httpx.NewID("bt")
	run := model.BacktestRun{
		ID: id, OwnerID: uid, StrategyID: ver.StrategyID, StrategyVersionID: ver.ID, Status: "queued",
		Progress: datatypes.JSON([]byte(`{"stage":"queued"}`)), Config: datatypes.JSON(raw), ConfigHash: hash,
		IdempotencyKey: idem, RequestHash: hash, ExecutionEpoch: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := h.DB.Create(&run).Error; err != nil {
		h.dec(uid)
		return nil, err
	}
	go h.runBacktest(id)
	return btAck(run), nil
}

func btAck(r model.BacktestRun) map[string]any {
	return map[string]any{"run_id": r.ID, "status": r.Status, "poll_url": "/api/finance/backtests/" + r.ID}
}

func (h *Hub) dec(uid uint) {
	h.mu.Lock()
	if h.active[uid] > 0 {
		h.active[uid]--
	}
	h.mu.Unlock()
}

func (h *Hub) runBacktest(id string) {
	var run model.BacktestRun
	if err := h.DB.Where("id = ?", id).Take(&run).Error; err != nil {
		return
	}
	defer h.dec(run.OwnerID)
	ctx := finance.WithUser(context.Background(), run.OwnerID, "user")
	h.DB.Model(&run).Updates(map[string]any{"status": "preparing_data", "progress": datatypes.JSON([]byte(`{"stage":"preparing_data"}`)), "updated_at": time.Now().UTC()})
	var cfg backtest.Config
	_ = json.Unmarshal(run.Config, &cfg)
	var ver model.Version
	if err := h.DB.Where("id = ?", run.StrategyVersionID).Take(&ver).Error; err != nil {
		h.failRun(run, "NOT_FOUND", "策略版本不存在")
		return
	}
	var doc strategy.Document
	if json.Unmarshal(ver.DSL, &doc) != nil {
		h.failRun(run, "STRATEGY_INVALID", "DSL 无法解析")
		return
	}
	compiled, err := strategy.Compile(doc)
	if err != nil {
		h.failRun(run, finance.ErrorCode(err), finance.ErrorMessage(err))
		return
	}
	inst, err := h.Market.Instrument(ctx, cfg.InstrumentID)
	if err != nil {
		h.failRun(run, finance.ErrorCode(err), finance.ErrorMessage(err))
		return
	}
	bars, snap, err := h.Market.IndicatorBars(ctx, cfg.InstrumentID, "1d", "raw")
	if err != nil {
		h.failRun(run, finance.ErrorCode(err), finance.ErrorMessage(err))
		return
	}
	if len(bars) > 10000 {
		h.failRun(run, "RANGE_TOO_LARGE", "超过 10000 根日线上限")
		return
	}
	rule := market.RuleFor(inst.Exchange, inst.Board, cfg.End)
	if inst.LotSize > 0 {
		rule.LotSize = inst.LotSize
	}
	if cfg.CommissionConfig != "" {
		rule.CommissionRate = cfg.CommissionConfig
	}
	actions, actErr := h.Market.Actions(ctx, cfg.InstrumentID)
	if actErr != nil && inst.Exchange != "HKEX" {
		h.failRun(run, finance.ErrorCode(actErr), finance.ErrorMessage(actErr))
		return
	}
	for _, a := range actions {
		if a.Kind == "unsupported" {
			h.failRun(run, "RULE_DATA_INCOMPLETE", "区间内存在未支持的公司行动，不能输出完整绩效")
			return
		}
	}
	cfg.DataSnapshotID = snap
	note := raiseCashForOneLot(&cfg, bars, rule.LotSize)
	if note != "" {
		if rawCfg, err := json.Marshal(cfg); err == nil {
			h.DB.Model(&run).Update("config", datatypes.JSON(rawCfg))
		}
	}
	h.DB.Model(&run).Updates(map[string]any{"status": "running", "progress": datatypes.JSON([]byte(`{"stage":"running","bars_total":` + itoa(len(bars)) + `}`)), "updated_at": time.Now().UTC()})
	res := backtest.Run(backtest.Input{
		Doc: doc, Compiled: compiled, Raw: bars, Instrument: inst, Rule: rule, Config: cfg, Actions: actions,
	})
	now := time.Now().UTC()
	if res.Status != "succeeded" {
		h.failRun(run, res.ErrorCode, res.Message)
		return
	}
	if note != "" {
		res.Assumptions = append(res.Assumptions, note)
		if res.Manifest == nil {
			res.Manifest = map[string]any{}
		}
		res.Manifest["initial_cash_raised"] = cfg.InitialCash
	}
	var latest model.BacktestRun
	if err := h.DB.Where("id = ?", run.ID).Take(&latest).Error; err != nil {
		return
	}
	if latest.Status == "canceled" {
		return
	}
	sum, _ := json.Marshal(res.Metrics)
	man, _ := json.Marshal(res.Manifest)
	h.persistBacktest(run.ID, res)
	h.DB.Model(&run).Updates(map[string]any{
		"status": "succeeded", "result_summary": datatypes.JSON(sum), "manifest": datatypes.JSON(man),
		"result_hash": res.ResultHash, "finished_at": now, "updated_at": now,
		"progress": datatypes.JSON([]byte(`{"stage":"succeeded"}`)),
	})
}

func (h *Hub) persistBacktest(runID string, res backtest.Result) {
	now := time.Now().UTC()
	idOf := map[string]string{}
	for i, o := range res.Orders {
		oid := httpx.NewID("ord")
		idOf[o.ID] = oid
		status := o.Status
		row := model.Order{ID: oid, RunID: runID, Side: o.Side, Status: status, Qty: o.Qty, CreatedAt: now}
		if o.SignalID != "" {
			s := o.SignalID
			row.SignalID = &s
		}
		if o.Submitted != "" {
			s := o.Submitted
			row.SubmittedDate = &s
		}
		if o.FillDate != "" {
			s := o.FillDate
			row.FillDate = &s
		}
		if o.Reason != "" {
			s := o.Reason
			row.Reason = &s
		}
		payload, _ := json.Marshal(o)
		row.Payload = datatypes.JSON(payload)
		_ = h.DB.Create(&row).Error
		_ = i
	}
	for _, f := range res.Fills {
		fees, _ := json.Marshal(f.Fees)
		oid := idOf[f.OrderID]
		if oid == "" {
			oid = httpx.NewID("ord")
		}
		payload, _ := json.Marshal(f)
		_ = h.DB.Create(&model.Fill{ID: httpx.NewID("fill"), RunID: runID, OrderID: oid, FillDate: f.Date, Qty: f.Qty, Price: f.Price, Fees: datatypes.JSON(fees), CashDelta: f.CashDelta, Payload: datatypes.JSON(payload), CreatedAt: now}).Error
	}
	for _, e := range res.Equity {
		dd := e.Drawdown
		_ = h.DB.Create(&model.EquityRow{RunID: runID, TradeDate: e.Date, Cash: e.Cash, PositionQty: e.Qty, MarketValue: e.MarketValue, Equity: e.Equity, Drawdown: &dd}).Error
	}
	metrics, _ := json.Marshal(res.Metrics)
	sigs, _ := json.Marshal(res.Signals)
	ass, _ := json.Marshal(res.Assumptions)
	qual, _ := json.Marshal(res.Quality)
	_ = h.DB.Create(&model.Result{RunID: runID, Metrics: datatypes.JSON(metrics), Signals: datatypes.JSON(sigs), Assumptions: datatypes.JSON(ass), Quality: datatypes.JSON(qual), ResultHash: res.ResultHash, CreatedAt: now}).Error
}

func (h *Hub) failRun(run model.BacktestRun, code, msg string) {
	var latest model.BacktestRun
	if err := h.DB.Where("id = ?", run.ID).Take(&latest).Error; err != nil {
		return
	}
	if latest.Status == "canceled" {
		return
	}
	now := time.Now().UTC()
	status := "failed"
	if code == "INSUFFICIENT_HISTORY" || code == "RULE_DATA_INCOMPLETE" {
		status = "insufficient_data"
	}
	h.DB.Model(&run).Updates(map[string]any{"status": status, "error_code": code, "error_message": msg, "finished_at": now, "updated_at": now})
}

func (h *Hub) GetBacktest(ctx context.Context, id string) (map[string]any, error) {
	uid := finance.UserIDFrom(ctx)
	var run model.BacktestRun
	if err := h.DB.WithContext(ctx).Where("id = ? AND owner_id = ?", id, uid).Take(&run).Error; err != nil {
		return nil, finance.NewError(404, "not_found", "NOT_FOUND", "回测不存在")
	}
	return map[string]any{
		"run_id": run.ID, "status": run.Status, "progress": json.RawMessage(run.Progress), "manifest": json.RawMessage(run.Manifest),
		"result_summary": json.RawMessage(run.ResultSummary), "error_code": run.ErrorCode, "error_message": run.ErrorMessage,
		"result_hash": run.ResultHash, "strategy_version_id": run.StrategyVersionID,
	}, nil
}

func (h *Hub) GetResults(ctx context.Context, id string) (map[string]any, error) {
	uid := finance.UserIDFrom(ctx)
	var run model.BacktestRun
	if err := h.DB.WithContext(ctx).Where("id = ? AND owner_id = ?", id, uid).Take(&run).Error; err != nil {
		return nil, finance.NewError(404, "not_found", "NOT_FOUND", "回测不存在")
	}
	if run.Status != "succeeded" {
		return nil, finance.NewError(409, "conflict", "RUN_NOT_SUCCEEDED", "回测未成功，没有完整绩效")
	}
	var res model.Result
	if err := h.DB.Where("run_id = ?", id).Take(&res).Error; err != nil {
		return nil, finance.NewError(409, "conflict", "RUN_NOT_SUCCEEDED", "结果未发布")
	}
	var eq []model.EquityRow
	_ = h.DB.Where("run_id = ?", id).Order("trade_date asc").Find(&eq).Error
	return map[string]any{"run_id": id, "metrics": json.RawMessage(res.Metrics), "equity": eq, "signals": json.RawMessage(res.Signals), "assumptions": json.RawMessage(res.Assumptions), "quality": json.RawMessage(res.Quality), "result_hash": res.ResultHash, "manifest": json.RawMessage(run.Manifest)}, nil
}

func (h *Hub) GetTrades(ctx context.Context, id string) (map[string]any, error) {
	uid := finance.UserIDFrom(ctx)
	var run model.BacktestRun
	if err := h.DB.WithContext(ctx).Where("id = ? AND owner_id = ?", id, uid).Take(&run).Error; err != nil {
		return nil, finance.NewError(404, "not_found", "NOT_FOUND", "回测不存在")
	}
	var orders []model.Order
	var fills []model.Fill
	_ = h.DB.Where("run_id = ?", id).Order("created_at asc").Find(&orders).Error
	_ = h.DB.Where("run_id = ?", id).Order("fill_date asc").Find(&fills).Error
	return map[string]any{"orders": orders, "fills": fills}, nil
}

func (h *Hub) ListBacktests(ctx context.Context, strategyID, status string, limit int) (map[string]any, error) {
	uid := finance.UserIDFrom(ctx)
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	q := h.DB.WithContext(ctx).Where("owner_id = ?", uid)
	if strategyID != "" {
		q = q.Where("strategy_id = ?", strategyID)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var rows []model.BacktestRun
	_ = q.Order("created_at desc").Limit(limit).Find(&rows).Error
	return map[string]any{"items": rows, "next_cursor": nil}, nil
}

func (h *Hub) CancelBacktest(ctx context.Context, id string) error {
	uid := finance.UserIDFrom(ctx)
	var run model.BacktestRun
	if err := h.DB.WithContext(ctx).Where("id = ? AND owner_id = ?", id, uid).Take(&run).Error; err != nil {
		return finance.NewError(404, "not_found", "NOT_FOUND", "回测不存在")
	}
	if run.Status == "succeeded" || run.Status == "failed" || run.Status == "canceled" || run.Status == "insufficient_data" {
		return nil
	}
	now := time.Now().UTC()
	return h.DB.Model(&run).Updates(map[string]any{"status": "canceled", "updated_at": now, "finished_at": now}).Error
}

func (h *Hub) Export(ctx context.Context, id string) (string, error) {
	uid := finance.UserIDFrom(ctx)
	var run model.BacktestRun
	if err := h.DB.WithContext(ctx).Where("id = ? AND owner_id = ?", id, uid).Take(&run).Error; err != nil {
		return "", finance.NewError(404, "not_found", "NOT_FOUND", "回测不存在")
	}
	if run.Status != "succeeded" {
		return "", finance.NewError(409, "conflict", "RUN_NOT_SUCCEEDED", "回测未成功")
	}
	var fills []model.Fill
	_ = h.DB.Where("run_id = ?", id).Order("fill_date").Find(&fills).Error
	var b strings.Builder
	b.WriteString("date,order_id,qty,price,cash_delta\n")
	for _, f := range fills {
		b.WriteString(f.FillDate + "," + f.OrderID + "," + f.Qty + "," + f.Price + "," + f.CashDelta + "\n")
	}
	return b.String(), nil
}

func raiseCashForOneLot(cfg *backtest.Config, bars []indicators.Bar, lot int) string {
	if cfg == nil || lot <= 0 {
		return ""
	}
	cash := market.MustDec(cfg.InitialCash)
	if !cash.GreaterThan(decimal.Zero) {
		return ""
	}
	px := decimal.Zero
	for _, b := range bars {
		if cfg.Start != "" && b.Time < cfg.Start {
			continue
		}
		if cfg.End != "" && b.Time > cfg.End {
			break
		}
		if b.Close.GreaterThan(decimal.Zero) {
			px = b.Close
			break
		}
	}
	if !px.GreaterThan(decimal.Zero) {
		return ""
	}
	need := px.Mul(decimal.NewFromInt(int64(lot)))
	if cash.GreaterThanOrEqual(need) {
		return ""
	}
	cfg.InitialCash = need.Mul(decimal.NewFromInt(2)).Ceil().String()
	return "初始资金不足以买入 1 手，已上调为 " + cfg.InitialCash + " 以便完成回测"
}
