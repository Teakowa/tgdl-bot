package taskevent

import (
	"testing"
	"time"

	"tgdl-bot/internal/queue"
	"tgdl-bot/internal/service"
)

func TestFromTaskAndToQueueMessage(t *testing.T) {
	updatedAt := time.Date(2026, 3, 22, 9, 0, 0, 0, time.UTC)
	event := FromTask(service.Task{
		TaskID:     "task-1",
		Status:     service.StatusRunning,
		RetryCount: 1,
		UpdatedAt:  updatedAt,
	})

	message := event.ToQueueMessage()
	if message.TaskID != "task-1" {
		t.Fatalf("expected task id task-1, got %s", message.TaskID)
	}
	if message.Status != string(service.StatusRunning) {
		t.Fatalf("expected running status, got %s", message.Status)
	}
	if message.RetryCount != 1 {
		t.Fatalf("expected retry count 1, got %d", message.RetryCount)
	}
	if !message.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("expected updated_at %s, got %s", updatedAt, message.UpdatedAt)
	}
}

func TestFromQueueMessage(t *testing.T) {
	updatedAt := time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC)
	event, ok := FromQueueMessage(queue.Message{
		TaskID:     "task-2",
		Status:     string(service.StatusDone),
		RetryCount: 2,
		UpdatedAt:  updatedAt,
	})
	if !ok {
		t.Fatal("expected queue message to parse")
	}
	if event.TaskID != "task-2" {
		t.Fatalf("expected task id task-2, got %s", event.TaskID)
	}
	if event.Status != service.StatusDone {
		t.Fatalf("expected done status, got %s", event.Status)
	}
	if event.RetryCount != 2 {
		t.Fatalf("expected retry count 2, got %d", event.RetryCount)
	}
	if !event.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("expected updated_at %s, got %s", updatedAt, event.UpdatedAt)
	}
}

func TestFromQueueMessageRejectsMissingTaskID(t *testing.T) {
	if _, ok := FromQueueMessage(queue.Message{}); ok {
		t.Fatal("expected missing task id to be rejected")
	}
}
