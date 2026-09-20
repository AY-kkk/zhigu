package finance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	modelfinance "zhigu/server/model/finance"
)

const (
	QueueTimeout    = 30 * time.Second
	RunTimeout      = 180 * time.Second
	LeaseTTL        = 15 * time.Second
	LeaseRenew      = 5 * time.Second
	GlobalActiveMax = 2
	MaxHistory      = 50
	DefaultHistory  = 20
)

var (
	ParsePerMinuteMax = 5
	DailyResearchMax  = 10
)

type ResearchService struct {
	DB     *gorm.DB
	Client ResearchClient
	Budget BudgetService
	Clock  Clock
}

func NewService(db *gorm.DB, client ResearchClient, budget BudgetService, _ *FixtureConfig) *ResearchService {
	if mb, ok := budget.(*MemoryBudget); ok {
		mb.Attach(db)
	}
	return &ResearchService{DB: db, Client: client, Budget: budget, Clock: SystemClock{}}
}

func (s *ResearchService) ParseClaim(ctx context.Context, in ParseInput) (ParseOutput, error) {
	owner := UserIDFrom(ctx)
	if owner == 0 {
		return ParseOutput{}, NewError(401, "forbidden", "UNAUTHENTICATED", "未登录")
	}
	text := strings.TrimSpace(in.Text)
	n := utf8.RuneCountInString(text)
	if n < 20 || n > 2000 {
		return ParseOutput{}, NewError(400, "validation", "INVALID_TEXT", "观点须为 20 至 2000 字")
	}
	now := s.Clock.Now()
	var recent int64
	if err := s.DB.WithContext(ctx).Model(&modelfinance.ClaimDraft{}).
		Where("owner_id = ? AND created_at >= ?", owner, now.Add(-time.Minute)).
		Count(&recent).Error; err != nil {
		return ParseOutput{}, err
	}
	if int(recent) >= ParsePerMinuteMax {
		return ParseOutput{}, NewError(429, "budget", "PARSE_RATE_LIMIT", "解析每分钟最多 5 次")
	}
	asOf := DefaultAsOf(now)
	if in.AsOf != nil {
		asOf = in.AsOf.UTC()
	}
	horizon := SuggestedHorizon(asOf)
	var candidates []Instrument
	needs := true
	if strings.Contains(text, "演示公司") {
		candidates = []Instrument{{ID: InstrumentDemo, Symbol: "DEMO:COMPANY", Name: "演示公司"}}
	}
	items := []ClaimItem{{
		ClaimID:   "claim_1",
		Text:      truncateRunes(text, 400),
		ClaimType: "fact",
	}}
	if strings.Contains(text, "股价") {
		items = append(items, ClaimItem{ClaimID: "claim_2", Text: "收入或基本面变化会推动未来股价。", ClaimType: "inference"})
	}
	if len(items) > 6 {
		items = items[:6]
	}
	itemJSON, _ := json.Marshal(items)
	cfg, _ := json.Marshal(map[string]string{
		"model":  "model_fixture_v1",
		"source": "source_fixture_v1",
		"prompt": "prompt_v1",
		"policy": "policy_v1",
		"budget": "budget_v1",
	})
	id := "draft_" + uuid.NewString()
	row := modelfinance.ClaimDraft{
		ID:             id,
		OwnerID:        owner,
		Text:           text,
		Horizon:        &horizon,
		Items:          datatypes.JSON(itemJSON),
		Revision:       1,
		AsOf:           asOf,
		Mode:           ModeFixture,
		ConfigVersions: datatypes.JSON(cfg),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.DB.WithContext(ctx).Create(&row).Error; err != nil {
		return ParseOutput{}, err
	}
	return ParseOutput{
		DraftID:           id,
		Revision:          1,
		Candidates:        candidates,
		SuggestedHorizon:  horizon,
		Items:             items,
		NeedsConfirmation: needs || len(candidates) != 1,
		Mode:              ModeFixture,
	}, nil
}

func (s *ResearchService) CreateResearch(ctx context.Context, idempotencyKey string, in CreateResearchInput) (CreateResearchOutput, error) {
	owner := UserIDFrom(ctx)
	if owner == 0 {
		return CreateResearchOutput{}, NewError(401, "forbidden", "UNAUTHENTICATED", "未登录")
	}
	if strings.TrimSpace(idempotencyKey) == "" {
		return CreateResearchOutput{}, NewError(400, "validation", "MISSING_IDEMPOTENCY_KEY", "缺少 Idempotency-Key")
	}
	if strings.TrimSpace(in.Horizon) == "" || in.InstrumentID == "" {
		return CreateResearchOutput{}, NewError(400, "validation", "SCOPE_REQUIRED", "须明确选择证券和期限")
	}
	if !CoveredInstrument(in.InstrumentID) {
		return CreateResearchOutput{}, NewError(422, "validation", "UNSUPPORTED_INSTRUMENT", "标的不在当前数据源覆盖范围")
	}
	reqHash, err := HashCanonical(in)
	if err != nil {
		return CreateResearchOutput{}, err
	}
	var out CreateResearchOutput
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing modelfinance.ResearchRun
		q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("owner_id = ? AND idempotency_key = ?", owner, idempotencyKey).Take(&existing)
		if q.Error == nil {
			if existing.RequestHash != reqHash {
				return NewError(409, "conflict", "IDEMPOTENCY_CONFLICT", "相同幂等键对应不同请求")
			}
			out = CreateResearchOutput{RunID: existing.ID, Status: existing.Status, PollURL: "/api/finance/research/" + existing.ID}
			return nil
		}
		if !errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return q.Error
		}
		var draft modelfinance.ClaimDraft
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", in.DraftID).Take(&draft).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return NewError(404, "not_found", "DRAFT_NOT_FOUND", "草稿不存在")
			}
			return err
		}
		if draft.OwnerID != owner {
			return NewError(404, "not_found", "DRAFT_NOT_FOUND", "草稿不存在")
		}
		if draft.Revision != in.Revision {
			return NewError(409, "conflict", "DRAFT_REVISION", "草稿版本不匹配")
		}
		if draft.ConfirmedRunID != nil && *draft.ConfirmedRunID != "" {
			return NewError(409, "conflict", "DRAFT_ALREADY_CONFIRMED", "同一草稿仅可确认一次")
		}
		if strings.TrimSpace(in.ClaimText) != "" && strings.TrimSpace(in.ClaimText) != strings.TrimSpace(draft.Text) {
			return NewError(409, "conflict", "DRAFT_TEXT_MISMATCH", "观点已修改，请重新解析后再确认")
		}
		var active int64
		if err := tx.Model(&modelfinance.ResearchRun{}).
			Where("owner_id = ? AND deleted_at IS NULL AND status IN ?", owner, activeRunStatuses).
			Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return NewError(409, "conflict", "ACTIVE_RUN_EXISTS", "同一用户同时只能有一个活动研究")
		}
		var global int64
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("singleton_id = ?", "global").Take(&modelfinance.Scheduler{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&modelfinance.ResearchRun{}).
			Where("deleted_at IS NULL AND status IN ?", activeRunStatuses).
			Count(&global).Error; err != nil {
			return err
		}
		if global >= GlobalActiveMax {
			return NewError(429, "budget", "GLOBAL_CONCURRENCY", "全局活动研究已达上限")
		}
		now := s.Clock.Now()
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		var today int64
		if err := tx.Model(&modelfinance.ResearchRun{}).
			Where("owner_id = ? AND created_at >= ?", owner, dayStart).
			Count(&today).Error; err != nil {
			return err
		}
		if int(today) >= DailyResearchMax {
			return NewError(429, "budget", "DAILY_RESEARCH_LIMIT", "每日最多 10 个研究")
		}
		asOf := in.AsOf.UTC()
		var items []ClaimItem
		_ = json.Unmarshal(draft.Items, &items)
		claim := Claim{Text: draft.Text, Horizon: in.Horizon, Items: items}
		claimJSON, _ := json.Marshal(claim)
		policy, _ := NewConfigService(tx).GetPolicy(ctx)
		budgetSnap := FreezeBudgetSnapshot(policy)
		cfg := map[string]string{
			"model":  "model_fixture_v1",
			"source": "source_fixture_v1",
			"prompt": "prompt_v1",
			"policy": "policy_v1",
			"budget": "budget_v1",
		}
		if policy.MaxToolCalls > 0 {
			cfg["policy"] = fmt.Sprintf("policy_tools_%d", policy.MaxToolCalls)
			cfg["budget"] = fmt.Sprintf("budget_tools_%d", policy.MaxToolCalls)
		}
		cfgJSON, _ := json.Marshal(cfg)
		budgetJSON, _ := json.Marshal(budgetSnap)
		runID := "run_" + uuid.NewString()
		var parent *string
		if in.ParentRunID != "" {
			parent = &in.ParentRunID
		}
		run := modelfinance.ResearchRun{
			ID:             runID,
			OwnerID:        owner,
			DraftID:        draft.ID,
			ParentRunID:    parent,
			ClaimSnapshot:  datatypes.JSON(claimJSON),
			InstrumentID:   in.InstrumentID,
			Horizon:        in.Horizon,
			AsOf:           asOf,
			Mode:           ModeFixture,
			Status:         StatusQueued,
			Stage:          "queued",
			ConfigVersions: datatypes.JSON(cfgJSON),
			BudgetSnapshot: datatypes.JSON(budgetJSON),
			IdempotencyKey: idempotencyKey,
			RequestHash:    reqHash,
			Version:        1,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := tx.Create(&run).Error; err != nil {
			if isUnique(err) {
				var again modelfinance.ResearchRun
				if tx.Where("owner_id = ? AND idempotency_key = ?", owner, idempotencyKey).Take(&again).Error == nil {
					if again.RequestHash != reqHash {
						return NewError(409, "conflict", "IDEMPOTENCY_CONFLICT", "相同幂等键对应不同请求")
					}
					out = CreateResearchOutput{RunID: again.ID, Status: again.Status, PollURL: "/api/finance/research/" + again.ID}
					return nil
				}
				return NewError(409, "conflict", "ACTIVE_RUN_EXISTS", "同一用户同时只能有一个活动研究")
			}
			return err
		}
		for _, role := range []string{RoleSupporter, RoleChallenger} {
			task := modelfinance.ResearchTask{
				ID:          "task_" + role + "_" + uuid.NewString(),
				RunID:       runID,
				Role:        role,
				Status:      "queued",
				PayloadHash: reqHash,
				Attempt:     1,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := tx.Create(&task).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&draft).Updates(map[string]any{"confirmed_run_id": runID, "updated_at": now}).Error; err != nil {
			return err
		}
		out = CreateResearchOutput{RunID: runID, Status: StatusQueued, PollURL: "/api/finance/research/" + runID}
		return nil
	})
	if err != nil {
		return CreateResearchOutput{}, err
	}
	return out, nil
}

func (s *ResearchService) GetResearch(ctx context.Context, runID string) (ResearchView, error) {
	owner := UserIDFrom(ctx)
	var run modelfinance.ResearchRun
	err := s.DB.WithContext(ctx).Where("id = ? AND owner_id = ? AND deleted_at IS NULL", runID, owner).Take(&run).Error
	if err != nil {
		return ResearchView{}, NewError(404, "not_found", "RUN_NOT_FOUND", "研究不存在")
	}
	view := ResearchView{
		RunID:        run.ID,
		Status:       run.Status,
		Stage:        run.Stage,
		Mode:         run.Mode,
		AsOf:         run.AsOf.UTC(),
		InstrumentID: run.InstrumentID,
		Horizon:      run.Horizon,
		Warnings:     []string{},
		UpdatedAt:    run.UpdatedAt.UTC(),
	}
	var claim Claim
	if json.Unmarshal(run.ClaimSnapshot, &claim) == nil {
		view.Claim = &claim
	}
	var report modelfinance.Report
	if err := s.DB.Where("run_id = ? AND published_at IS NOT NULL", run.ID).Order("version desc").Take(&report).Error; err == nil {
		var body VerifiedReport
		if json.Unmarshal(report.Body, &body) == nil {
			view.Report = &body
		}
	}
	if run.Status == StatusFailed {
		view.Error = &APIErrorBody{Code: "RUN_FAILED", Message: "研究失败或未完成"}
	}
	return view, nil
}

func (s *ResearchService) ListResearch(ctx context.Context, cursor string, limit int) (HistoryPage, error) {
	owner := UserIDFrom(ctx)
	if limit <= 0 {
		limit = DefaultHistory
	}
	if limit > MaxHistory {
		limit = MaxHistory
	}
	q := s.DB.WithContext(ctx).Where("owner_id = ? AND deleted_at IS NULL", owner).Order("created_at DESC, id DESC")
	if cursor != "" {
		var cur modelfinance.ResearchRun
		if err := s.DB.Where("id = ? AND owner_id = ?", cursor, owner).Take(&cur).Error; err == nil {
			q = q.Where("(created_at, id) < (?, ?)", cur.CreatedAt, cur.ID)
		}
	}
	var rows []modelfinance.ResearchRun
	if err := q.Limit(limit + 1).Find(&rows).Error; err != nil {
		return HistoryPage{}, err
	}
	page := HistoryPage{Items: []HistoryItem{}}
	for i, r := range rows {
		if i == limit {
			page.NextCursor = rows[limit-1].ID
			break
		}
		page.Items = append(page.Items, HistoryItem{
			RunID: r.ID, Status: r.Status, InstrumentID: r.InstrumentID, Mode: r.Mode, CreatedAt: r.CreatedAt.UTC(),
		})
	}
	return page, nil
}

func (s *ResearchService) CancelResearch(ctx context.Context, runID string) (ResearchView, error) {
	owner := UserIDFrom(ctx)
	var dispatched bool
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run modelfinance.ResearchRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND owner_id = ? AND deleted_at IS NULL", runID, owner).Take(&run).Error; err != nil {
			return NewError(404, "not_found", "RUN_NOT_FOUND", "研究不存在")
		}
		switch run.Status {
		case StatusCompleted, StatusIncomplete, StatusFailed, StatusCanceled:
			return nil
		case StatusCanceling:
			return nil
		}
		now := s.Clock.Now()
		dispatched = run.Status == StatusResearching || run.Status == StatusVerifying || run.LeaseOwner != nil
		status := StatusCanceled
		stage := "canceled"
		updates := map[string]any{"status": status, "stage": stage, "updated_at": now}
		if dispatched {
			status = StatusCanceling
			stage = "canceling"
			updates["status"] = status
			updates["stage"] = stage
		} else {
			updates["lease_owner"] = nil
			updates["lease_until"] = nil
		}
		if err := s.casUpdateRun(tx, run.ID, run.Version, []string{StatusQueued, StatusResearching, StatusVerifying}, updates); err != nil {
			return err
		}
		return tx.Model(&modelfinance.ResearchTask{}).Where("run_id = ?", runID).Updates(map[string]any{"status": "canceled", "updated_at": now}).Error
	})
	if err != nil {
		return ResearchView{}, err
	}
	s.revokeTokens(ctx, runID)
	if dispatched {
		_ = s.finalizeCancel(ctx, runID)
	} else {
		var tasks []modelfinance.ResearchTask
		_ = s.DB.WithContext(ctx).Where("run_id = ?", runID).Find(&tasks)
		for _, task := range tasks {
			_, _ = s.Client.Cancel(ctx, task.ID)
		}
	}
	return s.GetResearch(ctx, runID)
}

func (s *ResearchService) DeleteResearch(ctx context.Context, runID string) (string, error) {
	owner := UserIDFrom(ctx)
	now := s.Clock.Now()
	var wasActive bool
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run modelfinance.ResearchRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND owner_id = ?", runID, owner).Take(&run).Error; err != nil {
			return NewError(404, "not_found", "RUN_NOT_FOUND", "研究不存在")
		}
		if run.DeletedAt != nil {
			return nil
		}
		wasActive = run.Status == StatusQueued || run.Status == StatusResearching || run.Status == StatusVerifying || run.Status == StatusCanceling
		if wasActive {
			return s.casUpdateRun(tx, run.ID, run.Version, []string{StatusQueued, StatusResearching, StatusVerifying, StatusCanceling}, map[string]any{
				"status": StatusCanceling, "stage": "deleting", "updated_at": now,
			})
		}
		return tx.Model(&run).Updates(map[string]any{
			"deleted_at": now, "updated_at": now, "version": run.Version + 1,
			"lease_owner": nil, "lease_until": nil,
		}).Error
	})
	if err != nil {
		return "", err
	}
	s.revokeTokens(ctx, runID)
	var tasks []modelfinance.ResearchTask
	_ = s.DB.WithContext(ctx).Where("run_id = ?", runID).Find(&tasks)
	if wasActive {
		_ = s.finalizeCancel(ctx, runID)
	}
	run, _ := s.loadRun(runID)
	released := run.Status == StatusCanceled || run.DeletedAt != nil
	if released {
		for _, task := range tasks {
			_ = s.purgeExecutorCopy(ctx, task.ID)
		}
	}
	if wasActive && run.Status == StatusCanceled && run.DeletedAt == nil {
		_ = s.DB.WithContext(ctx).Model(&modelfinance.ResearchRun{}).Where("id = ?", runID).
			Updates(map[string]any{"deleted_at": s.Clock.Now(), "updated_at": s.Clock.Now()})
	}
	return "scheduled", nil
}

func (s *ResearchService) GetEvidence(ctx context.Context, evidenceID string) (map[string]any, error) {
	owner := UserIDFrom(ctx)
	var ev modelfinance.Evidence
	if err := s.DB.WithContext(ctx).Where("id = ?", evidenceID).Take(&ev).Error; err != nil {
		return nil, NewError(404, "not_found", "EVIDENCE_NOT_FOUND", "证据不存在")
	}
	var run modelfinance.ResearchRun
	if err := s.DB.Where("id = ? AND owner_id = ? AND deleted_at IS NULL", ev.RunID, owner).Take(&run).Error; err != nil {
		return nil, NewError(404, "not_found", "EVIDENCE_NOT_FOUND", "证据不存在")
	}
	var metrics any
	_ = json.Unmarshal(ev.Metrics, &metrics)
	return map[string]any{
		"schema_version": "1.0",
		"evidence_id":    ev.ID,
		"run_id":         ev.RunID,
		"instrument_id":  ev.InstrumentID,
		"source_id":      ev.SourceID,
		"title":          ev.Title,
		"source_url":     ev.SourceURL,
		"source_kind":    ev.SourceKind,
		"published_at":   ev.PublishedAt.UTC().Format(time.RFC3339),
		"available_at":   ev.AvailableAt.UTC().Format(time.RFC3339),
		"retrieved_at":   ev.RetrievedAt.UTC().Format(time.RFC3339),
		"content_hash":   ev.ContentHash,
		"data_version":   ev.DataVersion,
		"mode":           ev.Mode,
		"locator":        ev.Locator,
		"text":           ev.Text,
		"metrics":        metrics,
	}, nil
}

func (s *ResearchService) Publish(ctx context.Context, runID string, expectedVersion int64, report VerifiedReport) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run modelfinance.ResearchRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", runID).Take(&run).Error; err != nil {
			return NewError(404, "not_found", "RUN_NOT_FOUND", "研究不存在")
		}
		if run.DeletedAt != nil || run.Status == StatusCanceling || run.Status == StatusCanceled {
			return NewError(409, "conflict", "RUN_CANCELED", "已取消，拒绝发布")
		}
		if run.Status != StatusVerifying {
			return NewError(409, "conflict", "RUN_CLOSED", "仅核对中的研究可发布")
		}
		if run.LeaseUntil == nil || s.Clock.Now().After(*run.LeaseUntil) {
			return NewError(409, "conflict", "LEASE_LOST", "租约已失效，拒绝发布")
		}
		if run.DeadlineAt != nil && s.Clock.Now().After(*run.DeadlineAt) {
			return NewError(409, "conflict", "DEADLINE", "已过截止时间")
		}
		if run.Version != expectedVersion {
			return NewError(409, "conflict", "VERSION_MISMATCH", "版本不匹配")
		}
		structural := report.SchemaVersion == "1.0" && report.RunID == runID && report.Version >= 1 && report.Summary != ""
		if !structural {
			return NewError(400, "validation", "STRUCTURAL_INVALID", "报告未通过结构校验，拒绝发布")
		}
		var allowed []modelfinance.Evidence
		if err := tx.Where("run_id = ?", runID).Find(&allowed).Error; err != nil {
			return err
		}
		okIDs := map[string]struct{}{}
		for _, ev := range allowed {
			okIDs[ev.ID] = struct{}{}
		}
		citationsOK := true
		checkIDs := append([]string{}, report.EvidenceIDs...)
		for _, arg := range append(append([]Argument{}, report.Support...), report.Challenge...) {
			checkIDs = append(checkIDs, arg.EvidenceIDs...)
		}
		for _, id := range checkIDs {
			if id == "" {
				continue
			}
			if _, ok := okIDs[id]; !ok {
				citationsOK = false
				break
			}
		}
		if !citationsOK {
			return NewError(400, "validation", "UNREGISTERED_CITATION", "未知引用，拒绝发布")
		}
		now := s.Clock.Now()
		body, _ := json.Marshal(report)
		verification := map[string]any{
			"structural": structural,
			"citations":  citationsOK,
		}
		verJSON, _ := json.Marshal(verification)
		row := modelfinance.Report{
			ID:               "report_" + uuid.NewString(),
			RunID:            runID,
			Version:          report.Version,
			QualityStatus:    report.QualityStatus,
			Body:             datatypes.JSON(body),
			ValidatorVersion: "gate_v1",
			Verification:     datatypes.JSON(verJSON),
			PublishedAt:      &now,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		status := StatusCompleted
		if report.QualityStatus == "incomplete" {
			status = StatusIncomplete
		}
		if report.QualityStatus == "failed" {
			status = StatusFailed
		}
		res := tx.Model(&run).Where("id = ? AND version = ? AND status NOT IN ?", runID, expectedVersion, []string{StatusCanceled, StatusCanceling}).
			Updates(map[string]any{
				"status": status, "stage": "done", "version": run.Version + 1, "updated_at": now,
				"lease_owner": nil, "lease_until": nil,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return NewError(409, "conflict", "RUN_CLOSED", "状态已变更，拒绝发布")
		}
		return nil
	})
}

func (s *ResearchService) WriteTaskResult(ctx context.Context, runID, role string, result ResearchResult) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run modelfinance.ResearchRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", runID).Take(&run).Error; err != nil {
			return NewError(404, "not_found", "RUN_NOT_FOUND", "研究不存在")
		}
		if run.DeletedAt != nil || run.Status == StatusCanceling || run.Status == StatusCanceled || run.Status == StatusIncomplete || run.Status == StatusFailed || run.Status == StatusCompleted {
			return NewError(409, "conflict", "RUN_CLOSED", "拒绝晚到写回")
		}
		var task modelfinance.ResearchTask
		if err := tx.Where("run_id = ? AND role = ?", runID, role).Take(&task).Error; err != nil {
			return NewError(404, "not_found", "TASK_NOT_FOUND", "任务不存在")
		}
		if result.RunID != "" && result.RunID != runID {
			return NewError(409, "conflict", "RESULT_RUN_MISMATCH", "结果不属于当前研究")
		}
		if result.TaskID != "" && result.TaskID != task.ID {
			return NewError(409, "conflict", "RESULT_TASK_MISMATCH", "结果不属于当前任务")
		}
		raw, _ := json.Marshal(result)
		return tx.Model(&modelfinance.ResearchTask{}).Where("run_id = ? AND role = ?", runID, role).Updates(map[string]any{
			"result": datatypes.JSON(raw), "status": result.Status, "updated_at": s.Clock.Now(),
		}).Error
	})
}

func (s *ResearchService) loadRun(runID string) (modelfinance.ResearchRun, error) {
	var run modelfinance.ResearchRun
	err := s.DB.Where("id = ?", runID).Take(&run).Error
	return run, err
}

func (s *ResearchService) purgeExecutorCopy(ctx context.Context, taskID string) error {
	if s.Client == nil || taskID == "" {
		return nil
	}
	return s.Client.Purge(ctx, taskID)
}

func (s *ResearchService) revokeTokens(ctx context.Context, runID string) {
	now := s.Clock.Now()
	_ = s.DB.WithContext(ctx).Model(&modelfinance.InternalToken{}).Where("run_id = ? AND revoked_at IS NULL", runID).
		Updates(map[string]any{"revoked_at": now})
	_ = s.DB.WithContext(ctx).Model(&modelfinance.ToolGrant{}).Where("run_id = ? AND status = ?", runID, "granted").
		Updates(map[string]any{"status": "revoked", "updated_at": now})
}

func isUnique(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate")
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

func HTTPStatus(err error) int {
	var ae *AppError
	if errors.As(err, &ae) && ae.Status != 0 {
		return ae.Status
	}
	return 500
}

func ErrorCode(err error) string {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return "INTERNAL"
}

func ErrorMessage(err error) string {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae.Message
	}
	return "内部错误"
}

func AsAppError(err error) *AppError {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae
	}
	return NewError(500, "unavailable", "INTERNAL", fmt.Sprintf("%v", err))
}
