package intel

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	intelsvc "zhigu/server/service/intel"
)

type intelListCursor struct {
	Version         int    `json:"v"`
	NamespaceID     string `json:"n"`
	Kind            string `json:"k"`
	Generation      int64  `json:"g"`
	Offset          int    `json:"o"`
	FilterHash      string `json:"f"`
	KnowledgeCutoff int64  `json:"c"`
	ExpiresAt       int64  `json:"e"`
}

func hashFilter(kind string, filters map[string]any) string {
	raw, _ := json.Marshal(map[string]any{"kind": kind, "filters": filters})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func signCursor(svc *intelsvc.Service, cursor intelListCursor) (string, error) {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, svc.CookieKey)
	mac.Write(raw)
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func parseCursor(svc *intelsvc.Service, scope intelsvc.Scope, kind, filterHash, value string) (intelListCursor, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return intelListCursor{}, &appError{status: 400, code: "INVALID_PARAM", message: "cursor 无效"}
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return intelListCursor{}, &appError{status: 400, code: "INVALID_PARAM", message: "cursor 无效"}
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return intelListCursor{}, &appError{status: 400, code: "INVALID_PARAM", message: "cursor 无效"}
	}
	mac := hmac.New(sha256.New, svc.CookieKey)
	mac.Write(raw)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return intelListCursor{}, &appError{status: 400, code: "INVALID_PARAM", message: "cursor 签名无效"}
	}
	var cursor intelListCursor
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return intelListCursor{}, &appError{status: 400, code: "INVALID_PARAM", message: "cursor 无效"}
	}
	if cursor.Version != 1 || cursor.Kind != kind || cursor.FilterHash != filterHash {
		return intelListCursor{}, &appError{status: 400, code: "INVALID_PARAM", message: "cursor 与查询不匹配"}
	}
	if cursor.ExpiresAt <= time.Now().Unix() {
		return intelListCursor{}, &appError{status: 409, code: "CURSOR_EXPIRED", message: "cursor 已过期"}
	}
	if cursor.NamespaceID != scope.NamespaceID {
		return intelListCursor{}, &appError{status: 400, code: "INVALID_PARAM", message: "cursor 不属于当前空间"}
	}
	if cursor.Generation != scope.Generation {
		return intelListCursor{}, &appError{status: 409, code: "CURSOR_EXPIRED", message: "数据空间已重置"}
	}
	return cursor, nil
}

func pagingRequest(svc *intelsvc.Service, c *gin.Context, scope intelsvc.Scope, kind string, filters map[string]any, limit int) (offset, fetchLimit int, err error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	filterHash := hashFilter(kind, filters)
	if raw := strings.TrimSpace(c.Query("cursor")); raw != "" {
		cursor, err := parseCursor(svc, scope, kind, filterHash, raw)
		if err != nil {
			return 0, 0, err
		}
		offset = cursor.Offset
	}
	fetchLimit = offset + limit + 1
	if fetchLimit > 1000 {
		fetchLimit = 1000
	}
	if fetchLimit <= offset {
		return 0, 0, &appError{status: 409, code: "CURSOR_EXPIRED", message: "cursor 超出可查询范围"}
	}
	return offset, fetchLimit, nil
}

func (h *handler) nextCursor(scope intelsvc.Scope, kind string, filters map[string]any, offset, limit, total int) string {
	if total <= offset+limit {
		return ""
	}
	cursor := intelListCursor{
		Version: 1, NamespaceID: scope.NamespaceID, Kind: kind,
		Generation: scope.Generation, Offset: offset + limit,
		FilterHash:      hashFilter(kind, filters),
		KnowledgeCutoff: time.Now().Unix(),
		ExpiresAt:       time.Now().Add(15 * time.Minute).Unix(),
	}
	value, err := signCursor(h.svc, cursor)
	if err != nil {
		return ""
	}
	return value
}

func slicePage(items []any, offset, limit int) []any {
	if offset >= len(items) {
		return []any{}
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}
