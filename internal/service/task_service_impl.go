package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrTaskNotFound = errors.New("service: task not found")

type TaskRepository interface {
	Create(ctx context.Context, task Task) error
	Update(ctx context.Context, task Task) error
	FindByID(ctx context.Context, taskID string) (Task, error)
	FindByIdempotencyKey(ctx context.Context, idempotencyKey string) (Task, error)
	ListFailedForRetry(ctx context.Context, maxRetryCount int, limit int) ([]Task, error)
	DeleteFailedByIdempotencyKey(ctx context.Context, idempotencyKey string) (int64, error)
	ListRecentByUser(ctx context.Context, userID int64, limit int) ([]Task, error)
}

type taskService struct {
	repo TaskRepository
}

func NewTaskService(repo TaskRepository) TaskService {
	return taskService{repo: repo}
}

func (s taskService) CreateQueuedTask(ctx context.Context, req CreateQueuedTaskRequest) (Task, error) {
	if s.repo == nil {
		return Task{}, errors.New("service: task repository is required")
	}
	if req.TaskID == "" {
		return Task{}, errors.New("service: task id is required")
	}
	if req.UserID == 0 || req.ChatID == 0 || req.TargetChatID == 0 {
		return Task{}, errors.New("service: chat/user/target chat id are required")
	}
	if strings.TrimSpace(req.URL) == "" {
		return Task{}, errors.New("service: url is required")
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return Task{}, errors.New("service: idempotency key is required")
	}

	existing, err := s.repo.FindByIdempotencyKey(ctx, req.IdempotencyKey)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrTaskNotFound) {
		return Task{}, fmt.Errorf("service: find task by idempotency key: %w", err)
	}

	now := time.Now().UTC()
	task := Task{
		TaskID:         req.TaskID,
		ChatID:         req.ChatID,
		UserID:         req.UserID,
		TargetChatID:   req.TargetChatID,
		URL:            strings.TrimSpace(req.URL),
		Status:         StatusQueued,
		CreatedAt:      now,
		UpdatedAt:      now,
		IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
	}
	if err := s.repo.Create(ctx, task); err != nil {
		return Task{}, fmt.Errorf("service: create queued task: %w", err)
	}
	return task, nil
}

func (s taskService) GetTask(ctx context.Context, taskID string) (Task, error) {
	if s.repo == nil {
		return Task{}, errors.New("service: task repository is required")
	}
	task, err := s.repo.FindByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return Task{}, ErrTaskNotFound
		}
		return Task{}, fmt.Errorf("service: get task: %w", err)
	}
	return task, nil
}

func (s taskService) ListRecentTasks(ctx context.Context, userID int64, limit int) ([]Task, error) {
	if s.repo == nil {
		return nil, errors.New("service: task repository is required")
	}
	tasks, err := s.repo.ListRecentByUser(ctx, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("service: list recent tasks: %w", err)
	}
	return tasks, nil
}

func (s taskService) ListFailedTasksForRetry(ctx context.Context, maxRetryCount int, limit int) ([]Task, error) {
	if s.repo == nil {
		return nil, errors.New("service: task repository is required")
	}
	if maxRetryCount <= 0 {
		return nil, errors.New("service: max retry count must be greater than zero")
	}
	tasks, err := s.repo.ListFailedForRetry(ctx, maxRetryCount, limit)
	if err != nil {
		return nil, fmt.Errorf("service: list failed tasks for retry: %w", err)
	}
	return tasks, nil
}

func (s taskService) FindByIdempotencyKey(ctx context.Context, idempotencyKey string) (Task, error) {
	if s.repo == nil {
		return Task{}, errors.New("service: task repository is required")
	}
	task, err := s.repo.FindByIdempotencyKey(ctx, idempotencyKey)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return Task{}, ErrTaskNotFound
		}
		return Task{}, fmt.Errorf("service: find by idempotency key: %w", err)
	}
	return task, nil
}

func (s taskService) DeleteFailedByIdempotencyKey(ctx context.Context, idempotencyKey string) (int64, error) {
	if s.repo == nil {
		return 0, errors.New("service: task repository is required")
	}
	if strings.TrimSpace(idempotencyKey) == "" {
		return 0, errors.New("service: idempotency key is required")
	}
	rows, err := s.repo.DeleteFailedByIdempotencyKey(ctx, strings.TrimSpace(idempotencyKey))
	if err != nil {
		return 0, fmt.Errorf("service: delete failed tasks by idempotency key: %w", err)
	}
	return rows, nil
}

func (s taskService) UpdateTask(ctx context.Context, taskID string, update TaskUpdate) error {
	if s.repo == nil {
		return errors.New("service: task repository is required")
	}
	if taskID == "" {
		return errors.New("service: task id is required")
	}
	if update.Status != "" && !update.Status.Valid() {
		return fmt.Errorf("service: invalid status %q", update.Status)
	}

	task, err := s.repo.FindByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return ErrTaskNotFound
		}
		return fmt.Errorf("service: load task for update: %w", err)
	}

	if update.Status != "" {
		task.Status = update.Status
	}
	if update.RetryCount != nil {
		task.RetryCount = *update.RetryCount
	}
	if update.LeaseID != nil {
		task.LeaseID = update.LeaseID
	}
	if update.OutputSummary != nil {
		task.OutputSummary = update.OutputSummary
	}
	if update.ErrorMessage != nil {
		task.ErrorMessage = update.ErrorMessage
	}
	if update.ExitCode != nil {
		task.ExitCode = update.ExitCode
	}
	if update.StartedAt != nil {
		t := update.StartedAt.UTC()
		task.StartedAt = &t
	}
	if update.FinishedAt != nil {
		t := update.FinishedAt.UTC()
		task.FinishedAt = &t
	}
	task.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, task); err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return ErrTaskNotFound
		}
		return fmt.Errorf("service: update task: %w", err)
	}
	return nil
}
