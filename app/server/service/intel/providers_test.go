package intel

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"zhigu/server/ops"
)

func TestProviderRealHTTPCallsUpdateMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fail" {
			w.WriteHeader(503)
		}
		w.Write([]byte(`{}`))
	}))
	defer server.Close()
	monitor := ops.New(nil, nil, "metrics-test")
	for _, name := range []string{"ifind", "cninfo", "fuyao"} {
		client := NewProviderClient(ProviderConfig{Name: name, Enabled: true, Observer: monitor}, "https://authorized.example")
		client.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			clone := req.Clone(req.Context())
			target, _ := url.Parse(server.URL + req.URL.Path)
			clone.URL = target
			return server.Client().Do(clone)
		})}
		if _, _, err := client.Do(context.Background(), ProviderRequest{URL: "https://authorized.example/ok"}); err != nil {
			t.Fatal(err)
		}
		if _, _, err := client.Do(context.Background(), ProviderRequest{URL: "https://authorized.example/fail"}); err == nil {
			t.Fatal("expected upstream error")
		}
	}
	r := gin.New()
	monitor.Register(r)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Bearer metrics-test")
	r.ServeHTTP(w, req)
	for _, name := range []string{"ifind", "cninfo", "fuyao"} {
		for _, want := range []string{`zhigu_provider_calls_total{name="` + name + `"} 2`, `zhigu_provider_errors_total{name="` + name + `"} 1`} {
			if !strings.Contains(w.Body.String(), want) {
				t.Errorf("missing %s", want)
			}
		}
	}
}

func TestProviderObserverConcurrentHTTPCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{}`)) }))
	defer server.Close()
	monitor := ops.New(nil, nil, "metrics-test")
	client := NewProviderClient(ProviderConfig{Name: "ifind", Enabled: true, Observer: monitor}, "https://authorized.example")
	client.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		target, _ := url.Parse(server.URL)
		clone.URL = target
		return server.Client().Do(clone)
	})}
	var calls sync.WaitGroup
	for i := 0; i < 20; i++ {
		calls.Add(1)
		go func() {
			defer calls.Done()
			if _, _, err := client.Do(context.Background(), ProviderRequest{URL: "https://authorized.example"}); err != nil {
				t.Error(err)
			}
		}()
	}
	calls.Wait()
	r := gin.New()
	monitor.Register(r)
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Bearer metrics-test")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), `zhigu_provider_calls_total{name="ifind"} 20`) || !strings.Contains(w.Body.String(), `zhigu_provider_errors_total{name="ifind"} 0`) {
		t.Fatal("concurrent HTTP observations lost")
	}
	if strings.Contains(w.Body.String(), "authorized.example") {
		t.Fatal("high-cardinality endpoint leaked")
	}
}

func TestProviderURLRejectsInternalAndUnknownOrigins(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1:8080/x", "http://10.0.0.1/x", "https://evil.example/x"} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if providerURLAllowed(u, map[string]struct{}{"https://fuyao.aicubes.cn:443": {}}) {
			t.Fatalf("allowed unsafe URL %s", raw)
		}
	}
}

func TestModelExtractorIsFailClosedWithoutFrozenConfig(t *testing.T) {
	m := NewModelExtractor()
	if m.Enabled {
		t.Fatal("model extractor must default disabled")
	}
	if _, err := m.Extract(context.Background(), ExtractionRequest{Text: "x"}); err == nil || ErrorCode(err) != "MODEL_UNAVAILABLE" {
		t.Fatalf("extract err=%v", err)
	}
}
