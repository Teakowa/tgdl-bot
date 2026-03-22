package bot

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"tgdl-bot/internal/queue"
	"tgdl-bot/internal/service"
)

type fakeTaskQuery struct {
	task            service.Task
	tasks           []service.Task
	err             error
	createFn        func(service.CreateQueuedTaskRequest) (service.Task, error)
	deleteFailedFn  func(string) (int64, error)
	updateTaskCalls int
	lastUpdate      *service.TaskUpdate
	updatedTaskID   string
}

type fakeQueue struct {
	messages []queue.Message
	err      error
}

func (f *fakeQueue) Enqueue(_ context.Context, msg queue.Message) error {
	if f.err != nil {
		return f.err
	}
	f.messages = append(f.messages, msg)
	return nil
}

func (f *fakeQueue) EnqueueBatch(context.Context, []queue.Message) error {
	return f.err
}

func (f *fakeTaskQuery) CreateQueuedTask(_ context.Context, req service.CreateQueuedTaskRequest) (service.Task, error) {
	if f.createFn != nil {
		return f.createFn(req)
	}
	if f.err != nil {
		return service.Task{}, f.err
	}
	if f.task.TaskID == "" {
		return service.Task{TaskID: "task-created", Status: service.StatusQueued}, nil
	}
	return f.task, nil
}

func (f *fakeTaskQuery) GetTask(context.Context, string) (service.Task, error) {
	if f.err != nil {
		return service.Task{}, f.err
	}
	return f.task, nil
}

func (f *fakeTaskQuery) ListRecentTasks(context.Context, int64, int) ([]service.Task, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tasks, nil
}

func (f *fakeTaskQuery) ListFailedTasksForRetry(context.Context, int, int) ([]service.Task, error) {
	if f.err != nil {
		return nil, f.err
	}
	return nil, nil
}

func (f *fakeTaskQuery) FindByIdempotencyKey(context.Context, string) (service.Task, error) {
	if f.err != nil {
		return service.Task{}, f.err
	}
	return f.task, nil
}

func (f *fakeTaskQuery) DeleteFailedByIdempotencyKey(_ context.Context, key string) (int64, error) {
	if f.deleteFailedFn != nil {
		return f.deleteFailedFn(key)
	}
	if f.err != nil {
		return 0, f.err
	}
	return 0, nil
}

func (f *fakeTaskQuery) UpdateTask(_ context.Context, taskID string, update service.TaskUpdate) error {
	f.updateTaskCalls++
	f.updatedTaskID = taskID
	copied := update
	f.lastUpdate = &copied
	return f.err
}

func TestHandlerStatusCommand(t *testing.T) {
	now := time.Now()
	h := Handler{
		Tasks: &fakeTaskQuery{task: service.Task{
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
		Tasks: &fakeTaskQuery{tasks: []service.Task{
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
	h := Handler{Tasks: &fakeTaskQuery{err: errors.New("boom")}}
	_, err := h.HandleText(context.Background(), 1, 1, "/status t1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHandlerCreatesTaskFromURL(t *testing.T) {
	q := &fakeQueue{}
	tasks := &fakeTaskQuery{}
	var capturedReq service.CreateQueuedTaskRequest
	tasks.createFn = func(req service.CreateQueuedTaskRequest) (service.Task, error) {
		capturedReq = req
		return service.Task{
			TaskID:         req.TaskID,
			ChatID:         req.ChatID,
			UserID:         req.UserID,
			TargetChatID:   req.TargetChatID,
			URL:            req.URL,
			IdempotencyKey: req.IdempotencyKey,
			Status:         service.StatusQueued,
			CreatedAt:      time.Now().UTC(),
		}, nil
	}
	h := Handler{
		Tasks: tasks,
		Queue: q,
	}

	reply, err := h.HandleText(context.Background(), 1, 1, "https://t.me/c/1/2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reply, "Task ID:") {
		t.Fatalf("unexpected reply: %s", reply)
	}
	if len(q.messages) != 1 {
		t.Fatalf("expected queue enqueue call, got %d", len(q.messages))
	}
	if capturedReq.TargetChatID != 0 {
		t.Fatalf("expected create request target chat id to be omitted, got %d", capturedReq.TargetChatID)
	}
	if q.messages[0].TargetChatID != 0 {
		t.Fatalf("expected queue message target chat id to be omitted, got %d", q.messages[0].TargetChatID)
	}
}

func TestHandlerDuplicateURLReturnsExistingTask(t *testing.T) {
	q := &fakeQueue{}
	tasks := &fakeTaskQuery{task: service.Task{
		TaskID: "existing-task",
		Status: service.StatusRunning,
	}}
	h := Handler{
		Tasks: tasks,
		Queue: q,
	}

	reply, err := h.HandleText(context.Background(), 1, 1, "https://t.me/c/1/2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reply, "任务已存在") {
		t.Fatalf("unexpected reply: %s", reply)
	}
	if len(q.messages) != 0 {
		t.Fatalf("expected no enqueue for duplicate running task, got %d", len(q.messages))
	}
}

func TestHandlerDuplicateFailedTaskRebuildsAndEnqueues(t *testing.T) {
	q := &fakeQueue{}
	tasks := &fakeTaskQuery{}
	createCalls := 0
	tasks.createFn = func(req service.CreateQueuedTaskRequest) (service.Task, error) {
		createCalls++
		if createCalls == 1 {
			return service.Task{
				TaskID: "failed-task",
				Status: service.StatusFailed,
				URL:    req.URL,
			}, nil
		}
		return service.Task{
			TaskID:         req.TaskID,
			ChatID:         req.ChatID,
			UserID:         req.UserID,
			TargetChatID:   req.TargetChatID,
			URL:            req.URL,
			IdempotencyKey: req.IdempotencyKey,
			Status:         service.StatusQueued,
			CreatedAt:      time.Now().UTC(),
		}, nil
	}

	deleteCalls := 0
	tasks.deleteFailedFn = func(string) (int64, error) {
		deleteCalls++
		return 1, nil
	}

	h := Handler{
		Tasks: tasks,
		Queue: q,
	}

	reply, err := h.HandleText(context.Background(), 1, 1, "https://t.me/c/1/2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reply, "任务已入队") {
		t.Fatalf("unexpected reply: %s", reply)
	}
	if createCalls != 2 {
		t.Fatalf("expected 2 create calls, got %d", createCalls)
	}
	if deleteCalls != 1 {
		t.Fatalf("expected 1 delete call, got %d", deleteCalls)
	}
	if len(q.messages) != 1 {
		t.Fatalf("expected 1 enqueue, got %d", len(q.messages))
	}
}

func TestHandlerDuplicateDeadLetteredTaskRebuildsAndEnqueues(t *testing.T) {
	q := &fakeQueue{}
	tasks := &fakeTaskQuery{}
	createCalls := 0
	tasks.createFn = func(req service.CreateQueuedTaskRequest) (service.Task, error) {
		createCalls++
		if createCalls == 1 {
			return service.Task{
				TaskID: "dead-task",
				Status: service.StatusDeadLettered,
				URL:    req.URL,
			}, nil
		}
		return service.Task{
			TaskID:         req.TaskID,
			ChatID:         req.ChatID,
			UserID:         req.UserID,
			TargetChatID:   req.TargetChatID,
			URL:            req.URL,
			IdempotencyKey: req.IdempotencyKey,
			Status:         service.StatusQueued,
			CreatedAt:      time.Now().UTC(),
		}, nil
	}
	tasks.deleteFailedFn = func(string) (int64, error) { return 1, nil }

	h := Handler{
		Tasks: tasks,
		Queue: q,
	}

	reply, err := h.HandleText(context.Background(), 1, 1, "https://t.me/c/1/2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reply, "任务已入队") {
		t.Fatalf("unexpected reply: %s", reply)
	}
	if len(q.messages) != 1 {
		t.Fatalf("expected 1 enqueue, got %d", len(q.messages))
	}
}

func TestHandleTextWithOutcomeReturnsReactionEmoji(t *testing.T) {
	tasks := &fakeTaskQuery{task: service.Task{
		TaskID: "existing-task",
		Status: service.StatusRetrying,
	}}
	h := Handler{
		Tasks: tasks,
		Queue: &fakeQueue{},
	}

	outcome, err := h.HandleTextWithOutcome(context.Background(), 1, 1, "https://t.me/c/1/2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.ReactionEmoji != "🔄" {
		t.Fatalf("unexpected reaction emoji: %q", outcome.ReactionEmoji)
	}
}

func TestHandlerCreateTaskEmitsLifecycleLogs(t *testing.T) {
	q := &fakeQueue{}
	tasks := &fakeTaskQuery{}
	tasks.createFn = func(req service.CreateQueuedTaskRequest) (service.Task, error) {
		return service.Task{
			TaskID:         req.TaskID,
			ChatID:         req.ChatID,
			UserID:         req.UserID,
			TargetChatID:   req.TargetChatID,
			URL:            req.URL,
			IdempotencyKey: req.IdempotencyKey,
			Status:         service.StatusQueued,
			CreatedAt:      time.Now().UTC(),
		}, nil
	}

	logs := newLogRecorder()
	h := Handler{
		Tasks:  tasks,
		Queue:  q,
		Logger: logs.Logger(),
	}

	_, err := h.HandleText(context.Background(), 1, 1, "https://t.me/c/1/2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	messages := logs.Messages()
	for _, want := range []string{
		"bot task request parsed",
		"bot task created",
		"bot queue enqueue succeeded",
	} {
		if !containsLogMessage(messages, want) {
			t.Fatalf("expected log %q, got %v", want, messages)
		}
	}
}

func TestHandlerDuplicateTaskEmitsExistingHitLog(t *testing.T) {
	logs := newLogRecorder()
	h := Handler{
		Tasks: &fakeTaskQuery{task: service.Task{
			TaskID: "existing-task",
			Status: service.StatusRunning,
		}},
		Queue:  &fakeQueue{},
		Logger: logs.Logger(),
	}

	_, err := h.HandleText(context.Background(), 1, 1, "https://t.me/c/1/2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !containsLogMessage(logs.Messages(), "bot task existing hit") {
		t.Fatalf("expected existing-hit log, got %v", logs.Messages())
	}
}

func TestBindTaskMessageRefsUpdatesTaskMetadata(t *testing.T) {
	sourceID := int64(88)
	statusID := int64(99)
	tasks := &fakeTaskQuery{
		task: service.Task{TaskID: "task-1", Status: service.StatusQueued},
	}
	h := Handler{Tasks: tasks}

	task, err := h.BindTaskMessageRefs(context.Background(), "task-1", sourceID, statusID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.TaskID != "task-1" {
		t.Fatalf("unexpected task: %+v", task)
	}
	if tasks.updateTaskCalls != 1 {
		t.Fatalf("expected one update call, got %d", tasks.updateTaskCalls)
	}
	if tasks.lastUpdate == nil || tasks.lastUpdate.SourceMessageID == nil || *tasks.lastUpdate.SourceMessageID != sourceID {
		t.Fatalf("unexpected source message id update: %+v", tasks.lastUpdate)
	}
	if tasks.lastUpdate == nil || tasks.lastUpdate.StatusMessageID == nil || *tasks.lastUpdate.StatusMessageID != statusID {
		t.Fatalf("unexpected status message id update: %+v", tasks.lastUpdate)
	}
}

type logRecorder struct {
	handler *captureHandler
}

func newLogRecorder() *logRecorder {
	return &logRecorder{handler: &captureHandler{}}
}

func (r *logRecorder) Logger() *slog.Logger {
	return slog.New(r.handler)
}

func (r *logRecorder) Messages() []string {
	r.handler.mu.Lock()
	defer r.handler.mu.Unlock()

	out := make([]string, 0, len(r.handler.records))
	for _, record := range r.handler.records {
		out = append(out, record.Message)
	}
	return out
}

type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func containsLogMessage(messages []string, want string) bool {
	for _, msg := range messages {
		if msg == want {
			return true
		}
	}
	return false
}
