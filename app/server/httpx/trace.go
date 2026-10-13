package httpx

import (
	"regexp"

	"github.com/gin-gonic/gin"
)

var uuidTrace = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var ulidTrace = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Za-hjkmnp-tv-z]{25}$`)

func validTraceID(id string) bool { return uuidTrace.MatchString(id) || ulidTrace.MatchString(id) }

// InstallTrace must be the first engine middleware. Recovery stays inside the
// trace scope so even errors and unmatched routes share the same identifier.
func InstallTrace(engine *gin.Engine) {
	// Gin's automatic redirects bypass middleware, so unmatched slash variants
	// use the normal traced JSON 404 contract instead.
	engine.RedirectTrailingSlash = false
	engine.RedirectFixedPath = false
	engine.Use(func(c *gin.Context) {
		TraceID(c)
		c.Next()
		if value, ok := c.Get("httpx.after_response"); ok {
			for _, callback := range value.([]func()) {
				callback()
			}
		}
	})
	engine.Use(func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				// Gin's default recovery prints panic payloads and request headers, which
				// can contain private bodies and credentials. Expose only a stable error.
				if !c.Writer.Written() {
					Fail(c, 500, "INTERNAL", "内部错误")
				}
				c.Abort()
			}
		}()
		c.Next()
	})
	engine.HandleMethodNotAllowed = true
	engine.NoRoute(func(c *gin.Context) { Fail(c, 404, "NOT_FOUND", "接口不存在") })
	engine.NoMethod(func(c *gin.Context) { Fail(c, 405, "METHOD_NOT_ALLOWED", "请求方法不支持") })
}

// AfterResponse runs after recovery and final status are known. InstallTrace
// must precede middleware registering completion callbacks.
func AfterResponse(c *gin.Context, callback func()) {
	value, _ := c.Get("httpx.after_response")
	callbacks, _ := value.([]func())
	c.Set("httpx.after_response", append(callbacks, callback))
}
