package futures

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"zhigu/server/httpx"
	svc "zhigu/server/service/futures"
)

type GrantRequest struct {
	Domain     string `json:"domain"`
	OwnerID    uint   `json:"owner_id"`
	Mode       string `json:"mode"`
	RunID      string `json:"run_id"`
	TaskID     string `json:"task_id"`
	Generation int64  `json:"generation"`
	ManifestID string `json:"manifest_id"`
	Stage      string `json:"stage"`
	ToolName   string `json:"tool_name"`
	ArgsHash   string `json:"args_hash"`
}

type InternalToolRequest struct {
	Scope      map[string]any `json:"scope"`
	TaskID     string         `json:"task_id"`
	Generation int64          `json:"generation"`
	ManifestID string         `json:"manifest_id"`
	Name       string         `json:"name"`
	Arguments  map[string]any `json:"arguments"`
	Stage      string         `json:"stage,omitempty"`
}

type ObservationIngestRequest struct {
	OwnerID uint                   `json:"owner_id"`
	Mode    string                 `json:"mode"`
	Records []svc.ObservationInput `json:"records"`
}

// RegisterInternal is loopback-only transport. Model/tool payloads are not
// published here; the worker must return candidates to the Go verifier.
func RegisterInternal(engine *gin.Engine, grants *svc.GrantManager, domain *svc.Domain) {
	group := engine.Group("/internal/futures/v1")
	group.Use(func(c *gin.Context) {
		expected := os.Getenv("FUTURES_SERVICE_TOKEN")
		provided := c.GetHeader("X-Futures-Service-Token")
		if expected == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
			httpx.Fail(c, http.StatusUnauthorized, "FUTURES_SERVICE_TOKEN_INVALID", "服务身份无效")
			c.Abort()
			return
		}
		c.Next()
	})
	group.POST("/grants", func(c *gin.Context) {
		var request GrantRequest
		if !BindJSON(c, &request) {
			return
		}
		if domain == nil || domain.AuthorizeExecution(c.Request.Context(), svc.Identity{OwnerID: request.OwnerID, Mode: request.Mode}) != nil {
			httpx.Fail(c, http.StatusForbidden, "FUTURES_GRANT_DENIED", "任务授权被拒绝")
			return
		}
		token, err := grants.Issue(svc.GrantSpec{
			Domain: request.Domain, OwnerID: request.OwnerID, Mode: request.Mode, RunID: request.RunID,
			TaskID: request.TaskID, Generation: request.Generation, ManifestID: request.ManifestID,
			Stage: request.Stage, ToolName: request.ToolName, ArgsHash: request.ArgsHash,
		})
		if err != nil {
			httpx.Fail(c, http.StatusForbidden, "FUTURES_GRANT_DENIED", "任务授权被拒绝")
			return
		}
		httpx.OK(c, http.StatusCreated, gin.H{"grant": token, "expires_in_seconds": 60})
	})
	group.POST("/observations", func(c *gin.Context) {
		var request ObservationIngestRequest
		if !BindJSON(c, &request) {
			return
		}
		if domain == nil {
			httpx.Fail(c, http.StatusServiceUnavailable, "FUTURES_DATA_PORT_NOT_CONNECTED", "数据入库端口未接通")
			return
		}
		result, err := domain.IngestObservations(c.Request.Context(), svc.Identity{OwnerID: request.OwnerID, Mode: request.Mode}, request.Records)
		if err != nil {
			httpx.Fail(c, http.StatusForbidden, "FUTURES_OBSERVATION_REJECTED", "观测数据未通过准入或授权校验")
			return
		}
		httpx.OK(c, http.StatusOK, result)
	})
	group.POST("/grants/consume", func(c *gin.Context) {
		grantToken := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if grantToken == "" {
			httpx.Fail(c, http.StatusUnauthorized, "FUTURES_TASK_GRANT_MISSING", "缺少短任务授权")
			return
		}
		var task map[string]any
		if !BindJSON(c, &task) {
			return
		}
		scope, _ := task["scope"].(map[string]any)
		identity := svc.Identity{OwnerID: uintFromMap(scope["owner_id"]), Mode: stringFromMap(scope["mode"])}
		if domain == nil || domain.AuthorizeExecution(c.Request.Context(), identity) != nil {
			httpx.Fail(c, http.StatusForbidden, "FUTURES_GRANT_DENIED", "任务授权被拒绝")
			return
		}
		spec := svc.GrantSpec{
			Domain: stringFromMap(scope["domain"]), OwnerID: uintFromMap(scope["owner_id"]), Mode: stringFromMap(scope["mode"]),
			RunID: stringFromMap(task["run_id"]), TaskID: stringFromMap(task["task_id"]), Generation: int64FromMap(task["generation"]),
			ManifestID: stringFromMap(task["manifest_id"]), Stage: "execute", ArgsHash: svc.HashArguments(task),
		}
		if _, err := grants.Use(grantToken, spec); err != nil {
			httpx.Fail(c, http.StatusForbidden, "FUTURES_GRANT_DENIED", "短任务授权无效、过期或已消费")
			return
		}
		httpx.OK(c, http.StatusOK, gin.H{"consumed": true})
	})
	for _, path := range []string{"/tools", "/model"} {
		group.POST(path, func(c *gin.Context) {
			grantToken := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
			if grantToken == "" {
				httpx.Fail(c, http.StatusUnauthorized, "FUTURES_TASK_GRANT_MISSING", "缺少短任务授权")
				return
			}
			var request InternalToolRequest
			if !BindJSON(c, &request) {
				return
			}
			spec := svc.GrantSpec{
				Domain: "futures", OwnerID: uintFromMap(request.Scope["owner_id"]), Mode: "live",
				RunID: stringFromMap(request.Scope["run_id"]), TaskID: request.TaskID, Generation: request.Generation,
				ManifestID: request.ManifestID, Stage: request.Stage, ToolName: request.Name,
				ArgsHash: svc.HashArguments(request.Arguments),
			}
			if c.FullPath() == "/internal/futures/v1/model" {
				spec.ToolName = ""
			}
			if _, err := grants.Use(grantToken, spec); err != nil {
				httpx.Fail(c, http.StatusForbidden, "FUTURES_GRANT_DENIED", "短任务授权无效、过期或已消费")
				return
			}
			if c.FullPath() == "/internal/futures/v1/model" {
				if domain == nil {
					httpx.Fail(c, http.StatusServiceUnavailable, "FUTURES_MODEL_NOT_CONFIGURED", "模型端点尚未配置")
					return
				}
				value, err := domain.ExecuteInternalModel(c.Request.Context(), spec, request.Arguments)
				if err != nil {
					httpx.Fail(c, http.StatusServiceUnavailable, "FUTURES_MODEL_NOT_CONFIGURED", "模型端点不可用")
					return
				}
				httpx.OK(c, http.StatusOK, value)
				return
			}
			if domain == nil {
				httpx.Fail(c, http.StatusServiceUnavailable, "FUTURES_TOOL_NOT_CONNECTED", "受限工具端口尚未接通")
				return
			}
			value, err := domain.ExecuteInternalTool(c.Request.Context(), svc.InternalToolCall{GrantSpec: spec, Name: request.Name, Arguments: request.Arguments})
			if err != nil {
				httpx.Fail(c, http.StatusForbidden, "FUTURES_TOOL_DENIED", "工具调用被拒绝")
				return
			}
			httpx.OK(c, http.StatusOK, value)
		})
	}
}

func uintFromMap(value any) uint {
	switch value := value.(type) {
	case float64:
		return uint(value)
	case int:
		return uint(value)
	default:
		return 0
	}
}
func stringFromMap(value any) string { result, _ := value.(string); return result }
func int64FromMap(value any) int64 {
	switch value := value.(type) {
	case float64:
		return int64(value)
	case int:
		return int64(value)
	case int64:
		return value
	default:
		return 0
	}
}
