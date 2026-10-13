package finance

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"zhigu/server/ops"
)

func operationalMetrics(t *testing.T, monitor *ops.Monitor) string {
	t.Helper()
	r := gin.New()
	monitor.Register(r)
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Bearer metrics-test")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	return w.Body.String()
}

func TestResearchActualHTTPCallsUpdateWorkerMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/cancel"):
			w.Write([]byte(`{"status":"canceled"}`))
		case strings.HasSuffix(r.URL.Path, "/ack"):
			w.WriteHeader(200)
		case strings.HasSuffix(r.URL.Path, "/purge"):
			w.WriteHeader(503)
		case r.Method == "GET":
			w.Write([]byte(`{"status":"succeeded"}`))
		default:
			w.Write([]byte(`{"task_id":"task"}`))
		}
	}))
	defer server.Close()
	monitor := ops.New(nil, nil, "metrics-test")
	client := &HTTPResearchClient{Base: server.URL, Token: "private", Client: server.Client(), Observer: monitor}
	ctx := context.Background()
	if _, err := client.Submit(ctx, ResearchTask{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(ctx, "task"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Cancel(ctx, "task"); err != nil {
		t.Fatal(err)
	}
	if err := client.Purge(ctx, "task"); err == nil {
		t.Fatal("expected HTTP failure")
	}
	body := operationalMetrics(t, monitor)
	for _, want := range []string{`zhigu_worker_calls_total{name="research"} 5`, `zhigu_worker_errors_total{name="research"} 1`, `zhigu_worker_call_duration_seconds_count{name="research"} 5`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in %s", want, body)
		}
	}
}
