package futures

import (
	"strings"
	"testing"
)

func TestExportHTMLRedactsRestrictedValuesInProseAndDerivedContent(t *testing.T) {
	html, err := ExportHTML(ExportInput{
		Title:        "研究 987654321",
		Text:         "受限库存为987654321吨",
		Records:      []ExportRecord{{ID: "restricted", Value: "987654321", ExportAllowed: false}},
		Calculations: []ExportCalculation{{ID: "calc", InputRecordIDs: []string{"restricted"}, Value: "999"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "987654321") || strings.Contains(html, "999") {
		t.Fatalf("restricted content leaked: %s", html)
	}
}
