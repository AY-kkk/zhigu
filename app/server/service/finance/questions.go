package finance

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	modelfinance "zhigu/server/model/finance"
)

type QuestionOutput struct {
	Answer      string   `json:"answer"`
	ClaimType   string   `json:"claim_type"`
	EvidenceIDs []string `json:"evidence_ids"`
	Limitations []string `json:"limitations"`
	Mode        string   `json:"mode"`
}

func (s *ResearchService) Ask(ctx context.Context, runID, idempotencyKey, text string) (QuestionOutput, error) {
	owner := UserIDFrom(ctx)
	if strings.TrimSpace(idempotencyKey) == "" {
		return QuestionOutput{}, NewError(400, "validation", "MISSING_IDEMPOTENCY_KEY", "缺少 Idempotency-Key")
	}
	view, err := s.GetResearch(ctx, runID)
	if err != nil {
		return QuestionOutput{}, err
	}
	if view.Report == nil {
		return QuestionOutput{}, NewError(409, "conflict", "NO_REPORT", "仅已发布报告可追问")
	}
	hash, _ := HashCanonical(map[string]string{"text": text})
	var existing modelfinance.Question
	if err := s.DB.Where("owner_id = ? AND run_id = ? AND idempotency_key = ?", owner, runID, idempotencyKey).Take(&existing).Error; err == nil {
		var out QuestionOutput
		_ = json.Unmarshal(existing.Response, &out)
		return out, nil
	}
	var count int64
	s.DB.Model(&modelfinance.Question{}).Where("run_id = ? AND owner_id = ?", runID, owner).Count(&count)
	if count >= 3 {
		return QuestionOutput{}, NewError(429, "budget", "QUESTION_LIMIT", "每份报告最多 3 轮追问")
	}
	needsNew := strings.Contains(text, "新") && (strings.Contains(text, "行情") || strings.Contains(text, "最新") || strings.Contains(text, "检索"))
	out := QuestionOutput{
		Answer:      "未进行新一轮检索。回答仅基于本报告已引用的证据。",
		ClaimType:   "assumption",
		EvidenceIDs: view.Report.EvidenceIDs,
		Limitations: []string{"追问不调用外部工具。"},
		Mode:        view.Mode,
	}
	if needsNew || len(view.Report.EvidenceIDs) == 0 {
		out.Answer = "现有证据不足以回答该问题。请使用重新研究，而不是追问。"
		out.Limitations = append(out.Limitations, "需要新资料时请发起重新研究。")
	}
	now := time.Now().UTC()
	raw, _ := json.Marshal(out)
	row := modelfinance.Question{
		ID: "q_" + uuid.NewString(), RunID: runID, OwnerID: owner, IdempotencyKey: idempotencyKey,
		RequestHash: hash, Body: text, Response: datatypes.JSON(raw), Status: "answered", CreatedAt: now, UpdatedAt: now,
	}
	if err := s.DB.Create(&row).Error; err != nil {
		return QuestionOutput{}, err
	}
	return out, nil
}

type GrantIn struct {
	RequestID string `json:"request_id"`
	ToolName  string `json:"tool_name"`
	ArgsHash  string `json:"args_hash"`
	RunID     string `json:"run_id"`
	TaskID    string `json:"task_id"`
}

func (s *ResearchService) CreateGrant(ctx context.Context, runID, taskID string, in GrantIn) (modelfinance.ToolGrant, error) {
	if runID == "" {
		runID = in.RunID
	}
	if taskID == "" {
		taskID = in.TaskID
	}
	if tok := TaskTokenFrom(ctx); tok != "" {
		if err := s.AuthorizeTaskToken(ctx, tok, runID, taskID, "research"); err != nil {
			return modelfinance.ToolGrant{}, err
		}
	}
	var grant modelfinance.ToolGrant
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing modelfinance.ToolGrant
		if err := tx.Where("request_id = ?", in.RequestID).Take(&existing).Error; err == nil {
			grant = existing
			return nil
		}
		var run modelfinance.ResearchRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", runID).Take(&run).Error; err != nil {
			return NewError(404, "not_found", "RUN_NOT_FOUND", "研究不存在")
		}
		if !runAllowsToolsAt(run, s.Clock.Now()) {
			return NewError(409, "conflict", "RUN_CLOSED", "研究已结束，拒绝新工具请求")
		}
		var task modelfinance.ResearchTask
		if err := tx.Where("id = ? AND run_id = ?", taskID, runID).Take(&task).Error; err != nil {
			return NewError(403, "forbidden", "TASK_RUN_MISMATCH", "任务不属于该研究")
		}
		if strings.TrimSpace(in.ArgsHash) == "" {
			return NewError(400, "validation", "MISSING_ARGS_HASH", "缺少参数哈希")
		}
		if _, err := s.Budget.Reserve(ctx, BudgetRequest{RequestID: in.RequestID, RunID: runID, TaskID: taskID, Kind: "tool", Purpose: "research", BodyHash: in.ArgsHash, ReservedInput: 0, ReservedOutput: 0, OwnerID: run.OwnerID}); err != nil {
			return err
		}
		now := time.Now().UTC()
		grant = modelfinance.ToolGrant{
			ID: "grant_" + uuid.NewString(), RequestID: in.RequestID, RunID: runID, TaskID: taskID,
			ToolName: in.ToolName, ArgsHash: in.ArgsHash, Status: "granted", ExpiresAt: now.Add(15 * time.Second),
			ParamsHash: in.ArgsHash, CreatedAt: now, UpdatedAt: now,
		}
		return tx.Create(&grant).Error
	})
	return grant, err
}

func (s *ResearchService) DataQuery(ctx context.Context, grantID, operation string, params map[string]any) (DataQueryResult, error) {
	if tok := TaskTokenFrom(ctx); tok != "" {
		var g modelfinance.ToolGrant
		if err := s.DB.Where("id = ?", grantID).Take(&g).Error; err != nil {
			return DataQueryResult{}, NewError(404, "not_found", "GRANT_NOT_FOUND", "授权不存在")
		}
		if err := s.AuthorizeTaskToken(ctx, tok, g.RunID, g.TaskID, "research"); err != nil {
			return DataQueryResult{}, err
		}
	}
	var out DataQueryResult
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		grant, err := s.consumeGrant(tx, grantID, operation, params)
		if err != nil {
			return err
		}
		var run modelfinance.ResearchRun
		if err := tx.Where("id = ?", grant.RunID).Take(&run).Error; err != nil {
			return err
		}
		snap := RunSnapshot{ID: run.ID, InstrumentID: run.InstrumentID, AsOf: run.AsOf, Mode: run.Mode}
		var recs []ProviderRecord
		switch operation {
		case "get_financials":
			metrics := stringList(params["metrics"])
			periods := stringList(params["periods"])
			if run.Mode == ModeLive {
				live := s.Live
				if live == nil {
					live = NewLiveSource()
				}
				recs, _, err = live.Financials(ctx, snap, metrics, periods)
			} else {
				recs, err = fixtureFinancialRecords(run)
			}
		case "search_filings":
			query, _ := params["query"].(string)
			limit := 5
			switch v := params["limit"].(type) {
			case float64:
				limit = int(v)
			case int:
				limit = v
			}
			if run.Mode == ModeLive {
				live := s.Live
				if live == nil {
					live = NewLiveSource()
				}
				recs, _, err = live.Filings(ctx, snap, query, limit)
			} else {
				recs, err = fixtureFilingRecords(run)
			}
		default:
			return NewError(400, "validation", "INVALID_OPERATION", "operation 仅支持 get_financials 或 search_filings")
		}
		if err != nil {
			return err
		}
		issued := make([]IssuedRecord, 0, len(recs))
		now := time.Now().UTC()
		for _, rec := range recs {
			rec.Text = NormalizeText(rec.Text)
			h, err := rec.Hash()
			if err != nil {
				return err
			}
			raw, _ := json.Marshal(rec.CanonicalMap())
			row := modelfinance.ProviderRecordRow{ID: "prec_" + uuid.NewString(), GrantID: grantID, RecordHash: h, Payload: datatypes.JSON(raw), CreatedAt: now}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "grant_id"}, {Name: "record_hash"}}, DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
			var stored modelfinance.ProviderRecordRow
			if err := tx.Where("grant_id = ? AND record_hash = ?", grantID, h).Take(&stored).Error; err != nil {
				return err
			}
			issued = append(issued, IssuedRecord{RecordID: stored.ID, RecordHash: h, Record: rec})
		}
		_ = tx.Model(&grant).Update("status", "succeeded")
		out = DataQueryResult{Records: issued, QualityStatus: "verified", Warnings: []string{}}
		if run.Mode == ModeFixture {
			out.Warnings = []string{"fixture"}
			if operation == "search_filings" {
				out.QualityStatus = "insufficient"
			}
		}
		return nil
	})
	return out, err
}

func stringList(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func fixtureFinancialRecords(run modelfinance.ResearchRun) ([]ProviderRecord, error) {
	return []ProviderRecord{evidenceMapToRecord(run.InstrumentID, FixtureFinancials(), ModeFixture)}, nil
}

func fixtureFilingRecords(run modelfinance.ResearchRun) ([]ProviderRecord, error) {
	return []ProviderRecord{evidenceMapToRecord(run.InstrumentID, FixtureFilings(), ModeFixture)}, nil
}

func evidenceMapToRecord(instrumentID string, payload map[string]any, mode string) ProviderRecord {
	raw, _ := json.Marshal(payload)
	var ev EvidenceIn
	_ = json.Unmarshal(raw, &ev)
	unit := "CNY_million"
	if len(ev.Metrics) > 0 {
		unit = ev.Metrics[0].Unit
	}
	return ProviderRecord{
		InstrumentID: instrumentID, SourceID: ev.SourceID, SourceURL: ev.SourceURL, SourceKind: ev.SourceKind,
		Title: ev.Title, Locator: ev.Locator, Text: NormalizeText(ev.Text), Metrics: ev.Metrics,
		PublishedAt: ev.PublishedAt, AvailableAt: ev.AvailableAt, RetrievedAt: ev.RetrievedAt,
		DataVersion: ev.DataVersion, Mode: mode, Basis: defaultBasis("", unit),
	}
}

func (s *ResearchService) consumeGrant(tx *gorm.DB, grantID, toolName string, params any) (modelfinance.ToolGrant, error) {
	var grant modelfinance.ToolGrant
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", grantID).Take(&grant).Error; err != nil {
		return grant, NewError(404, "not_found", "GRANT_NOT_FOUND", "授权不存在")
	}
	if grant.Status != "granted" {
		return grant, NewError(409, "conflict", "GRANT_REVOKED", "授权已失效")
	}
	if time.Now().UTC().After(grant.ExpiresAt) {
		return grant, NewError(409, "conflict", "GRANT_EXPIRED", "授权已过期")
	}
	if grant.Executed {
		return grant, NewError(409, "conflict", "GRANT_CONSUMED", "同一 grant 只能执行一次真实查询")
	}
	if grant.ToolName != toolName {
		return grant, NewError(400, "validation", "TOOL_MISMATCH", "授权工具与操作不一致")
	}
	var run modelfinance.ResearchRun
	if err := tx.Where("id = ?", grant.RunID).Take(&run).Error; err != nil {
		return grant, NewError(404, "not_found", "RUN_NOT_FOUND", "研究不存在")
	}
	if !runAllowsToolsAt(run, s.Clock.Now()) {
		return grant, NewError(409, "conflict", "RUN_CLOSED", "研究已结束")
	}
	h, err := HashCanonical(params)
	if err != nil {
		return grant, NewError(400, "validation", "ARGS_HASH_MISMATCH", "参数无法规范化")
	}
	if grant.ArgsHash == "" || (h != grant.ArgsHash && h != grant.ParamsHash) {
		return grant, NewError(400, "validation", "ARGS_HASH_MISMATCH", "参数与授权不一致")
	}
	res := tx.Model(&grant).Where("id = ? AND executed = ?", grant.ID, false).Updates(map[string]any{"executed": true, "updated_at": time.Now().UTC()})
	if res.RowsAffected == 0 {
		return grant, NewError(409, "conflict", "GRANT_CONSUMED", "同一 grant 只能执行一次真实查询")
	}
	grant.Executed = true
	return grant, nil
}
