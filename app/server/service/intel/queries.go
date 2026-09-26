package intel

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

func jsonValue(raw any) any {
	switch v := raw.(type) {
	case string:
		var out any
		if err := json.Unmarshal([]byte(v), &out); err == nil {
			return out
		}
		return v
	case []byte:
		var out any
		if err := json.Unmarshal(v, &out); err == nil {
			return out
		}
		return string(v)
	default:
		return raw
	}
}

func (s *Service) SearchInstruments(ctx context.Context, scope Scope, q string, limit int) (map[string]any, error) {
	q = strings.TrimSpace(q)
	if len(q) > 50 {
		return nil, newError(400, "INVALID_PARAM", "q 最多 50 字")
	}
	rows := make([]map[string]any, 0)
	err := s.DB.WithContext(ctx).Raw(`SELECT code,name,exchange FROM finance_intel_instruments
WHERE namespace_id = ? AND (? = '' OR code ILIKE ? OR name ILIKE ?)
ORDER BY code LIMIT ?`, scope.NamespaceID, q, "%"+q+"%", "%"+q+"%", normalizeLimit(limit)).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	items := make([]any, len(rows))
	for i := range rows {
		items[i] = rows[i]
	}
	return map[string]any{"items": items}, nil
}

func (s *Service) ListWatchlist(ctx context.Context, scope Scope) (map[string]any, error) {
	rows := make([]map[string]any, 0)
	err := s.DB.WithContext(ctx).Raw(`SELECT w.code, i.name, w.subscribed_at
FROM finance_intel_watchlist w JOIN finance_intel_instruments i ON i.namespace_id=w.namespace_id AND i.code=w.code
WHERE w.namespace_id=? AND w.principal_key=? ORDER BY w.subscribed_at, w.code`, scope.NamespaceID, scope.PrincipalKey).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	items := make([]any, len(rows))
	for i := range rows {
		items[i] = rows[i]
	}
	return map[string]any{"items": items}, nil
}

func (s *Service) ListEvents(ctx context.Context, scope Scope, q EventQuery) (ListEventsResult, error) {
	args := []any{scope.NamespaceID, scope.PrincipalKey}
	sql := `SELECT e.logical_id AS event_id, e.event_type AS type, e.title, e.subject_code, e.subject_name,
 e.current_version AS event_version, e.updated_at, s.verification, s.phase, s.freshness, s.support_level,
 s.current_values::text AS current_values, s.coverage::text AS coverage,
 (SELECT count(*) FROM finance_intel_conflicts c WHERE c.event_id=e.id AND c.status='open') AS open_conflict_count
FROM finance_intel_events e
JOIN finance_intel_snapshots s ON s.id=e.current_snapshot_id
WHERE e.namespace_id=? AND EXISTS (
 SELECT 1 FROM finance_intel_event_bindings b JOIN finance_intel_watchlist w
 ON w.namespace_id=b.namespace_id AND w.code=b.code
 WHERE b.namespace_id=e.namespace_id AND b.event_id=e.id AND w.principal_key=?
)`
	if q.Code != "" {
		sql += ` AND EXISTS (SELECT 1 FROM finance_intel_event_bindings b2 WHERE b2.event_id=e.id AND b2.code=?)`
		args = append(args, strings.ToUpper(q.Code))
	}
	if q.Type != "" {
		sql += ` AND e.event_type=?`
		args = append(args, q.Type)
	}
	if q.Verification != "" {
		sql += ` AND s.verification=?`
		args = append(args, q.Verification)
	}
	sql += ` ORDER BY e.updated_at DESC, e.id DESC LIMIT ?`
	args = append(args, normalizeLimit(q.Limit))
	rows := make([]map[string]any, 0)
	if err := s.DB.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return ListEventsResult{}, err
	}
	items := make([]any, 0, len(rows))
	for _, row := range rows {
		row["current_values"] = jsonValue(row["current_values"])
		row["coverage"] = jsonValue(row["coverage"])
		row["subjects"] = s.eventSubjects(ctx, scope.NamespaceID, row["event_id"].(string))
		items = append(items, row)
	}
	return ListEventsResult{Items: items}, nil
}

func (s *Service) eventSubjects(ctx context.Context, namespaceID, eventID string) []any {
	rows := make([]map[string]any, 0)
	_ = s.DB.WithContext(ctx).Raw(`SELECT code, role, evidence_ids::text AS evidence_ids
FROM finance_intel_event_bindings WHERE namespace_id=? AND event_id=(SELECT id FROM finance_intel_events WHERE namespace_id=? AND logical_id=?) ORDER BY role, code`, namespaceID, namespaceID, eventID).Scan(&rows)
	out := make([]any, len(rows))
	for i := range rows {
		rows[i]["evidence_ids"] = jsonValue(rows[i]["evidence_ids"])
		out[i] = rows[i]
	}
	return out
}

func (s *Service) EventDetail(ctx context.Context, scope Scope, id string, version int) (map[string]any, error) {
	var event map[string]any
	if err := s.DB.WithContext(ctx).Raw(`SELECT * FROM finance_intel_events WHERE namespace_id=? AND logical_id=?`, scope.NamespaceID, id).Scan(&event).Error; err != nil {
		return nil, err
	}
	if len(event) == 0 {
		return nil, newError(404, "NOT_FOUND", "事件不存在")
	}
	latest := toInt(event["current_version"])
	selected := latest
	if version > 0 {
		selected = version
	}
	eventID, _ := event["id"].(string)
	var snapshot map[string]any
	err := s.DB.WithContext(ctx).Raw(`SELECT * FROM finance_intel_snapshots WHERE namespace_id=? AND event_id=? AND event_version=?`, scope.NamespaceID, eventID, selected).Scan(&snapshot).Error
	if err != nil {
		return nil, err
	}
	if len(snapshot) == 0 {
		return nil, newError(404, "NOT_FOUND", "事件版本不存在")
	}
	var change map[string]any
	_ = s.DB.WithContext(ctx).Raw(`SELECT logical_id AS id FROM finance_intel_changes WHERE namespace_id=? AND event_id=? AND event_version=?`, scope.NamespaceID, eventID, selected).Scan(&change)
	out := map[string]any{
		"event_id": id, "type": event["event_type"], "title": event["title"],
		"subjects":     s.eventSubjects(ctx, scope.NamespaceID, id),
		"verification": snapshot["verification"], "phase": snapshot["phase"], "freshness": snapshot["freshness"],
		"support_level": snapshot["support_level"], "event_version": selected, "updated_at": event["updated_at"],
		"core_claim": event["core_claim_text"], "summary": map[string]any{
			"facts": []any{}, "changes": []any{}, "unknowns": []any{}, "relations": s.eventSubjects(ctx, scope.NamespaceID, id),
		},
		"first_sources": []any{}, "current_snapshot_id": snapshot["id"],
		"calc_trace": jsonValue(snapshot["calc_trace"]), "redirect_event_ids": jsonValue(event["redirect_event_ids"]),
		"selected_version": selected, "latest_version": latest, "is_historical": selected != latest,
		"current_values": jsonValue(snapshot["current_values"]), "coverage": jsonValue(snapshot["coverage"]),
		"change_id": change["id"], "namespace_label": scope.NamespaceLabel,
	}
	return out, nil
}

func (s *Service) listRows(ctx context.Context, sql string, args ...any) (map[string]any, error) {
	rows := make([]map[string]any, 0)
	if err := s.DB.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		for _, key := range []string{"value_json", "locator", "evidence_ids", "values", "diff", "root_links", "quotes"} {
			if v, ok := row[key]; ok {
				row[key] = jsonValue(v)
			}
		}
	}
	items := make([]any, len(rows))
	for i := range rows {
		items[i] = rows[i]
	}
	return map[string]any{"items": items, "next_cursor": ""}, nil
}

func (s *Service) Timeline(ctx context.Context, scope Scope, id string, version, limit int) (map[string]any, error) {
	sql := `SELECT id AS node_id, CASE WHEN source_revision_id IS NULL THEN 'update' ELSE 'source' END AS kind,
occurred_at, disclosed_at, ingested_at AS recorded_at, source_revision_id, title, excerpt, backfilled
FROM finance_intel_timeline_nodes WHERE namespace_id=? AND event_id=(SELECT id FROM finance_intel_events WHERE namespace_id=? AND logical_id=?)`
	args := []any{scope.NamespaceID, scope.NamespaceID, id}
	if version > 0 {
		sql += ` AND event_version<=?`
		args = append(args, version)
	}
	sql += ` ORDER BY COALESCE(disclosed_at, ingested_at) DESC, id DESC LIMIT ?`
	args = append(args, normalizeLimit(limit))
	return s.listRows(ctx, sql, args...)
}

func (s *Service) Evidence(ctx context.Context, scope Scope, id string, version int, claimKey, grade string, includeInactive bool, limit int) (map[string]any, error) {
	sql := `SELECT e.logical_id AS evidence_id,e.event_id,r.logical_id AS source_revision_id,e.claim_key,e.quote,e.subject,e.predicate,e.object,e.period,e.currency,e.basis,
e.modality,e.direction AS stance,e.grade,e.source_weight AS base_weight,e.effective_weight AS weight,
%s AS active,%s AS status,e.value_json::text AS value_json,e.locator::text AS locator,e.source_cluster,e.extraction_run_id
FROM finance_intel_evidence e
JOIN finance_intel_source_revisions r ON r.id=e.source_revision_id
JOIN finance_intel_evidence_memberships m ON m.namespace_id=e.namespace_id AND m.evidence_id=e.id
WHERE e.namespace_id=? AND e.event_id=(SELECT id FROM finance_intel_events WHERE namespace_id=? AND logical_id=?)`
	args := []any{scope.NamespaceID, scope.NamespaceID, id}
	if version > 0 {
		sql = fmt.Sprintf(sql,
			"(m.from_version <= ? AND (m.to_version IS NULL OR m.to_version >= ?))",
			"CASE WHEN (m.from_version <= ? AND (m.to_version IS NULL OR m.to_version >= ?)) THEN 'active' WHEN m.validity='retracted' THEN 'retracted' WHEN m.validity='isolated' THEN 'quarantined' ELSE 'superseded' END")
		args = append([]any{version, version, version, version}, args...)
	} else {
		sql = fmt.Sprintf(sql, "e.active", "e.membership_status")
	}
	if claimKey != "" {
		sql += ` AND e.claim_key=?`
		args = append(args, claimKey)
	}
	if grade != "" {
		sql += ` AND e.grade=?`
		args = append(args, grade)
	}
	if !includeInactive {
		if version > 0 {
			sql += ` AND (m.from_version <= ? AND (m.to_version IS NULL OR m.to_version >= ?))`
			args = append(args, version, version)
		} else {
			sql += ` AND e.active=true`
		}
	}
	if version > 0 {
		sql += ` AND m.from_version <= ?`
		args = append(args, version)
	}
	sql += ` ORDER BY e.created_at,e.id LIMIT ?`
	args = append(args, normalizeLimit(limit))
	return s.listRows(ctx, sql, args...)
}

func (s *Service) Conflicts(ctx context.Context, scope Scope, id string, version int, status string, limit int) (map[string]any, error) {
	sql := `SELECT id,event_id,claim_key,evidence_ids::text AS evidence_ids,values::text AS values,
CASE WHEN resolved_from_version IS NOT NULL THEN 'resolved' ELSE 'open' END AS status,
resolution_revision_id AS resolved_by_revision_id,resolved_at,resolved_from_version
FROM finance_intel_conflicts WHERE namespace_id=? AND event_id=(SELECT id FROM finance_intel_events WHERE namespace_id=? AND logical_id=?)`
	args := []any{scope.NamespaceID, scope.NamespaceID, id}
	if version > 0 {
		sql += ` AND opened_from_version<=?`
		args = append(args, version)
	}
	if status != "" {
		if status == "resolved" {
			sql += ` AND resolved_from_version IS NOT NULL`
			if version > 0 {
				sql += ` AND resolved_from_version<=?`
				args = append(args, version)
			}
		} else if version > 0 {
			sql += ` AND (resolved_from_version IS NULL OR resolved_from_version>?)`
			args = append(args, version)
		} else {
			sql += ` AND resolved_from_version IS NULL`
		}
	}
	sql += ` ORDER BY opened_at DESC,id DESC LIMIT ?`
	args = append(args, normalizeLimit(limit))
	out, err := s.listRows(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	if version > 0 {
		items, _ := out["items"].([]any)
		for _, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			resolvedFrom := toInt(item["resolved_from_version"])
			if resolvedFrom > 0 && resolvedFrom <= version {
				item["status"] = "resolved"
			} else {
				item["status"] = "open"
			}
			delete(item, "resolved_from_version")
		}
	}
	return out, nil
}

func (s *Service) Changes(ctx context.Context, scope Scope, id string, version, limit int) (map[string]any, error) {
	sql := `SELECT logical_id AS change_id,event_id,event_version,kind,summary,before_snapshot_id,after_snapshot_id,diff::text AS diff,source_revision_id,created_at AS recorded_at
FROM finance_intel_changes WHERE namespace_id=? AND event_id=(SELECT id FROM finance_intel_events WHERE namespace_id=? AND logical_id=?)`
	args := []any{scope.NamespaceID, scope.NamespaceID, id}
	if version > 0 {
		sql += ` AND event_version<=?`
		args = append(args, version)
	}
	sql += ` ORDER BY event_version DESC LIMIT ?`
	args = append(args, normalizeLimit(limit))
	return s.listRows(ctx, sql, args...)
}

func (s *Service) SourceRevision(ctx context.Context, scope Scope, id string) (map[string]any, error) {
	var row map[string]any
	err := s.DB.WithContext(ctx).Raw(`SELECT r.logical_id AS revision_id,r.source_id,s.provider,s.publisher,s.normalized_url AS url,
r.title,r.content_hash,r.allowed_excerpt AS text,r.locator::text AS locator,r.disclosed_at,r.source_updated_at,r.fetched_at,
r.access_status AS origin_status,r.is_repost,'summary' AS rights, '[]'::text AS quotes, '[]'::text AS root_links
FROM finance_intel_source_revisions r JOIN finance_intel_sources s ON s.id=r.source_id
WHERE r.namespace_id=? AND r.logical_id=?`, scope.NamespaceID, id).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if len(row) == 0 {
		return nil, newError(404, "NOT_FOUND", "来源修订不存在")
	}
	for _, key := range []string{"locator", "quotes", "root_links"} {
		row[key] = jsonValue(row[key])
	}
	return row, nil
}

func (s *Service) ListNotifications(ctx context.Context, scope Scope, q NotificationQuery) (ListNotificationsResult, error) {
	sql := `SELECT n.id,c.logical_id AS change_id,e.logical_id AS event_id,n.target_codes::text AS target_codes,n.kind,n.title,n.reason,n.before::text AS before,n.after::text AS after,n.status,n.created_at,n.read_at,n.link_version
FROM finance_intel_notifications n
JOIN finance_intel_events e ON e.id=n.event_id
JOIN finance_intel_changes c ON c.id=n.change_id
WHERE n.namespace_id=? AND n.principal_key=?`
	args := []any{scope.NamespaceID, scope.PrincipalKey}
	if q.Status != "" {
		sql += ` AND n.status=?`
		args = append(args, q.Status)
	}
	sql += ` ORDER BY n.created_at DESC,n.id DESC LIMIT ?`
	args = append(args, normalizeLimit(q.Limit))
	rows := make([]map[string]any, 0)
	if err := s.DB.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return ListNotificationsResult{}, err
	}
	items := make([]any, len(rows))
	for i := range rows {
		for _, key := range []string{"target_codes", "before", "after"} {
			rows[i][key] = jsonValue(rows[i][key])
		}
		items[i] = rows[i]
	}
	var unread int
	_ = s.DB.WithContext(ctx).Raw(`SELECT count(*) FROM finance_intel_notifications WHERE namespace_id=? AND principal_key=? AND status='unread'`, scope.NamespaceID, scope.PrincipalKey).Scan(&unread)
	return ListNotificationsResult{Items: items, UnreadCount: unread}, nil
}

func (s *Service) DataStatus(ctx context.Context, scope Scope) (map[string]any, error) {
	var pending int
	_ = s.DB.WithContext(ctx).Raw(`SELECT count(*) FROM finance_intel_jobs WHERE namespace_id=? AND status IN ('queued','running')`, scope.NamespaceID).Scan(&pending)
	configs := providerConfigs()
	names := make([]string, 0, len(configs))
	for name := range configs {
		names = append(names, name)
	}
	sort.Strings(names)
	providers := make([]any, 0, len(names))
	for _, name := range names {
		config := configs[name]
		status := "enabled"
		reason := ""
		if !config.Enabled {
			status = "disabled"
			reason = config.Reason
		}
		item := map[string]any{"provider": config.Name, "status": status, "rights": "configured-scope-only"}
		if reason != "" {
			item["reason"] = reason
		}
		providers = append(providers, item)
	}
	coverage := map[string]any{"status": "complete", "scope": "events-v1", "last_success_at": s.now(), "pending_count": pending, "gaps": []any{}}
	if scope.Mode == "live" {
		coverage["status"] = "limited"
		coverage["scope"] = "configured-providers"
	}
	return map[string]any{"providers": providers, "coverage": coverage}, nil
}
