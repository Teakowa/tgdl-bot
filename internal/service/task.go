package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type Status string

const (
	StatusQueued       Status = "queued"
	StatusRunning      Status = "running"
	StatusDone         Status = "done"
	StatusFailed       Status = "failed"
	StatusRetrying     Status = "retrying"
	StatusDeadLettered Status = "dead_lettered"
)

var validStatuses = map[Status]struct{}{
	StatusQueued:       {},
	StatusRunning:      {},
	StatusDone:         {},
	StatusFailed:       {},
	StatusRetrying:     {},
	StatusDeadLettered: {},
}

func (s Status) Valid() bool {
	_, ok := validStatuses[s]
	return ok
}

func IsValidStatus(status string) bool {
	return Status(status).Valid()
}

type Task struct {
	TaskID         string     `json:"task_id"`
	ChatID         int64      `json:"chat_id"`
	UserID         int64      `json:"user_id"`
	TargetChatID   int64      `json:"target_chat_id"`
	URL            string     `json:"url"`
	Status         Status     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	RetryCount     int        `json:"retry_count"`
	LeaseID        *string    `json:"lease_id,omitempty"`
	OutputSummary  *string    `json:"output_summary,omitempty"`
	ErrorMessage   *string    `json:"error_message,omitempty"`
	ExitCode       *int       `json:"exit_code,omitempty"`
	IdempotencyKey string     `json:"idempotency_key"`
}

type CreateQueuedTaskRequest struct {
	TaskID         string
	ChatID         int64
	UserID         int64
	TargetChatID   int64
	URL            string
	IdempotencyKey string
}

type TaskUpdate struct {
	Status        Status
	RetryCount    *int
	LeaseID       *string
	OutputSummary *string
	ErrorMessage  *string
	ExitCode      *int
	StartedAt     *time.Time
	FinishedAt    *time.Time
}

type TaskService interface {
	CreateQueuedTask(ctx context.Context, req CreateQueuedTaskRequest) (Task, error)
	GetTask(ctx context.Context, taskID string) (Task, error)
	ListRecentTasks(ctx context.Context, userID int64, limit int) ([]Task, error)
	ListFailedTasksForRetry(ctx context.Context, maxRetryCount int, limit int) ([]Task, error)
	FindByIdempotencyKey(ctx context.Context, idempotencyKey string) (Task, error)
	DeleteFailedByIdempotencyKey(ctx context.Context, idempotencyKey string) (int64, error)
	UpdateTask(ctx context.Context, taskID string, update TaskUpdate) error
}

func NewIdempotencyKey(userID int64, normalizedURL string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%s", userID, strings.TrimSpace(normalizedURL))))
	return hex.EncodeToString(sum[:])
}
