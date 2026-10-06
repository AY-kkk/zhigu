package futures

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestStrictBodyRejectsUnknownAndTrailingJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{
		`{"product_id":"SHFE.CU","unknown":true}`,
		`{"product_id":"SHFE.CU"} {"product_id":"SHFE.CU"}`,
	} {
		router := gin.New()
		router.POST("/strict", func(c *gin.Context) {
			var value struct {
				ProductID string `json:"product_id"`
			}
			if BindJSON(c, &value) {
				c.Status(http.StatusNoContent)
			}
		})
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/strict", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("body=%q status=%d", body, recorder.Code)
		}
		var envelope struct {
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Error == nil || envelope.Error.Code != "FUTURES_INVALID_INPUT" {
			t.Fatalf("body=%q envelope=%s", body, recorder.Body.String())
		}
	}
}
