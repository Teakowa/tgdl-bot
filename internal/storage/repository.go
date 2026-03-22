package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"tgdl-bot/internal/service"
)

type TaskRepository interface {
	Create(ctx context.Context, task service.Task) error
	Update(ctx context.Context, task service.Task) error
	FindByID(ctx context.Context, taskID string) (service.Task, error)
	FindByIdempotencyKey(ctx context.Context, idempotencyKey string) (service.Task, error)
	ListActiveByUser(ctx context.Context, userID int64, limit int) ([]service.Task, error)
	ListFailedForRetry(ctx context.Context, maxRetryCount int, limit int) ([]service.Task, error)
	DeleteFailedByIdempotencyKey(ctx context.Context, idempotencyKey string) (int64, error)
	DeletePendingByUserTaskID(ctx context.Context, userID int64, taskID string) (int64, error)
	DeleteNonRunningByUserTaskID(ctx context.Context, userID int64, taskID string) (int64, error)
	ListRecentByUser(ctx context.Context, userID int64, limit int) ([]service.Task, error)
	ClaimForExecution(ctx context.Context, taskID, leaseID string, startedAt time.Time) (service.Task, bool, error)
}

type D1TaskRepository struct {
	Client *D1Client
}

func NewD1TaskRepository(client *D1Client) *D1TaskRepository {
	return &D1TaskRepository{Client: client}
}

func (r *D1TaskRepository) Create(ctx context.Context, task service.Task) error {
	if r == nil || r.Client == nil {
		return errors.New("storage: nil d1 task repository")
	}

	_, err := r.Client.Query(ctx, `
		INSERT INTO tasks (
			task_id, chat_id, user_id, target_chat_id, url, status, idempotency_key,
			retry_count, source_message_id, status_message_id, lease_id, output_summary, error_message, exit_code,
			created_at, updated_at, started_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.TaskID,
		task.ChatID,
		task.UserID,
		task.TargetChatID,
		task.URL,
		string(task.Status),
		task.IdempotencyKey,
		task.RetryCount,
		nullableInt64(task.SourceMessageID),
		nullableInt64(task.StatusMessageID),
		nullableString(task.LeaseID),
		nullableString(task.OutputSummary),
		nullableString(task.ErrorMessage),
		nullableInt(task.ExitCode),
		nullableTime(task.CreatedAt),
		nullableTime(task.UpdatedAt),
		nullableTimePointer(task.StartedAt),
		nullableTimePointer(task.FinishedAt),
	)
	if err != nil {
		return fmt.Errorf("storage: create task %q: %w", task.TaskID, err)
	}
	return nil
}

func (r *D1TaskRepository) Update(ctx context.Context, task service.Task) error {
	if r == nil || r.Client == nil {
		return errors.New("storage: nil d1 task repository")
	}

	result, err := r.Client.Query(ctx, `
		UPDATE tasks
		SET status = ?, retry_count = ?, source_message_id = ?, status_message_id = ?, lease_id = ?, output_summary = ?, error_message = ?,
		    exit_code = ?, updated_at = ?, started_at = ?, finished_at = ?
		WHERE task_id = ?`,
		string(task.Status),
		task.RetryCount,
		nullableInt64(task.SourceMessageID),
		nullableInt64(task.StatusMessageID),
		nullableString(task.LeaseID),
		nullableString(task.OutputSummary),
		nullableString(task.ErrorMessage),
		nullableInt(task.ExitCode),
		nullableTime(task.UpdatedAt),
		nullableTimePointer(task.StartedAt),
		nullableTimePointer(task.FinishedAt),
		task.TaskID,
	)
	if err != nil {
		return fmt.Errorf("storage: update task %q: %w", task.TaskID, err)
	}
	if result.Meta.Changes == 0 {
		return service.ErrTaskNotFound
	}
	return nil
}

func (r *D1TaskRepository) FindByID(ctx context.Context, taskID string) (service.Task, error) {
	if r == nil || r.Client == nil {
		return service.Task{}, errors.New("storage: nil d1 task repository")
	}

	result, err := r.Client.Query(ctx, `
		SELECT task_id, chat_id, user_id, target_chat_id, url, status, idempotency_key,
		       retry_count, source_message_id, status_message_id, lease_id, output_summary, error_message, exit_code,
		       created_at, updated_at, started_at, finished_at
		FROM tasks
		WHERE task_id = ?
		LIMIT 1`, taskID)
	if err != nil {
		return service.Task{}, fmt.Errorf("storage: find task by id %q: %w", taskID, err)
	}
	if len(result.Results) == 0 {
		return service.Task{}, service.ErrTaskNotFound
	}

	task, err := taskFromResultRow(result.Results[0])
	if err != nil {
		return service.Task{}, fmt.Errorf("storage: decode task by id %q: %w", taskID, err)
	}
	return task, nil
}

func (r *D1TaskRepository) FindByIdempotencyKey(ctx context.Context, idempotencyKey string) (service.Task, error) {
	if r == nil || r.Client == nil {
		return service.Task{}, errors.New("storage: nil d1 task repository")
	}

	result, err := r.Client.Query(ctx, `
		SELECT task_id, chat_id, user_id, target_chat_id, url, status, idempotency_key,
		       retry_count, source_message_id, status_message_id, lease_id, output_summary, error_message, exit_code,
		       created_at, updated_at, started_at, finished_at
		FROM tasks
		WHERE idempotency_key = ?
		ORDER BY created_at DESC
		LIMIT 1`, idempotencyKey)
	if err != nil {
		return service.Task{}, fmt.Errorf("storage: find task by idempotency key %q: %w", idempotencyKey, err)
	}
	if len(result.Results) == 0 {
		return service.Task{}, service.ErrTaskNotFound
	}

	task, err := taskFromResultRow(result.Results[0])
	if err != nil {
		return service.Task{}, fmt.Errorf("storage: decode task by idempotency key %q: %w", idempotencyKey, err)
	}
	return task, nil
}

func (r *D1TaskRepository) ListActiveByUser(ctx context.Context, userID int64, limit int) ([]service.Task, error) {
	if r == nil || r.Client == nil {
		return nil, errors.New("storage: nil d1 task repository")
	}
	if limit <= 0 {
		limit = 20
	}

	result, err := r.Client.Query(ctx, `
		SELECT task_id, chat_id, user_id, target_chat_id, url, status, idempotency_key,
		       retry_count, source_message_id, status_message_id, lease_id, output_summary, error_message, exit_code,
		       created_at, updated_at, started_at, finished_at
		FROM tasks
		WHERE user_id = ?
		  AND status IN (?, ?, ?)
		ORDER BY
		  CASE status
		    WHEN ? THEN 0
		    WHEN ? THEN 1
		    WHEN ? THEN 2
		    ELSE 3
		  END ASC,
		  created_at ASC
		LIMIT ?`,
		userID,
		string(service.StatusRunning),
		string(service.StatusQueued),
		string(service.StatusRetrying),
		string(service.StatusRunning),
		string(service.StatusRetrying),
		string(service.StatusQueued),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: list active tasks for user %d: %w", userID, err)
	}

	tasks := make([]service.Task, 0, len(result.Results))
	for _, row := range result.Results {
		task, err := taskFromResultRow(row)
		if err != nil {
			return nil, fmt.Errorf("storage: decode active task row: %w", err)
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (r *D1TaskRepository) ListFailedForRetry(ctx context.Context, maxRetryCount int, limit int) ([]service.Task, error) {
	if r == nil || r.Client == nil {
		return nil, errors.New("storage: nil d1 task repository")
	}
	if maxRetryCount <= 0 {
		return nil, errors.New("storage: max retry count must be greater than zero")
	}
	if limit <= 0 {
		limit = 100
	}

	result, err := r.Client.Query(ctx, `
		SELECT task_id, chat_id, user_id, target_chat_id, url, status, idempotency_key,
		       retry_count, source_message_id, status_message_id, lease_id, output_summary, error_message, exit_code,
		       created_at, updated_at, started_at, finished_at
		FROM tasks
		WHERE status IN (?, ?)
		  AND retry_count < ?
		ORDER BY updated_at ASC
		LIMIT ?`,
		string(service.StatusFailed),
		string(service.StatusDeadLettered),
		maxRetryCount,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: list failed tasks for retry: %w", err)
	}

	tasks := make([]service.Task, 0, len(result.Results))
	for _, row := range result.Results {
		task, err := taskFromResultRow(row)
		if err != nil {
			return nil, fmt.Errorf("storage: decode failed retry task row: %w", err)
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (r *D1TaskRepository) DeleteFailedByIdempotencyKey(ctx context.Context, idempotencyKey string) (int64, error) {
	if r == nil || r.Client == nil {
		return 0, errors.New("storage: nil d1 task repository")
	}

	result, err := r.Client.Query(ctx, `
		DELETE FROM tasks
		WHERE idempotency_key = ?
		  AND status IN (?, ?)`,
		idempotencyKey,
		string(service.StatusFailed),
		string(service.StatusDeadLettered),
	)
	if err != nil {
		return 0, fmt.Errorf("storage: delete failed tasks by idempotency key %q: %w", idempotencyKey, err)
	}
	return result.Meta.Changes, nil
}

func (r *D1TaskRepository) DeletePendingByUserTaskID(ctx context.Context, userID int64, taskID string) (int64, error) {
	if r == nil || r.Client == nil {
		return 0, errors.New("storage: nil d1 task repository")
	}

	result, err := r.Client.Query(ctx, `
		DELETE FROM tasks
		WHERE task_id = ?
		  AND user_id = ?
		  AND status IN (?, ?)`,
		taskID,
		userID,
		string(service.StatusQueued),
		string(service.StatusRetrying),
	)
	if err != nil {
		return 0, fmt.Errorf("storage: delete pending task %q for user %d: %w", taskID, userID, err)
	}
	return result.Meta.Changes, nil
}

func (r *D1TaskRepository) DeleteNonRunningByUserTaskID(ctx context.Context, userID int64, taskID string) (int64, error) {
	if r == nil || r.Client == nil {
		return 0, errors.New("storage: nil d1 task repository")
	}

	result, err := r.Client.Query(ctx, `
		DELETE FROM tasks
		WHERE task_id = ?
		  AND user_id = ?
		  AND status <> ?`,
		taskID,
		userID,
		string(service.StatusRunning),
	)
	if err != nil {
		return 0, fmt.Errorf("storage: delete non-running task %q for user %d: %w", taskID, userID, err)
	}
	return result.Meta.Changes, nil
}

func (r *D1TaskRepository) ListRecentByUser(ctx context.Context, userID int64, limit int) ([]service.Task, error) {
	if r == nil || r.Client == nil {
		return nil, errors.New("storage: nil d1 task repository")
	}
	if limit <= 0 {
		limit = 10
	}

	result, err := r.Client.Query(ctx, `
		SELECT task_id, chat_id, user_id, target_chat_id, url, status, idempotency_key,
		       retry_count, source_message_id, status_message_id, lease_id, output_summary, error_message, exit_code,
		       created_at, updated_at, started_at, finished_at
		FROM tasks
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT ?`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("storage: list recent tasks for user %d: %w", userID, err)
	}

	tasks := make([]service.Task, 0, len(result.Results))
	for _, row := range result.Results {
		task, err := taskFromResultRow(row)
		if err != nil {
			return nil, fmt.Errorf("storage: decode recent task row: %w", err)
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (r *D1TaskRepository) ClaimForExecution(ctx context.Context, taskID, leaseID string, startedAt time.Time) (service.Task, bool, error) {
	if r == nil || r.Client == nil {
		return service.Task{}, false, errors.New("storage: nil d1 task repository")
	}
	if taskID == "" {
		return service.Task{}, false, errors.New("storage: task id is required")
	}
	if leaseID == "" {
		return service.Task{}, false, errors.New("storage: lease id is required")
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	updatedAt := startedAt.UTC()

	result, err := r.Client.Query(ctx, `
		UPDATE tasks
		SET status = ?, lease_id = ?, started_at = ?, updated_at = ?, error_message = NULL
		WHERE task_id = ?
		  AND status IN (?, ?, ?, ?)`,
		string(service.StatusRunning),
		leaseID,
		nullableTime(startedAt),
		nullableTime(updatedAt),
		taskID,
		string(service.StatusQueued),
		string(service.StatusRetrying),
		string(service.StatusFailed),
		string(service.StatusDeadLettered),
	)
	if err != nil {
		return service.Task{}, false, fmt.Errorf("storage: claim task %q: %w", taskID, err)
	}
	if result.Meta.Changes == 0 {
		return service.Task{}, false, nil
	}

	task, err := r.FindByID(ctx, taskID)
	if err != nil {
		return service.Task{}, false, err
	}
	return task, true, nil
}

func taskFromResultRow(row map[string]any) (service.Task, error) {
	taskID, err := mustString(row, "task_id")
	if err != nil {
		return service.Task{}, err
	}
	chatID, err := mustInt64(row, "chat_id")
	if err != nil {
		return service.Task{}, err
	}
	userID, err := mustInt64(row, "user_id")
	if err != nil {
		return service.Task{}, err
	}
	targetChatID, err := mustInt64(row, "target_chat_id")
	if err != nil {
		return service.Task{}, err
	}
	url, err := mustString(row, "url")
	if err != nil {
		return service.Task{}, err
	}
	status, err := mustString(row, "status")
	if err != nil {
		return service.Task{}, err
	}
	idempotencyKey, err := mustString(row, "idempotency_key")
	if err != nil {
		return service.Task{}, err
	}
	retryCount, err := mustInt(row, "retry_count")
	if err != nil {
		return service.Task{}, err
	}
	createdAt, err := mustTime(row, "created_at")
	if err != nil {
		return service.Task{}, err
	}
	updatedAt, err := mustTime(row, "updated_at")
	if err != nil {
		return service.Task{}, err
	}

	task := service.Task{
		TaskID:         taskID,
		ChatID:         chatID,
		UserID:         userID,
		TargetChatID:   targetChatID,
		URL:            url,
		Status:         service.Status(status),
		IdempotencyKey: idempotencyKey,
		RetryCount:     retryCount,
		CreatedAt:      createdAt.UTC(),
		UpdatedAt:      updatedAt.UTC(),
	}

	if task.SourceMessageID, err = nullableInt64From(row, "source_message_id"); err != nil {
		return service.Task{}, err
	}
	if task.StatusMessageID, err = nullableInt64From(row, "status_message_id"); err != nil {
		return service.Task{}, err
	}
	if task.LeaseID, err = nullableStringFrom(row, "lease_id"); err != nil {
		return service.Task{}, err
	}
	if task.OutputSummary, err = nullableStringFrom(row, "output_summary"); err != nil {
		return service.Task{}, err
	}
	if task.ErrorMessage, err = nullableStringFrom(row, "error_message"); err != nil {
		return service.Task{}, err
	}
	if task.ExitCode, err = nullableIntFrom(row, "exit_code"); err != nil {
		return service.Task{}, err
	}
	if task.StartedAt, err = nullableTimeFrom(row, "started_at"); err != nil {
		return service.Task{}, err
	}
	if task.FinishedAt, err = nullableTimeFrom(row, "finished_at"); err != nil {
		return service.Task{}, err
	}

	return task, nil
}

func mustString(row map[string]any, key string) (string, error) {
	value, ok := row[key]
	if !ok || value == nil {
		return "", fmt.Errorf("storage: missing required column %q", key)
	}
	out, err := coerceString(value)
	if err != nil {
		return "", fmt.Errorf("storage: invalid %s: %w", key, err)
	}
	return out, nil
}

func mustInt64(row map[string]any, key string) (int64, error) {
	value, ok := row[key]
	if !ok || value == nil {
		return 0, fmt.Errorf("storage: missing required column %q", key)
	}
	out, err := coerceInt64(value)
	if err != nil {
		return 0, fmt.Errorf("storage: invalid %s: %w", key, err)
	}
	return out, nil
}

func mustInt(row map[string]any, key string) (int, error) {
	v, err := mustInt64(row, key)
	if err != nil {
		return 0, err
	}
	return int(v), nil
}

func mustTime(row map[string]any, key string) (time.Time, error) {
	value, ok := row[key]
	if !ok || value == nil {
		return time.Time{}, fmt.Errorf("storage: missing required column %q", key)
	}
	out, err := coerceTime(value)
	if err != nil {
		return time.Time{}, fmt.Errorf("storage: invalid %s: %w", key, err)
	}
	return out, nil
}

func nullableStringFrom(row map[string]any, key string) (*string, error) {
	value, ok := row[key]
	if !ok || value == nil {
		return nil, nil
	}
	out, err := coerceString(value)
	if err != nil {
		return nil, fmt.Errorf("storage: invalid %s: %w", key, err)
	}
	return &out, nil
}

func nullableInt64From(row map[string]any, key string) (*int64, error) {
	value, ok := row[key]
	if !ok || value == nil {
		return nil, nil
	}
	out, err := coerceInt64(value)
	if err != nil {
		return nil, fmt.Errorf("storage: invalid %s: %w", key, err)
	}
	return &out, nil
}

func nullableIntFrom(row map[string]any, key string) (*int, error) {
	v, err := nullableInt64From(row, key)
	if err != nil || v == nil {
		return nil, err
	}
	out := int(*v)
	return &out, nil
}

func nullableTimeFrom(row map[string]any, key string) (*time.Time, error) {
	value, ok := row[key]
	if !ok || value == nil {
		return nil, nil
	}
	out, err := coerceTime(value)
	if err != nil {
		return nil, fmt.Errorf("storage: invalid %s: %w", key, err)
	}
	out = out.UTC()
	return &out, nil
}

func coerceString(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case json.Number:
		return v.String(), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case int:
		return strconv.Itoa(v), nil
	default:
		return "", fmt.Errorf("unexpected type %T", value)
	}
}

func coerceInt64(value any) (int64, error) {
	switch v := value.(type) {
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	case json.Number:
		return v.Int64()
	case string:
		return strconv.ParseInt(v, 10, 64)
	default:
		return 0, fmt.Errorf("unexpected type %T", value)
	}
}

func coerceTime(value any) (time.Time, error) {
	raw, err := coerceString(value)
	if err != nil {
		return time.Time{}, err
	}
	raw = stringsTrimSpace(raw)
	if raw == "" {
		return time.Time{}, errors.New("empty datetime")
	}

	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05",
	}
	for _, format := range formats {
		if parsed, err := time.Parse(format, raw); err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported datetime format %q", raw)
}

func stringsTrimSpace(raw string) string {
	return strings.TrimSpace(raw)
}

func nullableString(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableTimePointer(v *time.Time) any {
	if v == nil {
		return nil
	}
	return nullableTime(*v)
}

func nullableTime(v time.Time) any {
	if v.IsZero() {
		return nil
	}
	return v.UTC().Format(time.RFC3339Nano)
}
