package finance

import (
	"context"
	"time"

	"github.com/cloudwego/eino/compose"
)

type Orchestrator struct {
	Svc      *ResearchService
	Evidence *EvidenceService
	RepairN  int
}

func NewOrchestrator(svc *ResearchService, ev *EvidenceService) *Orchestrator {
	return &Orchestrator{Svc: svc, Evidence: ev, RepairN: 0}
}

type wfState struct {
	RunID         string
	Supporter     *ResearchResult
	Challenger    *ResearchResult
	SupporterErr  error
	ChallengerErr error
	Report        VerifiedReport
	Repaired      bool
	FenceVersion  int64
	FailRun       bool
}

func (o *Orchestrator) Run(ctx context.Context, runID string) error {
	wf := compose.NewWorkflow[string, wfState]()
	wf.AddLambdaNode("freeze", compose.InvokableLambda(func(ctx context.Context, id string) (wfState, error) {
		return o.freeze(ctx, id)
	})).AddInput(compose.START)
	wf.AddLambdaNode("research", compose.InvokableLambda(func(ctx context.Context, st wfState) (wfState, error) {
		return o.research(ctx, st)
	})).AddInput("freeze")
	wf.AddLambdaNode("synthesize", compose.InvokableLambda(func(ctx context.Context, st wfState) (wfState, error) {
		return o.synthesize(ctx, st)
	})).AddInput("research")
	wf.AddLambdaNode("verify", compose.InvokableLambda(func(ctx context.Context, st wfState) (wfState, error) {
		return o.verify(ctx, st)
	})).AddInput("synthesize")
	wf.End().AddInput("verify")
	run, err := wf.Compile(ctx)
	if err != nil {
		return err
	}
	_, err = run.Invoke(ctx, runID)
	return err
}

func (o *Orchestrator) waitTask(ctx context.Context, runID, taskID string, deadline time.Time) (TaskSnapshot, error) {
	wait := 500 * time.Millisecond
	lastBeat := time.Time{}
	for {
		now := o.Svc.Clock.Now()
		if !now.Before(deadline) {
			return TaskSnapshot{}, NewError(504, "timeout", "ROLE_TIMEOUT", "角色任务超时")
		}
		if lastBeat.IsZero() || now.Sub(lastBeat) >= LeaseRenew {
			if err := o.Svc.renewLease(ctx, runID); err != nil {
				return TaskSnapshot{}, err
			}
			lastBeat = now
		}
		run, err := o.Svc.loadRun(runID)
		if err == nil && runIsCanceled(run) {
			return TaskSnapshot{}, NewError(409, "conflict", "RUN_CLOSED", "已取消或删除")
		}
		snap, err := o.Svc.Client.Get(ctx, taskID)
		if err != nil {
			return TaskSnapshot{}, err
		}
		switch snap.Status {
		case "succeeded", "insufficient", "failed", "canceled":
			return snap, nil
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return TaskSnapshot{}, ctx.Err()
		case <-timer.C:
		}
		if wait < 2*time.Second {
			wait *= 2
			if wait > 2*time.Second {
				wait = 2 * time.Second
			}
		}
	}
}
