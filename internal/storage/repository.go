package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"tgdl-bot/internal/service"
)

type TaskRepository interface {
	Create(ctx context.Context, task service.Task) error
	Update(ctx context.Context, task service.Task) error
	FindByID(ctx context.Context, taskID string) (service.Task, error)
	FindByIdempotencyKey(ctx context.Context, idempotencyKey string) (service.Task, error)
	ListFailedForRetry(ctx context.Context, maxRetryCount int, limit int) ([]service.Task, error)
	DeleteFailedByIdempotencyKey(ctx context.Context, idempotencyKey string) (int64, error)
	ListRecentByUser(ctx context.Context, userID int64, limit int) ([]service.Task, error)
}

type SQLiteTaskRepository struct {
	DB *sql.DB
}

func NewSQLiteTaskRepository(db *sql.DB) *SQLiteTaskRepository {
	return &SQLiteTaskRepository{DB: db}
}

func (r *SQLiteTaskRepository) Create(ctx context.Context, task service.Task) error {
	if r == nil || r.DB == nil {
		return errors.New("storage: nil task repository")
	}

	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO tasks (
			task_id, chat_id, user_id, target_chat_id, url, status, idempotency_key,
			retry_count, lease_id, output_summary, error_message, exit_code,
			created_at, updated_at, started_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.TaskID,
		task.ChatID,
		task.UserID,
		task.TargetChatID,
		task.URL,
		string(task.Status),
		task.IdempotencyKey,
		task.RetryCount,
		nullableString(task.LeaseID),
		nullableString(task.OutputSummary),
		nullableString(task.ErrorMessage),
		nullableInt(task.ExitCode),
		task.CreatedAt.UTC(),
		task.UpdatedAt.UTC(),
		nullableTime(task.StartedAt),
		nullableTime(task.FinishedAt),
	)
	if err != nil {
		return fmt.Errorf("storage: create task %q: %w", task.TaskID, err)
	}
	return nil
}

func (r *SQLiteTaskRepository) Update(ctx context.Context, task service.Task) error {
	if r == nil || r.DB == nil {
		return errors.New("storage: nil task repository")
	}

	res, err := r.DB.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, retry_count = ?, lease_id = ?, output_summary = ?, error_message = ?,
		    exit_code = ?, updated_at = ?, started_at = ?, finished_at = ?
		WHERE task_id = ?`,
		string(task.Status),
		task.RetryCount,
		nullableString(task.LeaseID),
		nullableString(task.OutputSummary),
		nullableString(task.ErrorMessage),
		nullableInt(task.ExitCode),
		task.UpdatedAt.UTC(),
		nullableTime(task.StartedAt),
		nullableTime(task.FinishedAt),
		task.TaskID,
	)
	if err != nil {
		return fmt.Errorf("storage: update task %q: %w", task.TaskID, err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("storage: update task %q rows affected: %w", task.TaskID, err)
	}
	if rows == 0 {
		return service.ErrTaskNotFound
	}
	return nil
}

func (r *SQLiteTaskRepository) FindByID(ctx context.Context, taskID string) (service.Task, error) {
	if r == nil || r.DB == nil {
		return service.Task{}, errors.New("storage: nil task repository")
	}
	row := r.DB.QueryRowContext(ctx, `
		SELECT task_id, chat_id, user_id, target_chat_id, url, status, idempotency_key,
		       retry_count, lease_id, output_summary, error_message, exit_code,
		       created_at, updated_at, started_at, finished_at
		FROM tasks
		WHERE task_id = ?`, taskID)
	task, err := scanTask(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.Task{}, service.ErrTaskNotFound
		}
		return service.Task{}, fmt.Errorf("storage: find task by id %q: %w", taskID, err)
	}
	return task, nil
}

func (r *SQLiteTaskRepository) FindByIdempotencyKey(ctx context.Context, idempotencyKey string) (service.Task, error) {
	if r == nil || r.DB == nil {
		return service.Task{}, errors.New("storage: nil task repository")
	}
	row := r.DB.QueryRowContext(ctx, `
		SELECT task_id, chat_id, user_id, target_chat_id, url, status, idempotency_key,
		       retry_count, lease_id, output_summary, error_message, exit_code,
		       created_at, updated_at, started_at, finished_at
		FROM tasks
		WHERE idempotency_key = ?
		ORDER BY created_at DESC
		LIMIT 1`, idempotencyKey)
	task, err := scanTask(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.Task{}, service.ErrTaskNotFound
		}
		return service.Task{}, fmt.Errorf("storage: find task by idempotency key %q: %w", idempotencyKey, err)
	}
	return task, nil
}

func (r *SQLiteTaskRepository) ListFailedForRetry(ctx context.Context, maxRetryCount int, limit int) ([]service.Task, error) {
	if r == nil || r.DB == nil {
		return nil, errors.New("storage: nil task repository")
	}
	if maxRetryCount <= 0 {
		return nil, errors.New("storage: max retry count must be greater than zero")
	}
	if limit <= 0 {
		limit = 100
	}

	rows, err := r.DB.QueryContext(ctx, `
		SELECT task_id, chat_id, user_id, target_chat_id, url, status, idempotency_key,
		       retry_count, lease_id, output_summary, error_message, exit_code,
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
	defer rows.Close()

	tasks := make([]service.Task, 0, limit)
	for rows.Next() {
		var row TaskRow
		if err := rows.Scan(
			&row.TaskID, &row.ChatID, &row.UserID, &row.TargetChatID, &row.URL, &row.Status, &row.IdempotencyKey,
			&row.RetryCount, &row.LeaseID, &row.OutputSummary, &row.ErrorMessage, &row.ExitCode,
			&row.CreatedAt, &row.UpdatedAt, &row.StartedAt, &row.FinishedAt,
		); err != nil {
			return nil, fmt.Errorf("storage: scan failed retry task row: %w", err)
		}
		tasks = append(tasks, fromRow(row))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: iterate failed retry task rows: %w", err)
	}

	return tasks, nil
}

func (r *SQLiteTaskRepository) DeleteFailedByIdempotencyKey(ctx context.Context, idempotencyKey string) (int64, error) {
	if r == nil || r.DB == nil {
		return 0, errors.New("storage: nil task repository")
	}

	res, err := r.DB.ExecContext(ctx, `
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

	rows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("storage: delete failed tasks rows affected: %w", err)
	}
	return rows, nil
}

func (r *SQLiteTaskRepository) ListRecentByUser(ctx context.Context, userID int64, limit int) ([]service.Task, error) {
	if r == nil || r.DB == nil {
		return nil, errors.New("storage: nil task repository")
	}
	if limit <= 0 {
		limit = 10
	}

	rows, err := r.DB.QueryContext(ctx, `
		SELECT task_id, chat_id, user_id, target_chat_id, url, status, idempotency_key,
		       retry_count, lease_id, output_summary, error_message, exit_code,
		       created_at, updated_at, started_at, finished_at
		FROM tasks
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT ?`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("storage: list recent tasks for user %d: %w", userID, err)
	}
	defer rows.Close()

	tasks := make([]service.Task, 0, limit)
	for rows.Next() {
		var row TaskRow
		if err := rows.Scan(
			&row.TaskID, &row.ChatID, &row.UserID, &row.TargetChatID, &row.URL, &row.Status, &row.IdempotencyKey,
			&row.RetryCount, &row.LeaseID, &row.OutputSummary, &row.ErrorMessage, &row.ExitCode,
			&row.CreatedAt, &row.UpdatedAt, &row.StartedAt, &row.FinishedAt,
		); err != nil {
			return nil, fmt.Errorf("storage: scan recent task row: %w", err)
		}
		tasks = append(tasks, fromRow(row))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: iterate recent tasks rows: %w", err)
	}
	return tasks, nil
}

type TaskRow struct {
	TaskID         string
	ChatID         int64
	UserID         int64
	TargetChatID   int64
	URL            string
	Status         string
	IdempotencyKey string
	RetryCount     int
	LeaseID        sql.NullString
	OutputSummary  sql.NullString
	ErrorMessage   sql.NullString
	ExitCode       sql.NullInt64
	CreatedAt      time.Time
	UpdatedAt      time.Time
	StartedAt      sql.NullTime
	FinishedAt     sql.NullTime
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTask(s scanner) (service.Task, error) {
	var row TaskRow
	if err := s.Scan(
		&row.TaskID, &row.ChatID, &row.UserID, &row.TargetChatID, &row.URL, &row.Status, &row.IdempotencyKey,
		&row.RetryCount, &row.LeaseID, &row.OutputSummary, &row.ErrorMessage, &row.ExitCode,
		&row.CreatedAt, &row.UpdatedAt, &row.StartedAt, &row.FinishedAt,
	); err != nil {
		return service.Task{}, err
	}
	return fromRow(row), nil
}

func fromRow(row TaskRow) service.Task {
	task := service.Task{
		TaskID:         row.TaskID,
		ChatID:         row.ChatID,
		UserID:         row.UserID,
		TargetChatID:   row.TargetChatID,
		URL:            row.URL,
		Status:         service.Status(row.Status),
		IdempotencyKey: row.IdempotencyKey,
		RetryCount:     row.RetryCount,
		CreatedAt:      row.CreatedAt.UTC(),
		UpdatedAt:      row.UpdatedAt.UTC(),
	}
	if row.LeaseID.Valid {
		v := row.LeaseID.String
		task.LeaseID = &v
	}
	if row.OutputSummary.Valid {
		v := row.OutputSummary.String
		task.OutputSummary = &v
	}
	if row.ErrorMessage.Valid {
		v := row.ErrorMessage.String
		task.ErrorMessage = &v
	}
	if row.ExitCode.Valid {
		v := int(row.ExitCode.Int64)
		task.ExitCode = &v
	}
	if row.StartedAt.Valid {
		v := row.StartedAt.Time.UTC()
		task.StartedAt = &v
	}
	if row.FinishedAt.Valid {
		v := row.FinishedAt.Time.UTC()
		task.FinishedAt = &v
	}
	return task
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

func nullableTime(v *time.Time) any {
	if v == nil {
		return nil
	}
	return v.UTC()
}
