package futures

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"zhigu/server/httpx"
	svc "zhigu/server/service/futures"
)

// CapabilityReader is deliberately narrower than the future research service.
type CapabilityReader interface{ Capabilities() svc.Capabilities }

// Register is not called by main.go yet. Keep this endpoint registered when off
// after host integration; later privacy cleanup routes must also survive off.
func Register(engine *gin.Engine, service CapabilityReader) {
	group := engine.Group("/api/v1/futures", httpx.AuthRequired())
	group.GET("/capabilities", func(c *gin.Context) {
		payload := service.Capabilities()
		if concrete, ok := service.(*svc.Service); ok && concrete.Domain() != nil {
			concrete.Domain().FillCapabilities(c.Request.Context(), serviceIdentity(c), &payload)
		}
		httpx.OK(c, http.StatusOK, payload)
	})
	pending := func(c *gin.Context) {
		httpx.Fail(c, http.StatusServiceUnavailable, "FUTURES_NOT_READY", "期货研究业务链路尚未就绪")
	}
	handler := pending
	if concrete, ok := service.(*svc.Service); ok && concrete.Domain() != nil {
		handler = func(c *gin.Context) { dispatchBusiness(c, concrete.Domain()) }
	}
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/products"},
		{http.MethodGet, "/products/:id/contracts"},
		{http.MethodGet, "/products/:id/workbench"},
		{http.MethodPost, "/drafts"},
		{http.MethodGet, "/drafts/:id"},
		{http.MethodPatch, "/drafts/:id"},
		{http.MethodDelete, "/drafts/:id"},
		{http.MethodPost, "/drafts/:id/parse"},
		{http.MethodGet, "/drafts/:id/claims"},
		{http.MethodPost, "/documents"},
		{http.MethodGet, "/documents/:id"},
		{http.MethodDelete, "/documents/:id"},
		{http.MethodPost, "/runs"},
		{http.MethodGet, "/runs"},
		{http.MethodGet, "/runs/:id"},
		{http.MethodDelete, "/runs/:id"},
		{http.MethodPost, "/runs/:id/cancel"},
		{http.MethodGet, "/runs/:id/evidence/:evidenceId"},
		{http.MethodGet, "/runs/:id/export"},
		{http.MethodGet, "/runs/:id/delete-impact"},
		{http.MethodPost, "/hypotheses"},
		{http.MethodGet, "/hypotheses"},
		{http.MethodGet, "/hypotheses/:id"},
		{http.MethodPatch, "/hypotheses/:id"},
		{http.MethodDelete, "/hypotheses/:id"},
		{http.MethodGet, "/hypotheses/:id/checks"},
		{http.MethodPost, "/hypotheses/:id/checks"},
		{http.MethodGet, "/hypotheses/:id/recap"},
		{http.MethodPut, "/hypotheses/:id/recap"},
		{http.MethodGet, "/watchlist"},
		{http.MethodPut, "/watchlist"},
		{http.MethodGet, "/notifications"},
		{http.MethodPatch, "/notifications/:id"},
	} {
		group.Handle(route.method, route.path, handler)
	}
	admin := engine.Group("/api/v1/admin/futures", httpx.AuthRequired(), httpx.AdminRequired())
	adminHandler := pending
	if concrete, ok := service.(*svc.Service); ok && concrete.Domain() != nil {
		adminHandler = func(c *gin.Context) { dispatchAdmin(c, concrete.Domain()) }
	}
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/sources"},
		{http.MethodPost, "/sources"},
		{http.MethodPatch, "/sources/:id"},
		{http.MethodPost, "/sources/:id/versions"},
		{http.MethodGet, "/admission"},
		{http.MethodPut, "/admission/:productId"},
		{http.MethodGet, "/operations"},
		{http.MethodPatch, "/operations"},
	} {
		admin.Handle(route.method, route.path, adminHandler)
	}
}
