package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepo struct {
	byTaskID             map[string]Task
	byIdempotency        map[string]Task
	deleteRows           int64
	createErr            error
	updateErr            error
	findByIDErr          error
	findByIdemErr        error
	deleteErr            error
	deleteTaskRows       int64
	deleteTaskErr        error
	deleteNonRunningRows int64
	deleteNonRunningErr  error
	listRecentResp       []Task
	listRecentErr        error
	listActiveResp       []Task
	listActiveErr        error
	listFailedResp       []Task
	listFailedErr        error
	claimResp            Task
	claimOK              bool
	claimErr             error
}

func (r *fakeRepo) Create(_ context.Context, task Task) error {
	if r.createErr != nil {
		return r.createErr
	}
	if r.byTaskID == nil {
		r.byTaskID = map[string]Task{}
	}
	if r.byIdempotency == nil {
		r.byIdempotency = map[string]Task{}
	}
	r.byTaskID[task.TaskID] = task
	r.byIdempotency[task.IdempotencyKey] = task
	return nil
}

func (r *fakeRepo) Update(_ context.Context, task Task) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	if r.byTaskID == nil {
		return ErrTaskNotFound
	}
	if _, ok := r.byTaskID[task.TaskID]; !ok {
		return ErrTaskNotFound
	}
	r.byTaskID[task.TaskID] = task
	if r.byIdempotency != nil {
		r.byIdempotency[task.IdempotencyKey] = task
	}
	return nil
}

func (r *fakeRepo) FindByID(_ context.Context, taskID string) (Task, error) {
	if r.findByIDErr != nil {
		return Task{}, r.findByIDErr
	}
	t, ok := r.byTaskID[taskID]
	if !ok {
		return Task{}, ErrTaskNotFound
	}
	return t, nil
}

func (r *fakeRepo) FindByIdempotencyKey(_ context.Context, idempotencyKey string) (Task, error) {
	if r.findByIdemErr != nil {
		return Task{}, r.findByIdemErr
	}
	t, ok := r.byIdempotency[idempotencyKey]
	if !ok {
		return Task{}, ErrTaskNotFound
	}
	return t, nil
}

func (r *fakeRepo) ListRecentByUser(context.Context, int64, int) ([]Task, error) {
	if r.listRecentErr != nil {
		return nil, r.listRecentErr
	}
	return r.listRecentResp, nil
}

func (r *fakeRepo) ListActiveByUser(context.Context, int64, int) ([]Task, error) {
	if r.listActiveErr != nil {
		return nil, r.listActiveErr
	}
	return r.listActiveResp, nil
}

func (r *fakeRepo) ListFailedForRetry(context.Context, int, int) ([]Task, error) {
	if r.listFailedErr != nil {
		return nil, r.listFailedErr
	}
	return r.listFailedResp, nil
}

func (r *fakeRepo) DeleteFailedByIdempotencyKey(_ context.Context, _ string) (int64, error) {
	if r.deleteErr != nil {
		return 0, r.deleteErr
	}
	return r.deleteRows, nil
}

func (r *fakeRepo) DeletePendingByUserTaskID(_ context.Context, _ int64, _ string) (int64, error) {
	if r.deleteTaskErr != nil {
		return 0, r.deleteTaskErr
	}
	return r.deleteTaskRows, nil
}

func (r *fakeRepo) DeleteNonRunningByUserTaskID(_ context.Context, _ int64, _ string) (int64, error) {
	if r.deleteNonRunningErr != nil {
		return 0, r.deleteNonRunningErr
	}
	return r.deleteNonRunningRows, nil
}

func (r *fakeRepo) ClaimForExecution(_ context.Context, _ string, _ string, _ time.Time) (Task, bool, error) {
	if r.claimErr != nil {
		return Task{}, false, r.claimErr
	}
	return r.claimResp, r.claimOK, nil
}

func TestCreateQueuedTaskReturnsExistingByIdempotency(t *testing.T) {
	existing := Task{TaskID: "existing", IdempotencyKey: "k", Status: StatusQueued}
	svc := NewTaskService(&fakeRepo{byIdempotency: map[string]Task{"k": existing}})

	task, err := svc.CreateQueuedTask(context.Background(), CreateQueuedTaskRequest{
		TaskID:         "new",
		ChatID:         1,
		UserID:         1,
		TargetPeer:     "target-a",
		URL:            "https://t.me/c/1/2",
		IdempotencyKey: "k",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.TaskID != "existing" {
		t.Fatalf("expected existing task, got %s", task.TaskID)
	}
}

func TestCreateQueuedTaskAllowsMissingTargetPeer(t *testing.T) {
	svc := NewTaskService(&fakeRepo{})

	task, err := svc.CreateQueuedTask(context.Background(), CreateQueuedTaskRequest{
		TaskID:         "new",
		ChatID:         1,
		UserID:         1,
		URL:            "https://t.me/c/1/2",
		IdempotencyKey: "k",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.TargetPeer != "" {
		t.Fatalf("expected target peer to remain unset, got %q", task.TargetPeer)
	}
}

func TestCreateQueuedTaskPersistsDropCaption(t *testing.T) {
	svc := NewTaskService(&fakeRepo{})

	task, err := svc.CreateQueuedTask(context.Background(), CreateQueuedTaskRequest{
		TaskID:         "new",
		ChatID:         1,
		UserID:         1,
		TargetPeer:     "channel_name",
		URL:            "https://t.me/c/1/2",
		DropCaption:    true,
		IdempotencyKey: "k-drop",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !task.DropCaption {
		t.Fatal("expected drop_caption to be persisted")
	}
}

func TestUpdateTaskAppliesStatusAndFinishedAt(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepo{byTaskID: map[string]Task{"t1": {TaskID: "t1", IdempotencyKey: "k", Status: StatusRunning}}}
	svc := NewTaskService(repo)

	if err := svc.UpdateTask(context.Background(), "t1", TaskUpdate{Status: StatusDone, FinishedAt: &now}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	updated := repo.byTaskID["t1"]
	if updated.Status != StatusDone {
		t.Fatalf("expected done status, got %s", updated.Status)
	}
	if updated.FinishedAt == nil {
		t.Fatal("expected finished_at to be set")
	}
}

func TestCreateQueuedTaskPropagatesRepositoryError(t *testing.T) {
	svc := NewTaskService(&fakeRepo{findByIdemErr: errors.New("db fail")})
	_, err := svc.CreateQueuedTask(context.Background(), CreateQueuedTaskRequest{
		TaskID:         "new",
		ChatID:         1,
		UserID:         1,
		TargetPeer:     "target-a",
		URL:            "https://t.me/c/1/2",
		IdempotencyKey: "k",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteFailedByIdempotencyKey(t *testing.T) {
	svc := NewTaskService(&fakeRepo{deleteRows: 2})
	rows, err := svc.DeleteFailedByIdempotencyKey(context.Background(), "k")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rows != 2 {
		t.Fatalf("expected 2 deleted rows, got %d", rows)
	}
}

func TestDeletePendingTask(t *testing.T) {
	svc := NewTaskService(&fakeRepo{deleteTaskRows: 1})
	deleted, err := svc.DeletePendingTask(context.Background(), 1, "t1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Fatal("expected deleted=true")
	}
}

func TestDeletePendingTaskReturnsFalseWhenNoRows(t *testing.T) {
	svc := NewTaskService(&fakeRepo{deleteTaskRows: 0})
	deleted, err := svc.DeletePendingTask(context.Background(), 1, "t1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted {
		t.Fatal("expected deleted=false")
	}
}

func TestDeleteTaskNonRunning(t *testing.T) {
	svc := NewTaskService(&fakeRepo{deleteNonRunningRows: 1})
	deleted, err := svc.DeleteTaskNonRunning(context.Background(), 1, "t1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Fatal("expected deleted=true")
	}
}

func TestDeleteTaskNonRunningReturnsFalseWhenNoRows(t *testing.T) {
	svc := NewTaskService(&fakeRepo{deleteNonRunningRows: 0})
	deleted, err := svc.DeleteTaskNonRunning(context.Background(), 1, "t1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted {
		t.Fatal("expected deleted=false")
	}
}

func TestListActiveTasks(t *testing.T) {
	svc := NewTaskService(&fakeRepo{
		listActiveResp: []Task{{TaskID: "t1", Status: StatusRunning}},
	})

	tasks, err := svc.ListActiveTasks(context.Background(), 1, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].TaskID != "t1" {
		t.Fatalf("unexpected tasks: %+v", tasks)
	}
}

func TestListFailedTasksForRetry(t *testing.T) {
	svc := NewTaskService(&fakeRepo{
		listFailedResp: []Task{{TaskID: "t1", Status: StatusFailed}},
	})

	tasks, err := svc.ListFailedTasksForRetry(context.Background(), 3, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].TaskID != "t1" {
		t.Fatalf("unexpected tasks: %+v", tasks)
	}
}

func TestClaimTaskForExecution(t *testing.T) {
	now := time.Now().UTC()
	expected := Task{TaskID: "t1", Status: StatusRunning}
	svc := NewTaskService(&fakeRepo{
		claimResp: expected,
		claimOK:   true,
	})

	task, claimed, err := svc.ClaimTaskForExecution(context.Background(), ClaimTaskExecutionRequest{
		TaskID:    "t1",
		LeaseID:   "lease-1",
		StartedAt: now,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !claimed {
		t.Fatal("expected claimed=true")
	}
	if task.TaskID != expected.TaskID || task.Status != expected.Status {
		t.Fatalf("unexpected claimed task: %+v", task)
	}
}
