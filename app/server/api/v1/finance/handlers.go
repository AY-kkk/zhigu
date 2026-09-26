package finance

import (
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"zhigu/server/httpx"
	modelfinance "zhigu/server/model/finance"
	"zhigu/server/service/finance"
)

type API struct {
	Svc   *finance.ResearchService
	Proxy *finance.ModelProxy
}

func Register(engine *gin.Engine, svc *finance.ResearchService, proxy *finance.ModelProxy) {
	a := &API{Svc: svc, Proxy: proxy}
	engine.POST("/api/finance/auth/login", a.Login)

	consumer := engine.Group("/api/finance")
	consumer.Use(httpx.AuthRequired())
	consumer.POST("/research-documents", a.UploadResearchDocument)
	consumer.GET("/research-documents/:id", a.GetResearchDocument)
	consumer.POST("/claims/parse", a.Parse)
	consumer.GET("/claims/:id", a.GetClaim)
	consumer.PATCH("/claims/:id", a.PatchClaim)
	consumer.POST("/research", a.Create)
	consumer.GET("/research/:id", a.Get)
	consumer.GET("/research", a.List)
	consumer.POST("/research/:id/cancel", a.Cancel)
	consumer.DELETE("/research/:id", a.Delete)
	consumer.GET("/evidence/:id", a.Evidence)
	consumer.POST("/research/:id/questions", a.Question)
	consumer.GET("/instruments", a.Instruments)

	internal := engine.Group("/internal")
	internal.Use(InternalAuth(), RequireContract())
	internal.POST("/llm/v1/chat/completions", a.ProxyLLMChat)
	internal.POST("/llm/v1/responses", a.ProxyLLMResponses)
	internal.POST("/finance/tool-grants", a.CreateGrant)
	internal.POST("/finance/tool-grants/:id/complete", a.CompleteGrant)
	internal.POST("/finance/evidence", a.RegisterEvidence)
	internal.GET("/finance/data-source-profile", a.DataSourceProfile)
	internal.POST("/finance/data-query", a.DataQuery)
	internal.POST("/finance/calculate", a.Calculate)
}

func (a *API) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	var user modelfinance.User
	if err := a.Svc.DB.Where("username = ?", req.Username).Take(&user).Error; err != nil {
		httpx.Fail(c, http.StatusUnauthorized, "UNAUTHENTICATED", "用户名或密码错误")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		httpx.Fail(c, http.StatusUnauthorized, "UNAUTHENTICATED", "用户名或密码错误")
		return
	}
	tok, err := httpx.SignToken(user.ID, user.Username, user.Role)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "INTERNAL", "签发令牌失败")
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"token": tok, "role": user.Role, "username": user.Username, "user_id": user.ID})
}

func (a *API) Parse(c *gin.Context) {
	var req struct {
		Text       string `json:"text"`
		DocumentID string `json:"document_id"`
		FocusText  string `json:"focus_text"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	ctx := finance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c))
	out, err := a.Svc.ParseClaim(ctx, finance.ParseInput{Text: req.Text, DocumentID: req.DocumentID, FocusText: req.FocusText})
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (a *API) GetClaim(c *gin.Context) {
	ctx := finance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c))
	out, err := a.Svc.GetClaim(ctx, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (a *API) PatchClaim(c *gin.Context) {
	var req finance.PatchDraftInput
	if !httpx.BindJSON(c, &req) {
		return
	}
	ctx := finance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c))
	out, err := a.Svc.PatchClaim(ctx, c.Param("id"), req)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (a *API) Create(c *gin.Context) {
	key := c.GetHeader("Idempotency-Key")
	var req struct {
		DraftID     string `json:"draft_id"`
		Revision    int    `json:"revision"`
		ParentRunID string `json:"parent_run_id"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	ctx := finance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c))
	out, err := a.Svc.CreateResearch(ctx, key, finance.CreateResearchInput{
		DraftID: req.DraftID, Revision: req.Revision, ParentRunID: req.ParentRunID,
	})
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusAccepted, out)
}

func (a *API) Get(c *gin.Context) {
	ctx := finance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c))
	out, err := a.Svc.GetResearch(ctx, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (a *API) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	ctx := finance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c))
	out, err := a.Svc.ListResearch(ctx, c.Query("cursor"), limit)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (a *API) Cancel(c *gin.Context) {
	ctx := finance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c))
	out, err := a.Svc.CancelResearch(ctx, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusAccepted, out)
}

func (a *API) Delete(c *gin.Context) {
	ctx := finance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c))
	status, err := a.Svc.DeleteResearch(ctx, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusAccepted, gin.H{"deletion_status": status})
}

func (a *API) UploadResearchDocument(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 22<<20)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "INVALID_MULTIPART", "缺少研报文件")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "INVALID_MULTIPART", "无法读取研报文件")
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, 20*1024*1024+1))
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "INVALID_MULTIPART", "读取研报文件失败")
		return
	}
	ctx := finance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c))
	out, err := a.Svc.Docs.Upload(ctx, finance.UploadDocumentInput{
		Filename: fileHeader.Filename,
		MediaType: fileHeader.Header.Get("Content-Type"),
		ByteSize: fileHeader.Size,
		Content: content,
		DraftID: c.PostForm("draft_id"),
	})
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, out)
}

func (a *API) GetResearchDocument(c *gin.Context) {
	ctx := finance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c))
	out, err := a.Svc.Docs.Get(ctx, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (a *API) Evidence(c *gin.Context) {
	ctx := finance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c))
	out, err := a.Svc.GetEvidence(ctx, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (a *API) Question(c *gin.Context) {
	var req struct {
		Text string `json:"text"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	ctx := finance.WithUser(c.Request.Context(), httpx.CurrentUserID(c), roleOf(c))
	out, err := a.Svc.Ask(ctx, c.Param("id"), c.GetHeader("Idempotency-Key"), req.Text)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (a *API) Instruments(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	market := strings.TrimSpace(c.Query("market"))
	limit := finance.CatalogLimit
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	items := finance.SearchCatalog(finance.DataMode(), q, market, limit)
	out := make([]finance.Instrument, 0, len(items))
	for _, row := range items {
		out = append(out, finance.Instrument{ID: row.ID, Symbol: row.Symbol, Name: row.Name, Market: row.Market})
	}
	httpx.OK(c, http.StatusOK, gin.H{
		"items":         out,
		"catalog_limit": finance.CatalogLimit,
		"mode":          finance.DataMode(),
	})
}

func (a *API) ProxyLLMChat(c *gin.Context) {
	a.proxyLLM(c, finance.ProtocolChatCompletions)
}

func (a *API) ProxyLLMResponses(c *gin.Context) {
	a.proxyLLM(c, finance.ProtocolResponses)
}

func (a *API) proxyLLM(c *gin.Context, protocol string) {
	if a.Proxy == nil {
		httpx.Fail(c, http.StatusServiceUnavailable, "NOT_READY", "模型代理未配置")
		return
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, "INVALID_JSON", "无法读取请求体")
		return
	}
	requestID := c.GetHeader("X-Request-ID")
	if requestID == "" {
		httpx.Fail(c, http.StatusBadRequest, "MISSING_REQUEST_ID", "缺少 X-Request-ID")
		return
	}
	token := c.GetHeader("X-Zhigu-Task-Token")
	if token == "" {
		httpx.Fail(c, http.StatusUnauthorized, "MISSING_TASK_TOKEN", "缺少任务凭据")
		return
	}
	raw, err := a.Proxy.Complete(c.Request.Context(), requestID, finance.NormalizeJSONHash(body), finance.HeaderTokenHash(token), body, protocol)
	if err != nil {
		fail(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

func (a *API) CreateGrant(c *gin.Context) {
	var in finance.GrantIn
	if !httpx.BindJSON(c, &in) {
		return
	}
	token := c.GetHeader("X-Zhigu-Task-Token")
	if token == "" {
		httpx.Fail(c, http.StatusUnauthorized, "MISSING_TASK_TOKEN", "缺少任务凭据")
		return
	}
	ctx := finance.WithTaskToken(c.Request.Context(), token)
	if err := a.Svc.AuthorizeTaskToken(ctx, token, in.RunID, in.TaskID, "research"); err != nil {
		fail(c, err)
		return
	}
	grant, err := a.Svc.CreateGrant(ctx, in.RunID, in.TaskID, in)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"grant_id": grant.ID, "expires_at": grant.ExpiresAt, "status": grant.Status})
}

func (a *API) CompleteGrant(c *gin.Context) {
	var req struct {
		Status     string `json:"status"`
		OutputHash string `json:"output_hash"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	token := c.GetHeader("X-Zhigu-Task-Token")
	if token == "" {
		httpx.Fail(c, http.StatusUnauthorized, "MISSING_TASK_TOKEN", "缺少任务凭据")
		return
	}
	var grant modelfinance.ToolGrant
	if err := a.Svc.DB.Where("id = ?", c.Param("id")).Take(&grant).Error; err != nil {
		httpx.Fail(c, http.StatusNotFound, "GRANT_NOT_FOUND", "授权不存在")
		return
	}
	ctx := finance.WithTaskToken(c.Request.Context(), token)
	if err := a.Svc.AuthorizeTaskToken(ctx, token, grant.RunID, grant.TaskID, "research"); err != nil {
		fail(c, err)
		return
	}
	if err := a.Svc.CompleteGrant(ctx, c.Param("id"), req.Status, req.OutputHash); err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"status": req.Status})
}

func (a *API) RegisterEvidence(c *gin.Context) {
	var req struct {
		GrantID   string   `json:"grant_id"`
		RecordIDs []string `json:"record_ids"`
		Records   []any    `json:"records"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	if len(req.Records) > 0 {
		httpx.Fail(c, http.StatusBadRequest, "EVIDENCE_REJECTED", "证据只接受 record_ids")
		return
	}
	token := c.GetHeader("X-Zhigu-Task-Token")
	if token == "" {
		httpx.Fail(c, http.StatusUnauthorized, "MISSING_TASK_TOKEN", "缺少任务凭据")
		return
	}
	var grant modelfinance.ToolGrant
	if err := a.Svc.DB.Where("id = ?", req.GrantID).Take(&grant).Error; err != nil {
		httpx.Fail(c, http.StatusNotFound, "GRANT_NOT_FOUND", "授权不存在")
		return
	}
	ctx := finance.WithTaskToken(c.Request.Context(), token)
	if err := a.Svc.AuthorizeTaskToken(ctx, token, grant.RunID, grant.TaskID, "research"); err != nil {
		fail(c, err)
		return
	}
	ids, err := finance.NewEvidenceService(a.Svc.DB).Register(ctx, req.GrantID, req.RecordIDs)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"evidence_ids": ids})
}

func (a *API) DataSourceProfile(c *gin.Context) {
	mode := finance.DataMode()
	cat := finance.CatalogForMode(mode)
	ids := make([]string, 0, len(cat))
	for _, row := range cat {
		ids = append(ids, row.ID)
	}
	httpx.OK(c, http.StatusOK, gin.H{
		"source_policy_version": finance.SourcePolicyVersion,
		"data_version":          finance.DataVersion,
		"connector_mode":        finance.ConnectorModeGoHTTP,
		"instruments":           ids,
		"metrics":               finance.FrozenMetrics,
		"periods":               []string{"YYYY-12-31"},
		"financial_http_limit":  finance.FinancialHTTPLimit,
		"filing_http_limit":     finance.FilingHTTPLimit,
		"tool_timeout_ms":       finance.ToolTimeout.Milliseconds(),
		"source_rules":          finance.SourceRules(),
		"mode":                  mode,
	})
}

func (a *API) DataQuery(c *gin.Context) {
	var req struct {
		GrantID   string         `json:"grant_id"`
		Operation string         `json:"operation"`
		Params    map[string]any `json:"params"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	token := c.GetHeader("X-Zhigu-Task-Token")
	if token == "" {
		httpx.Fail(c, http.StatusUnauthorized, "MISSING_TASK_TOKEN", "缺少任务凭据")
		return
	}
	ctx := finance.WithTaskToken(c.Request.Context(), token)
	out, err := a.Svc.DataQuery(ctx, req.GrantID, req.Operation, req.Params)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (a *API) Calculate(c *gin.Context) {
	var req struct {
		GrantID   string                       `json:"grant_id"`
		Operation string                       `json:"operation"`
		Inputs    map[string]finance.CalcInput `json:"inputs"`
	}
	if !httpx.BindJSON(c, &req) {
		return
	}
	token := c.GetHeader("X-Zhigu-Task-Token")
	if token == "" {
		httpx.Fail(c, http.StatusUnauthorized, "MISSING_TASK_TOKEN", "缺少任务凭据")
		return
	}
	ctx := finance.WithTaskToken(c.Request.Context(), token)
	out, err := a.Svc.CalculateMetric(ctx, req.GrantID, req.Operation, req.Inputs)
	if err != nil {
		fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func InternalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		want := "Bearer " + internalServiceToken()
		if token != want {
			httpx.Fail(c, http.StatusUnauthorized, "UNAUTHENTICATED", "内部服务凭据无效")
			c.Abort()
			return
		}
		c.Next()
	}
}

func RequireContract() gin.HandlerFunc {
	return func(c *gin.Context) {
		got := strings.TrimSpace(c.GetHeader(finance.ContractVersionHeader))
		if got != finance.ContractVersion {
			httpx.Fail(c, http.StatusConflict, "CONTRACT_VERSION_MISMATCH", "控制协议版本不匹配")
			c.Abort()
			return
		}
		c.Next()
	}
}

func internalServiceToken() string {
	v := os.Getenv("ZHIGU_INTERNAL_TOKEN")
	if v == "" {
		return "zhigu-internal-dev"
	}
	return v
}

func roleOf(c *gin.Context) string {
	v, _ := c.Get("role")
	s, _ := v.(string)
	return s
}

func fail(c *gin.Context, err error) {
	httpx.Fail(c, finance.HTTPStatus(err), finance.ErrorCode(err), finance.ErrorMessage(err))
}
