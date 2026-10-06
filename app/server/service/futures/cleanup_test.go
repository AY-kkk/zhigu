package futures

import (
	"strings"
	"testing"
	"time"
)

func TestExportEscapesTextAndHidesRestrictedValuesAndDerivations(t *testing.T) {
	html, err := ExportHTML(ExportInput{
		Title:        `<script>alert(1)</script>`,
		Text:         `用户文本 <img src="https://example.invalid/x">`,
		Records:      []ExportRecord{{ID: "r1", Value: "80000", ExportAllowed: false}},
		Calculations: []ExportCalculation{{ID: "c1", InputRecordIDs: []string{"r1"}, Value: "1000"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<script") || strings.Contains(html, "https://example.invalid") || strings.Contains(html, "80000") || strings.Contains(html, "1000") {
		t.Fatalf("export leaked restricted content: %s", html)
	}
	if !strings.Contains(html, "受限数据") || !strings.Contains(html, "&lt;script&gt;") {
		t.Fatalf("export missing safe explanations: %s", html)
	}
}

func TestDeleteImpactVersionAndOffModeCleanup(t *testing.T) {
	impact := DeleteImpactState{Version: "hash-1", Hypotheses: []string{"h1"}}
	if err := ConfirmDeleteImpact(impact, "hash-2", true); !errorsIsRevision(err) {
		t.Fatalf("changed impact err=%v", err)
	}
	job, err := DeleteRunOffline(DeletionRequest{ImpactVersion: "hash-1", CascadeHypotheses: true}, time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC))
	if err != nil || job.PurgeDueAt.IsZero() {
		t.Fatalf("off mode must still schedule deletion job=%+v err=%v", job, err)
	}
}

func errorsIsRevision(err error) bool { return err == ErrRevisionConflict }
