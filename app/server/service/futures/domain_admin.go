package futures

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	model "zhigu/server/model/futures"
)

type RightsInput struct {
	Display     bool      `json:"display"`
	ModelUse    bool      `json:"model_use"`
	Export      bool      `json:"export"`
	RetainUntil time.Time `json:"retain_until"`
	Version     string    `json:"version"`
}

type FieldMappingInput struct {
	SourceField  string `json:"source_field"`
	TargetMetric string `json:"target_metric"`
	Unit         string `json:"unit"`
	CaliberID    string `json:"caliber_id"`
}

type ScheduleInput struct {
	Frequency       string `json:"frequency"`
	Timezone        string `json:"timezone"`
	CalendarVersion string `json:"calendar_version"`
	Rule            string `json:"rule"`
	Precision       string `json:"precision"`
}

type PaginationInput struct {
	Method          string `json:"method"`
	TerminationRule string `json:"termination_rule"`
}

type SourceManifestInput struct {
	SourceID             string              `json:"source_id"`
	Version              int                 `json:"version"`
	Publisher            string              `json:"publisher"`
	URL                  string              `json:"url"`
	LocatorMethod        string              `json:"locator_method"`
	CredentialRef        *string             `json:"credential_ref"`
	Rights               RightsInput         `json:"rights"`
	Metrics              []string            `json:"metrics"`
	FieldMapping         []FieldMappingInput `json:"field_mapping"`
	Schedule             ScheduleInput       `json:"schedule"`
	Pagination           PaginationInput     `json:"pagination"`
	CoverageStart        string              `json:"coverage_start"`
	CoverageEnd          string              `json:"coverage_end"`
	RequestsPerMinute    int                 `json:"requests_per_minute"`
	TimeoutSeconds       int                 `json:"timeout_seconds"`
	MaxRetries           int                 `json:"max_retries"`
	RetryIntervalSeconds int                 `json:"retry_interval_seconds"`
	Priority             int                 `json:"priority"`
	AdapterVersion       string              `json:"adapter_version"`
	ResponsibleRole      string              `json:"responsible_role"`
}

type SourceValue struct {
	ID              string         `gorm:"column:id;primaryKey"`
	SourceID        string         `gorm:"column:source_id"`
	Version         int64          `gorm:"column:version"`
	Publisher       string         `gorm:"column:publisher"`
	URL             string         `gorm:"column:url"`
	Status          string         `gorm:"column:status"`
	CredentialRef   *string        `gorm:"column:credential_ref"`
	Rights          datatypes.JSON `gorm:"column:rights"`
	Metrics         datatypes.JSON `gorm:"column:metrics"`
	ScheduleVersion string         `gorm:"column:schedule_version"`
	AdapterVersion  string         `gorm:"column:adapter_version"`
	CoverageStart   *string        `gorm:"column:coverage_start"`
	CoverageEnd     *string        `gorm:"column:coverage_end"`
	Health          string         `gorm:"column:health"`
	Manifest        datatypes.JSON `gorm:"column:manifest"`
	CreatedAt       time.Time      `gorm:"column:created_at"`
	DeletedAt       *time.Time     `gorm:"column:deleted_at"`
}

func (SourceValue) TableName() string { return "futures_sources" }

type SourceVersionValue struct {
	SourceID  string         `gorm:"column:source_id;primaryKey"`
	Version   int64          `gorm:"column:version;primaryKey"`
	Manifest  datatypes.JSON `gorm:"column:manifest"`
	CreatedAt time.Time      `gorm:"column:created_at"`
}

func (SourceVersionValue) TableName() string { return "futures_source_versions" }

type AdmissionValue struct {
	ProductID          string    `gorm:"column:product_id;primaryKey"`
	Version            int64     `gorm:"column:version;primaryKey"`
	Status             string    `gorm:"column:status"`
	EvidenceManifestID string    `gorm:"column:evidence_manifest_id"`
	Reason             string    `gorm:"column:reason"`
	CreatedAt          time.Time `gorm:"column:created_at"`
}

func (AdmissionValue) TableName() string { return "futures_admission" }

type OperationsValue struct {
	Version        int64          `gorm:"column:version;primaryKey"`
	Mode           string         `gorm:"column:mode"`
	Reason         string         `gorm:"column:reason"`
	AllowedUserIDs datatypes.JSON `gorm:"column:allowed_user_ids"`
	UpdatedAt      time.Time      `gorm:"column:updated_at"`
}

func (OperationsValue) TableName() string { return "futures_operations" }

func (d *Domain) ListSources(ctx context.Context, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var values []SourceValue
	if err := d.DB.WithContext(ctx).Where(`deleted_at IS NULL`).Order(`created_at DESC, id DESC`).Limit(limit).Find(&values).Error; err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		out = append(out, sourceMap(value))
	}
	return out, nil
}

func (d *Domain) CreateSource(ctx context.Context, idempotencyKey string, input SourceManifestInput) (map[string]any, error) {
	if err := validateSourceManifest(input); err != nil {
		return nil, err
	}
	value := sourceFromManifest(input, "registered")
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if existing, err := replaySourceIdempotency(tx, "source.create", idempotencyKey, hashJSON(input)); err != nil {
			return err
		} else if existing != nil {
			value = *existing
			return nil
		}
		var count int64
		if err := tx.Raw(`SELECT count(*) FROM futures_sources WHERE source_id=? AND deleted_at IS NULL`, input.SourceID).Scan(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrIdempotencyConflict
		}
		if err := tx.Create(&value).Error; err != nil {
			return err
		}
		return d.writeIdempotency(tx, Identity{OwnerID: 0, Mode: "live"}, "source.create", idempotencyKey, hashJSON(input), value)
	})
	if err != nil {
		return nil, err
	}
	return sourceMap(value), nil
}

func (d *Domain) PatchSource(ctx context.Context, sourceID string, expected int64, status, reason string) (map[string]any, error) {
	if reason == "" || (status != "enabled" && status != "paused" && status != "revoked") {
		return nil, ErrInvalidInput
	}
	var value SourceValue
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&SourceValue{}).Where(`id=? AND version=? AND deleted_at IS NULL`, sourceID, expected).Updates(map[string]any{"status": status, "version": expected + 1})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrRevisionConflict
		}
		return tx.Where(`id=?`, sourceID).First(&value).Error
	})
	if err != nil {
		return nil, err
	}
	return sourceMap(value), nil
}

func (d *Domain) CreateSourceVersion(ctx context.Context, sourceID string, expected int64, idempotencyKey string, input SourceManifestInput) (map[string]any, error) {
	if input.SourceID != sourceID {
		return nil, ErrInvalidInput
	}
	if err := validateSourceManifest(input); err != nil {
		return nil, err
	}
	value := sourceFromManifest(input, "registered")
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if existing, err := replaySourceIdempotency(tx, "source.version", idempotencyKey, hashJSON(input)); err != nil {
			return err
		} else if existing != nil {
			value = *existing
			return nil
		}
		var current SourceValue
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(`id=? AND deleted_at IS NULL`, sourceID).First(&current).Error; err != nil {
			return ErrNotFound
		}
		if current.Version != expected {
			return ErrRevisionConflict
		}
		if int64(input.Version) != current.Version+1 {
			return ErrInvalidInput
		}
		value.Version = int64(input.Version)
		manifest, _ := json.Marshal(input)
		if err := tx.Create(SourceVersionValue{SourceID: sourceID, Version: int64(input.Version), Manifest: datatypes.JSON(manifest), CreatedAt: d.Now().UTC()}).Error; err != nil {
			return err
		}
		res := tx.Model(&SourceValue{}).Where(`id=? AND version=? AND deleted_at IS NULL`, sourceID, expected).Updates(map[string]any{
			`version`: value.Version, `publisher`: value.Publisher, `url`: value.URL, `rights`: value.Rights, `metrics`: value.Metrics,
			`schedule_version`: value.ScheduleVersion, `adapter_version`: value.AdapterVersion, `coverage_start`: value.CoverageStart,
			`coverage_end`: value.CoverageEnd, `manifest`: value.Manifest, `status`: value.Status, `health`: value.Health,
		})
		if res.RowsAffected != 1 {
			return ErrRevisionConflict
		}
		value.ID = sourceID
		return d.writeIdempotency(tx, Identity{OwnerID: 0, Mode: "live"}, "source.version", idempotencyKey, hashJSON(input), value)
	})
	if err != nil {
		return nil, err
	}
	return sourceMap(value), nil
}

func (d *Domain) ListAdmission(ctx context.Context, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var values []AdmissionValue
	if err := d.DB.WithContext(ctx).Order(`created_at DESC, product_id, version DESC`).Limit(limit).Find(&values).Error; err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		out = append(out, map[string]any{"product_id": value.ProductID, "version": value.Version, "status": value.Status, "evidence_manifest_id": value.EvidenceManifestID, "reason": value.Reason})
	}
	return out, nil
}

func (d *Domain) PutAdmission(ctx context.Context, productID string, expected int64, status, evidenceManifestID, reason string) (map[string]any, error) {
	if productID == "" || evidenceManifestID == "" || reason == "" || (status != "blocked" && status != "admitted" && status != "suspended") {
		return nil, ErrInvalidInput
	}
	newVersion := expected
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current AdmissionValue
		err := tx.Where(`product_id=?`, productID).Order(`version DESC`).First(&current).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if expected != 1 {
				return ErrRevisionConflict
			}
			newVersion = 1
		} else if err != nil {
			return err
		} else {
			if current.Version != expected {
				return ErrRevisionConflict
			}
			newVersion = expected + 1
		}
		return tx.Create(AdmissionValue{ProductID: productID, Version: newVersion, Status: status, EvidenceManifestID: evidenceManifestID, Reason: reason, CreatedAt: d.Now().UTC()}).Error
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"product_id": productID, "version": newVersion, "status": status, "evidence_manifest_id": evidenceManifestID, "reason": reason}, nil
}

func (d *Domain) GetOperations(ctx context.Context) (map[string]any, error) {
	var value OperationsValue
	err := d.DB.WithContext(ctx).Order(`version DESC`).First(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return map[string]any{"version": 1, "mode": "off", "reason": "FUTURES_DEFAULT_OFF", "allowed_user_ids": []any{}, "updated_at": d.Now().UTC()}, nil
	}
	if err != nil {
		return nil, err
	}
	var users any = []any{}
	_ = json.Unmarshal(value.AllowedUserIDs, &users)
	return map[string]any{"version": value.Version, "mode": value.Mode, "reason": value.Reason, "allowed_user_ids": users, "updated_at": value.UpdatedAt}, nil
}

func (d *Domain) FillCapabilities(ctx context.Context, id Identity, capabilities *Capabilities) {
	if !d.Enabled {
		capabilities.Enabled = false
		capabilities.Ready = false
		capabilities.Mode = "off"
		capabilities.Products = nil
		capabilities.Features.Research = false
		capabilities.Features.Tracking = false
		capabilities.Features.Export = false
		return
	}
	var operations OperationsValue
	if err := d.DB.WithContext(ctx).Order(`version DESC`).First(&operations).Error; err != nil {
		return
	}
	capabilities.Mode = operations.Mode
	var allowed []int64
	_ = json.Unmarshal(operations.AllowedUserIDs, &allowed)
	if operations.Mode == "off" {
		return
	}
	for _, userID := range allowed {
		if uint(userID) != id.OwnerID {
			continue
		}
		var products []string
		_ = d.DB.WithContext(ctx).Raw(`SELECT p.product_id FROM futures_products p JOIN (
SELECT product_id, max(version) AS version FROM futures_admission WHERE status='admitted' GROUP BY product_id
) a ON a.product_id=p.product_id JOIN futures_admission av ON av.product_id=a.product_id AND av.version=a.version
WHERE p.deleted_at IS NULL ORDER BY p.product_id`).Scan(&products).Error
		capabilities.Products = products
		break
	}
}

func (d *Domain) PatchOperations(ctx context.Context, expected int64, mode, reason string, allowedUserIDs []int64) (map[string]any, error) {
	if expected < 1 || reason == "" || (mode != "off" && mode != "read_only" && mode != "live") {
		return nil, ErrInvalidInput
	}
	if mode == "live" {
		return nil, ErrForbidden
	} // G0-G6 production evidence is not available in development.
	for _, userID := range allowedUserIDs {
		if userID < 1 {
			return nil, ErrInvalidInput
		}
	}
	raw, _ := json.Marshal(allowedUserIDs)
	newVersion := expected
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current OperationsValue
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order(`version DESC`).First(&current).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if expected != 1 {
				return ErrRevisionConflict
			}
			newVersion = 1
		} else if err != nil {
			return err
		} else {
			if current.Version != expected {
				return ErrRevisionConflict
			}
			newVersion = expected + 1
		}
		return tx.Create(OperationsValue{Version: newVersion, Mode: mode, Reason: reason, AllowedUserIDs: datatypes.JSON(raw), UpdatedAt: d.Now().UTC()}).Error
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"version": newVersion, "mode": mode, "reason": reason, "allowed_user_ids": allowedUserIDs, "updated_at": d.Now().UTC()}, nil
}

func validateSourceManifest(input SourceManifestInput) error {
	if input.SourceID == "" || input.Version < 1 || input.Publisher == "" || input.LocatorMethod == "" || input.AdapterVersion == "" || input.ResponsibleRole == "" {
		return ErrInvalidInput
	}
	parsed, err := url.Parse(input.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return ErrInvalidInput
	}
	if input.Rights.Version == "" || !input.Rights.ModelUse || !input.Rights.Display {
		return ErrForbidden
	}
	start, err := time.Parse("2006-01-02", input.CoverageStart)
	if err != nil {
		return ErrInvalidInput
	}
	end, err := time.Parse("2006-01-02", input.CoverageEnd)
	if err != nil || end.Before(start) {
		return ErrInvalidInput
	}
	if input.RequestsPerMinute < 1 || input.TimeoutSeconds < 1 || input.MaxRetries < 0 || input.MaxRetries > 3 || input.RetryIntervalSeconds < 900 {
		return ErrInvalidInput
	}
	if len(input.Metrics) == 0 || len(input.FieldMapping) == 0 {
		return ErrInvalidInput
	}
	if !oneOf(input.Schedule.Frequency, "daily", "weekly", "monthly", "irregular") || input.Schedule.Timezone == "" || input.Schedule.CalendarVersion == "" || input.Schedule.Rule == "" || !oneOf(input.Schedule.Precision, "timestamp", "date", "unknown") {
		return ErrInvalidInput
	}
	if !oneOf(input.Pagination.Method, "none", "cursor", "page") || input.Pagination.TerminationRule == "" {
		return ErrInvalidInput
	}
	return nil
}

func sourceFromManifest(input SourceManifestInput, status string) SourceValue {
	rights, _ := json.Marshal(input.Rights)
	metrics, _ := json.Marshal(input.Metrics)
	manifest, _ := json.Marshal(input)
	value := SourceValue{ID: input.SourceID, SourceID: input.SourceID, Version: int64(input.Version), Publisher: input.Publisher, URL: input.URL,
		Status: status, CredentialRef: input.CredentialRef, Rights: datatypes.JSON(rights), Metrics: datatypes.JSON(metrics),
		ScheduleVersion: input.Schedule.CalendarVersion, AdapterVersion: input.AdapterVersion,
		CoverageStart: &input.CoverageStart, CoverageEnd: &input.CoverageEnd, Health: "unknown", Manifest: datatypes.JSON(manifest), CreatedAt: time.Now().UTC()}
	return value
}

func sourceMap(value SourceValue) map[string]any {
	var rights, metrics any
	_ = json.Unmarshal(value.Rights, &rights)
	_ = json.Unmarshal(value.Metrics, &metrics)
	return map[string]any{"id": value.ID, "version": value.Version, "publisher": value.Publisher, "url": value.URL, "status": value.Status,
		"credential_ref": value.CredentialRef, "rights": rights, "metrics": metrics, "schedule_version": value.ScheduleVersion,
		"adapter_version": value.AdapterVersion, "coverage_start": value.CoverageStart, "coverage_end": value.CoverageEnd, "health": value.Health}
}

func replaySourceIdempotency(tx *gorm.DB, operation, key, requestHash string) (*SourceValue, error) {
	if key == "" {
		return nil, ErrInvalidInput
	}
	var row model.Idempotency
	err := tx.Where(`operation=? AND key=?`, operation, key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.RequestHash != requestHash {
		return nil, ErrIdempotencyConflict
	}
	var value SourceValue
	if err := json.Unmarshal(row.Response, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
