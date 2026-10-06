package futures

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"zhigu/server/httpx"
	model "zhigu/server/model/futures"
	svc "zhigu/server/service/futures"
)

func serviceIdentity(c *gin.Context) svc.Identity {
	return svc.Identity{OwnerID: httpx.CurrentUserID(c), Mode: "live"}
}

func writeDomainError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, svc.ErrNotFound):
		httpx.Fail(c, http.StatusNotFound, "FUTURES_NOT_FOUND", "对象不存在")
	case errors.Is(err, svc.ErrRevisionConflict):
		httpx.Fail(c, http.StatusConflict, "FUTURES_REVISION_CONFLICT", "版本已变化")
	case errors.Is(err, svc.ErrIdempotencyConflict):
		httpx.Fail(c, http.StatusConflict, "FUTURES_IDEMPOTENCY_CONFLICT", "同一幂等键对应不同请求")
	case errors.Is(err, svc.ErrReferenced):
		httpx.Fail(c, http.StatusConflict, "FUTURES_REFERENCED", "对象仍被研究引用")
	case errors.Is(err, svc.ErrQuotaExceeded):
		httpx.Fail(c, http.StatusTooManyRequests, "FUTURES_QUOTA_EXCEEDED", "期货研究额度已用尽")
	case errors.Is(err, svc.ErrForbidden):
		httpx.Fail(c, http.StatusForbidden, "FUTURES_NOT_ADMITTED", "品种或数据未准入")
	case errors.Is(err, svc.ErrReadOnly):
		httpx.Fail(c, http.StatusForbidden, "FUTURES_READ_ONLY", "当前为只读模式")
	case errors.Is(err, svc.ErrModuleOff):
		httpx.Fail(c, http.StatusServiceUnavailable, "FUTURES_OFF", "期货研究已停用")
	case errors.Is(err, svc.ErrInvalidInput):
		httpx.Fail(c, http.StatusBadRequest, "FUTURES_INVALID_INPUT", "输入不符合 futures v1 契约")
	case errors.Is(err, svc.ErrDocumentLimit):
		httpx.Fail(c, http.StatusRequestEntityTooLarge, "FUTURES_DOCUMENT_LIMIT", "文档超过限制")
	case errors.Is(err, svc.ErrDocumentUnreadable):
		httpx.Fail(c, http.StatusUnprocessableEntity, "FUTURES_DOCUMENT_UNREADABLE", "文档类型、编码或文本层不可读")
	case errors.Is(err, svc.ErrDocumentTimeout):
		httpx.Fail(c, http.StatusGatewayTimeout, "FUTURES_DOCUMENT_EXTRACTION_TIMEOUT", "文档提取超时")
	default:
		httpx.Fail(c, http.StatusServiceUnavailable, "FUTURES_NOT_READY", "期货研究运行依赖未就绪")
	}
}

func dispatchBusiness(c *gin.Context, domain *svc.Domain) {
	id := serviceIdentity(c)
	path := c.FullPath()
	if err := domain.AuthorizeAccess(c.Request.Context(), id, c.Request.Method, path); err != nil {
		writeDomainError(c, err)
		return
	}
	if requiresIdempotency(c.Request.Method, path) && c.GetHeader("Idempotency-Key") == "" {
		httpx.Fail(c, http.StatusBadRequest, "FUTURES_INVALID_INPUT", "缺少 Idempotency-Key")
		return
	}
	switch {
	case c.Request.Method == http.MethodGet && path == "/api/v1/futures/products":
		items, err := domain.Products(c.Request.Context(), id, queryLimit(c))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"items": items, "next_cursor": nil})
	case c.Request.Method == http.MethodGet && path == "/api/v1/futures/products/:id/contracts":
		items, err := domain.Contracts(c.Request.Context(), id, c.Param("id"), queryLimit(c))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"items": items, "next_cursor": nil})
	case c.Request.Method == http.MethodGet && path == "/api/v1/futures/products/:id/workbench":
		window, _ := strconv.Atoi(c.DefaultQuery("window", "60"))
		value, err := domain.Workbench(c.Request.Context(), id, c.Param("id"), c.Query("contract_id"), window, c.Query("price_type"))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, value)
	case c.Request.Method == http.MethodPost && path == "/api/v1/futures/drafts":
		var input model.DraftInput
		if !BindJSON(c, &input) {
			return
		}
		value, err := domain.CreateDraft(c.Request.Context(), id, c.GetHeader("Idempotency-Key"), input)
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusCreated, value)
	case c.Request.Method == http.MethodGet && path == "/api/v1/futures/drafts/:id":
		value, err := domain.GetDraft(c.Request.Context(), id, c.Param("id"))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, value)
	case c.Request.Method == http.MethodPatch && path == "/api/v1/futures/drafts/:id":
		var input struct {
			ExpectedRevision int64            `json:"expected_revision"`
			Input            model.DraftInput `json:"input"`
		}
		if !BindJSON(c, &input) {
			return
		}
		value, err := domain.PatchDraft(c.Request.Context(), id, c.Param("id"), input.ExpectedRevision, input.Input)
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, value)
	case c.Request.Method == http.MethodDelete && path == "/api/v1/futures/drafts/:id":
		value, err := domain.DeleteDraft(c.Request.Context(), id, c.Param("id"), c.GetHeader("Idempotency-Key"))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusAccepted, value)
	case c.Request.Method == http.MethodPost && path == "/api/v1/futures/drafts/:id/parse":
		var input struct {
			ExpectedRevision int64 `json:"expected_revision"`
		}
		if !BindJSON(c, &input) {
			return
		}
		value, err := domain.ParseDraft(c.Request.Context(), id, c.Param("id"), input.ExpectedRevision, c.GetHeader("Idempotency-Key"))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusAccepted, value)
	case c.Request.Method == http.MethodGet && path == "/api/v1/futures/drafts/:id/claims":
		value, err := domain.GetDraft(c.Request.Context(), id, c.Param("id"))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"items": value["claims"], "next_cursor": nil})
	case c.Request.Method == http.MethodPost && path == "/api/v1/futures/runs":
		var input svc.RunCreateInput
		if !BindJSON(c, &input) {
			return
		}
		value, err := domain.CreateRun(c.Request.Context(), id, c.GetHeader("Idempotency-Key"), input)
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusAccepted, value)
	case c.Request.Method == http.MethodGet && path == "/api/v1/futures/runs":
		items, err := domain.ListRuns(c.Request.Context(), id, queryLimit(c))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"items": items, "next_cursor": nil})
	case c.Request.Method == http.MethodGet && path == "/api/v1/futures/runs/:id":
		value, err := domain.GetRun(c.Request.Context(), id, c.Param("id"))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, value)
	case c.Request.Method == http.MethodPost && path == "/api/v1/futures/runs/:id/cancel":
		if err := svc.CancelRun(c.Request.Context(), domain.DB, id, c.Param("id")); err != nil {
			writeDomainError(c, err)
			return
		}
		value, _ := domain.GetRun(c.Request.Context(), id, c.Param("id"))
		httpx.OK(c, http.StatusAccepted, value)
	case c.Request.Method == http.MethodGet && path == "/api/v1/futures/runs/:id/delete-impact":
		value, err := domain.DeleteImpact(c.Request.Context(), id, c.Param("id"))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, value)
	case c.Request.Method == http.MethodDelete && path == "/api/v1/futures/runs/:id":
		var input struct {
			ImpactVersion     string `json:"impact_version"`
			CascadeHypotheses bool   `json:"cascade_hypotheses"`
		}
		if !BindJSON(c, &input) {
			return
		}
		value, err := domain.DeleteRun(c.Request.Context(), id, c.Param("id"), input.ImpactVersion, input.CascadeHypotheses, c.GetHeader("Idempotency-Key"))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusAccepted, value)
	case c.Request.Method == http.MethodGet && path == "/api/v1/futures/runs/:id/evidence/:evidenceId":
		value, err := domain.Evidence(c.Request.Context(), id, c.Param("id"), c.Param("evidenceId"))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, value)
	case c.Request.Method == http.MethodGet && path == "/api/v1/futures/runs/:id/export":
		value, err := domain.ExportRun(c.Request.Context(), id, c.Param("id"))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		c.Header("Content-Disposition", `attachment; filename="futures-report.html"`)
		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(value))
	case c.Request.Method == http.MethodPost && path == "/api/v1/futures/documents":
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			httpx.Fail(c, http.StatusBadRequest, "FUTURES_INVALID_INPUT", "缺少文件")
			return
		}
		defer file.Close()
		content, err := io.ReadAll(io.LimitReader(file, 20*1024*1024+1))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		value, err := domain.CreateDocument(c.Request.Context(), id, header.Filename, header.Header.Get("Content-Type"), content, c.GetHeader("Idempotency-Key"))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusAccepted, value)
	case c.Request.Method == http.MethodGet && path == "/api/v1/futures/documents/:id":
		value, err := domain.GetDocument(c.Request.Context(), id, c.Param("id"))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, value)
	case c.Request.Method == http.MethodDelete && path == "/api/v1/futures/documents/:id":
		value, err := domain.DeleteDocument(c.Request.Context(), id, c.Param("id"), c.GetHeader("Idempotency-Key"))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusAccepted, value)
	case c.Request.Method == http.MethodPost && path == "/api/v1/futures/hypotheses":
		var input svc.HypothesisCreateInput
		if !BindJSON(c, &input) {
			return
		}
		value, err := domain.CreateHypothesis(c.Request.Context(), id, c.GetHeader("Idempotency-Key"), input)
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusCreated, value)
	case c.Request.Method == http.MethodGet && path == "/api/v1/futures/hypotheses":
		items, err := domain.ListHypotheses(c.Request.Context(), id, queryLimit(c))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"items": items, "next_cursor": nil})
	case c.Request.Method == http.MethodGet && path == "/api/v1/futures/hypotheses/:id":
		value, err := domain.GetHypothesis(c.Request.Context(), id, c.Param("id"))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, value)
	case c.Request.Method == http.MethodPatch && path == "/api/v1/futures/hypotheses/:id":
		var input svc.HypothesisPatchInput
		if !BindJSON(c, &input) {
			return
		}
		value, err := domain.PatchHypothesis(c.Request.Context(), id, c.Param("id"), input)
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, value)
	case c.Request.Method == http.MethodDelete && path == "/api/v1/futures/hypotheses/:id":
		value, err := domain.DeleteHypothesis(c.Request.Context(), id, c.Param("id"), c.GetHeader("Idempotency-Key"))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusAccepted, value)
	case c.Request.Method == http.MethodGet && path == "/api/v1/futures/hypotheses/:id/checks":
		items, err := domain.ListChecks(c.Request.Context(), id, c.Param("id"), queryLimit(c))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"items": items, "next_cursor": nil})
	case c.Request.Method == http.MethodPost && path == "/api/v1/futures/hypotheses/:id/checks":
		var input svc.CheckCreateInput
		if !BindJSON(c, &input) {
			return
		}
		value, err := domain.CreateCheck(c.Request.Context(), id, c.Param("id"), c.GetHeader("Idempotency-Key"), input)
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusCreated, value)
	case c.Request.Method == http.MethodGet && path == "/api/v1/futures/hypotheses/:id/recap":
		value, err := domain.GetRecap(c.Request.Context(), id, c.Param("id"))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, value)
	case c.Request.Method == http.MethodPut && path == "/api/v1/futures/hypotheses/:id/recap":
		var input svc.RecapInput
		if !BindJSON(c, &input) {
			return
		}
		value, err := domain.PutRecap(c.Request.Context(), id, c.Param("id"), input)
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, value)
	case c.Request.Method == http.MethodGet && path == "/api/v1/futures/watchlist":
		value, err := domain.GetWatchlist(c.Request.Context(), id)
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, value)
	case c.Request.Method == http.MethodPut && path == "/api/v1/futures/watchlist":
		var input struct {
			ExpectedVersion int64    `json:"expected_version"`
			ProductIDs      []string `json:"product_ids"`
		}
		if !BindJSON(c, &input) {
			return
		}
		value, err := domain.PutWatchlist(c.Request.Context(), id, input.ExpectedVersion, input.ProductIDs)
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, value)
	case c.Request.Method == http.MethodGet && path == "/api/v1/futures/notifications":
		items, err := domain.ListNotifications(c.Request.Context(), id, queryLimit(c))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"items": items, "next_cursor": nil})
	case c.Request.Method == http.MethodPatch && path == "/api/v1/futures/notifications/:id":
		var input struct {
			Read bool `json:"read"`
		}
		if !BindJSON(c, &input) {
			return
		}
		if !input.Read {
			httpx.Fail(c, http.StatusBadRequest, "FUTURES_INVALID_INPUT", "read 必须为 true")
			return
		}
		value, err := domain.MarkNotification(c.Request.Context(), id, c.Param("id"))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, value)
	default:
		httpx.Fail(c, http.StatusServiceUnavailable, "FUTURES_NOT_READY", "期货研究运行依赖未就绪")
	}
}

func dispatchAdmin(c *gin.Context, domain *svc.Domain) {
	path := c.FullPath()
	if requiresIdempotency(c.Request.Method, path) && c.GetHeader("Idempotency-Key") == "" {
		httpx.Fail(c, http.StatusBadRequest, "FUTURES_INVALID_INPUT", "缺少 Idempotency-Key")
		return
	}
	switch {
	case c.Request.Method == http.MethodGet && path == "/api/v1/admin/futures/sources":
		items, err := domain.ListSources(c.Request.Context(), queryLimit(c))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"items": items, "next_cursor": nil})
	case c.Request.Method == http.MethodPost && path == "/api/v1/admin/futures/sources":
		var input struct {
			Manifest svc.SourceManifestInput `json:"manifest"`
		}
		if !BindJSON(c, &input) {
			return
		}
		value, err := domain.CreateSource(c.Request.Context(), c.GetHeader("Idempotency-Key"), input.Manifest)
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusCreated, value)
	case c.Request.Method == http.MethodPatch && path == "/api/v1/admin/futures/sources/:id":
		var input struct {
			ExpectedVersion int64  `json:"expected_version"`
			Status          string `json:"status"`
			Reason          string `json:"reason"`
		}
		if !BindJSON(c, &input) {
			return
		}
		value, err := domain.PatchSource(c.Request.Context(), c.Param("id"), input.ExpectedVersion, input.Status, input.Reason)
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, value)
	case c.Request.Method == http.MethodPost && path == "/api/v1/admin/futures/sources/:id/versions":
		var input struct {
			ExpectedVersion int64                   `json:"expected_version"`
			Manifest        svc.SourceManifestInput `json:"manifest"`
		}
		if !BindJSON(c, &input) {
			return
		}
		value, err := domain.CreateSourceVersion(c.Request.Context(), c.Param("id"), input.ExpectedVersion, c.GetHeader("Idempotency-Key"), input.Manifest)
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusCreated, value)
	case c.Request.Method == http.MethodGet && path == "/api/v1/admin/futures/admission":
		items, err := domain.ListAdmission(c.Request.Context(), queryLimit(c))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"items": items, "next_cursor": nil})
	case c.Request.Method == http.MethodPut && path == "/api/v1/admin/futures/admission/:productId":
		var input struct {
			ExpectedVersion    int64  `json:"expected_version"`
			Status             string `json:"status"`
			EvidenceManifestID string `json:"evidence_manifest_id"`
			Reason             string `json:"reason"`
		}
		if !BindJSON(c, &input) {
			return
		}
		value, err := domain.PutAdmission(c.Request.Context(), c.Param("productId"), input.ExpectedVersion, input.Status, input.EvidenceManifestID, input.Reason)
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, value)
	case c.Request.Method == http.MethodGet && path == "/api/v1/admin/futures/operations":
		value, err := domain.GetOperations(c.Request.Context())
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, value)
	case c.Request.Method == http.MethodPatch && path == "/api/v1/admin/futures/operations":
		var input struct {
			ExpectedVersion int64   `json:"expected_version"`
			Mode            string  `json:"mode"`
			Reason          string  `json:"reason"`
			AllowedUserIDs  []int64 `json:"allowed_user_ids"`
		}
		if !BindJSON(c, &input) {
			return
		}
		value, err := domain.PatchOperations(c.Request.Context(), input.ExpectedVersion, input.Mode, input.Reason, input.AllowedUserIDs)
		if err != nil {
			writeDomainError(c, err)
			return
		}
		httpx.OK(c, http.StatusOK, value)
	default:
		httpx.Fail(c, http.StatusServiceUnavailable, "FUTURES_NOT_READY", "管理员运行依赖未就绪")
	}
}

func requiresIdempotency(method, path string) bool {
	return method == http.MethodDelete || method == http.MethodPost
}

func queryLimit(c *gin.Context) int {
	value, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	return value
}
