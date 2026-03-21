package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"tgdl-bot/internal/service"
)

type TaskRepository interface {
	Create(ctx context.Context, task service.Task) error
	Update(ctx context.Context, task service.Task) error
	FindByID(ctx context.Context, taskID string) (service.Task, error)
	FindByIdempotencyKey(ctx context.Context, idempotencyKey string) (service.Task, error)
	ListRecentByUser(ctx context.Context, userID int64, limit int) ([]service.Task, error)
}

type SQLiteTaskRepository struct {
	DB *sql.DB
}

func NewSQLiteTaskRepository(db *sql.DB) *SQLiteTaskRepository {
	return &SQLiteTaskRepository{DB: db}
}

func (r *SQLiteTaskRepository) Create(ctx context.Context, task service.Task) error {
	return fmt.Errorf("%w: create task", ErrNotImplemented)
}

func (r *SQLiteTaskRepository) Update(ctx context.Context, task service.Task) error {
	return fmt.Errorf("%w: update task", ErrNotImplemented)
}

func (r *SQLiteTaskRepository) FindByID(ctx context.Context, taskID string) (service.Task, error) {
	return service.Task{}, fmt.Errorf("%w: find task by id %q", ErrNotImplemented, taskID)
}

func (r *SQLiteTaskRepository) FindByIdempotencyKey(ctx context.Context, idempotencyKey string) (service.Task, error) {
	return service.Task{}, fmt.Errorf("%w: find task by idempotency key %q", ErrNotImplemented, idempotencyKey)
}

func (r *SQLiteTaskRepository) ListRecentByUser(ctx context.Context, userID int64, limit int) ([]service.Task, error) {
	return nil, fmt.Errorf("%w: list recent tasks for user %d", ErrNotImplemented, userID)
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
