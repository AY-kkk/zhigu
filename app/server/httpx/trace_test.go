package httpx

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http/httptest"
	"testing"
)

func TestTraceMiddlewareRoutesAndRecovery(t *testing.T) {
	for _, path := range []string{"/ok", "/ok/", "/fail", "/missing", "/panic", "/method"} {
		for _, inbound := range []string{"f47ac10b-58cc-4372-a567-0e02b2c3d479", "01ARZ3NDEKTSV4RRFFQ69G5FAV", "bad", "tr_abc", "81ARZ3NDEKTSV4RRFFQ69G5FAV", "01ARZ3NDEKTSV4RRFFQ69G5FAI", ""} {
			r := gin.New()
			InstallTrace(r)
			r.GET("/ok", func(c *gin.Context) { OK(c, 200, nil) })
			r.GET("/fail", func(c *gin.Context) { Fail(c, 400, "INVALID_JSON", "safe") })
			r.GET("/panic", func(c *gin.Context) { panic("secret upstream SQL") })
			r.POST("/method", func(c *gin.Context) { OK(c, 200, nil) })
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", path, nil)
			req.Header.Set("X-Trace-ID", inbound)
			r.ServeHTTP(w, req)
			var body Envelope
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("%s: non-envelope: %s", path, w.Body.String())
			}
			if body.TraceID != w.Header().Get("X-Trace-ID") || body.TraceID == "" {
				t.Fatalf("trace mismatch: %s", w.Body.String())
			}
			if inbound == "f47ac10b-58cc-4372-a567-0e02b2c3d479" || inbound == "01ARZ3NDEKTSV4RRFFQ69G5FAV" {
				if body.TraceID != inbound {
					t.Errorf("valid trace replaced: %s", inbound)
				}
			} else if _, err := uuid.Parse(body.TraceID); err != nil {
				t.Errorf("invalid trace retained: %s", body.TraceID)
			}
			if path == "/missing" && w.Code != 404 {
				t.Errorf("missing status %d", w.Code)
			}
			if path == "/method" && w.Code != 405 {
				t.Errorf("method status %d", w.Code)
			}
			if path == "/panic" && (w.Code != 500 || body.Error.Message != "内部错误") {
				t.Errorf("panic leaked: %s", w.Body.String())
			}
		}
	}
}

func TestFailRetryExplicitContract(t *testing.T) {
	r := gin.New()
	r.GET("/", func(c *gin.Context) { FailRetry(c, 429, "RATE_LIMITED", "safe", false, 17) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	var body Envelope
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Error.Retryable || body.Error.RetryAfterSeconds == nil || *body.Error.RetryAfterSeconds != 17 || w.Header().Get("Retry-After") != "17" {
		t.Fatalf("explicit retry contract: %s", w.Body.String())
	}
}
