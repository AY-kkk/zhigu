package finance

import (
	"context"
	"sync"
)

type FakeResearchClient struct {
	mu     sync.Mutex
	tasks  map[string]TaskSnapshot
	purged []string
}

func NewFakeResearchClient() *FakeResearchClient {
	return &FakeResearchClient{tasks: map[string]TaskSnapshot{}}
}

func (f *FakeResearchClient) Submit(ctx context.Context, task ResearchTask) (TaskReceipt, error) {
	_ = ctx
	f.mu.Lock()
	defer f.mu.Unlock()
	if existing, ok := f.tasks[task.TaskID]; ok {
		return TaskReceipt{TaskID: existing.TaskID, Status: existing.Status}, nil
	}
	res := FixtureRoleResult(task.RunID, task.TaskID, task.Role)
	status := res.Status
	if status == "succeeded" || status == "insufficient" {
		// keep as succeeded/insufficient
	}
	snap := TaskSnapshot{TaskID: task.TaskID, Status: status, Result: &res}
	f.tasks[task.TaskID] = snap
	return TaskReceipt{TaskID: task.TaskID, Status: status}, nil
}

func (f *FakeResearchClient) Get(ctx context.Context, taskID string) (TaskSnapshot, error) {
	_ = ctx
	f.mu.Lock()
	defer f.mu.Unlock()
	snap, ok := f.tasks[taskID]
	if !ok {
		return TaskSnapshot{}, NewError(404, "not_found", "TASK_NOT_FOUND", "任务不存在")
	}
	return snap, nil
}

func (f *FakeResearchClient) Cancel(ctx context.Context, taskID string) (TaskSnapshot, error) {
	_ = ctx
	f.mu.Lock()
	defer f.mu.Unlock()
	snap, ok := f.tasks[taskID]
	if !ok {
		return TaskSnapshot{}, NewError(404, "not_found", "TASK_NOT_FOUND", "任务不存在")
	}
	if snap.Status == "succeeded" || snap.Status == "failed" || snap.Status == "canceled" || snap.Status == "insufficient" {
		return snap, nil
	}
	snap.Status = "canceled"
	f.tasks[taskID] = snap
	return snap, nil
}

func (f *FakeResearchClient) Purge(ctx context.Context, taskID string) error {
	_ = ctx
	f.mu.Lock()
	defer f.mu.Unlock()
	f.purged = append(f.purged, taskID)
	if snap, ok := f.tasks[taskID]; ok {
		snap.Result = nil
		f.tasks[taskID] = snap
	}
	return nil
}

func (f *FakeResearchClient) PurgedIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.purged))
	copy(out, f.purged)
	return out
}
