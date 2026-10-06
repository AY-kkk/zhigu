package futures

import (
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

type ObservationRecord struct {
	ID          string
	SeriesID    string
	Period      string
	Revision    int
	Value       decimal.Decimal
	Unit        string
	UsableAt    time.Time
	SourceType  string
	Frequency   string
	PeriodIndex int64
}

func Snapshot(records []ObservationRecord, asOf time.Time, seriesID string) ([]ObservationRecord, error) {
	selected := map[string]ObservationRecord{}
	for _, record := range records {
		if record.SeriesID != seriesID || record.UsableAt.After(asOf) {
			continue
		}
		current, ok := selected[record.Period]
		if !ok || record.Revision > current.Revision {
			selected[record.Period] = record
		}
	}
	out := make([]ObservationRecord, 0, len(selected))
	for _, record := range selected {
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Period == out[j].Period {
			return out[i].Revision < out[j].Revision
		}
		return out[i].Period < out[j].Period
	})
	return out, nil
}

func RevisionHistory(records []ObservationRecord, seriesID, period string) []ObservationRecord {
	out := make([]ObservationRecord, 0)
	for _, record := range records {
		if record.SeriesID == seriesID && record.Period == period {
			out = append(out, record)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Revision < out[j].Revision })
	return out
}

func DatePrecisionUsableAt(day, location string) time.Time {
	loc, err := time.LoadLocation(location)
	if err != nil {
		loc = time.UTC
	}
	value, err := time.ParseInLocation("2006-01-02", day, loc)
	if err != nil {
		return time.Time{}
	}
	return value.AddDate(0, 0, 1).Add(-time.Nanosecond).UTC()
}
