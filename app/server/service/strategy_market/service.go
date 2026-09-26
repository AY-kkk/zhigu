package strategy_market

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"zhigu/server/httpx"
	model "zhigu/server/model/strategy"
	"zhigu/server/service/finance"
	"zhigu/server/service/market"
)

// InstrumentLookup resolves real catalog instruments for validation binding and
// copy-time currency checks (implemented by market.Service).
type InstrumentLookup interface {
	Instrument(ctx context.Context, id string) (market.InstrumentView, error)
}

type Service struct {
	DB     *gorm.DB
	Lookup InstrumentLookup
}

func New(db *gorm.DB, lookup InstrumentLookup) *Service {
	return &Service{DB: db, Lookup: lookup}
}

func nowUTC() time.Time { return time.Now().UTC() }

// canonicalHash returns SHA-256 over canonical JSON (sorted keys) of v.
func canonicalHash(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	var norm any
	if err := json.Unmarshal(raw, &norm); err != nil {
		return "", err
	}
	out, err := json.Marshal(norm)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(out)
	return hex.EncodeToString(sum[:]), nil
}

// idempotent serializes on (owner, operation, key): the reservation row is
// inserted inside the caller's write path so concurrent same-key requests block
// on the unique key and resolve to the first stored response. fn performs the
// mutation in the same transaction; its rollback also rolls back the reservation.
func (s *Service) idempotent(ctx context.Context, tx *gorm.DB, uid uint, operation, key, requestHash string, fn func(tx *gorm.DB) (string, any, error)) (objectID string, resp any, replayed bool, err error) {
	rec := model.Idempotency{
		OwnerID: uid, Operation: operation, IdempotencyKey: key,
		RequestHash: requestHash, CreatedAt: nowUTC(),
	}
	// The reservation insert is savepoint-guarded: a unique-key hit poisons the
	// transaction otherwise, and the replay lookup below must still run in it.
	if err := tx.SavePoint("idem_reserve").Error; err != nil {
		return "", nil, false, err
	}
	if cerr := tx.Create(&rec).Error; cerr != nil {
		tx.RollbackTo("idem_reserve")
		var existing model.Idempotency
		if qerr := tx.Where("owner_id = ? AND operation = ? AND idempotency_key = ?", uid, operation, key).Take(&existing).Error; qerr != nil {
			return "", nil, false, cerr
		}
		if existing.RequestHash != requestHash {
			return "", nil, false, finance.NewError(409, "conflict", "IDEMPOTENCY_CONFLICT", "相同幂等键对应不同请求")
		}
		if len(existing.ResponseSnapshot) == 0 {
			return "", nil, false, finance.NewError(409, "conflict", "REQUEST_IN_PROGRESS", "相同幂等键请求仍在处理")
		}
		return existing.ObjectID, json.RawMessage(existing.ResponseSnapshot), true, nil
	}
	oid, out, ferr := fn(tx)
	if ferr != nil {
		return "", nil, false, ferr
	}
	snap, merr := json.Marshal(out)
	if merr != nil {
		return "", nil, false, merr
	}
	if uerr := tx.Model(&model.Idempotency{}).
		Where("owner_id = ? AND operation = ? AND idempotency_key = ?", uid, operation, key).
		Updates(map[string]any{"object_id": oid, "response_snapshot": datatypes.JSON(snap)}).Error; uerr != nil {
		return "", nil, false, uerr
	}
	return oid, out, false, nil
}

func (s *Service) audit(tx *gorm.DB, actor uint, action, requestID, itemID string, versionID *string, beforeRev, afterRev int, reason string, payload any) error {
	h, err := canonicalHash(payload)
	if err != nil {
		return err
	}
	row := model.MarketAudit{
		ID: httpx.NewID("sma"), ItemID: itemID, MarketVersionID: versionID,
		ActorID: actor, Action: action, RequestID: requestID,
		BeforeRevision: beforeRev, AfterRevision: afterRev, Reason: reason,
		PayloadHash: h, CreatedAt: nowUTC(),
	}
	return tx.Create(&row).Error
}

// itemLocked loads a market item with a row lock so publish/withdraw/copy serialize.
func itemLocked(tx *gorm.DB, id string) (model.MarketItem, error) {
	var item model.MarketItem
	err := tx.Raw(`SELECT * FROM finance_strategy_market_items WHERE id = ? FOR UPDATE`, id).Scan(&item).Error
	if err != nil {
		return item, err
	}
	if item.ID == "" {
		return item, finance.NewError(404, "not_found", "NOT_FOUND", "市场条目不存在")
	}
	return item, nil
}

func errItemState(item model.MarketItem) error {
	switch item.Status {
	case ItemPublished:
		return nil
	case ItemWithdrawn:
		return finance.NewError(410, "gone", "MARKET_ITEM_WITHDRAWN", "条目已下架")
	default:
		return finance.NewError(404, "not_found", "NOT_FOUND", "市场条目不存在")
	}
}

// ---------- read side ----------

type ListQuery struct {
	Q          string
	Category   string
	Market     string
	Period     string
	Validation string
	Evidence   string
	Cursor     string
	Limit      int
}

type itemRow struct {
	ID               string
	CurrentVersionID string
	Status           string
	UpdatedAt        time.Time
	PublishedAt      *time.Time
	VersionNo        int
	Name             string
	Summary          string
	Category         string
	Tags             datatypes.JSON
	Markets          datatypes.JSON
	SignalPeriod     string
	ValidationStatus string
	ValidationReport datatypes.JSON
	HasEvidence      bool
}

func (r itemRow) backtestStatus() string {
	if r.HasEvidence {
		return BacktestHasEvidence
	}
	return BacktestNotTested
}

func (r itemRow) copyable() (bool, string) {
	if r.ValidationStatus != ValidationPassed {
		return false, "未通过规则校验"
	}
	rep := parseValidationReport(r.ValidationReport)
	if !EngineSupported(rep.CompilerVersion) {
		return false, "当前引擎不支持该规则版本"
	}
	return true, ""
}

type validationReport struct {
	InstrumentID    string          `json:"instrument_id"`
	DSLHash         string          `json:"dsl_hash"`
	CompilerVersion string          `json:"compiler_version"`
	WarmupBars      int             `json:"warmup_bars"`
	RequiredData    []string        `json:"required_data"`
	Continuity      json.RawMessage `json:"continuity_checks"`
	Error           string          `json:"error,omitempty"`
}

func parseValidationReport(raw datatypes.JSON) validationReport {
	var rep validationReport
	_ = json.Unmarshal(raw, &rep)
	return rep
}

func (s *Service) List(ctx context.Context, q ListQuery) (map[string]any, error) {
	switch q.Validation {
	case "", ValidationPending, ValidationPassed, ValidationFailed:
	default:
		return nil, finance.NewError(400, "validation", "INVALID_PARAM", "validation 筛选值非法")
	}
	switch q.Evidence {
	case "", BacktestNotTested, BacktestHasEvidence:
	default:
		return nil, finance.NewError(400, "validation", "INVALID_PARAM", "evidence 筛选值非法")
	}
	switch q.Market {
	case "", "A", "HK":
	default:
		return nil, finance.NewError(400, "validation", "INVALID_PARAM", "market 筛选值非法")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		return nil, finance.NewError(400, "validation", "INVALID_PARAM", "limit 最大 50")
	}

	filter := map[string]string{"q": q.Q, "category": q.Category, "market": q.Market, "period": q.Period, "validation": q.Validation, "evidence": q.Evidence}
	after, afterID, err := decodeCursor(q.Cursor, filter)
	if err != nil {
		return nil, err
	}

	sql := strings.Builder{}
	args := []any{}
	sql.WriteString(`SELECT i.id AS id, i.current_version_id AS current_version_id, i.status AS status,
 i.updated_at AS updated_at, i.published_at AS published_at,
 v.version_no AS version_no, v.name AS name, v.summary AS summary, v.category AS category,
 v.tags AS tags, v.markets AS markets, v.signal_period AS signal_period,
 v.validation_status AS validation_status, v.validation_report AS validation_report,
 EXISTS (SELECT 1 FROM finance_strategy_market_evidence e
          WHERE e.market_version_id = v.id AND e.status = 'approved') AS has_evidence
 FROM finance_strategy_market_items i
 JOIN finance_strategy_market_versions v ON v.id = i.current_version_id AND v.item_id = i.id
 WHERE i.status = 'published'`)
	if q.Q != "" {
		sql.WriteString(` AND (v.name ILIKE ? OR v.summary ILIKE ? OR v.tags::text ILIKE ?)`)
		like := "%" + q.Q + "%"
		args = append(args, like, like, like)
	}
	if q.Category != "" {
		sql.WriteString(` AND v.category = ?`)
		args = append(args, q.Category)
	}
	if q.Market != "" {
		sql.WriteString(` AND v.markets @> ?::jsonb`)
		args = append(args, `["`+q.Market+`"]`)
	}
	if q.Period != "" {
		sql.WriteString(` AND v.signal_period = ?`)
		args = append(args, q.Period)
	}
	if q.Validation != "" {
		sql.WriteString(` AND v.validation_status = ?`)
		args = append(args, q.Validation)
	}
	if q.Evidence == BacktestHasEvidence {
		sql.WriteString(` AND EXISTS (SELECT 1 FROM finance_strategy_market_evidence e WHERE e.market_version_id = v.id AND e.status = 'approved')`)
	}
	if q.Evidence == BacktestNotTested {
		sql.WriteString(` AND NOT EXISTS (SELECT 1 FROM finance_strategy_market_evidence e WHERE e.market_version_id = v.id AND e.status = 'approved')`)
	}
	if after != nil {
		sql.WriteString(` AND (i.updated_at, i.id) < (?, ?)`)
		args = append(args, *after, afterID)
	}
	sql.WriteString(` ORDER BY i.updated_at DESC, i.id DESC LIMIT ?`)
	args = append(args, limit+1)

	var rows []itemRow
	if err := s.DB.WithContext(ctx).Raw(sql.String(), args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	next := nilCursor
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		c := encodeCursor(last.UpdatedAt, last.ID, filter)
		next = &c
	}
	items := make([]MarketCard, 0, len(rows))
	for _, r := range rows {
		copyable, reason := r.copyable()
		card := MarketCard{
			ID: r.ID, MarketVersionID: r.CurrentVersionID, VersionNo: r.VersionNo,
			Name: r.Name, Summary: r.Summary, Category: r.Category,
			Tags: json.RawMessage(r.Tags), Markets: json.RawMessage(r.Markets),
			SignalPeriod: r.SignalPeriod, ValidationStatus: r.ValidationStatus,
			BacktestStatus: r.backtestStatus(), PublishedAt: r.PublishedAt, UpdatedAt: r.UpdatedAt,
			Copyable: copyable, CopyDisabledReason: reason,
		}
		items = append(items, card)
	}
	return map[string]any{"items": items, "next_cursor": next}, nil
}

var nilCursor *string

func (s *Service) detailRow(ctx context.Context, itemID string) (model.MarketItem, itemRow, error) {
	var item model.MarketItem
	if err := s.DB.WithContext(ctx).Where("id = ?", itemID).Take(&item).Error; err != nil {
		return item, itemRow{}, finance.NewError(404, "not_found", "NOT_FOUND", "市场条目不存在")
	}
	if item.Status == ItemDraft {
		return item, itemRow{}, finance.NewError(404, "not_found", "NOT_FOUND", "市场条目不存在")
	}
	var rows []itemRow
	if err := s.DB.WithContext(ctx).Raw(`SELECT i.id AS id, i.current_version_id AS current_version_id, i.status AS status,
 i.updated_at AS updated_at, i.published_at AS published_at,
 v.version_no AS version_no, v.name AS name, v.summary AS summary, v.category AS category,
 v.tags AS tags, v.markets AS markets, v.signal_period AS signal_period,
 v.validation_status AS validation_status, v.validation_report AS validation_report,
 EXISTS (SELECT 1 FROM finance_strategy_market_evidence e
          WHERE e.market_version_id = v.id AND e.status = 'approved') AS has_evidence
 FROM finance_strategy_market_items i
 JOIN finance_strategy_market_versions v ON v.id = i.current_version_id AND v.item_id = i.id
 WHERE i.id = ?`, itemID).Scan(&rows).Error; err != nil {
		return item, itemRow{}, err
	}
	if len(rows) != 1 {
		return item, itemRow{}, finance.NewError(404, "not_found", "NOT_FOUND", "市场条目不存在")
	}
	return item, rows[0], nil
}

// Detail returns the full read-only rule, sources and evidence summaries.
// Unpublished/never-existed → 404; published-then-withdrawn → 410 minimal state.
func (s *Service) Detail(ctx context.Context, itemID string) (map[string]any, error) {
	item, row, err := s.detailRow(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if item.Status == ItemWithdrawn {
		return nil, finance.NewError(410, "gone", "MARKET_ITEM_WITHDRAWN", "条目已下架")
	}
	var ver model.MarketVersion
	if err := s.DB.WithContext(ctx).Where("id = ?", row.CurrentVersionID).Take(&ver).Error; err != nil {
		return nil, err
	}
	var summaries []EvidenceSummary
	if err := s.DB.WithContext(ctx).Raw(`SELECT id AS evidence_id, validation_instrument_id AS validation_instrument_id,
 result_hash AS result_hash, created_at AS created_at
 FROM finance_strategy_market_evidence
 WHERE market_version_id = ? AND status = 'approved' ORDER BY created_at DESC, id DESC`, ver.ID).Scan(&summaries).Error; err != nil {
		return nil, err
	}
	if summaries == nil {
		summaries = []EvidenceSummary{}
	}
	var tpl struct {
		EditorState json.RawMessage `json:"editor_state"`
	}
	_ = json.Unmarshal(ver.RuleTemplate, &tpl)
	editorState := tpl.EditorState
	if editorState == nil {
		editorState = json.RawMessage([]byte("null"))
	}
	copyable, reason := row.copyable()
	return map[string]any{
		"id": row.ID, "market_version_id": row.CurrentVersionID, "version_no": ver.VersionNo,
		"name": ver.Name, "summary": ver.Summary, "category": ver.Category,
		"tags": json.RawMessage(ver.Tags), "markets": json.RawMessage(ver.Markets),
		"signal_period": ver.SignalPeriod, "validation_status": ver.ValidationStatus,
		"backtest_status": row.backtestStatus(), "published_at": row.PublishedAt, "updated_at": row.UpdatedAt,
		"copyable": copyable, "copy_disabled_reason": reason,
		"description": ver.Description, "hypothesis": ver.Hypothesis, "failure_cases": ver.FailureCases,
		"sources": json.RawMessage(ver.Sources), "rights_note": ver.RightsNote,
		"editor_schema_version": ver.EditorSchemaVersion, "editor_state": editorState,
		"backtest_defaults":  json.RawMessage(ver.BacktestDefaults),
		"evidence_summaries": summaries,
	}, nil
}

// EvidenceSection serves one approved evidence of the current version.
// overview = config/manifest/metrics/limitations; equity and trades paginate.
func (s *Service) EvidenceSection(ctx context.Context, itemID, evidenceID, section, cursor string, limit int) (map[string]any, error) {
	item, row, err := s.detailRow(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if item.Status == ItemWithdrawn {
		return nil, finance.NewError(410, "gone", "MARKET_ITEM_WITHDRAWN", "条目已下架")
	}
	var ev model.MarketEvidence
	if err := s.DB.WithContext(ctx).Where("id = ?", evidenceID).Take(&ev).Error; err != nil {
		return nil, finance.NewError(404, "not_found", "NOT_FOUND", "证据不存在")
	}
	if ev.Status != EvidenceApproved || ev.MarketVersionID != row.CurrentVersionID {
		return nil, finance.NewError(404, "not_found", "NOT_FOUND", "证据不存在")
	}
	switch section {
	case "overview":
		var manifest map[string]any
		_ = json.Unmarshal(ev.Manifest, &manifest)
		return map[string]any{
			"evidence_id": ev.ID, "validation_instrument_id": ev.ValidationInstrumentID,
			"bound_dsl_hash": ev.BoundDSLHash, "config": json.RawMessage(ev.Config),
			"manifest": json.RawMessage(ev.Manifest), "metrics": json.RawMessage(ev.Metrics),
			"limitations": json.RawMessage(ev.Limitations), "result_hash": ev.ResultHash,
			"created_at": ev.CreatedAt,
		}, nil
	case "equity", "trades":
		if limit <= 0 {
			limit = 100
		}
		if limit > 100 {
			return nil, finance.NewError(400, "validation", "INVALID_PARAM", "limit 最大 100")
		}
		raw := ev.Equity
		if section == "trades" {
			raw = ev.Trades
		}
		var all []json.RawMessage
		_ = json.Unmarshal(raw, &all)
		filter := map[string]string{"item": itemID, "evidence": evidenceID, "section": section}
		off, _, err := decodeOffsetCursor(cursor, filter)
		if err != nil {
			return nil, err
		}
		if off > len(all) {
			return nil, finance.NewError(400, "validation", "INVALID_PARAM", "cursor 非法")
		}
		end := off + limit
		next := nilCursor
		if end < len(all) {
			c := encodeOffsetCursor(end, filter)
			next = &c
		} else {
			end = len(all)
		}
		page := all[off:end]
		if page == nil {
			page = []json.RawMessage{}
		}
		return map[string]any{"items": page, "next_cursor": next, "section": section, "evidence_id": ev.ID}, nil
	default:
		return nil, finance.NewError(400, "validation", "INVALID_PARAM", "section 仅支持 overview/equity/trades")
	}
}

// ---------- cursor codecs (opaque, bound to filter conditions) ----------

func encodeCursor(t time.Time, id string, filter map[string]string) string {
	fh := filterHash(filter)
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("v1|%s|%s|%s", fh, t.UTC().Format(time.RFC3339Nano), id)))
}

func decodeCursor(cursor string, filter map[string]string) (*time.Time, string, error) {
	if cursor == "" {
		return nil, "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, "", finance.NewError(400, "validation", "INVALID_PARAM", "cursor 非法")
	}
	parts := strings.SplitN(string(raw), "|", 4)
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != filterHash(filter) {
		return nil, "", finance.NewError(400, "validation", "INVALID_PARAM", "cursor 与当前筛选条件不匹配")
	}
	ts, err := time.Parse(time.RFC3339Nano, parts[2])
	if err != nil {
		return nil, "", finance.NewError(400, "validation", "INVALID_PARAM", "cursor 非法")
	}
	return &ts, parts[3], nil
}

func encodeOffsetCursor(offset int, filter map[string]string) string {
	fh := filterHash(filter)
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("v1o|%s|%d", fh, offset)))
}

func decodeOffsetCursor(cursor string, filter map[string]string) (int, string, error) {
	if cursor == "" {
		return 0, "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, "", finance.NewError(400, "validation", "INVALID_PARAM", "cursor 非法")
	}
	parts := strings.SplitN(string(raw), "|", 3)
	if len(parts) != 3 || parts[0] != "v1o" || parts[1] != filterHash(filter) {
		return 0, "", finance.NewError(400, "validation", "INVALID_PARAM", "cursor 与当前筛选条件不匹配")
	}
	var off int
	if _, err := fmt.Sscanf(parts[2], "%d", &off); err != nil || off < 0 {
		return 0, "", finance.NewError(400, "validation", "INVALID_PARAM", "cursor 非法")
	}
	return off, parts[2], nil
}

func filterHash(filter map[string]string) string {
	h, _ := canonicalHash(filter)
	return h[:16]
}
