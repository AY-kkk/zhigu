package finance

import "testing"

func TestClassifyClaimItems(t *testing.T) {
	items := extractClaimItems("公司收入增长是事实。预计现金流改善会推动盈利。假设行业需求保持稳定。", "", nil)
	if len(items) != 3 {
		t.Fatalf("items=%+v", items)
	}
	want := []string{"fact", "inference", "assumption"}
	for i, kind := range want {
		if items[i].ClaimType != kind {
			t.Fatalf("item %d type=%s want=%s", i, items[i].ClaimType, kind)
		}
	}
}

func TestExtractClaimItemsKeepsSourceSpan(t *testing.T) {
	spans := []DocumentSpan{{SpanID: "span_1", Text: "研报认为公司收入将继续增长。"}}
	items := extractClaimItems("", "", spans)
	if len(items) != 1 || items[0].SourceSpanID != "span_1" {
		t.Fatalf("items=%+v", items)
	}
}

func TestExtractNumbers(t *testing.T) {
	got := extractNumberMentions("收入增长 20%，现金流为 -3.5 亿元。", "span_1")
	if len(got) < 2 {
		t.Fatalf("numbers=%+v", got)
	}
	if got[0].Raw != "20%" || got[0].Unit != "%" {
		t.Fatalf("number=%+v", got[0])
	}
}
