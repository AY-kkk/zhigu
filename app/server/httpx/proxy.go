package httpx

import "github.com/gin-gonic/gin"

func ConfigureProductionProxy(engine *gin.Engine) error {
	// The ingress overwrites X-Real-IP. Never trust a client-supplied XFF chain.
	engine.RemoteIPHeaders = []string{"X-Real-IP"}
	engine.TrustedPlatform = ""
	engine.AppEngine = false
	return engine.SetTrustedProxies([]string{"127.0.0.1", "::1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
}
