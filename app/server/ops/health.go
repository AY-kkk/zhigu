// Package ops owns private operational endpoints. Probe errors are deliberately
// reduced to stable reasons before being exposed to a caller or metrics scraper.
package ops

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
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
	database        Probe
	modules         []Module
	token           string
	mu              sync.Mutex
	requests        map[requestKey]requestMetric
	loginLimited    uint64
	metricsProvider MetricsProvider
	workerCalls     map[string]callMetric
	providerCalls   map[string]callMetric
	budgets         map[string]MetricValue
	cleanup         map[string]MetricValue
	backup          MetricValue
}

func New(database Probe, modules []Module, internalToken string) *Monitor {
	return &Monitor{database: database, modules: append([]Module(nil), modules...), token: internalToken, requests: map[requestKey]requestMetric{}, workerCalls: map[string]callMetric{}, providerCalls: map[string]callMetric{}, budgets: map[string]MetricValue{}, cleanup: map[string]MetricValue{}}
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
		intentionalOff := state.OperationMode == runtime.Off && (state.Health != runtime.Unavailable || state.Reason == "MODULE_DISABLED" || state.Reason == "FUTURES_DISABLED")
		if !state.Enabled || intentionalOff {
			item.Health, item.Reason = runtime.Unavailable, "MODULE_DISABLED"
		} else if state.Health == runtime.Unavailable {
			item.Reason = safeModuleReason(state.Reason)
			ready = false
		} else if !report.DatabaseReady {
			item.Health, item.Reason = runtime.Unavailable, "DATABASE_UNAVAILABLE"
			ready = false
		} else if state.OperationMode == runtime.Live {
			started := time.Now()
			item.WorkerReady = module.Worker != nil && module.Worker.Check(ctx) == nil
			if module.Worker != nil {
				m.ObserveWorker(module.Name, time.Since(started), !item.WorkerReady)
			}
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

func safeModuleReason(reason string) string {
	switch reason {
	case "FUTURES_MIGRATION_FAILED", "FUTURES_MIGRATION_UNAVAILABLE", "FUTURES_DATABASE_UNAVAILABLE", "FUTURES_NOT_IMPLEMENTED", "MODULE_UNAVAILABLE":
		return reason
	}
	return "MODULE_UNAVAILABLE"
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
			if !listed(module.Name, workerNames) {
				continue
			}
			fmt.Fprintf(&out, "zhigu_module_ready{module=%q} %d\nzhigu_worker_ready{module=%q} %d\n", module.Name, bit(module.Enabled && module.Health == runtime.Ready), module.Name, bit(module.WorkerReady))
		}
		m.writeOperationalMetrics(c.Request.Context(), &out)
		c.Data(200, "text/plain; version=0.0.4; charset=utf-8", []byte(out.String()))
	})
}

// MetricsProvider returns a fresh immutable snapshot. Collection errors make
// source-backed gauges unavailable, without invalidating independent observations.
type MetricsProvider interface {
	Snapshot(context.Context) (MetricSnapshot, error)
}
type MetricsProviderFunc func(context.Context) (MetricSnapshot, error)

func (f MetricsProviderFunc) Snapshot(ctx context.Context) (MetricSnapshot, error) { return f(ctx) }

type MetricValue struct {
	Available bool
	Value     float64
}
type QueueMetric struct {
	Available        bool
	Length           uint64
	OldestAgeSeconds float64
}
type RunMetric struct {
	Available bool
	Counts    map[string]uint64
}
type MetricSnapshot struct {
	DBPool            *sql.DBStats
	Queues            map[string]QueueMetric
	Runs              map[string]RunMetric
	MigrationVersion  string
	BackupLastSuccess MetricValue
	BackupObserved    bool
}
type callMetric struct {
	Count, Errors uint64
	Seconds       float64
}

var metricDomains = []string{"finance", "futures", "intel"}
var workerNames = []string{"research", "futures", "intel"}
var providerNames = []string{"ifind", "cninfo", "fuyao", "model", "research", "futures"}
var runStatuses = []string{"queued", "researching", "running", "verifying", "completed", "incomplete", "succeeded", "failed", "canceling", "canceled"}
var migrationMetricVersion = regexp.MustCompile(`^[0-9]{3,4}_[a-z0-9_]{1,48}$`)

func listed(value string, allowed []string) bool {
	for _, entry := range allowed {
		if entry == value {
			return true
		}
	}
	return false
}
func validMetric(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
func (m *Monitor) SetMetricsProvider(provider MetricsProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metricsProvider = provider
}
func (m *Monitor) ObserveWorker(name string, duration time.Duration, failed bool) {
	m.observeCall(name, duration, failed, true)
}
func (m *Monitor) ObserveProvider(name string, duration time.Duration, failed bool) {
	m.observeCall(name, duration, failed, false)
}
func (m *Monitor) observeCall(name string, duration time.Duration, failed, worker bool) {
	names := providerNames
	if worker {
		names = workerNames
	}
	if !listed(name, names) || duration < 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	target := m.providerCalls
	if worker {
		target = m.workerCalls
	}
	value := target[name]
	value.Count++
	value.Seconds += duration.Seconds()
	if failed {
		value.Errors++
	}
	target[name] = value
}
func (m *Monitor) SetBudgetRemaining(module string, value float64, available bool) {
	if !listed(module, metricDomains) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.budgets[module] = MetricValue{Available: available && validMetric(value), Value: value}
}
func (m *Monitor) SetCleanupBacklog(module string, value float64, available bool) {
	if !listed(module, metricDomains) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanup[module] = MetricValue{Available: available && validMetric(value), Value: value}
}
func (m *Monitor) SetBackupLastSuccess(value time.Time, available bool) {
	stamp := float64(value.Unix())
	m.mu.Lock()
	defer m.mu.Unlock()
	m.backup = MetricValue{Available: available && stamp > 0 && stamp <= float64(time.Now().Unix()), Value: stamp}
}
func writeGauge(out *strings.Builder, name, labels string, value float64) {
	formatted := strconv.FormatFloat(value, 'f', -1, 64)
	if labels != "" {
		fmt.Fprintf(out, "%s{%s} %s\n", name, labels, formatted)
	} else {
		fmt.Fprintf(out, "%s %s\n", name, formatted)
	}
}
func (m *Monitor) writeOperationalMetrics(ctx context.Context, out *strings.Builder) {
	m.mu.Lock()
	provider := m.metricsProvider
	m.mu.Unlock()
	snapshot := MetricSnapshot{}
	if provider != nil {
		probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		value, err := provider.Snapshot(probeCtx)
		cancel()
		if err == nil {
			snapshot = value
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// All names and states come from fixed vocabularies. Provider supplied names,
	// URLs, owner IDs, run IDs, error messages, and unknown status strings are ignored.
	for _, name := range []string{"db_pool_available", "db_pool_open_connections", "db_pool_in_use_connections", "db_pool_idle_connections", "db_pool_wait_total", "db_pool_wait_seconds_total", "queue_available", "queue_length", "queue_oldest_age_seconds", "runs_available", "runs", "budget_available", "budget_remaining", "cleanup_available", "cleanup_backlog", "backup_available", "backup_last_success_timestamp_seconds", "migration_available", "migration_info"} {
		kind := "gauge"
		if strings.HasSuffix(name, "_total") {
			kind = "counter"
		}
		fmt.Fprintf(out, "# TYPE zhigu_%s %s\n", name, kind)
	}
	writeGauge(out, "zhigu_db_pool_available", "", float64(bit(snapshot.DBPool != nil)))
	if pool := snapshot.DBPool; pool != nil {
		writeGauge(out, "zhigu_db_pool_open_connections", "", float64(pool.OpenConnections))
		writeGauge(out, "zhigu_db_pool_in_use_connections", "", float64(pool.InUse))
		writeGauge(out, "zhigu_db_pool_idle_connections", "", float64(pool.Idle))
		writeGauge(out, "zhigu_db_pool_wait_total", "", float64(pool.WaitCount))
		writeGauge(out, "zhigu_db_pool_wait_seconds_total", "", pool.WaitDuration.Seconds())
	}
	for _, module := range metricDomains {
		labels := fmt.Sprintf(`module=%q`, module)
		queue := snapshot.Queues[module]
		available := queue.Available && validMetric(queue.OldestAgeSeconds)
		writeGauge(out, "zhigu_queue_available", labels, float64(bit(available)))
		if available {
			writeGauge(out, "zhigu_queue_length", labels, float64(queue.Length))
			writeGauge(out, "zhigu_queue_oldest_age_seconds", labels, queue.OldestAgeSeconds)
		}
		runs := snapshot.Runs[module]
		writeGauge(out, "zhigu_runs_available", labels, float64(bit(runs.Available)))
		if runs.Available {
			for _, status := range runStatuses {
				writeGauge(out, "zhigu_runs", labels+fmt.Sprintf(`,status=%q`, status), float64(runs.Counts[status]))
			}
		}
		for _, entry := range []struct {
			available, value string
			sample           MetricValue
		}{{"zhigu_budget_available", "zhigu_budget_remaining", m.budgets[module]}, {"zhigu_cleanup_available", "zhigu_cleanup_backlog", m.cleanup[module]}} {
			writeGauge(out, entry.available, labels, float64(bit(entry.sample.Available)))
			if entry.sample.Available {
				writeGauge(out, entry.value, labels, entry.sample.Value)
			}
		}
	}
	for _, kind := range []string{"worker", "provider"} {
		names, calls := providerNames, m.providerCalls
		if kind == "worker" {
			names, calls = workerNames, m.workerCalls
		}
		fmt.Fprintf(out, "# TYPE zhigu_%s_observations_available gauge\n# TYPE zhigu_%s_calls_total counter\n# TYPE zhigu_%s_errors_total counter\n# TYPE zhigu_%s_call_duration_seconds summary\n", kind, kind, kind, kind)
		for _, name := range names {
			sample, available := calls[name]
			labels := fmt.Sprintf(`name=%q`, name)
			writeGauge(out, "zhigu_"+kind+"_observations_available", labels, float64(bit(available)))
			if available {
				writeGauge(out, "zhigu_"+kind+"_calls_total", labels, float64(sample.Count))
				writeGauge(out, "zhigu_"+kind+"_errors_total", labels, float64(sample.Errors))
				writeGauge(out, "zhigu_"+kind+"_call_duration_seconds_sum", labels, sample.Seconds)
				writeGauge(out, "zhigu_"+kind+"_call_duration_seconds_count", labels, float64(sample.Count))
			}
		}
	}
	backup := m.backup
	if snapshot.BackupObserved || snapshot.BackupLastSuccess.Available {
		backup = snapshot.BackupLastSuccess
	}
	backup.Available = backup.Available && validMetric(backup.Value) && backup.Value > 0 && backup.Value <= float64(time.Now().Unix())
	writeGauge(out, "zhigu_backup_available", "", float64(bit(backup.Available)))
	if backup.Available {
		writeGauge(out, "zhigu_backup_last_success_timestamp_seconds", "", backup.Value)
	}
	migrationAvailable := migrationMetricVersion.MatchString(snapshot.MigrationVersion)
	writeGauge(out, "zhigu_migration_available", "", float64(bit(migrationAvailable)))
	if migrationAvailable {
		writeGauge(out, "zhigu_migration_info", fmt.Sprintf(`version=%q`, snapshot.MigrationVersion), 1)
	}
}

// NewSQLMetricsProvider reads only fixed operational tables, aggregates across
// owners, and never returns DSNs, SQL errors, job IDs, or private payloads. The
// futures pool may be nil when its schema is not installed/enabled.
func NewSQLMetricsProvider(primary, futures *sql.DB, backupStateFile, backupTimestamp string) MetricsProvider {
	return MetricsProviderFunc(func(ctx context.Context) (MetricSnapshot, error) {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		snapshot := MetricSnapshot{Queues: map[string]QueueMetric{}, Runs: map[string]RunMetric{}, BackupObserved: backupStateFile != "" || backupTimestamp != "", BackupLastSuccess: ReadBackupLastSuccess(backupStateFile, backupTimestamp)}
		if primary != nil {
			stats := primary.Stats()
			snapshot.DBPool = &stats
			snapshot.Queues["finance"], snapshot.Runs["finance"] = readRunMetrics(ctx, primary, "finance_research_runs")
			var version string
			if primary.QueryRowContext(ctx, "SELECT version FROM finance_schema_migrations ORDER BY version DESC LIMIT 1").Scan(&version) == nil {
				snapshot.MigrationVersion = version
			}
		}
		if futures != nil {
			snapshot.Queues["futures"], snapshot.Runs["futures"] = readRunMetrics(ctx, futures, "futures_runs")
		}
		return snapshot, nil
	})
}

func readRunMetrics(ctx context.Context, pool *sql.DB, table string) (QueueMetric, RunMetric) {
	// table is selected only by NewSQLMetricsProvider's hard-coded call sites.
	if table != "finance_research_runs" && table != "futures_runs" {
		return QueueMetric{}, RunMetric{}
	}
	rows, err := pool.QueryContext(ctx, "SELECT status, count(*), COALESCE(GREATEST(EXTRACT(EPOCH FROM (CURRENT_TIMESTAMP - min(created_at))), 0), 0)::double precision FROM "+table+" WHERE deleted_at IS NULL GROUP BY status")
	if err != nil {
		return QueueMetric{}, RunMetric{}
	}
	defer rows.Close()
	queue := QueueMetric{Available: true}
	runs := RunMetric{Available: true, Counts: map[string]uint64{}}
	for rows.Next() {
		var status string
		var count uint64
		var age float64
		if rows.Scan(&status, &count, &age) != nil {
			return QueueMetric{}, RunMetric{}
		}
		if listed(status, runStatuses) {
			runs.Counts[status] = count
		}
		if status == "queued" {
			queue.Length = count
			queue.OldestAgeSeconds = age
		}
	}
	if rows.Err() != nil {
		return QueueMetric{}, RunMetric{}
	}
	return queue, runs
}

// The backup script atomically replaces a small JSON state file containing
// {"last_success_unix":1700000000}. Without a file, an explicit Unix-seconds
// environment snapshot is accepted. Invalid/missing input stays unavailable.
func ReadBackupLastSuccess(path, fallback string) MetricValue {
	var timestamp int64
	if path != "" {
		file, err := os.Open(path)
		if err != nil {
			return MetricValue{}
		}
		defer file.Close()
		raw, err := io.ReadAll(io.LimitReader(file, 4097))
		if err != nil || len(raw) > 4096 {
			return MetricValue{}
		}
		var state struct {
			LastSuccess int64 `json:"last_success_unix"`
		}
		if json.Unmarshal(raw, &state) != nil {
			return MetricValue{}
		}
		timestamp = state.LastSuccess
	} else {
		value, err := strconv.ParseInt(fallback, 10, 64)
		if err != nil {
			return MetricValue{}
		}
		timestamp = value
	}
	return MetricValue{Available: timestamp > 0 && timestamp <= time.Now().Unix(), Value: float64(timestamp)}
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
		paidInternal := strings.HasPrefix(path, "/internal/llm/v1/") || path == "/internal/futures/v1/model" || path == "/internal/futures/v1/tools"
		if (paidInternal || strings.HasPrefix(path, "/api/") && !authRoute) && c.Request.Method != "GET" && c.Request.Method != "HEAD" && c.Request.Method != "OPTIONS" {
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
