package futures

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	model "zhigu/server/model/futures"
)

type Domain struct {
	DB         *gorm.DB
	StorageDir string
	Now        func() time.Time
	Mode       string
	Enabled    bool
}

type RuntimePolicy struct {
	Mode           string
	AllowedUserIDs []int64
}

func (d *Domain) runtimePolicy(ctx context.Context) (RuntimePolicy, error) {
	if !d.Enabled {
		return RuntimePolicy{Mode: "off"}, nil
	}
	if d.DB == nil {
		mode := d.Mode
		if mode == "" {
			mode = "off"
		}
		return RuntimePolicy{Mode: mode}, nil
	}
	var value OperationsValue
	err := d.DB.WithContext(ctx).Order(`version DESC`).First(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		mode := d.Mode
		if mode == "" {
			mode = "off"
		}
		return RuntimePolicy{Mode: mode}, nil
	}
	if err != nil {
		return RuntimePolicy{Mode: "off"}, err
	}
	var allowed []int64
	if len(value.AllowedUserIDs) > 0 {
		if err := json.Unmarshal(value.AllowedUserIDs, &allowed); err != nil {
			return RuntimePolicy{Mode: "off"}, err
		}
	}
	return RuntimePolicy{Mode: value.Mode, AllowedUserIDs: allowed}, nil
}

func (p RuntimePolicy) permitsUser(ownerID uint) bool {
	if p.AllowedUserIDs == nil {
		return ownerID > 0
	}
	for _, id := range p.AllowedUserIDs {
		if id > 0 && uint(id) == ownerID {
			return true
		}
	}
	return false
}

func (d *Domain) AuthorizeAccess(ctx context.Context, id Identity, method, path string) error {
	policy, err := d.runtimePolicy(ctx)
	if err != nil {
		return err
	}
	if !policy.permitsUser(id.OwnerID) {
		return ErrForbidden
	}
	maintenance := method == http.MethodDelete || strings.HasSuffix(path, "/cancel") ||
		(method == http.MethodPatch && strings.HasSuffix(path, "/notifications/:id"))
	switch policy.Mode {
	case "live":
		return nil
	case "read_only":
		if maintenance || method == http.MethodGet {
			return nil
		}
		return ErrReadOnly
	default:
		if maintenance {
			return nil
		}
		return ErrModuleOff
	}
}

func (d *Domain) AuthorizeExecution(ctx context.Context, id Identity) error {
	policy, err := d.runtimePolicy(ctx)
	if err != nil {
		return err
	}
	if policy.Mode != "live" || !policy.permitsUser(id.OwnerID) {
		return ErrForbidden
	}
	return nil
}

type DraftVersion struct {
	DraftID   string           `gorm:"column:draft_id;primaryKey"`
	OwnerID   uint             `gorm:"column:owner_id"`
	Mode      string           `gorm:"column:mode"`
	Revision  int64            `gorm:"column:revision;primaryKey"`
	Input     model.DraftInput `gorm:"column:input;serializer:json"`
	Claims    datatypes.JSON   `gorm:"column:claims"`
	CreatedAt time.Time        `gorm:"column:created_at"`
}

func (DraftVersion) TableName() string { return "futures_draft_versions" }

func NewDomain(db *gorm.DB, storageDir string) *Domain {
	return &Domain{DB: db, StorageDir: storageDir, Now: time.Now, Enabled: true}
}

func (d *Domain) WithMode(mode string) *Domain     { d.Mode = mode; return d }
func (d *Domain) WithEnabled(enabled bool) *Domain { d.Enabled = enabled; return d }

func (d *Domain) Products(ctx context.Context, id Identity, limit int) ([]model.Product, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var products []model.Product
	err := d.DB.WithContext(ctx).Raw(`SELECT p.product_id AS id, p.name, p.exchange, p.template_version,
  COALESCE((SELECT a.status FROM futures_admission a WHERE a.product_id=p.product_id ORDER BY a.version DESC LIMIT 1), 'blocked') AS admission
FROM futures_products p WHERE p.deleted_at IS NULL ORDER BY p.product_id LIMIT ?`, limit).Scan(&products).Error
	return products, err
}

func (d *Domain) Contracts(ctx context.Context, id Identity, productID string, limit int) ([]model.Contract, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var contracts []model.Contract
	err := d.DB.WithContext(ctx).Raw(`SELECT id, product_id, kind, last_trading_at, price_precision, price_unit, multiplier::text AS multiplier, calendar_version, source_version
FROM futures_contracts WHERE product_id=? AND deleted_at IS NULL ORDER BY last_trading_at, id LIMIT ?`, productID, limit).Scan(&contracts).Error
	if len(contracts) == 0 {
		var count int64
		if err := d.DB.WithContext(ctx).Raw(`SELECT count(*) FROM futures_products WHERE product_id=? AND deleted_at IS NULL`, productID).Scan(&count).Error; err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, ErrNotFound
		}
	}
	return contracts, err
}

func (d *Domain) Workbench(ctx context.Context, id Identity, productID, contractID string, window int, priceType string) (map[string]any, error) {
	if contractID != "" {
		var count int64
		if err := d.DB.WithContext(ctx).Raw(`SELECT count(*) FROM futures_contracts WHERE id=? AND product_id=? AND deleted_at IS NULL`, contractID, productID).Scan(&count).Error; err != nil {
			return nil, err
		}
		if count != 1 {
			return nil, ErrNotFound
		}
	}
	if window != 20 && window != 60 && window != 120 {
		window = 60
	}
	if priceType == "" {
		priceType = "settlement"
	}
	if priceType != "settlement" && priceType != "close" {
		return nil, ErrInvalidInput
	}
	var product model.Product
	if err := d.DB.WithContext(ctx).Raw(`SELECT p.product_id AS id, p.name, p.exchange, p.template_version,
  COALESCE((SELECT a.status FROM futures_admission a WHERE a.product_id=p.product_id ORDER BY a.version DESC LIMIT 1), 'blocked') AS admission
FROM futures_products p WHERE p.product_id=? AND p.deleted_at IS NULL`, productID).Scan(&product).Error; err != nil {
		return nil, err
	}
	if product.ID == "" {
		return nil, ErrNotFound
	}
	contracts, err := d.Contracts(ctx, id, productID, 100)
	if err != nil {
		return nil, err
	}

	var rows []struct {
		ID, Metric, Unit, CaliberID, SeriesID, ContractKind, TradingDay, PeriodStart, PeriodEnd string
		Currency                                                                                *string
		Value                                                                                   string
		UsableAt                                                                                time.Time
	}
	query := `WITH ranked AS (
SELECT o.id, s.metric, o.unit, s.currency, o.caliber_id, o.series_id,
CASE WHEN o.contract_id IS NULL THEN 'spot' ELSE c.kind END AS contract_kind,
coalesce(o.trading_day::text,'') AS trading_day, o.period_start::text AS period_start, o.period_end::text AS period_end,
coalesce(o.numeric_value::text,o.text_value,'') AS value, o.usable_at,
row_number() OVER (PARTITION BY o.series_id,o.period_start ORDER BY o.revision DESC,o.id DESC) AS rn
FROM futures_observations o
JOIN futures_series s ON s.id=o.series_id
JOIN futures_sources src ON src.source_id=o.source_id
LEFT JOIN futures_contracts c ON c.id=o.contract_id
WHERE o.product_id=? AND o.deleted_at IS NULL AND src.status='enabled' AND src.deleted_at IS NULL
AND coalesce((src.rights->>'model_use')::boolean,false)
AND coalesce((src.rights->>'retain_until')::timestamptz,to_timestamp(0)) >= now()
AND (o.scope='public' OR (o.scope='private' AND o.owner_id=? AND o.mode=?))
)
SELECT id,metric,unit,currency,caliber_id,series_id,contract_kind,trading_day,period_start,period_end,value,usable_at
FROM ranked WHERE rn=1 AND period_start::date>=current_date-?::integer`
	args := []any{productID, id.OwnerID, id.Mode, window}
	if contractID != "" {
		query = strings.Replace(query, "WHERE o.product_id=? AND o.deleted_at IS NULL", "WHERE o.product_id=? AND o.contract_id=? AND o.deleted_at IS NULL", 1)
		args = []any{productID, contractID, id.OwnerID, id.Mode, window}
	}
	query += ` ORDER BY metric,period_start,id LIMIT 500`
	if err := d.DB.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	records := make([]any, 0, len(rows))
	observed := map[string]bool{}
	var maxUsable time.Time
	formulaInputs := map[string][]FormulaInput{}
	for _, row := range rows {
		currency := any(nil)
		if row.Currency != nil {
			currency = *row.Currency
		}
		records = append(records, map[string]any{"id": row.ID, "series_id": row.SeriesID, "metric": row.Metric, "value": row.Value,
			"unit": row.Unit, "currency": currency, "caliber_id": row.CaliberID, "contract_kind": row.ContractKind,
			"trading_day": row.TradingDay, "period_start": row.PeriodStart, "period_end": row.PeriodEnd, "usable_at": row.UsableAt})
		observed[row.Metric] = true
		if row.UsableAt.After(maxUsable) {
			maxUsable = row.UsableAt
		}
		if parsed, err := decimal.NewFromString(row.Value); err == nil {
			tradingDay := row.TradingDay
			if len(tradingDay) >= 10 {
				tradingDay = tradingDay[:10]
			}
			currencyValue := ""
			if row.Currency != nil {
				currencyValue = *row.Currency
			}
			formulaInputs[row.SeriesID] = append(formulaInputs[row.SeriesID], FormulaInput{RecordID: row.ID, Role: "current", Value: parsed,
				Unit: row.Unit, Currency: currencyValue, CaliberID: row.CaliberID, SeriesID: row.SeriesID, ContractKind: row.ContractKind,
				TradingDay: tradingDay, PeriodStart: row.PeriodStart, PeriodEnd: row.PeriodEnd})
		}
	}
	required := []string{"actual_contract_daily", "spot_60d", "exchange_inventory_26w", "warehouse_receipt_60d", "refined_copper_output_12p", "copper_fabrication_output_12p"}
	missing := make([]string, 0)
	for _, metric := range required {
		if !observed[metric] {
			missing = append(missing, metric)
		}
	}
	calculations := make([]any, 0)
	for _, inputs := range formulaInputs {
		if len(inputs) < 2 {
			continue
		}
		sort.Slice(inputs, func(i, j int) bool { return inputs[i].PeriodStart > inputs[j].PeriodStart })
		previous, current := inputs[1], inputs[0]
		previous.Role, current.Role = "previous", "current"
		if result, err := Calculate(FormulaInventoryChange, []FormulaInput{current, previous}); err == nil {
			calculations = append(calculations, map[string]any{"formula": result.Formula, "formula_version": result.FormulaVersion,
				"input_record_ids": result.InputRecordIDs, "value": result.Value.String(), "unit": result.Unit})
		}
	}
	var dataAsOf any = nil
	if !maxUsable.IsZero() {
		dataAsOf = maxUsable
	}
	snapshot := fmt.Sprintf("workbench_%s_%s_%d_%s_%s", productID, contractID, window, priceType, hashJSON(records))
	return map[string]any{
		"product": product, "contracts": contracts, "snapshot_id": snapshot,
		"data_as_of": dataAsOf, "last_checked_at": d.Now().UTC(), "records": records, "calculations": calculations,
		"required_metrics": required, "missing_metrics": missing,
	}, nil
}

func (d *Domain) CreateDraft(ctx context.Context, id Identity, idempotencyKey string, input model.DraftInput) (map[string]any, error) {
	if err := validateDraftInput(input); err != nil {
		return nil, err
	}
	now := d.Now().UTC()
	value := model.Draft{ID: "draft_" + uuid.NewString(), OwnerID: id.OwnerID, Mode: id.Mode, Revision: 1, Input: input,
		ParseState: "not_started", Claims: datatypes.JSON(`[]`), CreatedAt: now, UpdatedAt: now}
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireAdmittedProduct(tx, input.ProductID); err != nil {
			return err
		}
		if err := validateDraftReferences(tx, id, input); err != nil {
			return err
		}
		if existing, err := replayIdempotency(tx, id, "draft.create", idempotencyKey, hashJSON(input)); err != nil {
			return err
		} else if existing != nil {
			value = *existing
			return nil
		}
		if err := tx.Create(&value).Error; err != nil {
			return err
		}
		return d.writeIdempotency(tx, id, "draft.create", idempotencyKey, hashJSON(input), value)
	})
	if err != nil {
		return nil, err
	}
	return draftMap(value), nil
}

func validateDraftInput(input model.DraftInput) error {
	if input.ProductID == "" || (input.HorizonDays != 7 && input.HorizonDays != 14 && input.HorizonDays != 30) {
		return ErrInvalidInput
	}
	length := len([]rune(strings.TrimSpace(input.Text)))
	if length != 0 && (length < 20 || length > 2000) {
		return ErrInvalidInput
	}
	if length == 0 && input.DocumentID == nil {
		return ErrInvalidInput
	}
	return nil
}

func (d *Domain) GetDraft(ctx context.Context, id Identity, draftID string) (map[string]any, error) {
	var value model.Draft
	err := d.DB.WithContext(ctx).Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, draftID, id.OwnerID, id.Mode).First(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return draftMap(value), nil
}

func (d *Domain) PatchDraft(ctx context.Context, id Identity, draftID string, expected int64, input model.DraftInput) (map[string]any, error) {
	if err := validateDraftInput(input); err != nil {
		return nil, err
	}
	var value model.Draft
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := validateDraftReferences(tx, id, input); err != nil {
			return err
		}
		res := tx.Model(&model.Draft{}).Where(`id=? AND owner_id=? AND mode=? AND revision=? AND deleted_at IS NULL`, draftID, id.OwnerID, id.Mode, expected).
			Updates(map[string]any{`input`: input, `revision`: gorm.Expr(`revision+1`), `parse_state`: `not_started`, `parsed_revision`: nil, `claims`: datatypes.JSON(`[]`), `updated_at`: d.Now().UTC()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrRevisionConflict
		}
		if err := tx.Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, draftID, id.OwnerID, id.Mode).First(&value).Error; err != nil {
			return err
		}
		return tx.Create(DraftVersion{DraftID: draftID, OwnerID: id.OwnerID, Mode: id.Mode, Revision: int64(value.Revision), Input: input, Claims: value.Claims, CreatedAt: value.UpdatedAt}).Error
	})
	if err != nil {
		return nil, err
	}
	return draftMap(value), nil
}

func (d *Domain) DeleteDraft(ctx context.Context, id Identity, draftID, idempotencyKey string) (map[string]any, error) {
	now := d.Now().UTC()
	requestHash := hashJSON(map[string]any{"id": draftID, "action": "delete"})
	result := map[string]any{"deletion_id": "del_" + uuid.NewString(), "hidden_at": now, "purge_due_at": now.Add(24 * time.Hour)}
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if existing, ok, err := replayDeletion(ctx, tx, id, "draft.delete", idempotencyKey, requestHash); err != nil {
			return err
		} else if ok {
			result = existing
			return nil
		}
		var draft model.Draft
		if err := tx.Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, draftID, id.OwnerID, id.Mode).First(&draft).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		var referenced int64
		if err := tx.Raw(`SELECT count(*) FROM futures_runs WHERE draft_id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, draftID, id.OwnerID, id.Mode).Scan(&referenced).Error; err != nil {
			return err
		}
		if referenced > 0 {
			return ErrReferenced
		}
		res := tx.Model(&model.Draft{}).Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, draftID, id.OwnerID, id.Mode).Update(`deleted_at`, d.Now().UTC())
		if res.RowsAffected != 1 {
			return ErrRevisionConflict
		}
		if err := tx.Exec(`INSERT INTO futures_deletion_jobs(id,owner_id,mode,target_type,target_id,impact_version,purge_due_at,state) VALUES(?,?,?,?,?,?,?,'scheduled')`,
			result["deletion_id"], id.OwnerID, id.Mode, "draft", draftID, hashJSON(draft), now.Add(24*time.Hour)).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO futures_tombstones(request_id,owner_id,object_type,object_id,hidden_at,purge_due_at) VALUES(?,?,?,?,?,?)`,
			uuid.NewString(), id.OwnerID, "draft", draftID, now, now.Add(24*time.Hour)).Error; err != nil {
			return err
		}
		if err := hideIdempotentObject(ctx, tx, id, "draft.create", draftID, now); err != nil {
			return err
		}
		return d.writeIdempotency(tx, id, "draft.delete", idempotencyKey, requestHash, result)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (d *Domain) ParseDraft(ctx context.Context, id Identity, draftID string, expected int64, idempotencyKey string) (map[string]any, error) {
	var value model.Draft
	err := d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if existing, err := replayDraftIdempotency(tx, id, "draft.parse", idempotencyKey, hashJSON(map[string]any{"draft_id": draftID, "expected_revision": expected})); err != nil {
			return err
		} else if existing != nil {
			value = *existing
			return nil
		}
		if err := tx.Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, draftID, id.OwnerID, id.Mode).First(&value).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if int64(value.Revision) != expected {
			return ErrRevisionConflict
		}
		claimText := strings.TrimSpace(value.Input.Text)
		if claimText == "" && value.Input.DocumentID != nil {
			var document model.Document
			if err := tx.Where(`id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, *value.Input.DocumentID, id.OwnerID, id.Mode).First(&document).Error; err != nil {
				return ErrNotFound
			}
			claimText = strings.TrimSpace(document.ExtractedText)
		}
		if claimText == "" {
			return ErrInvalidInput
		}
		claims := []map[string]any{{"id": "claim_" + uuid.NewString(), "kind": "hypothesis", "text": claimText, "locator": nil}}
		if len(claims) > 12 {
			return ErrInvalidInput
		}
		raw, _ := json.Marshal(claims)
		res := tx.Model(&model.Draft{}).Where(`id=? AND owner_id=? AND mode=? AND revision=?`, draftID, id.OwnerID, id.Mode, expected).
			Updates(map[string]any{`parse_state`: `succeeded`, `parsed_revision`: expected, `claims`: datatypes.JSON(raw), `claims_overflow`: false, `updated_at`: d.Now().UTC()})
		if res.RowsAffected != 1 {
			return ErrRevisionConflict
		}
		parsedRevision := int(expected)
		value.ParseState, value.ParsedRevision, value.Claims = "succeeded", &parsedRevision, datatypes.JSON(raw)
		return d.writeIdempotency(tx, id, "draft.parse", idempotencyKey, hashJSON(map[string]any{"draft_id": draftID, "expected_revision": expected}), value)
	})
	if err != nil {
		return nil, err
	}
	return draftMap(value), nil
}

func draftMap(value model.Draft) map[string]any {
	var claims any = []any{}
	_ = json.Unmarshal(value.Claims, &claims)
	parsed := any(nil)
	if value.ParsedRevision != nil {
		parsed = *value.ParsedRevision
	}
	return map[string]any{
		"id": value.ID, "revision": value.Revision, "input": value.Input, "parse_state": value.ParseState,
		"parsed_revision": parsed, "claims": claims, "claims_overflow": value.ClaimsOverflow,
		"consumed_model_calls": value.ConsumedModelCalls, "consumed_tokens": value.ConsumedTokens, "created_at": value.CreatedAt,
	}
}

func requireAdmittedProduct(tx *gorm.DB, productID string) error {
	var status string
	err := tx.Raw(`SELECT status FROM futures_admission WHERE product_id=? ORDER BY version DESC LIMIT 1`, productID).Row().Scan(&status)
	if err != nil || status != "admitted" {
		return ErrForbidden
	}
	return nil
}

func validateDraftReferences(tx *gorm.DB, id Identity, input model.DraftInput) error {
	if input.ContractID != nil {
		var lastTrading time.Time
		if err := tx.Raw(`SELECT last_trading_at FROM futures_contracts WHERE id=? AND product_id=? AND kind='actual' AND deleted_at IS NULL`, *input.ContractID, input.ProductID).Scan(&lastTrading).Error; err != nil {
			return err
		}
		if lastTrading.IsZero() || time.Now().UTC().AddDate(0, 0, input.HorizonDays).After(lastTrading) {
			return ErrInvalidInput
		}
	}
	if input.DocumentID != nil {
		var count int64
		if err := tx.Raw(`SELECT count(*) FROM futures_documents WHERE id=? AND owner_id=? AND mode=? AND deleted_at IS NULL`, *input.DocumentID, id.OwnerID, id.Mode).Scan(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return ErrNotFound
		}
	}
	return nil
}

func hashJSON(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func replayIdempotency(tx *gorm.DB, id Identity, operation, key, requestHash string) (*model.Draft, error) {
	if key == "" {
		return nil, ErrInvalidInput
	}
	var row model.Idempotency
	err := tx.Where(`owner_id=? AND mode=? AND operation=? AND key=?`, id.OwnerID, id.Mode, operation, key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.RequestHash != requestHash {
		return nil, ErrIdempotencyConflict
	}
	var draft model.Draft
	if err := json.Unmarshal(row.Response, &draft); err != nil {
		return nil, err
	}
	if !row.ExpiresAt.After(time.Now().UTC()) {
		return nil, ErrNotFound
	}
	if err := ensureIdempotentObjectVisible(context.Background(), tx, id, draft.ID, draft); err != nil {
		return nil, err
	}
	return &draft, nil
}

func replayDraftIdempotency(tx *gorm.DB, id Identity, operation, key, requestHash string) (*model.Draft, error) {
	if key == "" {
		return nil, ErrInvalidInput
	}
	var row model.Idempotency
	err := tx.Where(`owner_id=? AND mode=? AND operation=? AND key=?`, id.OwnerID, id.Mode, operation, key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.RequestHash != requestHash {
		return nil, ErrIdempotencyConflict
	}
	var value model.Draft
	if err := json.Unmarshal(row.Response, &value); err != nil {
		return nil, err
	}
	if !row.ExpiresAt.After(time.Now().UTC()) {
		return nil, ErrNotFound
	}
	if err := ensureIdempotentObjectVisible(context.Background(), tx, id, value.ID, value); err != nil {
		return nil, err
	}
	return &value, nil
}

func (d *Domain) writeIdempotency(tx *gorm.DB, id Identity, operation, key, requestHash string, response any) error {
	raw, err := json.Marshal(response)
	if err != nil {
		return err
	}
	return tx.Create(model.Idempotency{OwnerID: id.OwnerID, Mode: id.Mode, Operation: operation, Key: key, RequestHash: requestHash,
		Response: datatypes.JSON(raw), Status: "complete", ExpiresAt: d.Now().UTC().Add(24 * time.Hour), CreatedAt: d.Now().UTC()}).Error
}
