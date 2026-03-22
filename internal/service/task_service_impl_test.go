package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepo struct {
	byTaskID       map[string]Task
	byIdempotency  map[string]Task
	deleteRows     int64
	createErr      error
	updateErr      error
	findByIDErr    error
	findByIdemErr  error
	deleteErr      error
	listRecentResp []Task
	listRecentErr  error
	listFailedResp []Task
	listFailedErr  error
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

func TestCreateQueuedTaskReturnsExistingByIdempotency(t *testing.T) {
	existing := Task{TaskID: "existing", IdempotencyKey: "k", Status: StatusQueued}
	svc := NewTaskService(&fakeRepo{byIdempotency: map[string]Task{"k": existing}})

	task, err := svc.CreateQueuedTask(context.Background(), CreateQueuedTaskRequest{
		TaskID:         "new",
		ChatID:         1,
		UserID:         1,
		TargetChatID:   1,
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
		TargetChatID:   1,
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
