package taskevent

import (
	"strings"
	"time"

	"tgdl-bot/internal/queue"
	"tgdl-bot/internal/service"
)

type Event struct {
	TaskID     string         `json:"task_id"`
	Status     service.Status `json:"status"`
	RetryCount int            `json:"retry_count"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

func FromTask(task service.Task) Event {
	updatedAt := task.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	return Event{
		TaskID:     task.TaskID,
		Status:     task.Status,
		RetryCount: task.RetryCount,
		UpdatedAt:  updatedAt,
	}
}

func FromQueueMessage(message queue.Message) (Event, bool) {
	taskID := strings.TrimSpace(message.TaskID)
	if taskID == "" {
		return Event{}, false
	}

	updatedAt := message.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	return Event{
		TaskID:     taskID,
		Status:     service.Status(strings.TrimSpace(message.Status)),
		RetryCount: message.RetryCount,
		UpdatedAt:  updatedAt,
	}, true
}

func (e Event) ToQueueMessage() queue.Message {
	updatedAt := e.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	return queue.Message{
		TaskID:     strings.TrimSpace(e.TaskID),
		Status:     string(e.Status),
		RetryCount: e.RetryCount,
		UpdatedAt:  updatedAt,
		CreatedAt:  updatedAt,
	}
}
