package futures

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestObservationSnapshotRespectsUsableAtAndKeepsRevisions(t *testing.T) {
	base := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	records := []ObservationRecord{
		{ID: "r1", SeriesID: "s", Period: "2026-10-05", Revision: 1, Value: decimal.RequireFromString("1"), Unit: "t", UsableAt: base.Add(-time.Second), SourceType: "exchange"},
		{ID: "r2", SeriesID: "s", Period: "2026-10-05", Revision: 2, Value: decimal.RequireFromString("2"), Unit: "t", UsableAt: base.Add(time.Second), SourceType: "exchange"},
	}
	got, err := Snapshot(records, base, "s")
	if err != nil || len(got) != 1 || got[0].ID != "r1" {
		t.Fatalf("snapshot=%+v err=%v", got, err)
	}
	if len(RevisionHistory(records, "s", "2026-10-05")) != 2 {
		t.Fatal("revisions must remain separately readable")
	}
}

func TestDatePrecisionObservationIsUnavailableDuringLocalDay(t *testing.T) {
	at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	usable := DatePrecisionUsableAt("2026-10-05", "UTC")
	if !usable.After(at) {
		t.Fatalf("date precision usable_at=%s must be after midday", usable)
	}
}
