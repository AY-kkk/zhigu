package finance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

func SHA256Text(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func CanonicalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	var n any
	if err := json.Unmarshal(b, &n); err != nil {
		return "", err
	}
	norm := normalize(n)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(norm); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

func HashCanonical(v any) (string, error) {
	s, err := CanonicalJSON(v)
	if err != nil {
		return "", err
	}
	return SHA256Text(s), nil
}

func normalize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		m := make(map[string]any, len(keys))
		for _, k := range keys {
			m[k] = normalize(t[k])
		}
		return m
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = normalize(x)
		}
		return out
	default:
		return t
	}
}
