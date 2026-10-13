package ops

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"zhigu/server/httpx"
	"zhigu/server/runtime"
)

func routerFor(p Probe, modules []Module) (*gin.Engine, *Monitor) {
	r := gin.New()
	httpx.InstallTrace(r)
	m := New(p, modules, "private-metrics-secret")
	r.Use(m.Instrument())
	m.Register(r)
	return r, m
}

func request(r http.Handler, path, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	r.ServeHTTP(w, req)
	return w
}

func TestLivenessNeverProbesDependencies(t *testing.T) {
	calls := 0
	r, _ := routerFor(ProbeFunc(func(context.Context) error { calls++; return errors.New("postgres://secret") }), nil)
	for _, path := range []string{"/livez", "/healthz"} {
		if w := request(r, path, ""); w.Code != 200 {
			t.Fatalf("liveness status: %d", w.Code)
		}
	}
	if calls != 0 {
		t.Fatal("liveness performed dependency IO")
	}
}

func TestReadinessFailsClosedWithoutSecretsAndReportsModules(t *testing.T) {
	module := Module{Name: "research", State: runtime.Module{Enabled: true, State: runtime.State{DeploymentEnvironment: runtime.Production, OperationMode: runtime.Live, DataProvenance: runtime.Real, Health: runtime.Ready}}, Version: "abc", ConfigDigest: "digest", Worker: ProbeFunc(func(context.Context) error { return errors.New("api-key=secret") })}
	r, _ := routerFor(ProbeFunc(func(context.Context) error { return nil }), []Module{module})
	w := request(r, "/readyz", "")
	if w.Code != 503 || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("unsafe readiness: %s", w.Body.String())
	}
	var body struct {
		Data struct {
			Modules []map[string]any `json:"modules"`
		}
		Error *httpx.APIErr
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error == nil || !body.Error.Retryable {
		t.Fatal("missing retryable readiness error")
	}
	if len(body.Data.Modules) != 1 {
		t.Fatal("readiness modules absent")
	}
	for _, key := range []string{"enabled", "deployment_environment", "operation_mode", "data_provenance", "health", "reason", "version", "config_digest"} {
		if _, ok := body.Data.Modules[0][key]; !ok {
			t.Errorf("missing module field %s", key)
		}
	}
	r, _ = routerFor(ProbeFunc(func(context.Context) error { return errors.New("dsn=secret") }), nil)
	w = request(r, "/readyz", "")
	if w.Code != 503 || strings.Contains(w.Body.String(), "secret") {
		t.Fatal("database failure not isolated")
	}
}

func TestReadinessDisabledModulesDoNotNeedWorkers(t *testing.T) {
	r, _ := routerFor(ProbeFunc(func(context.Context) error { return nil }), []Module{{Name: "futures", State: runtime.Module{State: runtime.State{OperationMode: runtime.Off, Health: runtime.Unavailable}}, Worker: ProbeFunc(func(context.Context) error { t.Fatal("disabled worker probed"); return nil })}})
	if w := request(r, "/readyz", ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
}

func TestPrivateMetricsCountsRequestsWithoutUnboundedLabels(t *testing.T) {
	r, _ := routerFor(ProbeFunc(func(context.Context) error { return nil }), nil)
	r.GET("/login", func(c *gin.Context) { httpx.Fail(c, 429, "RATE_LIMITED", "safe") })
	r.GET("/panic", func(c *gin.Context) { panic("secret") })
	request(r, "/panic", "")
	request(r, "/login", "")
	request(r, "/unknown/account-secret", "")
	request(r, "/readyz", "")
	for _, token := range []string{"", "wrong"} {
		if w := request(r, "/metrics", token); w.Code != 401 {
			t.Fatal("metrics accessible without token")
		}
	}
	w := request(r, "/metrics", "private-metrics-secret")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("metrics status/type: %d %s", w.Code, w.Body.String())
	}
	for _, metric := range []string{"zhigu_http_requests_total", "zhigu_http_request_duration_seconds_sum", "zhigu_http_request_duration_seconds_count", "zhigu_login_rate_limited_total 1", "zhigu_ready 1", `status="429"`, `route="/panic",status="500"`, `route="unmatched"`} {
		if !strings.Contains(w.Body.String(), metric) {
			t.Errorf("missing metric %s", metric)
		}
	}
	if strings.Contains(w.Body.String(), "account-secret") || strings.Contains(w.Body.String(), "/metrics") || strings.Contains(w.Body.String(), "private-metrics-secret") {
		t.Fatal("metrics contains unsafe labels or counts itself")
	}
	r, _ = routerFor(ProbeFunc(func(context.Context) error { return nil }), nil)
	m := New(nil, nil, "")
	r = gin.New()
	httpx.InstallTrace(r)
	m.Register(r)
	if w := request(r, "/metrics", "anything"); w.Code != 401 {
		t.Fatal("empty metrics token must fail closed")
	}
}

func TestReadinessAdmissionRejectsWritesAndAllowsOperationalAndAuthRoutes(t *testing.T) {
	r, m := routerFor(ProbeFunc(func(context.Context) error { return errors.New("db down") }), nil)
	r.Use(m.RequireReady())
	r.POST("/api/finance/research", func(c *gin.Context) { httpx.OK(c, 202, nil) })
	r.POST("/api/finance/auth/login", func(c *gin.Context) { httpx.OK(c, 200, nil) })
	r.POST("/api/v1/session/logout", func(c *gin.Context) { httpx.OK(c, 200, nil) })
	for _, tt := range []struct {
		method, path string
		status       int
	}{{"POST", "/api/finance/research", 503}, {"POST", "/api/finance/auth/login", 200}, {"POST", "/api/v1/session/logout", 200}, {"GET", "/livez", 200}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
		if w.Code != tt.status {
			t.Errorf("%s status %d", tt.path, w.Code)
		}
	}
}

func TestHTTPWorkerProbeUsesAuthTimeoutAndFailsOnNonReady(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" || r.Header.Get("Authorization") != "Bearer worker-token" {
			t.Error("worker auth/path missing")
		}
		w.WriteHeader(503)
	}))
	defer server.Close()
	p := HTTPProbe(server.URL, "worker-token", &http.Client{Timeout: time.Second})
	if p.Check(context.Background()) == nil {
		t.Fatal("unready worker accepted")
	}
	if HTTPProbe("", "", nil).Check(context.Background()) == nil {
		t.Fatal("missing worker URL accepted")
	}
}
