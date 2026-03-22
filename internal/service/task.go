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
	StatusPaused       Status = "paused"
	StatusCancelled    Status = "cancelled"
	StatusDone         Status = "done"
	StatusFailed       Status = "failed"
	StatusRetrying     Status = "retrying"
	StatusDeadLettered Status = "dead_lettered"
)

var validStatuses = map[Status]struct{}{
	StatusQueued:       {},
	StatusRunning:      {},
	StatusPaused:       {},
	StatusCancelled:    {},
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
	TaskID          string     `json:"task_id"`
	ChatID          int64      `json:"chat_id"`
	UserID          int64      `json:"user_id"`
	TargetPeer      string     `json:"target_peer,omitempty"`
	URL             string     `json:"url"`
	DropCaption     bool       `json:"drop_caption,omitempty"`
	Status          Status     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	SourceMessageID *int64     `json:"source_message_id,omitempty"`
	StatusMessageID *int64     `json:"status_message_id,omitempty"`
	RetryCount      int        `json:"retry_count"`
	LeaseID         *string    `json:"lease_id,omitempty"`
	OutputSummary   *string    `json:"output_summary,omitempty"`
	ErrorMessage    *string    `json:"error_message,omitempty"`
	ExitCode        *int       `json:"exit_code,omitempty"`
	IdempotencyKey  string     `json:"idempotency_key"`
}

type CreateQueuedTaskRequest struct {
	TaskID         string
	ChatID         int64
	UserID         int64
	TargetPeer     string
	URL            string
	DropCaption    bool
	IdempotencyKey string
}

type TaskUpdate struct {
	Status          Status
	RetryCount      *int
	LeaseID         *string
	OutputSummary   *string
	ErrorMessage    *string
	ExitCode        *int
	StartedAt       *time.Time
	FinishedAt      *time.Time
	SourceMessageID *int64
	StatusMessageID *int64
}

type ClaimTaskExecutionRequest struct {
	TaskID    string
	LeaseID   string
	StartedAt time.Time
}

type TaskService interface {
	CreateQueuedTask(ctx context.Context, req CreateQueuedTaskRequest) (Task, error)
	GetTask(ctx context.Context, taskID string) (Task, error)
	ListRecentTasks(ctx context.Context, userID int64, limit int) ([]Task, error)
	ListActiveTasks(ctx context.Context, userID int64, limit int) ([]Task, error)
	ListQueueTasks(ctx context.Context, userID int64, limit int) ([]Task, error)
	ListFailedTasksForRetry(ctx context.Context, maxRetryCount int, limit int) ([]Task, error)
	FindByIdempotencyKey(ctx context.Context, idempotencyKey string) (Task, error)
	DeleteFailedByIdempotencyKey(ctx context.Context, idempotencyKey string) (int64, error)
	DeletePendingTask(ctx context.Context, userID int64, taskID string) (bool, error)
	DeleteTaskNonRunning(ctx context.Context, userID int64, taskID string) (bool, error)
	PauseTask(ctx context.Context, userID int64, taskID string) (bool, error)
	ResumeTask(ctx context.Context, userID int64, taskID string) (bool, error)
	CancelTask(ctx context.Context, userID int64, taskID string) (bool, error)
	ClaimTaskForExecution(ctx context.Context, req ClaimTaskExecutionRequest) (Task, bool, error)
	UpdateTask(ctx context.Context, taskID string, update TaskUpdate) error
}

func NewIdempotencyKey(userID int64, normalizedURL, normalizedTargetPeer string, dropCaption bool) string {
	mode := "keep-caption"
	if dropCaption {
		mode = "drop-caption"
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%s|%s",
		userID,
		strings.TrimSpace(normalizedURL),
		strings.TrimSpace(normalizedTargetPeer),
		mode,
	)))
	return hex.EncodeToString(sum[:])
}
