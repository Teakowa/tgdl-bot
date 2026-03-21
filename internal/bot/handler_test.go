package bot

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"tgdl-bot/internal/service"
)

type fakeTaskQuery struct {
	task  service.Task
	tasks []service.Task
	err   error
}

func (f fakeTaskQuery) GetTask(context.Context, string) (service.Task, error) {
	if f.err != nil {
		return service.Task{}, f.err
	}
	return f.task, nil
}

func (f fakeTaskQuery) ListRecentTasks(context.Context, int64, int) ([]service.Task, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tasks, nil
}

func TestHandlerStatusCommand(t *testing.T) {
	now := time.Now()
	h := Handler{
		Tasks: fakeTaskQuery{task: service.Task{
			TaskID:    "task-1",
			Status:    service.StatusDone,
			URL:       "https://t.me/c/1/2",
			CreatedAt: now,
		}},
	}

	reply, err := h.HandleText(context.Background(), 1, 1, "/status task-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reply, "Task ID: task-1") {
		t.Fatalf("unexpected reply: %s", reply)
	}
}

func TestHandlerLastCommand(t *testing.T) {
	h := Handler{
		Tasks: fakeTaskQuery{tasks: []service.Task{
			{TaskID: "a", Status: service.StatusQueued, URL: "https://t.me/c/1/2"},
			{TaskID: "b", Status: service.StatusDone, URL: "https://t.me/c/1/3"},
		}},
	}

	reply, err := h.HandleText(context.Background(), 1, 1, "/last")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reply, "最近任务:") || !strings.Contains(reply, "a | queued") {
		t.Fatalf("unexpected reply: %s", reply)
	}
}

func TestHandlerDeniedUser(t *testing.T) {
	h := Handler{AllowedUserIDs: []int64{100}}
	reply, err := h.HandleText(context.Background(), 200, 1, "/start")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reply, "无权限") {
		t.Fatalf("unexpected reply: %s", reply)
	}
}

func TestHandlerTaskQueryError(t *testing.T) {
	h := Handler{Tasks: fakeTaskQuery{err: errors.New("boom")}}
	_, err := h.HandleText(context.Background(), 1, 1, "/status t1")
	if err == nil {
		t.Fatal("expected error")
	}
}
