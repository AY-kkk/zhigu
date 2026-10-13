package httpx_test

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"zhigu/server/httpx"
)

func TestProductionProxyIgnoresSpoofedForwardedForForLimiter(t *testing.T) {
	engine := gin.New()
	if err := httpx.ConfigureProductionProxy(engine); err != nil {
		t.Fatal(err)
	}
	limiter := httpx.NewLoginLimiter()
	now := time.Now()
	engine.GET("/ip", func(c *gin.Context) {
		if ip := c.ClientIP(); ip != "198.51.100.7" {
			t.Errorf("spoofed XFF changed client IP to %s", ip)
		}
		if limiter.Allow(c.ClientIP(), "owner", now) > 0 {
			c.Status(429)
			return
		}
		c.Status(204)
	})
	for i := 0; i < 6; i++ {
		r := httptest.NewRequest("GET", "/ip", nil)
		r.RemoteAddr = "172.18.0.2:1234"
		r.Header.Set("X-Real-IP", "198.51.100.7")
		r.Header.Set("X-Forwarded-For", fmt.Sprintf("192.0.2.%d", i+1))
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		want := 204
		if i == 5 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("spoofed XFF bypassed limiter: request %d status %d", i, w.Code)
		}
	}
}
