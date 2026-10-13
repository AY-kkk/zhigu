// Package ops owns private operational endpoints. Probe errors are deliberately
// reduced to stable reasons before being exposed to a caller or metrics scraper.
package ops

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"zhigu/server/httpx"
	"zhigu/server/runtime"
)

type Probe interface{ Check(context.Context) error }
type ProbeFunc func(context.Context) error

func (f ProbeFunc) Check(ctx context.Context) error { return f(ctx) }

type Module struct {
	Name         string         `json:"name"`
	State        runtime.Module `json:"-"`
	Version      string         `json:"version"`
	ConfigDigest string         `json:"config_digest"`
	Worker       Probe          `json:"-"`
}

type moduleReport struct {
	Name                  string                        `json:"name"`
	Enabled               bool                          `json:"enabled"`
	DeploymentEnvironment runtime.DeploymentEnvironment `json:"deployment_environment"`
	OperationMode         runtime.OperationMode         `json:"operation_mode"`
	DataProvenance        runtime.DataProvenance        `json:"data_provenance"`
	Health                runtime.Health                `json:"health"`
	Reason                string                        `json:"reason"`
	Version               string                        `json:"version"`
	ConfigDigest          string                        `json:"config_digest"`
	WorkerReady           bool                          `json:"worker_ready"`
}

type readiness struct {
	Status        string         `json:"status"`
	DatabaseReady bool           `json:"database_ready"`
	Modules       []moduleReport `json:"modules"`
}

type requestKey struct {
	Method, Route string
	Status        int
}
type requestMetric struct {
	Count   uint64
	Seconds float64
}

type Monitor struct {
	database     Probe
	modules      []Module
	token        string
	mu           sync.Mutex
	requests     map[requestKey]requestMetric
	loginLimited uint64
}

func New(database Probe, modules []Module, internalToken string) *Monitor {
	return &Monitor{database: database, modules: append([]Module(nil), modules...), token: internalToken, requests: map[requestKey]requestMetric{}}
}

func (m *Monitor) snapshot(ctx context.Context) (readiness, bool) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	report := readiness{Status: "ready", Modules: []moduleReport{}}
	report.DatabaseReady = m.database != nil && m.database.Check(ctx) == nil
	ready := report.DatabaseReady
	for _, module := range m.modules {
		state := module.State
		item := moduleReport{Name: module.Name, Enabled: state.Enabled, DeploymentEnvironment: state.DeploymentEnvironment, OperationMode: state.OperationMode, DataProvenance: state.DataProvenance, Health: state.Health, Version: module.Version, ConfigDigest: module.ConfigDigest}
		if !state.Enabled || state.OperationMode == runtime.Off {
			item.Health, item.Reason = runtime.Unavailable, "MODULE_DISABLED"
		} else if !report.DatabaseReady {
			item.Health, item.Reason = runtime.Unavailable, "DATABASE_UNAVAILABLE"
			ready = false
		} else if state.Health == runtime.Unavailable {
			item.Reason = "MODULE_UNAVAILABLE"
			ready = false
		} else if state.OperationMode == runtime.Live {
			item.WorkerReady = module.Worker != nil && module.Worker.Check(ctx) == nil
			if item.WorkerReady {
				item.Health = runtime.Ready
			} else {
				item.Health, item.Reason = runtime.Unavailable, "WORKER_UNAVAILABLE"
				ready = false
			}
		} else {
			// Read-only modules do not require an execution worker; their data health
			// remains independent from the host's ability to serve existing records.
			item.Reason = "READ_ONLY"
		}
		report.Modules = append(report.Modules, item)
	}
	if !ready {
		report.Status = "unavailable"
	}
	return report, ready
}

func (m *Monitor) Register(engine *gin.Engine) {
	alive := func(c *gin.Context) { httpx.OK(c, 200, gin.H{"status": "ok"}) }
	engine.GET("/livez", alive)
	// Compatibility alias: liveness only, never a database or worker probe.
	engine.GET("/healthz", alive)
	engine.GET("/readyz", func(c *gin.Context) {
		report, ready := m.snapshot(c.Request.Context())
		if ready {
			httpx.OK(c, 200, report)
			return
		}
		delay := 30
		c.Header("Retry-After", strconv.Itoa(delay))
		c.JSON(503, httpx.Envelope{Data: report, Error: &httpx.APIErr{Code: "NOT_READY", Message: "服务依赖暂不可用", Retryable: true, RetryAfterSeconds: &delay}, TraceID: httpx.TraceID(c)})
	})
	engine.GET("/metrics", func(c *gin.Context) {
		// Hashing makes the comparison constant time independent of token length.
		want := sha256.Sum256([]byte(m.token))
		raw := c.GetHeader("Authorization")
		provided := strings.TrimPrefix(raw, "Bearer ")
		got := sha256.Sum256([]byte(provided))
		if m.token == "" || !strings.HasPrefix(raw, "Bearer ") || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			httpx.Fail(c, 401, "UNAUTHENTICATED", "内部服务凭据无效")
			return
		}
		report, ready := m.snapshot(c.Request.Context())
		m.mu.Lock()
		entries := make(map[requestKey]requestMetric, len(m.requests))
		for key, value := range m.requests {
			entries[key] = value
		}
		limited := m.loginLimited
		m.mu.Unlock()
		keys := make([]requestKey, 0, len(entries))
		for key := range entries {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j]) })
		var out strings.Builder
		out.WriteString("# TYPE zhigu_http_requests_total counter\n# TYPE zhigu_http_request_duration_seconds summary\n")
		for _, key := range keys {
			labels := fmt.Sprintf(`method=%q,route=%q,status=%q`, key.Method, key.Route, strconv.Itoa(key.Status))
			value := entries[key]
			fmt.Fprintf(&out, "zhigu_http_requests_total{%s} %d\nzhigu_http_request_duration_seconds_sum{%s} %g\nzhigu_http_request_duration_seconds_count{%s} %d\n", labels, value.Count, labels, value.Seconds, labels, value.Count)
		}
		fmt.Fprintf(&out, "# TYPE zhigu_login_rate_limited_total counter\nzhigu_login_rate_limited_total %d\n# TYPE zhigu_ready gauge\nzhigu_ready %d\n# TYPE zhigu_database_ready gauge\nzhigu_database_ready %d\n# TYPE zhigu_module_ready gauge\n# TYPE zhigu_worker_ready gauge\n", limited, bit(ready), bit(report.DatabaseReady))
		for _, module := range report.Modules {
			fmt.Fprintf(&out, "zhigu_module_ready{module=%q} %d\nzhigu_worker_ready{module=%q} %d\n", module.Name, bit(module.Enabled && module.Health == runtime.Ready), module.Name, bit(module.WorkerReady))
		}
		c.Data(200, "text/plain; version=0.0.4; charset=utf-8", []byte(out.String()))
	})
}

func bit(value bool) int {
	if value {
		return 1
	}
	return 0
}

// Instrument uses Gin route templates and a bounded method set, never raw paths,
// query strings, account identifiers, tokens, or metrics requests as labels.
func (m *Monitor) Instrument() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}
		start := time.Now()
		httpx.AfterResponse(c, func() {
			route := c.FullPath()
			if route == "" {
				route = "unmatched"
			}
			method := c.Request.Method
			switch method {
			case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS":
			default:
				method = "OTHER"
			}
			key := requestKey{Method: method, Route: route, Status: c.Writer.Status()}
			m.mu.Lock()
			defer m.mu.Unlock()
			value := m.requests[key]
			value.Count++
			value.Seconds += time.Since(start).Seconds()
			m.requests[key] = value
			if key.Status == 429 && (route == "/login" || strings.Contains(route, "/auth/login")) {
				m.loginLimited++
			}
		})
		c.Next()
	}
}

// RequireReady gates production writes and paid operations at the Go boundary.
func (m *Monitor) RequireReady() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		authRoute := strings.HasPrefix(path, "/api/finance/auth/") || strings.HasPrefix(path, "/api/v1/session/")
		if strings.HasPrefix(path, "/api/") && c.Request.Method != "GET" && c.Request.Method != "HEAD" && c.Request.Method != "OPTIONS" && !authRoute {
			if _, ready := m.snapshot(c.Request.Context()); !ready {
				httpx.Fail(c, 503, "NOT_READY", "服务依赖暂不可用")
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// HTTPProbe checks the private worker readiness contract with a bounded body.
func HTTPProbe(base, token string, client *http.Client) Probe {
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return ProbeFunc(func(ctx context.Context) error {
		if base == "" || token == "" {
			return errors.New("worker configuration unavailable")
		}
		req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(base, "/")+"/readyz", nil)
		if err != nil {
			return errors.New("worker configuration unavailable")
		}
		req.Header.Set("Authorization", "Bearer "+token)
		response, err := client.Do(req)
		if err != nil {
			return errors.New("worker unavailable")
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			return errors.New("worker unavailable")
		}
		return nil
	})
}
