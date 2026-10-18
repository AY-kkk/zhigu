package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBackupBarrierMiddlewareBlocksMutationsOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	barrier := &BackupBarrier{}
	barrier.active.Store(true)
	barrier.epoch = "backup-test"
	router := gin.New()
	InstallTrace(router)
	router.Use(barrier.Middleware())
	router.GET("/read", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.POST("/write", func(c *gin.Context) { c.Status(http.StatusOK) })

	read := httptest.NewRecorder()
	router.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/read", nil))
	if read.Code != http.StatusOK {
		t.Fatalf("read status=%d", read.Code)
	}
	write := httptest.NewRecorder()
	router.ServeHTTP(write, httptest.NewRequest(http.MethodPost, "/write", nil))
	if write.Code != http.StatusServiceUnavailable {
		t.Fatalf("write status=%d body=%s", write.Code, write.Body.String())
	}
	if write.Header().Get("X-Trace-ID") == "" {
		t.Fatal("backup-blocked response lacks trace ID")
	}
}
