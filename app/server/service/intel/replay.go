package intel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type fixtureSource struct {
	LogicalID    string    `json:"source_revision_id"`
	SourceID     string    `json:"source_id"`
	Provider     string    `json:"provider"`
	Publisher    string    `json:"publisher"`
	DocumentID   string    `json:"document_id"`
	URL          string    `json:"url"`
	Title        string    `json:"title"`
	Rights       string    `json:"rights"`
	DisclosedAt  time.Time `json:"disclosed_at"`
	AccessStatus string    `json:"access_status"`
	IsRepost     bool      `json:"is_repost"`
	RootLinks    []string  `json:"root_links"`
}

func loadFixtureSources(dir string) (map[string]fixtureSource, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "sources.json"))
	if err != nil {
		return nil, err
	}
	var doc struct {
		Items []fixtureSource `json:"items"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := make(map[string]fixtureSource, len(doc.Items))
	for _, item := range doc.Items {
		out[item.LogicalID] = item
	}
	return out, nil
}

func canonicalJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func sha256Text(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func mustJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func logicalEvidenceID(tx *gorm.DB, namespaceID, logicalID string) string {
	var id string
	_ = tx.Raw(`SELECT id FROM finance_intel_evidence WHERE namespace_id=? AND logical_id=?`, namespaceID, logicalID).Scan(&id).Error
	return id
}

func currentMuted(tx *gorm.DB, namespaceID, principalKey, eventID string) bool {
	var muted bool
	_ = tx.Raw(`SELECT muted FROM finance_intel_mutes WHERE namespace_id=? AND principal_key=? AND event_id=?`, namespaceID, principalKey, eventID).Scan(&muted).Error
	return muted
}

func eligibleCodes(tx *gorm.DB, namespaceID, principalKey, eventID string) []string {
	rows := make([]string, 0)
	_ = tx.Raw(`SELECT b.code FROM finance_intel_event_bindings b
JOIN finance_intel_watchlist w ON w.namespace_id=b.namespace_id AND w.code=b.code AND w.principal_key=?
WHERE b.namespace_id=? AND b.event_id=? ORDER BY b.code`, principalKey, namespaceID, eventID).Scan(&rows).Error
	return rows
}

func (s *Service) ReplayAction(ctx context.Context, scope Scope, req ReplayRequest) (ReplayResult, error) {
	if scope.Mode != "demo" {
		return ReplayResult{}, newError(403, "REPLAY_FORBIDDEN", "真实数据空间禁止回放")
	}
	if req.Action != "step" && req.Action != "reset" {
		return ReplayResult{}, newError(400, "INVALID_PARAM", "action 只能是 step 或 reset")
	}
	if req.Branch != "" && req.Branch != "main" && req.Branch != "denial" {
		return ReplayResult{}, newError(400, "INVALID_PARAM", "branch 只能是 main 或 denial")
	}
	var output ReplayResult
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row map[string]any
		if err := tx.Raw(`SELECT * FROM finance_intel_namespaces WHERE id=? FOR UPDATE`, scope.NamespaceID).Scan(&row).Error; err != nil {
			return err
		}
		if len(row) == 0 {
			return newError(404, "NOT_FOUND", "演示空间不存在")
		}
		current := scanScope(row)
		if req.ExpectedVersion != current.ReplayVersion {
			return newError(409, "VERSION_CONFLICT", "回放版本已变化")
		}
		if req.Action == "reset" {
			branch := req.Branch
			if branch == "" {
				branch = "main"
			}
			for _, table := range []string{
				"finance_intel_notifications", "finance_intel_outbox", "finance_intel_mutes",
				"finance_intel_conflicts", "finance_intel_timeline_nodes", "finance_intel_changes",
				"finance_intel_evidence_memberships", "finance_intel_evidence", "finance_intel_snapshots",
				"finance_intel_event_bindings", "finance_intel_events", "finance_intel_extraction_runs",
				"finance_intel_source_revisions", "finance_intel_sources", "finance_intel_watchlist",
				"finance_intel_jobs", "finance_intel_review_items", "finance_intel_idempotency",
				"finance_intel_usage", "finance_intel_audit",
			} {
				if err := execSQL(tx, `DELETE FROM `+table+` WHERE namespace_id=?`, scope.NamespaceID); err != nil {
					return err
				}
			}
			if err := execSQL(tx, `UPDATE finance_intel_namespaces
SET generation=generation+1,replay_version=replay_version+1,branch=?,step_index=0,simulated_at=NULL,updated_at=?
WHERE id=? AND generation=?`, branch, s.now(), scope.NamespaceID, current.Generation); err != nil {
				return err
			}
			output = ReplayResult{
				ReplayVersion: current.ReplayVersion + 1, Generation: current.Generation + 1,
				StepIndex: 0, Changes: []string{}, Done: false,
			}
			return nil
		}

		branch := current.Branch
		if req.Branch != "" {
			branch = req.Branch
		}
		steps, _, err := loadFixtureSteps(s.FixtureDir, branch)
		if err != nil {
			return err
		}
		results, err := RunFixture(s.FixtureDir, branch)
		if err != nil {
			return err
		}
		if current.StepIndex >= len(steps) {
			output = ReplayResult{
				ReplayVersion: current.ReplayVersion, Generation: current.Generation,
				StepIndex: current.StepIndex, SimulatedAt: current.SimulatedAt,
				EventVersion:      results[len(results)-1].EventVersion,
				NotificationCount: results[len(results)-1].NotificationCount,
				Changes:           []string{}, Done: true,
			}
			return nil
		}
		index := current.StepIndex
		step := steps[index]
		result := results[index]
		sources, err := loadFixtureSources(s.FixtureDir)
		if err != nil {
			return err
		}

		eventInternalID := ""
		var currentVersion int
		var eventRow map[string]any
		if err := tx.Raw(`SELECT id,current_version FROM finance_intel_events WHERE namespace_id=? AND logical_id=?`, scope.NamespaceID, "evt_demo_acq").Scan(&eventRow).Error; err != nil {
			return err
		}
		if len(eventRow) > 0 {
			eventInternalID, _ = eventRow["id"].(string)
			switch v := eventRow["current_version"].(type) {
			case int64:
				currentVersion = int(v)
			case int32:
				currentVersion = int(v)
			case int:
				currentVersion = v
			}
		}
		if eventInternalID == "" {
			eventInternalID = "evt_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			if err := execSQL(tx, `INSERT INTO finance_intel_events
(id,logical_id,namespace_id,event_type,subject_code,subject_name,matter_key,title,core_claim_key,core_claim_text,current_version,current_snapshot_id,latest_valid_disclosed_at,review_status,redirect_event_ids,created_at,updated_at)
VALUES (?, ?, ?, 'acquisition','DEMO.A','演示公司A','deal_demo_ab','DEMO.A 收购 DEMO.B','__core__','DEMO.A 存在收购 DEMO.B 的该项交易安排',0,NULL,NULL,'normal','[]'::jsonb,?,?)`,
				eventInternalID, "evt_demo_acq", scope.NamespaceID, step.SimulatedAt, step.SimulatedAt); err != nil {
				return err
			}
			for _, binding := range []struct{ code, role string }{{"DEMO.A", "subject"}, {"DEMO.B", "counterparty"}} {
				if err := execSQL(tx, `INSERT INTO finance_intel_event_bindings(namespace_id,event_id,code,role,evidence_ids)
VALUES (?,?,?,?,'[]'::jsonb) ON CONFLICT DO NOTHING`, scope.NamespaceID, eventInternalID, binding.code, binding.role); err != nil {
					return err
				}
			}
		}

		var revisionInternalID string
		if !step.Simulation && step.SourceRevisionID != "" {
			src := sources[step.SourceRevisionID]
			var sourceInternalID string
			var sourceRow map[string]any
			if err := tx.Raw(`SELECT id FROM finance_intel_sources WHERE namespace_id=? AND publisher=? AND document_id=?`, scope.NamespaceID, src.Publisher, src.DocumentID).Scan(&sourceRow).Error; err != nil {
				return err
			}
			if len(sourceRow) > 0 {
				sourceInternalID, _ = sourceRow["id"].(string)
			} else {
				sourceInternalID = "src_" + strings.ReplaceAll(uuid.NewString(), "-", "")
				if err := execSQL(tx, `INSERT INTO finance_intel_sources(id,namespace_id,publisher,document_id,normalized_url,rights,created_at)
VALUES (?,?,?,?,?,?,?)`, sourceInternalID, scope.NamespaceID, src.Publisher, src.DocumentID, src.URL, src.Rights, s.now()); err != nil {
					return err
				}
			}
			revisionInternalID = "rev_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			locator := map[string]any{"start": 0, "end": len([]rune(step.Text)), "type": "unicode"}
			if err := execSQL(tx, `INSERT INTO finance_intel_source_revisions
(id,logical_id,namespace_id,source_id,revision_no,content_hash,title,allowed_excerpt,locator,disclosed_at,disclosed_date,fetched_at,source_updated_at,access_status,is_repost,supersedes_revision_id,created_at)
VALUES (?,?,?,?,1,?,?,?,?,?,?,?,?,?,?,NULL,?)`,
				revisionInternalID, src.LogicalID, scope.NamespaceID, sourceInternalID,
				sha256Text(step.Text), src.Title, step.Text, mustJSON(locator), src.DisclosedAt,
				src.DisclosedAt.Format("2006-01-02"), step.SimulatedAt, src.DisclosedAt,
				src.AccessStatus, src.IsRepost, s.now()); err != nil {
				return err
			}
		}

		evidenceFromVersion := result.EventVersion
		if evidenceFromVersion <= 0 {
			evidenceFromVersion = currentVersion
		}
		if evidenceFromVersion <= 0 {
			evidenceFromVersion = 1
		}
		for _, raw := range step.Evidence {
			e := toEvidenceRule(raw)
			for _, oldLogical := range e.Supersedes {
				oldID := logicalEvidenceID(tx, scope.NamespaceID, oldLogical)
				if oldID != "" {
					if err := execSQL(tx, `UPDATE finance_intel_evidence SET active=false,membership_status='superseded' WHERE namespace_id=? AND id=?`, scope.NamespaceID, oldID); err != nil {
						return err
					}
					if err := execSQL(tx, `UPDATE finance_intel_evidence_memberships SET to_version=?,validity='superseded'
WHERE namespace_id=? AND evidence_id=? AND to_version IS NULL`, evidenceFromVersion-1, scope.NamespaceID, oldID); err != nil {
						return err
					}
				}
			}
			if e.Authoritative {
				if err := execSQL(tx, `UPDATE finance_intel_evidence SET active=false,membership_status='superseded'
WHERE namespace_id=? AND event_id=? AND claim_key=? AND authoritative=false`, scope.NamespaceID, eventInternalID, e.ClaimKey); err != nil {
					return err
				}
				if err := execSQL(tx, `UPDATE finance_intel_evidence_memberships m SET to_version=?,validity='superseded'
FROM finance_intel_evidence e
WHERE m.namespace_id=e.namespace_id AND m.evidence_id=e.id AND m.namespace_id=? AND e.event_id=? AND e.claim_key=? AND e.authoritative=false AND m.to_version IS NULL`,
					evidenceFromVersion-1, scope.NamespaceID, eventInternalID, e.ClaimKey); err != nil {
					return err
				}
			}
			evidenceID := "evid_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			weight := effectiveWeight(e)
			valueJSON := map[string]any{
				"value": e.Value, "fixture_id": e.ID, "authoritative": e.Authoritative,
				"direct": e.Direct, "subject_unique": e.SubjectUnique, "modality_explicit": e.ModalityExplicit,
				"supersedes": e.Supersedes,
			}
			modality := "reported"
			if e.Grade == GradeFact {
				modality = "actual"
			}
			if err := execSQL(tx, `INSERT INTO finance_intel_evidence
(id,logical_id,namespace_id,event_id,source_revision_id,claim_key,subject,predicate,object,period,currency,basis,modality,grade,authoritative,subject_unique,modality_explicit,source_weight,effective_weight,direction,value_json,quote,locator,source_cluster,extraction_run_id,active,membership_status,supersedes_evidence_id,created_at)
VALUES (?,?,?,?,?,?,?,?,?,'','','CNY',?,?,?,?,?,?,?,?,?,?,'{}'::jsonb,?,NULL,true,'current',NULL,?)`,
				evidenceID, e.ID, scope.NamespaceID, eventInternalID, revisionInternalID,
				e.ClaimKey, "DEMO.A", e.Value, "DEMO.B", modality, string(e.Grade),
				e.Authoritative, e.SubjectUnique, e.ModalityExplicit, e.SourceWeight.String(), weight.String(),
				string(e.Stance), mustJSON(valueJSON), step.Text, e.SourceCluster, s.now()); err != nil {
				return err
			}
			if err := execSQL(tx, `INSERT INTO finance_intel_evidence_memberships
(namespace_id,event_id,evidence_id,from_version,to_version,validity,supersedes_evidence_id)
VALUES (?,?,?,?,NULL,'current',NULL) ON CONFLICT DO NOTHING`, scope.NamespaceID, eventInternalID, evidenceID, evidenceFromVersion); err != nil {
				return err
			}
		}

		if revisionInternalID != "" {
			nodeID := "node_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			axis := "disclosure"
			if step.Repost {
				axis = "ingestion"
			}
			if err := execSQL(tx, `INSERT INTO finance_intel_timeline_nodes
(id,namespace_id,event_id,source_revision_id,event_version,axis,occurred_at,disclosed_at,ingested_at,title,excerpt,backfilled)
VALUES (?,?,?,?,?,?,?,?,?,?,?,false) ON CONFLICT DO NOTHING`,
				nodeID, scope.NamespaceID, eventInternalID, revisionInternalID, evidenceFromVersion, axis,
				step.SimulatedAt, sources[step.SourceRevisionID].DisclosedAt, step.SimulatedAt,
				sources[step.SourceRevisionID].Title, step.Text); err != nil {
				return err
			}
		} else if step.Simulation {
			nodeID := "node_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			if err := execSQL(tx, `INSERT INTO finance_intel_timeline_nodes
(id,namespace_id,event_id,source_revision_id,event_version,axis,occurred_at,disclosed_at,ingested_at,title,excerpt,backfilled)
VALUES (?,?,?,NULL,?,'ingestion',NULL,NULL,?,'信息新鲜度评估','模拟时钟推进，无新增材料',false)`,
				nodeID, scope.NamespaceID, eventInternalID, result.EventVersion, step.SimulatedAt); err != nil {
				return err
			}
		}

		if result.OpenConflicts > 0 {
			evidenceIDs := []string{}
			for _, e := range step.Evidence {
				if e.ClaimKey == "transaction_amount" {
					evidenceIDs = append(evidenceIDs, e.ID)
				}
			}
			if len(evidenceIDs) < 2 {
				evidenceIDs = []string{"ev_n2_amount", "ev_n3_amount"}
			}
			if err := execSQL(tx, `INSERT INTO finance_intel_conflicts
(id,namespace_id,event_id,claim_key,status,values,evidence_ids,opened_at,opened_from_version,resolved_at,resolved_from_version,resolution_revision_id)
VALUES (?,?,?,'transaction_amount','open',?::jsonb,?::jsonb,?,?,NULL,NULL,NULL)
ON CONFLICT(namespace_id,event_id,claim_key,status) DO UPDATE SET values=EXCLUDED.values,evidence_ids=EXCLUDED.evidence_ids`,
				"cflt_"+strings.ReplaceAll(uuid.NewString(), "-", ""), scope.NamespaceID, eventInternalID,
				`["800000000","1000000000"]`, mustJSON(evidenceIDs), step.SimulatedAt, evidenceFromVersion); err != nil {
				return err
			}
		} else {
			if err := execSQL(tx, `UPDATE finance_intel_conflicts SET status='resolved',resolved_at=?,resolved_from_version=?,resolution_revision_id=?
WHERE namespace_id=? AND event_id=? AND status='open'`, step.SimulatedAt, result.EventVersion, revisionInternalID, scope.NamespaceID, eventInternalID); err != nil {
				return err
			}
		}

		changeIDs := make([]string, 0)
		if result.EventVersion > currentVersion {
			snapshotID := "snap_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			currentValues := make([]map[string]any, 0)
			if result.TransactionAmount != "" {
				for _, value := range strings.Split(result.TransactionAmount, "|") {
					currentValues = append(currentValues, map[string]any{
						"field": "transaction_amount", "value": value, "currency": "CNY",
						"evidence_ids": []string{"ev_n2_amount", "ev_n4_amount"},
					})
				}
			}
			coverage := map[string]any{"status": "complete", "scope": "events-v1", "last_success_at": step.SimulatedAt, "pending_count": 0, "gaps": []any{}}
			calcTrace := map[string]any{
				"rule_version": "rules-v1", "fixture_step": step.ID,
				"verification": result.Verification, "phase": result.Phase,
				"freshness": result.Freshness, "support_level": result.SupportLevel,
				"open_conflicts": result.OpenConflicts,
			}
			if err := execSQL(tx, `INSERT INTO finance_intel_snapshots
(id,namespace_id,event_id,event_version,verification,phase,freshness,support_score,support_level,current_values,conclusion,calc_trace,coverage,as_of,created_at)
VALUES (?,?,?,?,?,?,?,?,?,?::jsonb,?,?::jsonb,?::jsonb,?,?)`,
				snapshotID, scope.NamespaceID, eventInternalID, result.EventVersion,
				result.Verification, result.Phase, result.Freshness, "0", result.SupportLevel,
				mustJSON(currentValues), "已知事实、最新变化、未决事项、标的关系和来源覆盖均按事件详情展示。",
				mustJSON(calcTrace), mustJSON(coverage), step.SimulatedAt, s.now()); err != nil {
				return err
			}
			kind := "update"
			if result.EventVersion == 1 {
				kind = "new_event"
			} else if result.Freshness == FreshnessExpired {
				kind = "expiration"
			}
			changeID := "chg_" + step.ID
			diff := map[string]any{"step": step.ID, "verification": result.Verification, "phase": result.Phase, "freshness": result.Freshness, "transaction_amount": result.TransactionAmount}
			if err := execSQL(tx, `INSERT INTO finance_intel_changes
(id,logical_id,namespace_id,event_id,event_version,kind,summary,before_snapshot_id,after_snapshot_id,diff,source_revision_id,created_at)
VALUES (?,?,?,?,?,?,?,NULL,?,?::jsonb,?,?)`,
				"change_"+strings.ReplaceAll(uuid.NewString(), "-", ""), changeID, scope.NamespaceID, eventInternalID,
				result.EventVersion, kind, "事件情报更新："+step.ID, snapshotID, mustJSON(diff), revisionInternalID, step.SimulatedAt); err != nil {
				return err
			}
			changeIDs = append(changeIDs, changeID)
			if err := execSQL(tx, `UPDATE finance_intel_events SET current_version=?,current_snapshot_id=?,latest_valid_disclosed_at=?,updated_at=? WHERE id=?`,
				result.EventVersion, snapshotID, step.SimulatedAt, step.SimulatedAt, eventInternalID); err != nil {
				return err
			}
			codes := eligibleCodes(tx, scope.NamespaceID, scope.PrincipalKey, eventInternalID)
			if len(codes) > 0 && !currentMuted(tx, scope.NamespaceID, scope.PrincipalKey, eventInternalID) {
				outboxID := "outbox_" + strings.ReplaceAll(uuid.NewString(), "-", "")
				if err := execSQL(tx, `INSERT INTO finance_intel_outbox
(id,namespace_id,change_id,principal_key,target_codes,status,lease_owner,lease_epoch,lease_until,attempts,available_at,created_at)
SELECT ?,?,id,?,?::jsonb,'pending',NULL,0,NULL,0,?,? FROM finance_intel_changes WHERE namespace_id=? AND logical_id=?`,
					outboxID, scope.NamespaceID, scope.PrincipalKey, mustJSON(codes), step.SimulatedAt, s.now(), scope.NamespaceID, changeID); err != nil {
					return err
				}
				notificationID := "ntf_" + strings.ReplaceAll(uuid.NewString(), "-", "")
				if err := execSQL(tx, `INSERT INTO finance_intel_notifications
(id,namespace_id,principal_key,event_id,change_id,outbox_id,title,reason,before,after,target_codes,link_version,status,created_at,updated_at)
SELECT ?,?,?,e.id,c.id,?,'事件情报更新',?,'{}'::jsonb,?::jsonb,?::jsonb,?,'unread',?,?
FROM finance_intel_events e JOIN finance_intel_changes c ON c.event_id=e.id
WHERE e.namespace_id=? AND e.logical_id=? AND c.logical_id=?`,
					notificationID, scope.NamespaceID, scope.PrincipalKey, outboxID,
					"固定回放步骤 "+step.ID, mustJSON(diff), mustJSON(codes), result.EventVersion,
					step.SimulatedAt, s.now(), scope.NamespaceID, "evt_demo_acq", changeID); err != nil {
					return err
				}
			}
		}

		if err := execSQL(tx, `UPDATE finance_intel_namespaces SET branch=?,step_index=step_index+1,replay_version=replay_version+1,simulated_at=?,updated_at=? WHERE id=? AND generation=?`,
			branch, step.SimulatedAt, s.now(), scope.NamespaceID, current.Generation); err != nil {
			return err
		}
		output = ReplayResult{
			ReplayVersion: current.ReplayVersion + 1, Generation: current.Generation,
			StepIndex: index + 1, SimulatedAt: &step.SimulatedAt,
			EventVersion: result.EventVersion, NotificationCount: result.NotificationCount,
			Changes: changeIDs, Done: index+1 >= len(steps),
		}
		return nil
	})
	return output, err
}

func execSQL(tx *gorm.DB, sql string, args ...any) error {
	want := strings.Count(sql, "?")
	if want != len(args) {
		return fmt.Errorf("SQL parameter mismatch: placeholders=%d args=%d sql=%s", want, len(args), sql)
	}
	return tx.Exec(sql, args...).Error
}
