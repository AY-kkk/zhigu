package finance

import (
	"time"
)

type RecordBasis struct {
	OriginalValue     string  `json:"original_value"`
	OriginalUnit      string  `json:"original_unit"`
	ScaleFactor       string  `json:"scale_factor"`
	Consolidation     string  `json:"consolidation"`
	RevisionID        *string `json:"revision_id"`
	DatePrecision     string  `json:"date_precision"`
	AvailabilityBasis string  `json:"availability_basis"`
}

type ProviderRecord struct {
	InstrumentID string      `json:"instrument_id"`
	SourceID     string      `json:"source_id"`
	SourceURL    string      `json:"source_url"`
	SourceKind   string      `json:"source_kind"`
	Title        string      `json:"title"`
	Locator      string      `json:"locator"`
	Text         string      `json:"text"`
	Metrics      []Metric    `json:"metrics"`
	PublishedAt  time.Time   `json:"published_at"`
	AvailableAt  time.Time   `json:"available_at"`
	RetrievedAt  time.Time   `json:"retrieved_at"`
	DataVersion  string      `json:"data_version"`
	Mode         string      `json:"mode"`
	Basis        RecordBasis `json:"basis"`
}

type IssuedRecord struct {
	RecordID   string         `json:"record_id"`
	RecordHash string         `json:"record_hash"`
	Record     ProviderRecord `json:"record"`
}

type DataQueryResult struct {
	Records       []IssuedRecord `json:"records"`
	QualityStatus string         `json:"quality_status"`
	Warnings      []string       `json:"warnings"`
}

func (r ProviderRecord) CanonicalMap() map[string]any {
	metrics := make([]map[string]string, 0, len(r.Metrics))
	for _, m := range r.Metrics {
		metrics = append(metrics, map[string]string{
			"metric": m.Metric, "period_start": m.PeriodStart, "period_end": m.PeriodEnd,
			"value": m.Value, "unit": m.Unit, "value_type": m.ValueType,
		})
	}
	basis := map[string]any{
		"original_value": r.Basis.OriginalValue, "original_unit": r.Basis.OriginalUnit,
		"scale_factor": r.Basis.ScaleFactor, "consolidation": r.Basis.Consolidation,
		"date_precision": r.Basis.DatePrecision, "availability_basis": r.Basis.AvailabilityBasis,
	}
	if r.Basis.RevisionID != nil {
		basis["revision_id"] = *r.Basis.RevisionID
	} else {
		basis["revision_id"] = nil
	}
	return map[string]any{
		"instrument_id": r.InstrumentID, "source_id": r.SourceID, "source_url": r.SourceURL,
		"source_kind": r.SourceKind, "title": r.Title, "locator": r.Locator, "text": r.Text,
		"metrics":      metrics,
		"published_at": r.PublishedAt.UTC().Format(time.RFC3339),
		"available_at": r.AvailableAt.UTC().Format(time.RFC3339),
		"retrieved_at": r.RetrievedAt.UTC().Format(time.RFC3339),
		"data_version": r.DataVersion, "mode": r.Mode, "basis": basis,
	}
}

func (r ProviderRecord) Hash() (string, error) {
	return HashCanonical(r.CanonicalMap())
}

func (r ProviderRecord) ToEvidenceIn() EvidenceIn {
	return EvidenceIn{
		SourceID: r.SourceID, Title: r.Title, SourceURL: r.SourceURL, SourceKind: r.SourceKind,
		Locator: r.Locator, Text: r.Text, Metrics: r.Metrics,
		PublishedAt: r.PublishedAt, AvailableAt: r.AvailableAt, RetrievedAt: r.RetrievedAt,
		ContentHash: ContentHash(r.Text), DataVersion: r.DataVersion, Mode: r.Mode,
	}
}

func defaultBasis(value, unit string) RecordBasis {
	return RecordBasis{
		OriginalValue: value, OriginalUnit: unit, ScaleFactor: "1",
		Consolidation: "consolidated", DatePrecision: "day", AvailabilityBasis: "conservative_day_end",
	}
}
