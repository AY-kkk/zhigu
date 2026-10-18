package intel

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"zhigu/server/httpx"
	model "zhigu/server/model/finance"

	intelsvc "zhigu/server/service/intel"
	"zhigu/server/testdb"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testdb.StopIfStarted()
	os.Exit(code)
}

func TestIntelErrorRetryAndTraceContract(t *testing.T) {
	for _, status := range []int{400, 429, 503} {
		engine := gin.New()
		httpx.InstallTrace(engine)
		h := &handler{}
		engine.GET("/", func(c *gin.Context) { h.fail(c, &appError{status: status, code: "NOT_READY", message: "safe"}) })
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Trace-ID", "01ARZ3NDEKTSV4RRFFQ69G5FAV")
		req.Header.Set("X-Request-ID", "business-id")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["trace_id"] != w.Header().Get("X-Trace-ID") {
			t.Fatal("trace mismatch")
		}
		e := body["error"].(map[string]any)
		if e["request_id"] != "business-id" {
			t.Fatal("business request ID lost")
		}
		if _, ok := body["meta"]; !ok {
			t.Fatal("meta compatibility field absent")
		}
		if _, ok := e["retryable"].(bool); !ok {
			t.Fatal("retryable absent")
		}
		if status == 429 || status == 503 {
			delay, ok := e["retry_after_seconds"].(float64)
			if !ok || delay <= 0 || w.Header().Get("Retry-After") == "" {
				t.Errorf("retry delay absent: %s", w.Body.String())
			}
		} else if _, ok := e["retry_after_seconds"]; ok {
			t.Fatal("nonretryable error has retry delay")
		}
	}
}

func fixtureDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "..", "..", "contracts", "intel", "fixtures", "events-v1")
}

func TestProductionIntelUsesDurableCookieAndRefererCSRF(t *testing.T) {
	db := testdb.Start(t)
	user := model.User{Username: "research-owner", PasswordHash: "hash", Role: "admin"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	httpx.ConfigureAuth(httpx.AuthConfig{DB: db, Production: true, PublicOrigin: "https://zhigu.example"})
	t.Cleanup(func() { httpx.ConfigureAuth(httpx.AuthConfig{}) })
	t.Setenv("ZHIGU_INTEL_PUBLIC_ORIGIN", "https://zhigu.example")
	engine := gin.New()
	Register(engine, intelsvc.NewService(db, []byte("api-test-cookie-secret"), fixtureDir(t)))
	engine.GET("/csrf", httpx.AuthRequired(), httpx.SessionCSRF)
	token, err := httpx.CreateSession(db, user)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, csrf, referer string, bearer bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r.Header.Set("X-Intel-Mode", "live")
		if bearer {
			jwt, _ := httpx.SignToken(user.ID, user.Username, user.Role)
			r.Header.Set("Authorization", "Bearer "+jwt)
		} else {
			r.AddCookie(&http.Cookie{Name: httpx.SessionCookieName, Value: token})
		}
		r.Header.Set("X-CSRF-Token", csrf)
		r.Header.Set("Referer", referer)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "/api/finance/intel/v1/session", "", "", false); w.Code != 200 {
		t.Fatalf("cookie not accepted: %d %s", w.Code, w.Body)
	}
	if w := request("GET", "/api/finance/intel/v1/session", "", "", true); w.Code != 401 {
		t.Fatalf("bearer accepted: %d", w.Code)
	}
	csrf := request("GET", "/csrf", "", "", false).Header().Get("X-CSRF-Token")
	w := request("PUT", "/api/finance/intel/v1/watchlist/600519.SH", csrf, "https://zhigu.example/watchlist", false)
	if w.Code == 401 || w.Code == 403 {
		t.Fatalf("valid referer rejected: %d %s", w.Code, w.Body)
	}
	if w := request("PUT", "/api/finance/intel/v1/watchlist/600519.SH", csrf, "https://evil.example/watchlist", false); w.Code != 403 {
		t.Fatalf("evil referer accepted: %d", w.Code)
	}
}

func doJSON(t *testing.T, engine *gin.Engine, method, path, mode, token, cookie string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "test-idem-"+time.Now().Format("150405.000000000"))
	req.Header.Set("X-Intel-Mode", mode)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func cookieValue(t *testing.T, w *httptest.ResponseRecorder, name string) string {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			return c.Name + "=" + url.QueryEscape(c.Value)
		}
	}
	return ""
}

func TestDemoAPIFlowAndModeConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Start(t)
	svc := intelsvc.NewService(db, []byte("api-test-cookie-secret"), fixtureDir(t))
	engine := gin.New()
	Register(engine, svc)

	session := doJSON(t, engine, http.MethodGet, "/api/finance/intel/v1/session", "demo", "", "", nil)
	if session.Code != http.StatusUnauthorized {
		t.Fatalf("session status = %d, body=%s", session.Code, session.Body.String())
	}
	bootstrap := cookieValue(t, session, "zhigu_intel_bootstrap")
	if bootstrap == "" {
		t.Fatalf("missing bootstrap cookie: %s", session.Body.String())
	}

	create := doJSON(t, engine, http.MethodPost, "/api/finance/intel/v1/demo-sessions", "demo", "", bootstrap, map[string]any{"fixture_set": "events-v1"})
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", create.Code, create.Body.String())
	}
	demoCookie := cookieValue(t, create, "zhigu_intel_demo")
	if demoCookie == "" {
		t.Fatalf("missing demo cookie: %s", create.Body.String())
	}
	if strings.Contains(create.Body.String(), "api-test-cookie-secret") {
		t.Fatal("cookie secret leaked")
	}

	for _, code := range []string{"DEMO.A", "DEMO.B"} {
		w := doJSON(t, engine, http.MethodPut, "/api/finance/intel/v1/watchlist/"+code, "demo", "", demoCookie, map[string]any{})
		if w.Code != http.StatusOK {
			t.Fatalf("watch %s status=%d body=%s", code, w.Code, w.Body.String())
		}
	}
	step := doJSON(t, engine, http.MethodPost, "/api/finance/intel/v1/replay/actions", "demo", "", demoCookie, map[string]any{"action": "step", "expected_version": 0})
	if step.Code != http.StatusOK {
		t.Fatalf("step status=%d body=%s", step.Code, step.Body.String())
	}
	events := doJSON(t, engine, http.MethodGet, "/api/finance/intel/v1/events", "demo", "", demoCookie, nil)
	if events.Code != http.StatusOK || !strings.Contains(events.Body.String(), "evt_demo_acq") {
		t.Fatalf("events status=%d body=%s", events.Code, events.Body.String())
	}
	for _, key := range []string{"data", "error", "trace_id", "meta"} {
		if !strings.Contains(events.Body.String(), "\""+key+"\"") {
			t.Fatalf("envelope missing %s: %s", key, events.Body.String())
		}
	}

	conflict := doJSON(t, engine, http.MethodGet, "/api/finance/intel/v1/events", "demo", "real-token", demoCookie, nil)
	if conflict.Code != http.StatusBadRequest || !strings.Contains(conflict.Body.String(), "MODE_CONFLICT") {
		t.Fatalf("mode conflict status=%d body=%s", conflict.Code, conflict.Body.String())
	}
}

func TestDemoSessionRejectsMissingBootstrap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Start(t)
	svc := intelsvc.NewService(db, []byte("api-test-cookie-secret"), fixtureDir(t))
	engine := gin.New()
	Register(engine, svc)
	w := doJSON(t, engine, http.MethodPost, "/api/finance/intel/v1/demo-sessions", "demo", "", "", map[string]any{"fixture_set": "events-v1"})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "BOOTSTRAP_REQUIRED") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestIntelAPIRouteContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Start(t)
	svc := intelsvc.NewService(db, []byte("api-test-cookie-secret"), fixtureDir(t))
	engine := gin.New()
	Register(engine, svc)
	got := make(map[string]bool)
	for _, route := range engine.Routes() {
		got[route.Method+" "+route.Path] = true
	}
	for _, want := range []string{
		"GET /api/finance/intel/v1/session",
		"POST /api/finance/intel/v1/demo-sessions",
		"GET /api/finance/intel/v1/instruments",
		"GET /api/finance/intel/v1/watchlist",
		"PUT /api/finance/intel/v1/watchlist/:code",
		"DELETE /api/finance/intel/v1/watchlist/:code",
		"GET /api/finance/intel/v1/events",
		"GET /api/finance/intel/v1/events/:id",
		"GET /api/finance/intel/v1/events/:id/timeline",
		"GET /api/finance/intel/v1/events/:id/evidence",
		"GET /api/finance/intel/v1/events/:id/conflicts",
		"GET /api/finance/intel/v1/events/:id/changes",
		"GET /api/finance/intel/v1/source-revisions/:id",
		"GET /api/finance/intel/v1/notifications",
		"PATCH /api/finance/intel/v1/notifications/:id",
		"PUT /api/finance/intel/v1/events/:id/mute",
		"GET /api/finance/intel/v1/data-status",
		"POST /api/finance/intel/v1/replay/actions",
		"POST /api/finance/intel/v1/admin/ingestion-jobs",
		"GET /api/finance/intel/v1/admin/ingestion-jobs/:id",
		"POST /api/finance/intel/v1/admin/source-revisions",
		"GET /api/finance/intel/v1/admin/review-items",
		"POST /api/finance/intel/v1/admin/review-items/:id/resolve",
		"POST /api/finance/intel/v1/admin/events/:id/reassign",
	} {
		if !got[want] {
			t.Fatalf("missing route %s", want)
		}
	}
}

func TestSignedListCursorAndGenerationFence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Start(t)
	svc := intelsvc.NewService(db, []byte("api-test-cookie-secret"), fixtureDir(t))
	engine := gin.New()
	Register(engine, svc)

	session := doJSON(t, engine, http.MethodGet, "/api/finance/intel/v1/session", "demo", "", "", nil)
	bootstrap := cookieValue(t, session, "zhigu_intel_bootstrap")
	create := doJSON(t, engine, http.MethodPost, "/api/finance/intel/v1/demo-sessions", "demo", "", bootstrap, map[string]any{"fixture_set": "events-v1"})
	demoCookie := cookieValue(t, create, "zhigu_intel_demo")
	for _, code := range []string{"DEMO.A", "DEMO.B"} {
		doJSON(t, engine, http.MethodPut, "/api/finance/intel/v1/watchlist/"+code, "demo", "", demoCookie, map[string]any{})
	}
	doJSON(t, engine, http.MethodPost, "/api/finance/intel/v1/replay/actions", "demo", "", demoCookie, map[string]any{"action": "step", "expected_version": 0})
	doJSON(t, engine, http.MethodPost, "/api/finance/intel/v1/replay/actions", "demo", "", demoCookie, map[string]any{"action": "step", "expected_version": 1})
	doJSON(t, engine, http.MethodPost, "/api/finance/intel/v1/replay/actions", "demo", "", demoCookie, map[string]any{"action": "step", "expected_version": 2})
	page := doJSON(t, engine, http.MethodGet, "/api/finance/intel/v1/notifications?limit=1", "demo", "", demoCookie, nil)
	if page.Code != http.StatusOK {
		t.Fatalf("page status=%d body=%s", page.Code, page.Body.String())
	}
	var body struct {
		Data struct {
			NextCursor string `json:"next_cursor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(page.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.NextCursor == "" {
		t.Fatalf("missing signed cursor: %s", page.Body.String())
	}
	tampered := "x" + body.Data.NextCursor
	bad := doJSON(t, engine, http.MethodGet, "/api/finance/intel/v1/notifications?limit=1&cursor="+url.QueryEscape(tampered), "demo", "", demoCookie, nil)
	if bad.Code != http.StatusBadRequest || !strings.Contains(bad.Body.String(), "INVALID_PARAM") {
		t.Fatalf("tampered cursor status=%d body=%s", bad.Code, bad.Body.String())
	}
	good := doJSON(t, engine, http.MethodGet, "/api/finance/intel/v1/notifications?limit=1&cursor="+url.QueryEscape(body.Data.NextCursor), "demo", "", demoCookie, nil)
	if good.Code != http.StatusOK {
		t.Fatalf("valid cursor status=%d body=%s", good.Code, good.Body.String())
	}
	reset := doJSON(t, engine, http.MethodPost, "/api/finance/intel/v1/replay/actions", "demo", "", demoCookie, map[string]any{"action": "reset", "expected_version": 3, "branch": "main"})
	if reset.Code != http.StatusOK {
		t.Fatalf("reset status=%d body=%s", reset.Code, reset.Body.String())
	}
	stale := doJSON(t, engine, http.MethodGet, "/api/finance/intel/v1/notifications?limit=1&cursor="+url.QueryEscape(body.Data.NextCursor), "demo", "", demoCookie, nil)
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "CURSOR_EXPIRED") {
		t.Fatalf("stale cursor status=%d body=%s", stale.Code, stale.Body.String())
	}
}

func TestReplayIdempotencyReplaysSameBodyAndRejectsDifferentBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Start(t)
	svc := intelsvc.NewService(db, []byte("api-test-cookie-secret"), fixtureDir(t))
	engine := gin.New()
	Register(engine, svc)
	session := doJSON(t, engine, http.MethodGet, "/api/finance/intel/v1/session", "demo", "", "", nil)
	bootstrap := cookieValue(t, session, "zhigu_intel_bootstrap")
	create := doJSON(t, engine, http.MethodPost, "/api/finance/intel/v1/demo-sessions", "demo", "", bootstrap, map[string]any{"fixture_set": "events-v1"})
	demoCookie := cookieValue(t, create, "zhigu_intel_demo")
	t.Logf("create=%s", create.Body.String())
	state := doJSON(t, engine, http.MethodGet, "/api/finance/intel/v1/session", "demo", "", demoCookie, nil)
	t.Logf("state=%s", state.Body.String())
	req := httptest.NewRequest(http.MethodPost, "/api/finance/intel/v1/replay/actions", bytes.NewReader([]byte(`{"action":"step","expected_version":0}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Intel-Mode", "demo")
	req.Header.Set("Idempotency-Key", "replay-idem-0001")
	req.Header.Set("Cookie", demoCookie)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first replay status=%d body=%s", w.Code, w.Body.String())
	}
	req2 := httptest.NewRequest(http.MethodPost, "/api/finance/intel/v1/replay/actions", bytes.NewReader([]byte(`{"action":"step","expected_version":0}`)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Intel-Mode", "demo")
	req2.Header.Set("Idempotency-Key", "replay-idem-0001")
	req2.Header.Set("Cookie", demoCookie)
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK || w2.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("replay status=%d header=%q body=%s", w2.Code, w2.Header().Get("Idempotency-Replayed"), w2.Body.String())
	}
	req3 := httptest.NewRequest(http.MethodPost, "/api/finance/intel/v1/replay/actions", bytes.NewReader([]byte(`{"action":"reset","expected_version":0}`)))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("X-Intel-Mode", "demo")
	req3.Header.Set("Idempotency-Key", "replay-idem-0001")
	req3.Header.Set("Cookie", demoCookie)
	w3 := httptest.NewRecorder()
	engine.ServeHTTP(w3, req3)
	if w3.Code != http.StatusConflict || !strings.Contains(w3.Body.String(), "IDEMPOTENCY_CONFLICT") {
		t.Fatalf("different body status=%d body=%s", w3.Code, w3.Body.String())
	}
}
