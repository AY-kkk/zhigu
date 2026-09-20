package finance

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"zhigu/server/httpx"
	svc "zhigu/server/service/finance"
)

func RegisterAdmin(engine *gin.Engine, cfg *svc.ConfigService, research *svc.ResearchService) {
	g := engine.Group("/api/finance/admin")
	g.Use(httpx.AuthRequired(), httpx.AdminRequired())
	g.GET("/model-configs", func(c *gin.Context) {
		out, err := cfg.List(c.Request.Context(), c.DefaultQuery("kind", "model"))
		if err != nil {
			httpx.Fail(c, svc.HTTPStatus(err), svc.ErrorCode(err), svc.ErrorMessage(err))
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"items": out})
	})
	g.POST("/model-configs", func(c *gin.Context) {
		var req struct {
			Public map[string]any `json:"public_config"`
			APIKey string         `json:"api_key"`
		}
		if !httpx.BindJSON(c, &req) {
			return
		}
		out, err := cfg.SaveModel(c.Request.Context(), req.Public, req.APIKey)
		if err != nil {
			httpx.Fail(c, svc.HTTPStatus(err), svc.ErrorCode(err), svc.ErrorMessage(err))
			return
		}
		httpx.OK(c, http.StatusCreated, out)
	})
	g.GET("/model-configs/:id", func(c *gin.Context) {
		out, err := cfg.Get(c.Request.Context(), c.Param("id"))
		if err != nil {
			httpx.Fail(c, svc.HTTPStatus(err), svc.ErrorCode(err), svc.ErrorMessage(err))
			return
		}
		httpx.OK(c, http.StatusOK, out)
	})
	g.POST("/model-config-versions/:id/test", func(c *gin.Context) {
		out, err := cfg.StartTest(c.Request.Context(), c.Param("id"))
		if err != nil {
			httpx.Fail(c, svc.HTTPStatus(err), svc.ErrorCode(err), svc.ErrorMessage(err))
			return
		}
		httpx.OK(c, http.StatusAccepted, out)
	})
	g.GET("/tests/:test_id", func(c *gin.Context) {
		out, err := cfg.GetTest(c.Request.Context(), c.Param("test_id"))
		if err != nil {
			httpx.Fail(c, svc.HTTPStatus(err), svc.ErrorCode(err), svc.ErrorMessage(err))
			return
		}
		httpx.OK(c, http.StatusOK, out)
	})
	g.POST("/model-config-versions/:id/activate", func(c *gin.Context) {
		if err := cfg.Activate(c.Request.Context(), c.Param("id")); err != nil {
			httpx.Fail(c, svc.HTTPStatus(err), svc.ErrorCode(err), svc.ErrorMessage(err))
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"status": "activated"})
	})
	g.GET("/data-sources", func(c *gin.Context) {
		out, err := cfg.List(c.Request.Context(), "source")
		if err != nil {
			httpx.Fail(c, svc.HTTPStatus(err), svc.ErrorCode(err), svc.ErrorMessage(err))
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"items": out})
	})
	g.POST("/data-sources", func(c *gin.Context) {
		var req struct {
			Public map[string]any `json:"public_config"`
			APIKey string         `json:"api_key"`
		}
		if !httpx.BindJSON(c, &req) {
			return
		}
		out, err := cfg.SaveDataSource(c.Request.Context(), req.Public, req.APIKey)
		if err != nil {
			httpx.Fail(c, svc.HTTPStatus(err), svc.ErrorCode(err), svc.ErrorMessage(err))
			return
		}
		httpx.OK(c, http.StatusCreated, out)
	})
	g.POST("/data-sources/:id/test", func(c *gin.Context) {
		out, err := cfg.StartTest(c.Request.Context(), c.Param("id"))
		if err != nil {
			httpx.Fail(c, svc.HTTPStatus(err), svc.ErrorCode(err), svc.ErrorMessage(err))
			return
		}
		httpx.OK(c, http.StatusAccepted, out)
	})
	g.POST("/data-sources/:id/activate", func(c *gin.Context) {
		if err := cfg.Activate(c.Request.Context(), c.Param("id")); err != nil {
			httpx.Fail(c, svc.HTTPStatus(err), svc.ErrorCode(err), svc.ErrorMessage(err))
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"status": "activated"})
	})
	g.GET("/policy", func(c *gin.Context) {
		out, err := cfg.GetPolicy(c.Request.Context())
		if err != nil {
			httpx.Fail(c, svc.HTTPStatus(err), svc.ErrorCode(err), svc.ErrorMessage(err))
			return
		}
		httpx.OK(c, http.StatusOK, out)
	})
	g.PATCH("/policy", func(c *gin.Context) {
		var in svc.PolicyView
		if !httpx.BindJSON(c, &in) {
			return
		}
		out, err := cfg.PatchPolicy(c.Request.Context(), in)
		if err != nil {
			httpx.Fail(c, svc.HTTPStatus(err), svc.ErrorCode(err), svc.ErrorMessage(err))
			return
		}
		httpx.OK(c, http.StatusOK, out)
	})
	g.GET("/research-runs", func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		ctx := svc.WithUser(c.Request.Context(), httpx.CurrentUserID(c), "admin")
		items, err := research.AdminListRuns(ctx, limit)
		if err != nil {
			httpx.Fail(c, svc.HTTPStatus(err), svc.ErrorCode(err), svc.ErrorMessage(err))
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"items": items})
	})
}
