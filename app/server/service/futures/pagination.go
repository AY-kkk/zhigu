package futures

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const pageCursorVersion = 1

type PageCursor struct {
	Version   int    `json:"v"`
	OwnerID   uint   `json:"o"`
	Mode      string `json:"m"`
	Kind      string `json:"k"`
	Filter    string `json:"f"`
	Primary   string `json:"p"`
	Secondary string `json:"s"`
	Tertiary  string `json:"t,omitempty"`
}

type Page[T any] struct {
	Items      []T     `json:"items"`
	Limit      int     `json:"limit"`
	Cursor     *string `json:"cursor"`
	NextCursor *string `json:"next_cursor"`
}

func NewPage[T any](items []T, limit int, cursor, nextCursor *string) Page[T] {
	if items == nil {
		items = []T{}
	}
	return Page[T]{Items: items, Limit: limit, Cursor: cursor, NextCursor: nextCursor}
}

func ParsePageLimit(raw string) (int, error) {
	if raw == "" {
		return 20, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > 100 {
		return 0, fmt.Errorf("limit must be between 1 and 100")
	}
	return value, nil
}

func hashPageFilter(filter string) string {
	sum := sha256.Sum256([]byte(filter))
	return hex.EncodeToString(sum[:16])
}

func NewPageCursor(id Identity, kind, filter string, primary time.Time, secondary string, tertiary int64) (string, error) {
	return newPageCursor(id, kind, hashPageFilter(filter), primary.Format(time.RFC3339Nano), secondary, strconv.FormatInt(tertiary, 10))
}

func NewStringPageCursor(id Identity, kind, filter, primary, secondary string) (string, error) {
	return newPageCursor(id, kind, hashPageFilter(filter), primary, secondary, "")
}

func newPageCursor(id Identity, kind, filterHash, primary, secondary, tertiary string) (string, error) {
	if id.OwnerID == 0 || id.Mode == "" || kind == "" || filterHash == "" || primary == "" || secondary == "" {
		return "", ErrInvalidCursor
	}
	raw, err := json.Marshal(PageCursor{
		Version: pageCursorVersion, OwnerID: id.OwnerID, Mode: id.Mode,
		Kind: kind, Filter: filterHash, Primary: primary, Secondary: secondary, Tertiary: tertiary,
	})
	if err != nil {
		return "", err
	}
	digest := pageCursorDigest(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(digest[:]), nil
}

func DecodePageCursor(token string, id Identity, kind, filter string) (PageCursor, error) {
	var cursor PageCursor
	parts := strings.Split(token, ".")
	if len(parts) != 2 || token == "" {
		return cursor, ErrInvalidCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return cursor, ErrInvalidCursor
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !equalDigest(signature, pageCursorDigest(raw)) {
		return cursor, ErrInvalidCursor
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return cursor, ErrInvalidCursor
	}
	if cursor.Version != pageCursorVersion || cursor.OwnerID != id.OwnerID || cursor.Mode != id.Mode ||
		cursor.Kind != kind || cursor.Filter != hashPageFilter(filter) ||
		cursor.Primary == "" || cursor.Secondary == "" {
		return PageCursor{}, ErrInvalidCursor
	}
	return cursor, nil
}

func pageCursorDigest(raw []byte) [32]byte {
	return sha256.Sum256(append([]byte("zhigu-futures-page-cursor-v1|"), raw...))
}

func equalDigest(left []byte, right [32]byte) bool {
	if len(left) != len(right) {
		return false
	}
	var diff byte
	for i := range left {
		diff |= left[i] ^ right[i]
	}
	return diff == 0
}

type PageOptions struct {
	Limit       int
	Cursor      *PageCursor
	CursorToken *string
	Filter      string
}

func BuildPage[T any](id Identity, kind, filter string, options PageOptions, rows []T, position func(T) (string, string, string)) (Page[T], error) {
	hasMore := len(rows) > options.Limit
	if hasMore {
		rows = rows[:options.Limit]
	}
	if rows == nil {
		rows = []T{}
	}
	next := (*string)(nil)
	if hasMore && len(rows) > 0 {
		primary, secondary, tertiary := position(rows[len(rows)-1])
		token, err := newPageCursor(id, kind, hashPageFilter(filter), primary, secondary, tertiary)
		if err != nil {
			return Page[T]{}, err
		}
		next = &token
	}
	return NewPage(rows, options.Limit, options.CursorToken, next), nil
}

func TimePagePosition(value time.Time, id string, version int64) (string, string, string) {
	return value.UTC().Format(time.RFC3339Nano), id, strconv.FormatInt(version, 10)
}

func StringPagePosition(value, id string) (string, string, string) {
	return value, id, ""
}
