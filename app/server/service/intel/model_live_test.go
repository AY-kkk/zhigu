package intel

import (
	"context"
	"os"
	"testing"
)

func TestLiveModelExtraction(t *testing.T) {
	if os.Getenv("ZHIGU_INTEL_LIVE_TEST") != "1" {
		t.Skip("live model test requires ZHIGU_INTEL_LIVE_TEST=1")
	}
	extractor := NewModelExtractor()
	if !extractor.Enabled {
		t.Fatal("live model config is not enabled")
	}
	result, err := extractor.Extract(context.Background(), ExtractionRequest{
		SourceRevisionID: "live-model-smoke",
		Text:             "公司于2026年9月26日发布收购进展公告，拟交易金额为8亿元。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ConfigID == "" || result.ConfigDigest == "" || result.Output == nil {
		t.Fatalf("live model result missing frozen identity/output: %#v", result)
	}
}
