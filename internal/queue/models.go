package queue

import (
	"context"
	"time"
)

type Producer interface {
	Enqueue(ctx context.Context, message Message) error
	EnqueueBatch(ctx context.Context, messages []Message) error
}

type Consumer interface {
	Pull(ctx context.Context, batchSize, visibilityTimeoutMs int) ([]ReceivedMessage, error)
	Ack(ctx context.Context, leaseIDs []string) error
	Retry(ctx context.Context, leaseIDs []string) error
	AckAndRetry(ctx context.Context, ackLeaseIDs, retryLeaseIDs []string) error
}

type Message struct {
	TaskID       string    `json:"task_id"`
	ChatID       int64     `json:"chat_id"`
	UserID       int64     `json:"user_id"`
	TargetChatID int64     `json:"target_chat_id,omitempty"`
	URL          string    `json:"url"`
	CreatedAt    time.Time `json:"created_at"`
	Idempotency  string    `json:"idempotency_key,omitempty"`
	Status       string    `json:"status,omitempty"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
	RetryCount   int       `json:"retry_count,omitempty"`
}

type ReceivedMessage struct {
	LeaseID string
	Body    Message
	RawBody []byte
}
