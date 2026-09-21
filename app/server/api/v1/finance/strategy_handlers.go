package finance

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"zhigu/server/httpx"
	"zhigu/server/service/backtest"
	svcfinance "zhigu/server/service/finance"
	"zhigu/server/service/indicators"
	"zhigu/server/service/workbench"
)

func RegisterStrategy(engine *gin.Engine, hub *workbench.Hub) {
	g := engine.Group("/api/finance")
	g.Use(httpx.AuthRequired())
	g.GET("/market/instruments", func(c *gin.Context) { strategySearch(c, hub) })
	g.GET("/market/instruments/*id", func(c *gin.Context) { strategyInstrument(c, hub) })
	g.GET("/market/ohlcv", func(c *gin.Context) { strategyOHLCV(c, hub) })
	g.GET("/market/quotes", func(c *gin.Context) { strategyQuotes(c, hub) })
	g.GET("/market/quote", func(c *gin.Context) { strategyQuote(c, hub) })
	g.GET("/market/indicators", func(c *gin.Context) {
		httpx.OK(c, http.StatusOK, gin.H{"items": hub.IndicatorRegistry(), "engine_version": indicators.EngineVersion})
	})
	g.POST("/market/catalog/sync", httpx.AdminRequired(), func(c *gin.Context) {
		if err := hub.SyncCatalog(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c))); err != nil {
			fail(c, err)
			return
		}
		httpx.OK(c, http.StatusAccepted, gin.H{"status": "syncing_done"})
	})
	g.POST("/market/indicator-series", func(c *gin.Context) { strategyIndicatorSeries(c, hub) })
	g.GET("/strategies/workspace", func(c *gin.Context) { strategyWorkspaceGet(c, hub) })
	g.PUT("/strategies/workspace", func(c *gin.Context) { strategyWorkspacePut(c, hub) })
	g.POST("/strategy-drafts/generate", func(c *gin.Context) { strategyGenerate(c, hub) })
	g.POST("/strategy-drafts", func(c *gin.Context) { strategyDraftImport(c, hub) })
	g.GET("/strategy-generations/:id", func(c *gin.Context) { strategyGeneration(c, hub) })
	g.POST("/strategy-generations/:id/cancel", func(c *gin.Context) { strategyGenCancel(c, hub) })
	g.GET("/strategy-drafts/:id", func(c *gin.Context) { strategyDraftGet(c, hub) })
	g.PATCH("/strategy-drafts/:id", func(c *gin.Context) { strategyDraftPatch(c, hub) })
	g.POST("/strategies", func(c *gin.Context) { strategySave(c, hub, "") })
	g.POST("/strategies/:id/versions", func(c *gin.Context) { strategySave(c, hub, c.Param("id")) })
	g.GET("/strategies", func(c *gin.Context) { strategyList(c, hub) })
	g.GET("/strategies/:id", func(c *gin.Context) { strategyGet(c, hub) })
	g.DELETE("/strategies/:id", func(c *gin.Context) { strategyDelete(c, hub) })
	g.POST("/backtests", func(c *gin.Context) { backtestCreate(c, hub) })
	g.GET("/backtests", func(c *gin.Context) { backtestList(c, hub) })
	g.GET("/backtests/:id", func(c *gin.Context) { backtestGet(c, hub) })
	g.GET("/backtests/:id/results", func(c *gin.Context) { backtestResults(c, hub) })
	g.GET("/backtests/:id/trades", func(c *gin.Context) { backtestTrades(c, hub) })
	g.POST("/backtests/:id/cancel", func(c *gin.Context) { backtestCancel(c, hub) })
	g.GET("/backtests/:id/export", func(c *gin.Context) { backtestExport(c, hub) })
}

func strategySearch(c *gin.Context, hub *workbench.Hub) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	out, err := hub.Search(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), c.Query("q"), c.Query("market"), c.Query("exchange"), c.Query("cursor"), limit)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func strategyInstrument(c *gin.Context, hub *workbench.Hub) {
	id := strings.TrimPrefix(c.Param("id"), "/")
	out, err := hub.Instrument(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), id)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func strategyOHLCV(c *gin.Context, hub *workbench.Hub) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "250"))
	out, err := hub.OHLCV(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), c.Query("instrument_id"), c.DefaultQuery("period", "1d"), c.DefaultQuery("adjust", "raw"), c.Query("start"), c.Query("end"), c.Query("cursor"), limit)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func strategyQuote(c *gin.Context, hub *workbench.Hub) {
	out, err := hub.Quote(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), c.Query("instrument_id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func strategyQuotes(c *gin.Context, hub *workbench.Hub) {
	raw := strings.TrimSpace(c.Query("ids"))
	var ids []string
	if raw != "" {
		ids = strings.Split(raw, ",")
	}
	out, err := hub.Quotes(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), ids)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"items": out})
}

func strategyIndicatorSeries(c *gin.Context, hub *workbench.Hub) {
	var req struct {
		InstrumentID string            `json:"instrument_id"`
		Period       string            `json:"period"`
		Adjust       string            `json:"adjust"`
		Indicators   []indicators.Spec `json:"indicators"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, err := hub.IndicatorSeries(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), req.InstrumentID, req.Period, req.Adjust, req.Indicators)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func strategyWorkspaceGet(c *gin.Context, hub *workbench.Hub) {
	out, err := hub.GetWorkspace(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func strategyWorkspacePut(c *gin.Context, hub *workbench.Hub) {
	var req struct {
		Revision         int     `json:"revision"`
		Watchlist        any     `json:"watchlist"`
		Layout           any     `json:"layout"`
		ChartIndicators  any     `json:"chart_indicators"`
		LastInstrumentID *string `json:"last_instrument_id"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, err := hub.PutWorkspace(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), req.Revision, req.Watchlist, req.Layout, req.ChartIndicators, req.LastInstrumentID)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func strategyGenerate(c *gin.Context, hub *workbench.Hub) {
	var req workbench.GenerateReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, err := hub.StartGenerate(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), c.GetHeader("Idempotency-Key"), req)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusAccepted, out)
}

func strategyGeneration(c *gin.Context, hub *workbench.Hub) {
	out, err := hub.GetGeneration(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func strategyGenCancel(c *gin.Context, hub *workbench.Hub) {
	if err := hub.CancelGeneration(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"status": "canceled"})
}

func strategyDraftImport(c *gin.Context, hub *workbench.Hub) {
	var req struct {
		DSL          json.RawMessage `json:"dsl"`
		InstrumentID string          `json:"instrument_id"`
		Text         string          `json:"text"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, err := hub.ImportDraft(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), req.DSL, req.InstrumentID, req.Text)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func strategyDraftGet(c *gin.Context, hub *workbench.Hub) {
	out, err := hub.GetDraft(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func strategyDraftPatch(c *gin.Context, hub *workbench.Hub) {
	var req struct {
		Revision int             `json:"revision"`
		DSL      json.RawMessage `json:"dsl"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, err := hub.PatchDraft(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), c.Param("id"), req.Revision, req.DSL)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func strategySave(c *gin.Context, hub *workbench.Hub, strategyID string) {
	var req struct {
		DraftID       string `json:"draft_id"`
		Revision      int    `json:"revision"`
		BaseVersionID string `json:"base_version_id"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, err := hub.SaveStrategy(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), c.GetHeader("Idempotency-Key"), req.DraftID, req.Revision, strategyID, req.BaseVersionID)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func strategyList(c *gin.Context, hub *workbench.Hub) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	out, err := hub.ListStrategies(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), limit)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func strategyGet(c *gin.Context, hub *workbench.Hub) {
	out, err := hub.GetStrategy(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), c.Param("id"), c.Query("version_id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func strategyDelete(c *gin.Context, hub *workbench.Hub) {
	out, err := hub.DeleteStrategy(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusAccepted, out)
}

func backtestCreate(c *gin.Context, hub *workbench.Hub) {
	var cfg backtest.Config
	if !httpx.BindJSON(c, &cfg) {
		return
	}
	out, err := hub.StartBacktest(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), c.GetHeader("Idempotency-Key"), cfg)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusAccepted, out)
}

func backtestGet(c *gin.Context, hub *workbench.Hub) {
	out, err := hub.GetBacktest(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func backtestResults(c *gin.Context, hub *workbench.Hub) {
	out, err := hub.GetResults(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func backtestTrades(c *gin.Context, hub *workbench.Hub) {
	out, err := hub.GetTrades(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func backtestList(c *gin.Context, hub *workbench.Hub) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	out, err := hub.ListBacktests(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), c.Query("strategy_id"), c.Query("status"), limit)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func backtestCancel(c *gin.Context, hub *workbench.Hub) {
	if err := hub.CancelBacktest(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"status": "canceled"})
}

func backtestExport(c *gin.Context, hub *workbench.Hub) {
	csv, err := hub.Export(svcfinance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c)), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.String(http.StatusOK, csv)
}
