package finance

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"zhigu/server/httpx"
	strategymarket "zhigu/server/service/strategy_market"
)

// RegisterStrategyMarket wires the consumer market API and the protected admin
// content-maintenance API (§12.5 / §12.6).
func RegisterStrategyMarket(engine *gin.Engine, svc *strategymarket.Service) {
	pub := engine.Group("/api/finance/strategy-market")
	pub.Use(httpx.AuthRequired())
	pub.GET("/items", func(c *gin.Context) { marketItems(c, svc) })
	pub.GET("/items/:id", func(c *gin.Context) { marketItemDetail(c, svc) })
	pub.GET("/items/:id/evidence/:evidenceId", func(c *gin.Context) { marketItemEvidence(c, svc) })
	pub.POST("/items/:id/copies", func(c *gin.Context) { marketItemCopy(c, svc) })

	adm := engine.Group("/api/admin/strategy-market")
	adm.Use(httpx.AuthRequired(), httpx.AdminRequired())
	adm.POST("/items", func(c *gin.Context) { adminCreateItem(c, svc) })
	adm.POST("/items/:id/versions", func(c *gin.Context) { adminCreateVersion(c, svc) })
	adm.POST("/items/:id/versions/:versionId/validate", func(c *gin.Context) { adminValidate(c, svc) })
	adm.POST("/items/:id/publish", func(c *gin.Context) { adminPublish(c, svc) })
	adm.POST("/items/:id/withdraw", func(c *gin.Context) { adminWithdraw(c, svc) })
	adm.POST("/items/:id/versions/:versionId/evidence", func(c *gin.Context) { adminImportEvidence(c, svc) })
	adm.POST("/items/:id/evidence/:evidenceId/review", func(c *gin.Context) { adminReviewEvidence(c, svc) })
}

func writeOp(c *gin.Context, out map[string]any, replayed bool, err error) {
	if err != nil {
		fail(c, err)
		return
	}
	status := http.StatusOK
	if !replayed {
		status = http.StatusCreated
	}
	httpx.OK(c, status, out)
}

func marketItems(c *gin.Context, svc *strategymarket.Service) {
	q := strings.TrimSpace(c.Query("q"))
	if utf8.RuneCountInString(q) > 100 {
		httpx.Fail(c, http.StatusBadRequest, "INVALID_PARAM", "q 最多 100 字")
		return
	}
	limit := 20
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			httpx.Fail(c, http.StatusBadRequest, "INVALID_PARAM", "limit 非法")
			return
		}
		limit = n
	}
	out, err := svc.List(c.Request.Context(), strategymarket.ListQuery{
		Q: q, Category: c.Query("category"), Market: c.Query("market"),
		Period: c.Query("period"), Validation: c.Query("validation"),
		Evidence: c.Query("evidence"), Cursor: c.Query("cursor"), Limit: limit,
	})
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func marketItemDetail(c *gin.Context, svc *strategymarket.Service) {
	out, err := svc.Detail(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func marketItemEvidence(c *gin.Context, svc *strategymarket.Service) {
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			httpx.Fail(c, http.StatusBadRequest, "INVALID_PARAM", "limit 非法")
			return
		}
		limit = n
	}
	out, err := svc.EvidenceSection(c.Request.Context(), c.Param("id"), c.Param("evidenceId"),
		c.DefaultQuery("section", "overview"), c.Query("cursor"), limit)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func marketItemCopy(c *gin.Context, svc *strategymarket.Service) {
	var req strategymarket.CopyReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, replayed, err := svc.Copy(c.Request.Context(), httpx.CurrentUserID(c), c.GetHeader("Idempotency-Key"), c.Param("id"), req)
	writeOp(c, out, replayed, err)
}

func adminCreateItem(c *gin.Context, svc *strategymarket.Service) {
	var req strategymarket.CreateItemReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, replayed, err := svc.CreateItem(c.Request.Context(), httpx.CurrentUserID(c), c.GetHeader("Idempotency-Key"), req)
	writeOp(c, out, replayed, err)
}

func adminCreateVersion(c *gin.Context, svc *strategymarket.Service) {
	var req strategymarket.CreateVersionReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, replayed, err := svc.CreateVersion(c.Request.Context(), httpx.CurrentUserID(c), c.GetHeader("Idempotency-Key"), c.Param("id"), req)
	writeOp(c, out, replayed, err)
}

func adminValidate(c *gin.Context, svc *strategymarket.Service) {
	var req strategymarket.ValidateReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, replayed, err := svc.Validate(c.Request.Context(), httpx.CurrentUserID(c), c.GetHeader("Idempotency-Key"), c.Param("id"), c.Param("versionId"), req)
	writeOp(c, out, replayed, err)
}

func adminPublish(c *gin.Context, svc *strategymarket.Service) {
	var req strategymarket.PublishReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, replayed, err := svc.Publish(c.Request.Context(), httpx.CurrentUserID(c), c.GetHeader("Idempotency-Key"), c.Param("id"), req)
	writeOp(c, out, replayed, err)
}

func adminWithdraw(c *gin.Context, svc *strategymarket.Service) {
	var req strategymarket.WithdrawReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, replayed, err := svc.Withdraw(c.Request.Context(), httpx.CurrentUserID(c), c.GetHeader("Idempotency-Key"), c.Param("id"), req)
	writeOp(c, out, replayed, err)
}

func adminImportEvidence(c *gin.Context, svc *strategymarket.Service) {
	var req strategymarket.EvidenceImportReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, replayed, err := svc.ImportEvidence(c.Request.Context(), httpx.CurrentUserID(c), c.GetHeader("Idempotency-Key"), c.Param("id"), c.Param("versionId"), req)
	writeOp(c, out, replayed, err)
}

func adminReviewEvidence(c *gin.Context, svc *strategymarket.Service) {
	var req strategymarket.EvidenceReviewReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	out, replayed, err := svc.ReviewEvidence(c.Request.Context(), httpx.CurrentUserID(c), c.GetHeader("Idempotency-Key"), c.Param("id"), c.Param("evidenceId"), req)
	writeOp(c, out, replayed, err)
}
