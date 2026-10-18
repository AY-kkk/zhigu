package futures

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	model "zhigu/server/model/futures"
	"zhigu/server/ops"
)

func scrapeOperationalMetrics(monitor *ops.Monitor) string {
	r := gin.New()
	monitor.Register(r)
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Bearer metrics-test")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Body.String()
}

func TestActualFuturesWorkerAndModelCallsAreObserved(t *testing.T) {
	t.Setenv("FUTURES_MODEL_CONFIG_VERSION", "synthetic-v1")
	db := repoTestDB(t)
	draft := reviewRegressionDraft(t, db)
	monitor := ops.New(nil, nil, "metrics-test")
	domain := NewDomain(db, "").WithMode("live").WithObserver(monitor)
	created, err := domain.CreateRun(context.Background(), Identity{OwnerID: 1, Mode: "live"}, "observed-run", RunCreateInput{DraftID: draft.ID, ExpectedRevision: 1, Claims: []map[string]any{{"id": "claim_regression", "kind": "hypothesis", "text": "synthetic-only", "locator": nil}}, AcceptLimits: true})
	if err != nil {
		t.Fatal(err)
	}
	var run model.Run
	if err := db.First(&run, "id=?", created["id"]).Error; err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "execute") {
			w.WriteHeader(503)
			return
		}
		if r.URL.Path == "/invalid" {
			w.Write([]byte(`{`))
			return
		}
		w.Write([]byte(`{"content":"ok","usage":{"prompt_tokens":4,"completion_tokens":2}}`))
	}))
	defer server.Close()
	t.Setenv("FUTURES_WORKER_URL", server.URL)
	grants := NewGrantManager(NewMemoryGrantStore(), LeaseAuthorizerFunc(func(TaskLease) error { return nil }), time.Now)
	if err := ExecuteRun(context.Background(), db, domain, grants, &run); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected worker 503, got %v", err)
	}
	t.Setenv("FUTURES_MODEL_URL", server.URL+"/model")
	t.Setenv("FUTURES_MODEL_NAME", "synthetic-model")
	t.Setenv("FUTURES_MODEL_PRICE_CNY_PER_1K", "0.01")
	budget := NewDBBudget(db)
	day := beijingDay(time.Now().UTC())
	if err := budget.SeedAccount(context.Background(), UserAccount, 1, day, decimal.RequireFromString("5"), 8, 24, 48000); err != nil {
		t.Fatal(err)
	}
	if err := budget.SeedAccount(context.Background(), ModuleAccount, 0, day, decimal.RequireFromString("50"), 80, 240, 480000); err != nil {
		t.Fatal(err)
	}
	if _, err := domain.ExecuteInternalModel(context.Background(), GrantSpec{OwnerID: 1, Mode: "live", RunID: run.ID}, map[string]any{"messages": []any{map[string]any{"role": "user", "content": "synthetic"}}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FUTURES_MODEL_URL", server.URL+"/invalid")
	if _, err := domain.ExecuteInternalModel(context.Background(), GrantSpec{OwnerID: 1, Mode: "live", RunID: run.ID}, map[string]any{"messages": []any{}}); err == nil {
		t.Fatal("malformed model response accepted")
	}
	body := scrapeOperationalMetrics(monitor)
	for _, want := range []string{`zhigu_worker_calls_total{name="futures"} 1`, `zhigu_worker_errors_total{name="futures"} 1`, `zhigu_provider_calls_total{name="model"} 2`, `zhigu_provider_errors_total{name="model"} 1`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s", want)
		}
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	monitor.SetMetricsProvider(ops.NewSQLMetricsProvider(nil, pool, "", ""))
	if body := scrapeOperationalMetrics(monitor); !strings.Contains(body, `zhigu_budget_available{module="futures"} 1`) || !strings.Contains(body, `zhigu_budget_unknown_calls{module="futures"} 1`) {
		t.Fatal("actual model accounting day or unknown usage not observed")
	}
}

func TestMaintenanceObservesRealBacklog(t *testing.T) {
	db := repoTestDB(t)
	monitor := ops.New(nil, nil, "metrics-test")
	domain := NewDomain(db, "").WithObserver(monitor)
	if err := db.Exec(`INSERT INTO futures_deletion_jobs(id,owner_id,mode,target_type,target_id,impact_version,purge_due_at,state) VALUES('synthetic',1,'live','run','synthetic','v1',now(),'retry')`).Error; err != nil {
		t.Fatal(err)
	}
	domain.ObserveCleanupBacklog(context.Background())
	if body := scrapeOperationalMetrics(monitor); !strings.Contains(body, `zhigu_cleanup_backlog{module="futures"} 1`) {
		t.Fatal("database backlog not observed")
	}
}
