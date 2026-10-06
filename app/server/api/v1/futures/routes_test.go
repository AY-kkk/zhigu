package futures

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"zhigu/server/httpx"
	svc "zhigu/server/service/futures"
)

func TestCapabilityRequiresIdentityAndReturnsOff(t *testing.T) {
	t.Setenv("ZHIGU_JWT_SECRET", "futures-unit-test-only")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	Register(router, svc.NewService(false))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/futures/capabilities", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != 401 {
		t.Fatalf("anonymous status=%d", recorder.Code)
	}
	token, err := httpx.SignToken(1, "tester", "user")
	if err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/futures/capabilities", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var body struct {
		Data    svc.Capabilities `json:"data"`
		TraceID string           `json:"trace_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != 200 || body.Data.Ready || body.Data.Mode != "off" || body.TraceID == "" {
		t.Fatalf("unexpected envelope: %s", recorder.Body.String())
	}
}
