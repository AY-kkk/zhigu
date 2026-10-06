package futures

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"zhigu/server/httpx"
)

// BindJSON rejects unknown fields and any second JSON value after the body.
func BindJSON(c *gin.Context, dst any) bool {
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "FUTURES_INVALID_INPUT", "请求体不符合 futures v1 契约")
		return false
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		httpx.Fail(c, http.StatusBadRequest, "FUTURES_INVALID_INPUT", "请求体包含额外 JSON 或数据")
		return false
	}
	return true
}
