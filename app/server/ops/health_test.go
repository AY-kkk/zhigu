package ops

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"zhigu/server/httpx"
	"zhigu/server/runtime"
	"zhigu/server/testdb"
)

func TestMain(m *testing.M) { code := m.Run(); testdb.StopIfStarted(); os.Exit(code) }

func TestSQLMetricsProviderReadsActualQueueRunsPoolAndLedger(t *testing.T) {
	db := testdb.Start(t)
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	for _, statement := range []string{
		"CREATE TEMP TABLE finance_research_runs(status text,created_at timestamptz,deleted_at timestamptz,claim_snapshot jsonb DEFAULT '[{\"text\":\"private\"}]'::jsonb)",
		"CREATE TEMP TABLE futures_runs(status text,created_at timestamptz,deleted_at timestamptz)",
		"CREATE TEMP TABLE finance_intel_jobs(status text,created_at timestamptz)",
		"CREATE TEMP TABLE finance_usage_ledger(status text,reserved_tokens bigint,actual_tokens bigint)",
		"CREATE TEMP TABLE finance_intel_usage(status text,reserved_tokens bigint,actual_tokens bigint)",
		"CREATE TEMP TABLE futures_budget_accounts(scope text,budget_date date,limit_cny numeric,reserved_cny numeric,spent_cny numeric)",
		"CREATE TEMP TABLE futures_budget_reservations(state text,amount_cny numeric)",
		"CREATE TEMP TABLE futures_usage(usage_state text)",
		"CREATE TEMP TABLE futures_deletion_jobs(state text)",
		"CREATE TEMP TABLE futures_documents(purge_at timestamptz,deleted_at timestamptz)",
		"CREATE TEMP TABLE finance_intel_namespaces(expires_at timestamptz)",
		"INSERT INTO finance_research_runs(status,created_at,deleted_at) VALUES('queued',now()-interval '20 seconds',NULL),('queued',now()-interval '10 seconds',NULL),('completed',now(),NULL),('queued',now()-interval '60 seconds',now()-interval '25 hours'),('queued',now(),now()-interval '1 hour')",
		"INSERT INTO finance_research_runs VALUES('queued',now(),now()-interval '25 hours','[ ]'::jsonb),('queued',now(),now()-interval '25 hours',NULL),('queued',now(),now()-interval '25 hours','null'::jsonb)",
		"INSERT INTO futures_runs VALUES('queued',now()-interval '5 seconds',NULL),('succeeded',now(),NULL)",
		"INSERT INTO finance_intel_jobs VALUES('queued',now()-interval '10 seconds'),('partial',now())",
		"INSERT INTO finance_usage_ledger VALUES('reserved',10,NULL),('succeeded',20,7),('unknown',30,NULL)",
		"INSERT INTO finance_intel_usage VALUES('reserved',11,NULL),('settled',20,8),('usage_unknown',31,NULL)",
		"INSERT INTO futures_budget_accounts VALUES('module',(date_trunc('day',now() AT TIME ZONE 'Asia/Shanghai') AT TIME ZONE 'Asia/Shanghai' AT TIME ZONE 'UTC')::date,50,2,3),('user',(date_trunc('day',now() AT TIME ZONE 'Asia/Shanghai') AT TIME ZONE 'Asia/Shanghai' AT TIME ZONE 'UTC')::date,5,2,3)",
		"INSERT INTO futures_budget_reservations VALUES('reserved',2),('unknown',4)",
		"INSERT INTO futures_usage VALUES('unknown')",
		"INSERT INTO futures_deletion_jobs VALUES('scheduled'),('retry'),('purged')",
		"INSERT INTO futures_documents VALUES(now()-interval '1 day',NULL)",
		"INSERT INTO finance_intel_namespaces VALUES(now()-interval '1 day'),(NULL)",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	defer db.Exec("DROP TABLE pg_temp.finance_research_runs, pg_temp.futures_runs, pg_temp.finance_intel_jobs, pg_temp.finance_usage_ledger, pg_temp.finance_intel_usage, pg_temp.futures_budget_accounts, pg_temp.futures_budget_reservations, pg_temp.futures_usage, pg_temp.futures_deletion_jobs, pg_temp.futures_documents, pg_temp.finance_intel_namespaces")
	provider := NewSQLMetricsProvider(pool, pool, "", "")
	snapshot, err := provider.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.DBPool == nil || !snapshot.Queues["finance"].Available || snapshot.Queues["finance"].Length != 2 || snapshot.Queues["finance"].OldestAgeSeconds < 19 || snapshot.Queues["finance"].OldestAgeSeconds > 30 {
		t.Fatalf("finance SQL snapshot incorrect: %+v", snapshot)
	}
	if snapshot.Runs["finance"].Counts["completed"] != 1 || snapshot.Queues["futures"].Length != 1 || snapshot.Runs["futures"].Counts["succeeded"] != 1 || snapshot.MigrationVersion == "" {
		t.Fatalf("actual futures/ledger snapshot missing: %+v", snapshot)
	}
	r, monitor := routerFor(ProbeFunc(func(context.Context) error { return nil }), nil)
	monitor.SetMetricsProvider(provider)
	body := request(r, "/metrics", "private-metrics-secret").Body.String()
	for _, want := range []string{
		`zhigu_queue_length{module="intel"} 1`, `zhigu_runs{module="intel",status="partial"} 1`,
		`zhigu_budget_usage{module="finance",state="reservation",unit="tokens"} 10`,
		`zhigu_budget_usage{module="finance",state="spent",unit="tokens"} 7`,
		`zhigu_budget_usage{module="finance",state="unknown",unit="tokens"} 30`,
		`zhigu_budget_usage{module="intel",state="unknown",unit="tokens"} 31`,
		`zhigu_budget_usage{module="futures",state="spent",unit="cny"} 3`,
		`zhigu_budget_usage{module="futures",state="unknown",unit="cny"} 4`,
		`zhigu_budget_unknown_calls{module="futures"} 1`,
		`zhigu_budget_remaining{module="futures"} 45`,
		`zhigu_cleanup_backlog{module="finance"} 1`, `zhigu_cleanup_backlog{module="futures"} 3`, `zhigu_cleanup_backlog{module="intel"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing SQL-backed metric: %s", want)
		}
	}
	if err := db.Exec("UPDATE finance_intel_jobs SET status='running' WHERE status='queued'").Error; err != nil {
		t.Fatal(err)
	}
	if body := request(r, "/metrics", "private-metrics-secret").Body.String(); !strings.Contains(body, `zhigu_queue_length{module="intel"} 0`) || !strings.Contains(body, `zhigu_runs{module="intel",status="running"} 1`) {
		t.Fatal("job status change not reflected by SQL snapshot")
	}
	if err := db.Exec("DELETE FROM futures_budget_accounts WHERE scope='module'").Error; err != nil {
		t.Fatal(err)
	}
	if body := request(r, "/metrics", "private-metrics-secret").Body.String(); !strings.Contains(body, `zhigu_budget_available{module="futures"} 0`) || strings.Contains(body, `zhigu_budget_remaining{module="futures"}`) {
		t.Fatal("missing module account invents remaining budget")
	}
	snapshot, err = NewSQLMetricsProvider(pool, nil, "", "").Snapshot(context.Background())
	if err != nil || snapshot.Queues["futures"].Available {
		t.Fatal("disabled/unconfigured futures queue reported available")
	}
}

func TestBackupMetricsAtomicStateAndMalformedFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup.json")
	if err := os.WriteFile(path, []byte(`{"last_success_unix":1700000000}`), 0600); err != nil {
		t.Fatal(err)
	}
	value := ReadBackupLastSuccess(path, "")
	if !value.Available || value.Value != 1700000000 {
		t.Fatalf("backup state unreadable: %+v", value)
	}
	if err := os.WriteFile(path, []byte(`{"last_success_unix":"secret"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if value := ReadBackupLastSuccess(path, "1700000000"); value.Available {
		t.Fatal("invalid configured state hidden by env fallback")
	}
	if value := ReadBackupLastSuccess("", "1700000000"); !value.Available {
		t.Fatal("backup env timestamp absent")
	}
	if value := ReadBackupLastSuccess("", "0"); value.Available {
		t.Fatal("unknown backup masquerades as success")
	}
}

func routerFor(p Probe, modules []Module) (*gin.Engine, *Monitor) {
	r := gin.New()
	httpx.InstallTrace(r)
	m := New(p, modules, "private-metrics-secret")
	r.Use(m.Instrument())
	m.Register(r)
	return r, m
}

func TestOperationalMetricsSnapshotsAndUnknownAvailability(t *testing.T) {
	r, m := routerFor(ProbeFunc(func(context.Context) error { return nil }), nil)
	m.SetMetricsProvider(MetricsProviderFunc(func(context.Context) (MetricSnapshot, error) {
		return MetricSnapshot{
			DBPool:           &sql.DBStats{OpenConnections: 4, InUse: 2, Idle: 2, WaitCount: 3, WaitDuration: 2 * time.Second},
			Queues:           map[string]QueueMetric{"finance": {Available: true, Length: 3, OldestAgeSeconds: 8}},
			Runs:             map[string]RunMetric{"finance": {Available: true, Counts: map[string]uint64{"queued": 3, "completed": 7}}},
			MigrationVersion: "010_sessions",
		}, nil
	}))
	m.ObserveWorker("research", time.Second, false)
	m.ObserveProvider("ifind", 2*time.Second, true)
	m.SetBudgetRemaining("finance", 12.5, true)
	m.SetCleanupBacklog("futures", 4, true)
	m.SetBackupLastSuccess(time.Unix(1700000000, 0), true)
	w := request(r, "/metrics", "private-metrics-secret")
	for _, metric := range []string{"zhigu_db_pool_open_connections 4", "zhigu_db_pool_in_use_connections 2", "zhigu_db_pool_idle_connections 2", "zhigu_db_pool_wait_total 3", "zhigu_db_pool_wait_seconds_total 2", `zhigu_queue_length{module="finance"} 3`, `zhigu_queue_oldest_age_seconds{module="finance"} 8`, `zhigu_runs{module="finance",status="completed"} 7`, `zhigu_worker_call_duration_seconds_sum{name="research"} 1`, `zhigu_provider_errors_total{name="ifind"} 1`, `zhigu_budget_remaining{module="finance"} 12.5`, `zhigu_cleanup_backlog{module="futures"} 4`, "zhigu_backup_last_success_timestamp_seconds 1700000000", `zhigu_migration_info{version="010_sessions"} 1`} {
		if !strings.Contains(w.Body.String(), metric) {
			t.Errorf("missing %s\n%s", metric, w.Body.String())
		}
	}
	r, _ = routerFor(ProbeFunc(func(context.Context) error { return nil }), nil)
	unknown := request(r, "/metrics", "private-metrics-secret").Body.String()
	for _, metric := range []string{"zhigu_db_pool_available 0", `zhigu_queue_available{module="finance"} 0`, `zhigu_budget_available{module="finance"} 0`, `zhigu_cleanup_available{module="finance"} 0`, "zhigu_backup_available 0", "zhigu_migration_available 0", `zhigu_worker_observations_available{name="research"} 0`, `zhigu_provider_observations_available{name="ifind"} 0`} {
		if !strings.Contains(unknown, metric) {
			t.Errorf("unknown availability not exposed: %s", metric)
		}
	}
	if strings.Contains(unknown, "\nzhigu_budget_remaining{") || strings.Contains(unknown, "\nzhigu_backup_last_success_timestamp_seconds ") {
		t.Fatal("unknown value reported as known zero")
	}
}

func TestOperationalMetricsConcurrentBoundedObservation(t *testing.T) {
	r, m := routerFor(ProbeFunc(func(context.Context) error { return nil }), nil)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				m.ObserveWorker("research", time.Millisecond, j%2 == 0)
				m.ObserveProvider("ifind", time.Millisecond, false)
				m.ObserveProvider("https://secret-provider/account-id", time.Second, true)
				m.SetBudgetRemaining("account-secret", 4, true)
				m.SetBudgetRemaining("finance", math.NaN(), true)
				m.SetCleanupBacklog("finance", 3, true)
				m.SetBackupLastSuccess(time.Now(), false)
				w := request(r, "/metrics", "private-metrics-secret")
				if strings.Contains(w.Body.String(), "secret-provider") || strings.Contains(w.Body.String(), "account-secret") || strings.Contains(w.Body.String(), "NaN") {
					t.Error("unsafe metric value/label")
				}
			}
		}()
	}
	wg.Wait()
	w := request(r, "/metrics", "private-metrics-secret")
	if !strings.Contains(w.Body.String(), `zhigu_worker_calls_total{name="research"} 160`) {
		t.Fatal("concurrent observations lost")
	}
}

func TestExternalMetricSnapshotsCannotIntroduceSensitiveLabels(t *testing.T) {
	r, m := routerFor(ProbeFunc(func(context.Context) error { return nil }), []Module{{Name: "https://secret-module/owner", State: runtime.Module{}}})
	m.SetMetricsProvider(MetricsProviderFunc(func(context.Context) (MetricSnapshot, error) {
		return MetricSnapshot{
			Queues:           map[string]QueueMetric{"owner-secret": {Available: true, Length: 99}},
			Runs:             map[string]RunMetric{"finance": {Available: true, Counts: map[string]uint64{"queued": 2, "status-secret": 9}}},
			Budgets:          map[string]BudgetMetric{"finance": {Available: true, Unit: "unit-secret"}, "owner-secret": {Available: true, Unit: "tokens"}},
			Cleanup:          map[string]MetricValue{"finance": {Available: true, Value: math.NaN()}, "owner-secret": {Available: true, Value: 9}},
			MigrationVersion: "/token-secret/path",
		}, nil
	}))
	w := request(r, "/metrics", "private-metrics-secret")
	if strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), "NaN") {
		t.Fatalf("snapshot introduced secret label: %s", w.Body.String())
	}
	m.SetMetricsProvider(MetricsProviderFunc(func(context.Context) (MetricSnapshot, error) { return MetricSnapshot{}, errors.New("dsn=secret") }))
	w = request(r, "/metrics", "private-metrics-secret")
	if strings.Contains(w.Body.String(), "secret") || !strings.Contains(w.Body.String(), "zhigu_db_pool_available 0") {
		t.Fatal("failed provider leaked error or fabricated availability")
	}
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

func TestReadinessBlocksInternalPaidCallsBeforeReservation(t *testing.T) {
	r, m := routerFor(ProbeFunc(func(context.Context) error { return errors.New("unavailable") }), nil)
	r.Use(m.RequireReady())
	reservations := 0
	for _, path := range []string{"/internal/llm/v1/chat/completions", "/internal/llm/v1/responses", "/internal/futures/v1/model", "/internal/futures/v1/tools", "/internal/finance/tool-grants", "/internal/finance/data-query"} {
		r.POST(path, func(c *gin.Context) { reservations++; httpx.OK(c, 200, nil) })
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", path, nil))
		if w.Code != 503 {
			t.Errorf("paid path admitted: %s status=%d", path, w.Code)
		}
	}
	if reservations != 0 {
		t.Fatal("unready request reached reservation handler")
	}
	r.POST("/internal/finance/tool-grants/:id/complete", func(c *gin.Context) { httpx.OK(c, 200, nil) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/internal/finance/tool-grants/grant/complete", nil))
	if w.Code != 200 {
		t.Fatal("non-paid callback blocked")
	}
}

func TestEnabledBootstrapFailureCannotMasqueradeAsOff(t *testing.T) {
	for _, reason := range []string{"FUTURES_MIGRATION_FAILED", "FUTURES_MIGRATION_UNAVAILABLE", "FUTURES_DATABASE_UNAVAILABLE"} {
		module := Module{Name: "futures", State: runtime.Module{Enabled: true, State: runtime.State{OperationMode: runtime.Off, Health: runtime.Unavailable, Reason: reason}}}
		r, _ := routerFor(ProbeFunc(func(context.Context) error { return nil }), []Module{module})
		w := request(r, "/readyz", "")
		if w.Code != 503 || !strings.Contains(w.Body.String(), `"reason":"`+reason+`"`) {
			t.Errorf("bootstrap failure hidden: %s", w.Body.String())
		}
		r, _ = routerFor(ProbeFunc(func(context.Context) error { return errors.New("db unavailable") }), []Module{module})
		w = request(r, "/readyz", "")
		if !strings.Contains(w.Body.String(), `"reason":"`+reason+`"`) {
			t.Errorf("bootstrap reason lost during database outage: %s", w.Body.String())
		}
	}
	module := Module{Name: "futures", State: runtime.Module{Enabled: true, State: runtime.State{OperationMode: runtime.Off, Health: runtime.Unavailable, Reason: "MODULE_DISABLED"}}}
	r, _ := routerFor(ProbeFunc(func(context.Context) error { return nil }), []Module{module})
	if w := request(r, "/readyz", ""); w.Code != 200 {
		t.Fatal("intentional off blocked host")
	}
}
