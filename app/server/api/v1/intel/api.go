package intel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"

	"zhigu/server/httpx"
	svcfinance "zhigu/server/service/finance"
	intelsvc "zhigu/server/service/intel"
)

type handler struct{ svc *intelsvc.Service }

type intelMeta struct {
	RequestID string         `json:"request_id"`
	AsOf      time.Time      `json:"as_of"`
	Coverage  map[string]any `json:"coverage"`
	Warnings  []string       `json:"warnings"`
}

type intelEnvelope struct {
	Data    any        `json:"data"`
	Error   *intelErr  `json:"error"`
	TraceID string     `json:"trace_id"`
	Meta    *intelMeta `json:"meta"`
}

type intelErr struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	RequestID string `json:"request_id"`
}

func Register(engine *gin.Engine, svc *intelsvc.Service) {
	h := &handler{svc: svc}
	g := engine.Group("/api/finance/intel/v1")
	g.Use(h.origin())
	g.GET("/session", h.scope(), h.session)
	g.POST("/demo-sessions", h.createDemoSession)
	g.Use(h.scope())
	g.GET("/instruments", h.instruments)
	g.GET("/watchlist", h.watchlist)
	g.PUT("/watchlist/:code", h.addWatchlist)
	g.DELETE("/watchlist/:code", h.removeWatchlist)
	g.GET("/events", h.events)
	g.GET("/events/:id", h.eventDetail)
	g.GET("/events/:id/timeline", h.timeline)
	g.GET("/events/:id/evidence", h.evidence)
	g.GET("/events/:id/conflicts", h.conflicts)
	g.GET("/events/:id/changes", h.changes)
	g.GET("/source-revisions/:id", h.sourceRevision)
	g.GET("/notifications", h.notifications)
	g.PATCH("/notifications/:id", h.patchNotification)
	g.PUT("/events/:id/mute", h.mute)
	g.GET("/data-status", h.dataStatus)
	g.POST("/replay/actions", h.replay)
	admin := g.Group("/admin")
	admin.Use(h.admin())
	admin.POST("/ingestion-jobs", h.createIngestionJob)
	admin.GET("/ingestion-jobs/:id", h.ingestionJob)
	admin.POST("/source-revisions", h.importSourceRevision)
	admin.GET("/review-items", h.reviewItems)
	admin.POST("/review-items/:id/resolve", h.resolveReviewItem)
	admin.POST("/events/:id/reassign", h.reassign)
}

func (h *handler) origin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		public := strings.TrimRight(os.Getenv("ZHIGU_INTEL_PUBLIC_ORIGIN"), "/")
		origin := strings.TrimRight(c.GetHeader("Origin"), "/")
		if public != "" && origin != public {
			h.fail(c, &appError{status: 403, code: "FORBIDDEN", message: "请求 Origin 不属于当前部署"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func requestID(c *gin.Context) string {
	if id := strings.TrimSpace(c.GetHeader("X-Request-ID")); id != "" && len(id) <= 128 {
		return id
	}
	return httpx.NewID("req")
}

func (h *handler) coverage(scope intelsvc.Scope) map[string]any {
	status := "complete"
	scopeName := "events-v1"
	if scope.Mode == "live" {
		status = "limited"
		scopeName = "configured-providers"
	}
	return map[string]any{"status": status, "scope": scopeName, "last_success_at": h.svc.Now(), "pending_count": 0, "gaps": []any{}}
}

func (h *handler) ok(c *gin.Context, status int, data any, scope intelsvc.Scope) {
	id := requestID(c)
	asOf := h.svc.Now()
	if scope.SimulatedAt != nil {
		asOf = *scope.SimulatedAt
	}
	warnings := []string{"演示数据"}
	if scope.Mode == "live" {
		warnings = []string{"仅整理信息，不构成投资建议"}
	}
	c.JSON(status, intelEnvelope{Data: data, Error: nil, TraceID: httpx.TraceID(c), Meta: &intelMeta{
		RequestID: id, AsOf: asOf, Coverage: h.coverage(scope), Warnings: warnings,
	}})
}

func (h *handler) fail(c *gin.Context, err error) {
	status, code, message := http.StatusInternalServerError, "INTERNAL", "服务内部错误"
	var app *appError
	if errors.As(err, &app) {
		status, code, message = app.status, app.code, app.message
	}
	id := requestID(c)
	c.JSON(status, intelEnvelope{Data: nil, Error: &intelErr{Code: code, Message: message, Retryable: status == 429 || status >= 500, RequestID: id}, TraceID: httpx.TraceID(c), Meta: nil})
}

type appError struct {
	status  int
	code    string
	message string
}

func (e *appError) Error() string { return e.code + ": " + e.message }

func wrapServiceError(err error) error {
	if err == nil {
		return nil
	}
	var legacy *svcfinance.AppError
	if errors.As(err, &legacy) {
		return &appError{status: legacy.Status, code: legacy.Code, message: legacy.Message}
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "VERSION_CONFLICT"):
		return &appError{status: 409, code: "VERSION_CONFLICT", message: "版本已变化"}
	case strings.Contains(msg, "IDEMPOTENCY_CONFLICT"):
		return &appError{status: 409, code: "IDEMPOTENCY_CONFLICT", message: "幂等请求冲突"}
	case strings.Contains(msg, "NOTIFICATION_STATE_CONFLICT"):
		return &appError{status: 409, code: "NOTIFICATION_STATE_CONFLICT", message: "通知状态冲突"}
	case strings.Contains(msg, "WATCHLIST_LIMIT"):
		return &appError{status: 422, code: "WATCHLIST_LIMIT", message: "最多关注 20 个标的"}
	case strings.Contains(msg, "DEMO_SESSION_EXPIRED"):
		return &appError{status: 401, code: "DEMO_SESSION_EXPIRED", message: "演示会话已过期"}
	case strings.Contains(msg, "REPLAY_FORBIDDEN"):
		return &appError{status: 403, code: "REPLAY_FORBIDDEN", message: "真实数据空间禁止回放"}
	case strings.Contains(msg, "BOOTSTRAP_REQUIRED"):
		return &appError{status: 400, code: "BOOTSTRAP_REQUIRED", message: "请先初始化演示会话"}
	case strings.Contains(msg, "NOT_FOUND"):
		return &appError{status: 404, code: "NOT_FOUND", message: "资源不存在或不可见"}
	case strings.Contains(msg, "INVALID_PARAM"):
		return &appError{status: 400, code: "INVALID_PARAM", message: "参数不合法"}
	case strings.Contains(msg, "DATA_UNAVAILABLE"):
		return &appError{status: 503, code: "DATA_UNAVAILABLE", message: "数据暂时不可用"}
	default:
		return err
	}
}

func (h *handler) session(c *gin.Context) {
	scope := scopeFrom(c)
	h.ok(c, http.StatusOK, map[string]any{
		"mode": scope.Mode, "principal_id": scope.PrincipalKey, "role": scope.Role,
		"namespace_label": scope.NamespaceLabel, "expires_at": scope.ExpiresAt,
		"replay_version": scope.ReplayVersion, "generation": scope.Generation,
		"branch": scope.Branch, "step_index": scope.StepIndex,
	}, scope)
}

func (h *handler) createDemoSession(c *gin.Context) {
	if c.GetHeader("X-Intel-Mode") != "demo" {
		h.fail(c, &appError{status: 400, code: "MODE_CONFLICT", message: "演示初始化必须使用 demo 模式"})
		return
	}
	if httpx.TokenFromRequest(c) != "" {
		h.fail(c, &appError{status: 400, code: "MODE_CONFLICT", message: "演示请求不能携带真实令牌"})
		return
	}
	bootstrap, err := c.Cookie("zhigu_intel_bootstrap")
	if err != nil || bootstrap == "" {
		h.fail(c, &appError{status: 400, code: "BOOTSTRAP_REQUIRED", message: "缺少 bootstrap Cookie"})
		return
	}
	bootstrapID, ok := h.svc.VerifyDemoToken(bootstrap)
	if !ok {
		h.fail(c, &appError{status: 400, code: "BOOTSTRAP_REQUIRED", message: "bootstrap Cookie 无效"})
		return
	}
	key := c.GetHeader("Idempotency-Key")
	if len(key) < 8 || len(key) > 128 {
		h.fail(c, &appError{status: 400, code: "INVALID_PARAM", message: "Idempotency-Key 长度必须为 8–128"})
		return
	}
	var req struct {
		FixtureSet string `json:"fixture_set"`
	}
	if !bindStrict(c, &req) {
		return
	}
	if req.FixtureSet != "events-v1" {
		h.fail(c, &appError{status: 422, code: "UNSUPPORTED_EVENT", message: "仅支持 events-v1 fixture"})
		return
	}
	bodyHash := hashJSON(req)
	scope, replayed, err := h.svc.CreateDemoSession(c.Request.Context(), hashString(bootstrapID), key, bodyHash)
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	setDemoCookie(c, h.svc.SignDemoToken(scope.SessionID), 24*time.Hour)
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	h.ok(c, status, map[string]any{"session_id": scope.SessionID, "expires_at": scope.ExpiresAt, "namespace_label": scope.NamespaceLabel, "generation": scope.Generation, "replay_version": scope.ReplayVersion}, scope)
}

func (h *handler) scope() gin.HandlerFunc {
	return func(c *gin.Context) {
		mode := c.GetHeader("X-Intel-Mode")
		if mode != "demo" && mode != "live" {
			h.fail(c, &appError{status: 400, code: "MODE_CONFLICT", message: "X-Intel-Mode 必须是 live 或 demo"})
			c.Abort()
			return
		}
		if mode == "demo" {
			if httpx.TokenFromRequest(c) != "" {
				h.fail(c, &appError{status: 400, code: "MODE_CONFLICT", message: "demo 请求不能携带真实令牌"})
				c.Abort()
				return
			}
			token, err := c.Cookie("zhigu_intel_demo")
			if err != nil || token == "" {
				if c.Request.URL.Path == "/api/finance/intel/v1/session" {
					setBootstrapCookie(c, h.svc.SignDemoToken("boot:"+httpx.NewID("bootstrap")), time.Hour)
				}
				h.fail(c, &appError{status: 401, code: "DEMO_SESSION_EXPIRED", message: "演示会话不存在"})
				c.Abort()
				return
			}
			sessionID, ok := h.svc.VerifyDemoToken(token)
			if !ok {
				h.fail(c, &appError{status: 401, code: "DEMO_SESSION_EXPIRED", message: "演示 Cookie 无效"})
				c.Abort()
				return
			}
			scope, err := h.svc.ResolveDemoSession(c.Request.Context(), sessionID)
			if err != nil {
				h.fail(c, wrapServiceError(err))
				c.Abort()
				return
			}
			c.Set("intel_scope", scope)
			c.Next()
			return
		}
		raw := httpx.TokenFromRequest(c)
		if raw == "" {
			h.fail(c, &appError{status: 401, code: "UNAUTHENTICATED", message: "未登录"})
			c.Abort()
			return
		}
		claims := &httpx.Claims{}
		tok, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) { return httpx.JWTSecret(), nil })
		if err != nil || !tok.Valid {
			h.fail(c, &appError{status: 401, code: "UNAUTHENTICATED", message: "登录令牌无效"})
			c.Abort()
			return
		}
		scope, err := h.svc.LiveScope(c.Request.Context(), claims.UserID, claims.Role)
		if err != nil {
			h.fail(c, wrapServiceError(err))
			c.Abort()
			return
		}
		c.Set("intel_scope", scope)
		c.Next()
	}
}

func (h *handler) admin() gin.HandlerFunc {
	return func(c *gin.Context) {
		scope := scopeFrom(c)
		if scope.Role != "admin" {
			h.fail(c, &appError{status: 403, code: "FORBIDDEN", message: "需要管理员权限"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func scopeFrom(c *gin.Context) intelsvc.Scope {
	v, _ := c.Get("intel_scope")
	scope, _ := v.(intelsvc.Scope)
	return scope
}

func bindStrict(c *gin.Context, dst any) bool {
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		h := &handler{}
		h.fail(c, &appError{status: 400, code: "INVALID_JSON", message: "请求体无法解析"})
		return false
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		h := &handler{}
		h.fail(c, &appError{status: 400, code: "INVALID_JSON", message: "请求体只能包含一个 JSON 对象"})
		return false
	}
	return true
}

func hashJSON(v any) string { return hashString(fmt.Sprintf("%v", v)) }

func hashString(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

func setDemoCookie(c *gin.Context, value string, ttl time.Duration) {
	http.SetCookie(c.Writer, &http.Cookie{Name: "zhigu_intel_demo", Value: value, Path: "/api/finance/intel/v1", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secureCookies(), MaxAge: int(ttl / time.Second)})
}

func setBootstrapCookie(c *gin.Context, value string, ttl time.Duration) {
	http.SetCookie(c.Writer, &http.Cookie{Name: "zhigu_intel_bootstrap", Value: value, Path: "/api/finance/intel/v1", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secureCookies(), MaxAge: int(ttl / time.Second)})
}

func secureCookies() bool {
	return strings.HasPrefix(os.Getenv("ZHIGU_INTEL_PUBLIC_ORIGIN"), "https://") || os.Getenv("ZHIGU_INTEL_SECURE_COOKIES") == "1"
}

func (h *handler) instruments(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	out, err := h.svc.SearchInstruments(c.Request.Context(), scopeFrom(c), c.Query("q"), limit)
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	h.ok(c, http.StatusOK, out, scopeFrom(c))
}

func (h *handler) watchlist(c *gin.Context) {
	out, err := h.svc.ListWatchlist(c.Request.Context(), scopeFrom(c))
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	h.ok(c, http.StatusOK, out, scopeFrom(c))
}

func (h *handler) addWatchlist(c *gin.Context) {
	var req map[string]any
	if c.Request.ContentLength > 0 {
		if !bindStrict(c, &req) {
			return
		}
	}
	out, err := h.svc.AddWatchlist(c.Request.Context(), scopeFrom(c), c.Param("code"))
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	h.ok(c, http.StatusOK, out, scopeFrom(c))
}

func (h *handler) removeWatchlist(c *gin.Context) {
	if err := h.svc.RemoveWatchlist(c.Request.Context(), scopeFrom(c), c.Param("code")); err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *handler) events(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	scope := scopeFrom(c)
	filters := map[string]any{"code": c.Query("code"), "type": c.Query("type"), "verification": c.Query("verification")}
	offset, fetchLimit, err := pagingRequest(h.svc, c, scope, "events", filters, limit)
	if err != nil {
		h.fail(c, err)
		return
	}
	raw, err := h.svc.ListEvents(c.Request.Context(), scope, intelsvc.EventQuery{Code: c.Query("code"), Type: c.Query("type"), Verification: c.Query("verification"), Cursor: c.Query("cursor"), Limit: fetchLimit})
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	total := len(raw.Items)
	raw.Items = slicePage(raw.Items, offset, limit)
	raw.NextCursor = h.nextCursor(scope, "events", filters, offset, limit, total)
	h.ok(c, http.StatusOK, raw, scope)
}

func (h *handler) eventDetail(c *gin.Context) {
	version, _ := strconv.Atoi(c.Query("version"))
	out, err := h.svc.EventDetail(c.Request.Context(), scopeFrom(c), c.Param("id"), version)
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	h.ok(c, http.StatusOK, out, scopeFrom(c))
}

func (h *handler) timeline(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	version, _ := strconv.Atoi(c.Query("version"))
	scope := scopeFrom(c)
	filters := map[string]any{"id": c.Param("id"), "version": version}
	offset, fetchLimit, err := pagingRequest(h.svc, c, scope, "timeline", filters, limit)
	if err != nil {
		h.fail(c, err)
		return
	}
	out, err := h.svc.Timeline(c.Request.Context(), scope, c.Param("id"), version, fetchLimit)
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	items, _ := out["items"].([]any)
	total := len(items)
	out["items"] = slicePage(items, offset, limit)
	out["next_cursor"] = h.nextCursor(scope, "timeline", filters, offset, limit, total)
	h.ok(c, http.StatusOK, out, scope)
}

func (h *handler) evidence(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	version, _ := strconv.Atoi(c.Query("version"))
	includeInactive := c.Query("include_inactive") == "true"
	scope := scopeFrom(c)
	filters := map[string]any{"id": c.Param("id"), "version": version, "claim_key": c.Query("claim_key"), "grade": c.Query("grade"), "include_inactive": includeInactive}
	offset, fetchLimit, err := pagingRequest(h.svc, c, scope, "evidence", filters, limit)
	if err != nil {
		h.fail(c, err)
		return
	}
	out, err := h.svc.Evidence(c.Request.Context(), scope, c.Param("id"), version, c.Query("claim_key"), c.Query("grade"), includeInactive, fetchLimit)
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	items, _ := out["items"].([]any)
	total := len(items)
	out["items"] = slicePage(items, offset, limit)
	out["next_cursor"] = h.nextCursor(scope, "evidence", filters, offset, limit, total)
	h.ok(c, http.StatusOK, out, scope)
}

func (h *handler) conflicts(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	version, _ := strconv.Atoi(c.Query("version"))
	scope := scopeFrom(c)
	filters := map[string]any{"id": c.Param("id"), "status": c.Query("status")}
	offset, fetchLimit, err := pagingRequest(h.svc, c, scope, "conflicts", filters, limit)
	if err != nil {
		h.fail(c, err)
		return
	}
	out, err := h.svc.Conflicts(c.Request.Context(), scope, c.Param("id"), version, c.Query("status"), fetchLimit)
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	items, _ := out["items"].([]any)
	total := len(items)
	out["items"] = slicePage(items, offset, limit)
	out["next_cursor"] = h.nextCursor(scope, "conflicts", filters, offset, limit, total)
	h.ok(c, http.StatusOK, out, scope)
}

func (h *handler) changes(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	version, _ := strconv.Atoi(c.Query("version"))
	scope := scopeFrom(c)
	filters := map[string]any{"id": c.Param("id"), "version": version}
	offset, fetchLimit, err := pagingRequest(h.svc, c, scope, "changes", filters, limit)
	if err != nil {
		h.fail(c, err)
		return
	}
	out, err := h.svc.Changes(c.Request.Context(), scope, c.Param("id"), version, fetchLimit)
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	items, _ := out["items"].([]any)
	total := len(items)
	out["items"] = slicePage(items, offset, limit)
	out["next_cursor"] = h.nextCursor(scope, "changes", filters, offset, limit, total)
	h.ok(c, http.StatusOK, out, scope)
}

func (h *handler) sourceRevision(c *gin.Context) {
	out, err := h.svc.SourceRevision(c.Request.Context(), scopeFrom(c), c.Param("id"))
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	h.ok(c, http.StatusOK, out, scopeFrom(c))
}

func (h *handler) notifications(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	scope := scopeFrom(c)
	filters := map[string]any{"status": c.Query("status")}
	offset, fetchLimit, err := pagingRequest(h.svc, c, scope, "notifications", filters, limit)
	if err != nil {
		h.fail(c, err)
		return
	}
	raw, err := h.svc.ListNotifications(c.Request.Context(), scope, intelsvc.NotificationQuery{Status: c.Query("status"), Cursor: c.Query("cursor"), Limit: fetchLimit})
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	total := len(raw.Items)
	out := map[string]any{"items": slicePage(raw.Items, offset, limit), "unread_count": raw.UnreadCount, "next_cursor": h.nextCursor(scope, "notifications", filters, offset, limit, total)}
	h.ok(c, http.StatusOK, out, scope)
}

func (h *handler) patchNotification(c *gin.Context) {
	var req struct {
		Status string `json:"status"`
	}
	if !bindStrict(c, &req) {
		return
	}
	out, err := h.svc.PatchNotification(c.Request.Context(), scopeFrom(c), c.Param("id"), req.Status)
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	h.ok(c, http.StatusOK, out, scopeFrom(c))
}

func (h *handler) mute(c *gin.Context) {
	var req struct {
		Muted bool `json:"muted"`
	}
	if !bindStrict(c, &req) {
		return
	}
	out, err := h.svc.SetMute(c.Request.Context(), scopeFrom(c), c.Param("id"), req.Muted)
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	h.ok(c, http.StatusOK, out, scopeFrom(c))
}

func (h *handler) dataStatus(c *gin.Context) {
	out, err := h.svc.DataStatus(c.Request.Context(), scopeFrom(c))
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	h.ok(c, http.StatusOK, out, scopeFrom(c))
}

func requireIdempotency(c *gin.Context) bool {
	key := c.GetHeader("Idempotency-Key")
	if len(key) < 8 || len(key) > 128 {
		h := &handler{}
		h.fail(c, &appError{status: 400, code: "INVALID_PARAM", message: "Idempotency-Key 长度必须为 8–128"})
		return false
	}
	return true
}

func (h *handler) replay(c *gin.Context) {
	if !requireIdempotency(c) {
		return
	}
	key := c.GetHeader("Idempotency-Key")
	var req intelsvc.ReplayRequest
	if !bindStrict(c, &req) {
		return
	}
	result, err := h.svc.Idempotent(c.Request.Context(), scopeFrom(c), key, http.MethodPost, "/replay/actions", hashJSON(req), func(_ *gorm.DB) (any, error) {
		return h.svc.ReplayAction(c.Request.Context(), scopeFrom(c), req)
	})
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	if result.Replayed {
		c.Header("Idempotency-Replayed", "true")
	}
	h.ok(c, http.StatusOK, result.Data, scopeFrom(c))
}

func (h *handler) createIngestionJob(c *gin.Context) {
	if !requireIdempotency(c) {
		return
	}
	var req intelsvc.IngestionJobRequest
	if !bindStrict(c, &req) {
		return
	}
	key := c.GetHeader("Idempotency-Key")
	result, err := h.svc.Idempotent(c.Request.Context(), scopeFrom(c), key, http.MethodPost, "/admin/ingestion-jobs", hashJSON(req), func(_ *gorm.DB) (any, error) {
		return h.svc.CreateIngestionJob(c.Request.Context(), scopeFrom(c), req)
	})
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	if result.Replayed {
		c.Header("Idempotency-Replayed", "true")
	}
	h.ok(c, http.StatusAccepted, result.Data, scopeFrom(c))
}

func (h *handler) ingestionJob(c *gin.Context) {
	out, err := h.svc.IngestionJob(c.Request.Context(), scopeFrom(c), c.Param("id"))
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	h.ok(c, http.StatusOK, out, scopeFrom(c))
}

func (h *handler) importSourceRevision(c *gin.Context) {
	if !requireIdempotency(c) {
		return
	}
	var req intelsvc.ImportSourceRequest
	if !bindStrict(c, &req) {
		return
	}
	key := c.GetHeader("Idempotency-Key")
	result, err := h.svc.Idempotent(c.Request.Context(), scopeFrom(c), key, http.MethodPost, "/admin/source-revisions", hashJSON(req), func(_ *gorm.DB) (any, error) {
		return h.svc.ImportSourceRevision(c.Request.Context(), scopeFrom(c), req)
	})
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	if result.Replayed {
		c.Header("Idempotency-Replayed", "true")
	}
	status := http.StatusCreated
	if data, ok := result.Data.(map[string]any); ok {
		if existing, _ := data["existing"].(bool); existing {
			status = http.StatusOK
		}
	}
	h.ok(c, status, result.Data, scopeFrom(c))
}

func (h *handler) reviewItems(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	out, err := h.svc.ListReviewItems(c.Request.Context(), scopeFrom(c), c.Query("kind"), limit)
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	h.ok(c, http.StatusOK, out, scopeFrom(c))
}

func (h *handler) resolveReviewItem(c *gin.Context) {
	if !requireIdempotency(c) {
		return
	}
	key := c.GetHeader("Idempotency-Key")
	var req intelsvc.ReviewResolveRequest
	if !bindStrict(c, &req) {
		return
	}
	result, err := h.svc.Idempotent(c.Request.Context(), scopeFrom(c), key, http.MethodPost, "/admin/review-items/"+c.Param("id")+"/resolve", hashJSON(req), func(_ *gorm.DB) (any, error) {
		return h.svc.ResolveReviewItem(c.Request.Context(), scopeFrom(c), c.Param("id"), req)
	})
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	if result.Replayed {
		c.Header("Idempotency-Replayed", "true")
	}
	h.ok(c, http.StatusOK, result.Data, scopeFrom(c))
}

func (h *handler) reassign(c *gin.Context) {
	if !requireIdempotency(c) {
		return
	}
	key := c.GetHeader("Idempotency-Key")
	var req intelsvc.ReassignRequest
	if !bindStrict(c, &req) {
		return
	}
	result, err := h.svc.Idempotent(c.Request.Context(), scopeFrom(c), key, http.MethodPost, "/admin/events/"+c.Param("id")+"/reassign", hashJSON(req), func(_ *gorm.DB) (any, error) {
		return h.svc.ReassignEvidence(c.Request.Context(), scopeFrom(c), c.Param("id"), req)
	})
	if err != nil {
		h.fail(c, wrapServiceError(err))
		return
	}
	if result.Replayed {
		c.Header("Idempotency-Replayed", "true")
	}
	h.ok(c, http.StatusOK, result.Data, scopeFrom(c))
}

var _ = url.QueryEscape
