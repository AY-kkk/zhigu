package intel

import (
	"context"
	"net/url"
	"testing"
)

func TestProviderURLRejectsInternalAndUnknownOrigins(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1:8080/x", "http://10.0.0.1/x", "https://evil.example/x"} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if providerURLAllowed(u, map[string]struct{}{"https://fuyao.aicubes.cn:443": {}}) {
			t.Fatalf("allowed unsafe URL %s", raw)
		}
	}
}

func TestModelExtractorIsFailClosedWithoutFrozenConfig(t *testing.T) {
	m := NewModelExtractor()
	if m.Enabled {
		t.Fatal("model extractor must default disabled")
	}
	if _, err := m.Extract(context.Background(), ExtractionRequest{Text: "x"}); err == nil || ErrorCode(err) != "MODEL_UNAVAILABLE" {
		t.Fatalf("extract err=%v", err)
	}
}
