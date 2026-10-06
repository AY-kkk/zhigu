package futures

import (
	"testing"

	"github.com/gin-gonic/gin"
	svc "zhigu/server/service/futures"
)

func TestPublicRouteMatrixMatchesContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	Register(router, svc.NewService(false))
	got := map[string]bool{}
	for _, route := range router.Routes() {
		got[route.Method+" "+route.Path] = true
	}
	want := []string{
		"GET /api/v1/futures/capabilities",
		"GET /api/v1/futures/products",
		"GET /api/v1/futures/products/:id/contracts",
		"GET /api/v1/futures/products/:id/workbench",
		"POST /api/v1/futures/drafts",
		"GET /api/v1/futures/drafts/:id",
		"PATCH /api/v1/futures/drafts/:id",
		"DELETE /api/v1/futures/drafts/:id",
		"POST /api/v1/futures/drafts/:id/parse",
		"POST /api/v1/futures/documents",
		"GET /api/v1/futures/documents/:id",
		"DELETE /api/v1/futures/documents/:id",
		"POST /api/v1/futures/runs",
		"GET /api/v1/futures/runs",
		"GET /api/v1/futures/runs/:id",
		"DELETE /api/v1/futures/runs/:id",
		"POST /api/v1/futures/runs/:id/cancel",
		"GET /api/v1/futures/runs/:id/evidence/:evidenceId",
		"GET /api/v1/futures/runs/:id/export",
		"GET /api/v1/futures/runs/:id/delete-impact",
		"DELETE /api/v1/futures/hypotheses/:id",
		"GET /api/v1/futures/hypotheses/:id",
		"PATCH /api/v1/futures/hypotheses/:id",
		"POST /api/v1/futures/hypotheses",
		"GET /api/v1/futures/hypotheses",
		"GET /api/v1/futures/hypotheses/:id/checks",
		"POST /api/v1/futures/hypotheses/:id/checks",
		"GET /api/v1/futures/hypotheses/:id/recap",
		"PUT /api/v1/futures/hypotheses/:id/recap",
		"GET /api/v1/futures/watchlist",
		"PUT /api/v1/futures/watchlist",
		"GET /api/v1/futures/notifications",
		"PATCH /api/v1/futures/notifications/:id",
		"GET /api/v1/admin/futures/sources",
		"POST /api/v1/admin/futures/sources",
		"PATCH /api/v1/admin/futures/sources/:id",
		"GET /api/v1/admin/futures/admission",
		"PUT /api/v1/admin/futures/admission/:productId",
		"GET /api/v1/admin/futures/operations",
		"PATCH /api/v1/admin/futures/operations",
		"GET /api/v1/futures/drafts/:id/claims",
		"POST /api/v1/admin/futures/sources/:id/versions",
	}
	for _, route := range want {
		if !got[route] {
			t.Errorf("missing route %s", route)
		}
	}
	if len(want) != 42 {
		t.Fatalf("contract matrix count=%d", len(want))
	}
}
