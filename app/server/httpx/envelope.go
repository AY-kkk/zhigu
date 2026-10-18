package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// OperationalObserver is installed before serving requests or starting workers.
// Implementations must be safe for concurrent calls and bound all label values.
type OperationalObserver interface {
	ObserveWorker(string, time.Duration, bool)
	ObserveProvider(string, time.Duration, bool)
	SetCleanupBacklog(string, float64, bool)
}

type Envelope struct {
	Data    any     `json:"data"`
	Error   *APIErr `json:"error"`
	TraceID string  `json:"trace_id"`
}

type APIErr struct {
	Code              string `json:"code"`
	Message           string `json:"message"`
	Retryable         bool   `json:"retryable"`
	RetryAfterSeconds *int   `json:"retry_after_seconds,omitempty"`
}

func TraceID(c *gin.Context) string {
	if v, ok := c.Get("trace_id"); ok {
		if s, _ := v.(string); validTraceID(s) {
			c.Header("X-Trace-ID", s)
			return s
		}
	}
	id := c.GetHeader("X-Trace-ID")
	if !validTraceID(id) {
		id = uuid.NewString()
	}
	c.Set("trace_id", id)
	c.Header("X-Trace-ID", id)
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
	apiError := DefaultError(status, code, message)
	delay := 0
	if apiError.RetryAfterSeconds != nil {
		delay = *apiError.RetryAfterSeconds
	}
	// Preserve a precise delay set by existing login throttling handlers.
	if n, err := strconv.Atoi(c.Writer.Header().Get("Retry-After")); err == nil && n > 0 {
		delay = n
	}
	FailRetry(c, status, code, message, apiError.Retryable, delay)
}

// DefaultError supplies the common retry contract to envelopes with additional
// module metadata, so they do not invent a separate retry policy.
func DefaultError(status int, code, message string) *APIErr {
	retryable := status == http.StatusTooManyRequests || status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
	delay := 0
	if status == http.StatusTooManyRequests {
		delay = 60
	}
	if status == http.StatusServiceUnavailable {
		delay = 30
	}
	err := &APIErr{Code: code, Message: message, Retryable: retryable}
	if delay > 0 {
		err.RetryAfterSeconds = &delay
	}
	return err
}

// FailRetry describes read retry safety; it never authorizes replaying a write.
// A positive delay is exposed in both the JSON error and Retry-After header.
func FailRetry(c *gin.Context, status int, code, message string, retryable bool, retryAfterSeconds int) {
	err := &APIErr{Code: code, Message: message, Retryable: retryable}
	if retryAfterSeconds > 0 {
		err.RetryAfterSeconds = &retryAfterSeconds
		c.Header("Retry-After", strconv.Itoa(retryAfterSeconds))
	} else {
		c.Writer.Header().Del("Retry-After")
	}
	c.JSON(status, Envelope{Data: nil, Error: err, TraceID: TraceID(c)})
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
