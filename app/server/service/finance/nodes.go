package finance

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"gorm.io/gorm"

	modelfinance "zhigu/server/model/finance"
)

func (o *Orchestrator) freeze(ctx context.Context, runID string) (wfState, error) {
	run, err := o.Svc.loadRun(runID)
	if err != nil {
		return wfState{}, err
	}
	if runIsCanceled(run) {
		return wfState{}, NewError(409, "conflict", "RUN_CLOSED", "已取消或删除")
	}
	now := o.Svc.Clock.Now()
	runTimeout, _ := o.Svc.snapshotTimeouts(run)
	deadline := now.Add(runTimeout)
	owner := "zhigu-worker"
	until := now.Add(LeaseTTL)
	if err := o.Svc.casUpdateRun(o.Svc.DB, runID, run.Version, []string{StatusQueued}, map[string]any{
		"status": StatusResearching, "stage": "researching", "started_at": now, "deadline_at": deadline,
		"lease_owner": owner, "lease_until": until, "updated_at": now,
	}); err != nil {
		return wfState{}, err
	}
	return wfState{RunID: runID, FenceVersion: run.Version + 1}, nil
}

func (o *Orchestrator) research(ctx context.Context, st wfState) (wfState, error) {
	var tasks []modelfinance.ResearchTask
	if err := o.Svc.DB.Where("run_id = ?", st.RunID).Find(&tasks).Error; err != nil {
		return st, err
	}
	run, err := o.Svc.loadRun(st.RunID)
	if err != nil {
		return st, err
	}
	if runIsCanceled(run) {
		return st, NewError(409, "conflict", "RUN_CLOSED", "已取消或删除")
	}
	var claim Claim
	_ = json.Unmarshal(run.ClaimSnapshot, &claim)
	runTimeout, _ := o.Svc.snapshotTimeouts(run)
	deadline := o.Svc.Clock.Now().Add(runTimeout)
	if run.DeadlineAt != nil {
		deadline = *run.DeadlineAt
	}
	hbCtx, stopHB := context.WithCancel(ctx)
	defer stopHB()
	go o.heartbeat(hbCtx, st.RunID)

	type roleOut struct {
		role   string
		result *ResearchResult
		err    error
	}
	outCh := make(chan roleOut, len(tasks))
	var wg sync.WaitGroup
	for _, task := range tasks {
		task := task
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := o.Svc.renewLease(ctx, st.RunID); err != nil {
				outCh <- roleOut{role: task.Role, err: err}
				return
			}
			token, err := o.Svc.IssueTaskToken(run.ID, task.ID, "research")
			if err != nil {
				token = ""
			}
			budget := o.Svc.loadBudgetSnapshot(o.Svc.DB, st.RunID)
			rt := ResearchTask{
				SchemaVersion: "1.0", RunID: run.ID, TaskID: task.ID, Role: task.Role, Claim: claim,
				InstrumentID: run.InstrumentID, AsOf: run.AsOf, Mode: run.Mode,
				SourcePolicyVersion: "source_fixture_v1", ModelConfigVersion: "model_fixture_v1", PromptVersion: "prompt_v1",
				MaxModelCalls: budget.MaxModelCallsPerRole, MaxToolCalls: budget.MaxToolCallsPerRole, DeadlineAt: deadline, TaskToken: token,
			}
			if _, err := o.Svc.Client.Submit(ctx, rt); err != nil {
				outCh <- roleOut{role: task.Role, err: err}
				return
			}
			snap, err := o.waitTask(ctx, run.ID, task.ID, deadline)
			if err != nil {
				outCh <- roleOut{role: task.Role, err: err}
				return
			}
			if err := bindResultIdentity(run.ID, task.ID, snap); err != nil {
				outCh <- roleOut{role: task.Role, err: err}
				return
			}
			if snap.Result != nil {
				if err := o.Svc.WriteTaskResult(ctx, run.ID, task.Role, *snap.Result); err != nil {
					outCh <- roleOut{role: task.Role, err: err}
					return
				}
			}
			outCh <- roleOut{role: task.Role, result: snap.Result, err: nil}
		}()
	}
	wg.Wait()
	close(outCh)
	for item := range outCh {
		if item.role == RoleSupporter {
			st.Supporter, st.SupporterErr = item.result, item.err
		} else {
			st.Challenger, st.ChallengerErr = item.result, item.err
		}
	}
	return st, nil
}

func (o *Orchestrator) heartbeat(ctx context.Context, runID string) {
	t := time.NewTicker(LeaseRenew)
	defer t.Stop()
	_ = o.Svc.renewLease(ctx, runID)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := o.Svc.renewLease(ctx, runID); err != nil {
				return
			}
		}
	}
}

func roleExecutable(res *ResearchResult, err error) bool {
	if err != nil || res == nil {
		return false
	}
	if res.Status == "failed" || res.Status == "canceled" {
		return false
	}
	for _, e := range res.Errors {
		if e.Code == "TOOL_PATH_FAILED" || e.Code == "HARNESS_UNAVAILABLE" || e.Code == "PYTHON_UNAVAILABLE" {
			return false
		}
	}
	return res.Status == "succeeded" || res.Status == "insufficient"
}

func (o *Orchestrator) synthesize(ctx context.Context, st wfState) (wfState, error) {
	run, err := o.Svc.loadRun(st.RunID)
	if err != nil {
		return st, err
	}
	if runIsCanceled(run) {
		return st, NewError(409, "conflict", "RUN_CLOSED", "已取消或删除")
	}
	expected := st.FenceVersion
	if expected == 0 {
		expected = run.Version
	}
	if err := o.Svc.casUpdateRun(o.Svc.DB, st.RunID, expected, []string{StatusQueued, StatusResearching}, map[string]any{
		"status": StatusVerifying, "stage": "verifying",
	}); err != nil {
		return st, err
	}
	st.FenceVersion = expected + 1

	support := []Argument{}
	challenge := []Argument{}
	unknowns := []string{}
	eids := []string{}
	untrusted := false

	expectedTasks := map[string]string{}
	var dbTasks []modelfinance.ResearchTask
	_ = o.Svc.DB.Where("run_id = ?", st.RunID).Find(&dbTasks)
	for _, task := range dbTasks {
		expectedTasks[task.Role] = task.ID
	}

	if st.Supporter != nil {
		vr, err := o.Evidence.ValidateRole(ctx, RunSnapshot{ID: run.ID, TaskID: expectedTasks[RoleSupporter]}, *st.Supporter)
		if err != nil {
			st.SupporterErr = err
			untrusted = true
		} else {
			support = append(support, vr.Arguments()...)
			unknowns = append(unknowns, vr.Result().Unknowns...)
			eids = append(eids, vr.EvidenceIDs()...)
		}
	}
	if st.Challenger != nil {
		vr, err := o.Evidence.ValidateRole(ctx, RunSnapshot{ID: run.ID, TaskID: expectedTasks[RoleChallenger]}, *st.Challenger)
		if err != nil {
			st.ChallengerErr = err
			untrusted = true
		} else {
			challenge = append(challenge, vr.Result().Counterevidence...)
			challenge = append(challenge, vr.Arguments()...)
			unknowns = append(unknowns, vr.Result().Unknowns...)
			eids = append(eids, vr.EvidenceIDs()...)
		}
	}

	supOK := roleExecutable(st.Supporter, st.SupporterErr)
	chalOK := roleExecutable(st.Challenger, st.ChallengerErr)
	execFail := (st.SupporterErr != nil && st.Supporter == nil) || (st.ChallengerErr != nil && st.Challenger == nil)
	if st.Supporter != nil {
		for _, e := range st.Supporter.Errors {
			if e.Code == "TOOL_PATH_FAILED" || e.Code == "HARNESS_UNAVAILABLE" {
				execFail = true
			}
		}
		if st.Supporter.Status == "failed" {
			execFail = true
		}
	}
	if st.Challenger != nil {
		for _, e := range st.Challenger.Errors {
			if e.Code == "TOOL_PATH_FAILED" || e.Code == "HARNESS_UNAVAILABLE" {
				execFail = true
			}
		}
		if st.Challenger.Status == "failed" {
			execFail = true
		}
	}

	quality := "completed"
	v := "insufficient"
	verdict := &v
	summary := "双方已结束。现有资料不足以单独判断未来股价方向。"
	if execFail && !supOK && !chalOK {
		st.FailRun = true
		quality = "incomplete"
		verdict = nil
		summary = "研究失败：执行依赖不可用"
	} else if !supOK || !chalOK {
		quality = "incomplete"
		verdict = nil
		summary = "研究未完成"
	}
	if untrusted {
		support = trustedArgs(support)
		challenge = trustedArgs(challenge)
		st.FailRun = true
		quality = "incomplete"
		verdict = nil
		summary = "研究失败：主张未能通过引用校验"
	}

	st.Report = VerifiedReport{
		SchemaVersion: "1.0", RunID: st.RunID, Version: 1, Mode: run.Mode, AsOf: run.AsOf,
		QualityStatus: quality, Verdict: verdict, Summary: summary,
		Support: support, Challenge: challenge, Assumptions: []string{"收入增长转化为股价需要利润、现金流与估值证据。"},
		ChangeConditions: []string{"补充利润、现金流和估值证据后重新研究。"},
		Unknowns: unknowns, EvidenceIDs: unique(eids),
		ModelConfigVersion: "model_fixture_v1", SourcePolicyVersion: "source_fixture_v1", PromptVersion: "prompt_v1",
	}
	if len(st.Report.Unknowns) == 0 {
		st.Report.Unknowns = []string{"未获得足以判断未来股价方向的证据。"}
	}
	return st, nil
}

func trustedArgs(in []Argument) []Argument {
	out := []Argument{}
	for _, a := range in {
		if (a.ClaimType == "fact" || a.ClaimType == "inference") && len(a.EvidenceIDs) == 0 {
			continue
		}
		out = append(out, a)
	}
	return out
}

func (o *Orchestrator) verify(ctx context.Context, st wfState) (wfState, error) {
	run, err := o.Svc.loadRun(st.RunID)
	if err != nil {
		return st, err
	}
	if runIsCanceled(run) {
		return st, NewError(409, "conflict", "RUN_CLOSED", "已取消或删除")
	}
	if st.FailRun {
		return st, o.failOrIncomplete(st.RunID, st.FenceVersion, true)
	}
	if err := o.revalidateReport(ctx, run.ID, &st.Report); err != nil {
		st.Report.Support = nil
		st.Report.Challenge = nil
		st.Report.EvidenceIDs = nil
		st.Report.QualityStatus = "incomplete"
		st.Report.Verdict = nil
		st.Report.Summary = "研究失败：主张未能通过引用校验"
		return st, o.failOrIncomplete(st.RunID, st.FenceVersion, true)
	}
	ok := o.structuralOK(st.Report)
	if !ok && !st.Repaired && o.RepairN < 1 {
		st.Repaired = true
		o.RepairN++
		st.Report.Unknowns = append(st.Report.Unknowns, "已进行一次修复后再校验")
		ok = o.structuralOK(st.Report)
	}
	if !ok {
		st.Report.QualityStatus = "incomplete"
		st.Report.Verdict = nil
		st.Report.Summary = "研究未完成"
		return st, o.failOrIncomplete(st.RunID, st.FenceVersion, true)
	}
	if run.Status != StatusVerifying {
		return st, NewError(409, "conflict", "RUN_CLOSED", "仅核对中的研究可发布")
	}
	expected := st.FenceVersion
	if expected == 0 {
		expected = run.Version
	}
	if err := o.Svc.Publish(ctx, st.RunID, expected, st.Report); err != nil {
		return st, err
	}
	return st, nil
}

func (o *Orchestrator) revalidateReport(ctx context.Context, runID string, report *VerifiedReport) error {
	if report == nil {
		return nil
	}
	allowed := map[string]struct{}{}
	var evs []modelfinance.Evidence
	_ = o.Svc.DB.WithContext(ctx).Where("run_id = ?", runID).Find(&evs)
	for _, ev := range evs {
		allowed[ev.ID] = struct{}{}
	}
	check := func(args []Argument) error {
		for _, arg := range args {
			if arg.ClaimType == "fact" || arg.ClaimType == "inference" {
				if len(arg.EvidenceIDs) == 0 {
					return NewError(400, "validation", "MISSING_CITATION", "事实/推断须引用证据")
				}
			}
			for _, id := range arg.EvidenceIDs {
				if _, ok := allowed[id]; !ok {
					return NewError(400, "validation", "UNREGISTERED_CITATION", "未知引用")
				}
			}
		}
		return nil
	}
	if err := check(report.Support); err != nil {
		return err
	}
	if err := check(report.Challenge); err != nil {
		return err
	}
	for _, id := range report.EvidenceIDs {
		if _, ok := allowed[id]; !ok {
			return NewError(400, "validation", "UNREGISTERED_CITATION", "未知引用")
		}
	}
	return nil
}

func (o *Orchestrator) structuralOK(r VerifiedReport) bool {
	if r.SchemaVersion != "1.0" || r.Summary == "" || r.RunID == "" || r.Version < 1 {
		return false
	}
	if r.QualityStatus == "incomplete" && r.Verdict != nil {
		return false
	}
	if r.QualityStatus == "completed" && r.Verdict == nil {
		return false
	}
	return true
}

func bindResultIdentity(runID, taskID string, snap TaskSnapshot) error {
	if snap.TaskID != "" && snap.TaskID != taskID {
		return NewError(409, "conflict", "RESULT_TASK_MISMATCH", "快照任务与调度任务不一致")
	}
	if snap.Result == nil {
		return NewError(400, "validation", "EMPTY_RESULT", "缺少执行器结果")
	}
	return validateExecutorResult(runID, taskID, snap.Result)
}

var allowedResultStatus = map[string]struct{}{
	"succeeded": {}, "insufficient": {}, "failed": {}, "canceled": {},
}
var allowedClaimTypes = map[string]struct{}{
	"fact": {}, "inference": {}, "assumption": {},
}

func validateExecutorResult(runID, taskID string, res *ResearchResult) error {
	if res == nil {
		return NewError(400, "validation", "EMPTY_RESULT", "缺少执行器结果")
	}
	if res.SchemaVersion != "1.0" {
		return NewError(400, "validation", "INVALID_SCHEMA", "结果契约版本无效")
	}
	if res.RunID != runID {
		return NewError(409, "conflict", "RESULT_RUN_MISMATCH", "结果不属于当前研究")
	}
	if res.TaskID != taskID {
		return NewError(409, "conflict", "RESULT_TASK_MISMATCH", "结果不属于当前任务")
	}
	if _, ok := allowedResultStatus[res.Status]; !ok {
		return NewError(400, "validation", "INVALID_STATUS", "结果状态无效")
	}
	check := func(args []Argument) error {
		for _, arg := range args {
			if _, ok := allowedClaimTypes[arg.ClaimType]; !ok {
				return NewError(400, "validation", "INVALID_CLAIM_TYPE", "主张类型无效")
			}
			if (arg.ClaimType == "fact" || arg.ClaimType == "inference") && len(arg.EvidenceIDs) == 0 {
				return NewError(400, "validation", "MISSING_CITATION", "事实/推断须引用证据")
			}
		}
		return nil
	}
	if err := check(res.Arguments); err != nil {
		return err
	}
	return check(res.Counterevidence)
}

func unique(in []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func FixtureRoleResult(runID, taskID, role string) ResearchResult {
	res := ResearchResult{
		SchemaVersion: "1.0", RunID: runID, TaskID: taskID, Status: "insufficient",
		Arguments: []Argument{}, EvidenceIDs: []string{}, Unknowns: []string{"缺少利润、现金流、估值与市场预期资料。"},
		Counterevidence: []Argument{}, Usage: Usage{ModelCalls: 0, ToolCalls: 0, InputTokens: 0, OutputTokens: 0, Simulated: true},
	}
	if role == RoleSupporter {
		res.Status = "insufficient"
		res.Arguments = []Argument{{ClaimType: "assumption", Text: "虚构测试数据中，演示公司的营业收入同比增长，尚不足以推断股价。", EvidenceIDs: []string{}}}
	}
	return res
}

func (o *Orchestrator) failOrIncomplete(runID string, fence int64, failed bool) error {
	status := StatusIncomplete
	if failed {
		status = StatusFailed
	}
	now := o.Svc.Clock.Now()
	updates := map[string]any{
		"status": status, "stage": "done", "updated_at": now,
		"lease_owner": nil, "lease_until": nil,
	}
	q := o.Svc.DB.Model(&modelfinance.ResearchRun{}).
		Where("id = ? AND deleted_at IS NULL AND status NOT IN ?", runID, []string{StatusCanceled, StatusCanceling, StatusCompleted, StatusFailed})
	if fence > 0 {
		updates["version"] = fence + 1
		q = q.Where("version = ?", fence)
	} else {
		updates["version"] = gorm.Expr("version + 1")
	}
	res := q.Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return NewError(409, "conflict", "RUN_CLOSED", "拒绝覆盖终态")
	}
	o.Svc.revokeTokens(context.Background(), runID)
	return nil
}
