package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Envelope struct {
	Data    any     `json:"data"`
	Error   *APIErr `json:"error"`
	TraceID string  `json:"trace_id"`
}

type APIErr struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func TraceID(c *gin.Context) string {
	if v, ok := c.Get("trace_id"); ok {
		if s, _ := v.(string); s != "" {
			return s
		}
	}
	id := NewID("tr")
	c.Set("trace_id", id)
	return id
}

func NewID(prefix string) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	if prefix == "" {
		return hex.EncodeToString(b)
	}
	return prefix + "_" + hex.EncodeToString(b)
}

func OK(c *gin.Context, status int, data any) {
	c.JSON(status, Envelope{Data: data, Error: nil, TraceID: TraceID(c)})
}

func Fail(c *gin.Context, status int, code, message string) {
	c.JSON(status, Envelope{Data: nil, Error: &APIErr{Code: code, Message: message}, TraceID: TraceID(c)})
}

func BindJSON(c *gin.Context, dst any) bool {
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_JSON", "请求体无法解析")
		return false
	}
	return true
}
