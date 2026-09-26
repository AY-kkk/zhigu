package intel

import (
	"context"
	"os"
	"strings"
	"testing"
)

func liveProviderEnabled(name string) bool {
	if os.Getenv("ZHIGU_INTEL_LIVE_TEST") != "1" {
		return false
	}
	for _, provider := range strings.Split(os.Getenv("ZHIGU_INTEL_PROVIDERS"), ",") {
		if strings.TrimSpace(provider) == name {
			return true
		}
	}
	return false
}

func TestLiveProviderEvidence(t *testing.T) {
	if os.Getenv("ZHIGU_INTEL_LIVE_TEST") != "1" {
		t.Skip("live provider tests require ZHIGU_INTEL_LIVE_TEST=1")
	}
	ctx := context.Background()
	if liveProviderEnabled("fuyao") {
		out, err := NewFuyaoClient().SearchTickers(ctx, "贵州茅台", 5)
		if err != nil {
			t.Fatalf("fuyao live search: %v", err)
		}
		if out.RequestID == "" || len(out.Items) == 0 {
			t.Fatalf("fuyao live evidence missing request_id/items: %#v", out)
		}
	}
	if liveProviderEnabled("cninfo") {
		out, err := NewCNInfoClient().SearchAnnouncements(ctx, CNInfoQuery{
			Symbol: os.Getenv("ZHIGU_LIVE_CNINFO_SYMBOL"), OrgID: os.Getenv("ZHIGU_LIVE_CNINFO_ORG_ID"),
			Column: os.Getenv("ZHIGU_LIVE_CNINFO_COLUMN"), Plate: os.Getenv("ZHIGU_LIVE_CNINFO_PLATE"),
			SearchKey: "公告", StartDate: os.Getenv("ZHIGU_LIVE_CNINFO_FROM"), EndDate: os.Getenv("ZHIGU_LIVE_CNINFO_TO"),
			Limit: 3,
		})
		if err != nil {
			t.Fatalf("cninfo live search: %v", err)
		}
		for _, source := range out {
			if source.ProviderDocID == "" || source.URL == "" {
				t.Fatalf("cninfo source missing identity/url: %#v", source)
			}
		}
	}
	if liveProviderEnabled("ifind") {
		capability, err := NewIFindMCPClient().Discover(ctx)
		if err != nil {
			t.Fatalf("ifind live discovery: %v", err)
		}
		if capability.RequestID == "" || len(capability.Tools) == 0 {
			t.Fatalf("ifind capability missing evidence: %#v", capability)
		}
	}
}
