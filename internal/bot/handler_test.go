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
	task                 service.Task
	tasks                []service.Task
	activeTasks          []service.Task
	queueTasks           []service.Task
	err                  error
	createFn             func(service.CreateQueuedTaskRequest) (service.Task, error)
	getTaskFn            func(string) (service.Task, error)
	deleteFailedFn       func(string) (int64, error)
	deletePendingFn      func(int64, string) (bool, error)
	deleteNonRunningFn   func(int64, string) (bool, error)
	forceDeleteFn        func(int64, string) (bool, error)
	pauseFn              func(int64, string) (bool, error)
	resumeFn             func(int64, string) (bool, error)
	cancelFn             func(int64, string) (bool, error)
	deletePendingResp    bool
	deleteNonRunningResp bool
	forceDeleteResp      bool
	pauseResp            bool
	resumeResp           bool
	cancelResp           bool
	updateTaskCalls      int
	lastUpdate           *service.TaskUpdate
	updatedTaskID        string
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

func (f *fakeTaskQuery) GetTask(_ context.Context, taskID string) (service.Task, error) {
	if f.getTaskFn != nil {
		return f.getTaskFn(taskID)
	}
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

func (f *fakeTaskQuery) ListActiveTasks(context.Context, int64, int) ([]service.Task, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.activeTasks, nil
}

func (f *fakeTaskQuery) ListQueueTasks(context.Context, int64, int) ([]service.Task, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.queueTasks != nil {
		return f.queueTasks, nil
	}
	return f.activeTasks, nil
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

func (f *fakeTaskQuery) DeletePendingTask(_ context.Context, userID int64, taskID string) (bool, error) {
	if f.deletePendingFn != nil {
		return f.deletePendingFn(userID, taskID)
	}
	if f.err != nil {
		return false, f.err
	}
	return f.deletePendingResp, nil
}

func (f *fakeTaskQuery) DeleteTaskNonRunning(_ context.Context, userID int64, taskID string) (bool, error) {
	if f.deleteNonRunningFn != nil {
		return f.deleteNonRunningFn(userID, taskID)
	}
	if f.err != nil {
		return false, f.err
	}
	return f.deleteNonRunningResp, nil
}

func (f *fakeTaskQuery) ForceDeleteTask(_ context.Context, userID int64, taskID string) (bool, error) {
	if f.forceDeleteFn != nil {
		return f.forceDeleteFn(userID, taskID)
	}
	if f.err != nil {
		return false, f.err
	}
	return f.forceDeleteResp, nil
}

func (f *fakeTaskQuery) PauseTask(_ context.Context, userID int64, taskID string) (bool, error) {
	if f.pauseFn != nil {
		return f.pauseFn(userID, taskID)
	}
	if f.err != nil {
		return false, f.err
	}
	return f.pauseResp, nil
}

func (f *fakeTaskQuery) ResumeTask(_ context.Context, userID int64, taskID string) (bool, error) {
	if f.resumeFn != nil {
		return f.resumeFn(userID, taskID)
	}
	if f.err != nil {
		return false, f.err
	}
	return f.resumeResp, nil
}

func (f *fakeTaskQuery) CancelTask(_ context.Context, userID int64, taskID string) (bool, error) {
	if f.cancelFn != nil {
		return f.cancelFn(userID, taskID)
	}
	if f.err != nil {
		return false, f.err
	}
	return f.cancelResp, nil
}

func (f *fakeTaskQuery) ListStaleRunningTasks(context.Context, time.Time, int) ([]service.Task, error) {
	if f.err != nil {
		return nil, f.err
	}
	return nil, nil
}

func (f *fakeTaskQuery) RecoverRunningTaskAsFailed(context.Context, string, time.Time, time.Time, string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return false, nil
}

func (f *fakeTaskQuery) ClaimTaskForExecution(context.Context, service.ClaimTaskExecutionRequest) (service.Task, bool, error) {
	if f.err != nil {
		return service.Task{}, false, f.err
	}
	return f.task, false, nil
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

func TestHandlerQueueCommand(t *testing.T) {
	h := Handler{
		Tasks: &fakeTaskQuery{queueTasks: []service.Task{
			{TaskID: "task-a", Status: service.StatusRunning, URL: "https://t.me/c/1/2"},
			{TaskID: "task-b", Status: service.StatusDeadLettered, URL: "https://t.me/channel_name/3"},
		}},
	}

	outcome, err := h.HandleTextWithOutcome(context.Background(), 1, 1, "/queue")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Reply != "请选择要操作的任务：" {
		t.Fatalf("unexpected queue reply: %s", outcome.Reply)
	}
	if outcome.ReplyMarkup == nil || len(outcome.ReplyMarkup.InlineKeyboard) != 2 {
		t.Fatalf("expected queue task keyboard, got %+v", outcome.ReplyMarkup)
	}
	firstButton := outcome.ReplyMarkup.InlineKeyboard[0][0]
	if strings.Contains(firstButton.Text, "https://t.me/") {
		t.Fatalf("expected compact queue button label, got %q", firstButton.Text)
	}
	if !strings.Contains(firstButton.Text, "⚡") || !strings.Contains(firstButton.Text, "c/1/2") {
		t.Fatalf("unexpected first queue button label: %q", firstButton.Text)
	}
}

func TestHandlerDeleteCommandWithoutTaskIDShowsQueueSelection(t *testing.T) {
	h := Handler{
		Tasks: &fakeTaskQuery{queueTasks: []service.Task{
			{TaskID: "task-a", Status: service.StatusRunning, URL: "https://t.me/c/1/2"},
			{TaskID: "task-b", Status: service.StatusQueued, URL: "https://t.me/c/1/3"},
		}},
	}

	outcome, err := h.HandleTextWithOutcome(context.Background(), 1, 1, "/delete")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Reply != "请选择要删除的任务：" {
		t.Fatalf("unexpected delete selection reply: %s", outcome.Reply)
	}
	if outcome.ReplyMarkup == nil || len(outcome.ReplyMarkup.InlineKeyboard) != 2 {
		t.Fatalf("expected delete selection keyboard, got %+v", outcome.ReplyMarkup)
	}
}

func TestHandlerDeleteCommandWithoutTaskIDShowsEmptyQueue(t *testing.T) {
	h := Handler{
		Tasks: &fakeTaskQuery{queueTasks: nil},
	}

	outcome, err := h.HandleTextWithOutcome(context.Background(), 1, 1, "/delete")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Reply != "当前无可操作任务。" {
		t.Fatalf("unexpected empty delete reply: %s", outcome.Reply)
	}
	if outcome.ReplyMarkup != nil {
		t.Fatalf("expected no keyboard for empty delete queue, got %+v", outcome.ReplyMarkup)
	}
}

func TestHandlerDeleteCommand(t *testing.T) {
	tasks := &fakeTaskQuery{
		task: service.Task{
			TaskID: "task-a",
			UserID: 1,
			Status: service.StatusQueued,
		},
	}
	h := Handler{Tasks: tasks}

	outcome, err := h.HandleTextWithOutcome(context.Background(), 1, 1, "/delete task-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(outcome.Reply, "确认删除任务") {
		t.Fatalf("unexpected delete reply: %s", outcome.Reply)
	}
	if outcome.ReplyMarkup == nil {
		t.Fatalf("expected delete confirmation keyboard, got %+v", outcome.ReplyMarkup)
	}
}

func TestHandlerDeleteCommandShowsRunningTaskMenu(t *testing.T) {
	tasks := &fakeTaskQuery{
		task: service.Task{
			TaskID: "task-a",
			UserID: 1,
			Status: service.StatusRunning,
		},
	}
	h := Handler{Tasks: tasks}

	outcome, err := h.HandleTextWithOutcome(context.Background(), 1, 1, "/delete task-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(outcome.Reply, "可强制删除记录") {
		t.Fatalf("unexpected running delete reply: %s", outcome.Reply)
	}
	if outcome.ReplyMarkup == nil || outcome.ReplyMarkup.InlineKeyboard[0][0].Text != "强制删除" {
		t.Fatalf("expected running delete menu, got %+v", outcome.ReplyMarkup)
	}
}

func TestHandlerDeleteCommandForce(t *testing.T) {
	tasks := &fakeTaskQuery{
		task: service.Task{
			TaskID: "task-a",
			UserID: 1,
			Status: service.StatusFailed,
		},
	}
	h := Handler{Tasks: tasks}

	outcome, err := h.HandleTextWithOutcome(context.Background(), 1, 1, "/delete task-a -f")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(outcome.Reply, "确认强制删除任务") {
		t.Fatalf("unexpected force delete reply: %s", outcome.Reply)
	}
	if outcome.ReplyMarkup == nil {
		t.Fatalf("expected force delete confirmation keyboard, got %+v", outcome.ReplyMarkup)
	}
}

func TestHandlerDeleteCommandUnsupportedStatus(t *testing.T) {
	tasks := &fakeTaskQuery{
		task: service.Task{
			TaskID: "task-a",
			UserID: 1,
			Status: service.StatusDone,
		},
	}
	h := Handler{Tasks: tasks}

	reply, err := h.HandleText(context.Background(), 1, 1, "/delete task-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reply, "当前状态不支持删除") {
		t.Fatalf("unexpected delete reply: %s", reply)
	}
}

func TestHandlerDeleteCommandForceAllowsRunning(t *testing.T) {
	tasks := &fakeTaskQuery{
		task: service.Task{
			TaskID: "task-a",
			UserID: 1,
			Status: service.StatusRunning,
		},
	}
	h := Handler{Tasks: tasks}

	outcome, err := h.HandleTextWithOutcome(context.Background(), 1, 1, "/delete task-a --force")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(outcome.Reply, "确认强制删除任务") || !strings.Contains(outcome.Reply, "这只会删除记录，不会终止当前执行") {
		t.Fatalf("unexpected force delete running reply: %s", outcome.Reply)
	}
	if outcome.ReplyMarkup == nil {
		t.Fatalf("expected force delete confirmation keyboard, got %+v", outcome.ReplyMarkup)
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
			TargetPeer:     req.TargetPeer,
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
	if capturedReq.TargetPeer != "" {
		t.Fatalf("expected create request target peer to be omitted, got %q", capturedReq.TargetPeer)
	}
	if q.messages[0].TargetPeer != "" {
		t.Fatalf("expected queue message target peer to be omitted, got %q", q.messages[0].TargetPeer)
	}
	if q.messages[0].DropCaption {
		t.Fatal("expected plain URL flow to preserve caption")
	}
}

func TestHandlerForwardCommandCreatesTaskWithTargetPeerAndDropCaption(t *testing.T) {
	q := &fakeQueue{}
	tasks := &fakeTaskQuery{}
	var capturedReq service.CreateQueuedTaskRequest
	tasks.createFn = func(req service.CreateQueuedTaskRequest) (service.Task, error) {
		capturedReq = req
		return service.Task{
			TaskID:         req.TaskID,
			ChatID:         req.ChatID,
			UserID:         req.UserID,
			TargetPeer:     req.TargetPeer,
			URL:            req.URL,
			DropCaption:    req.DropCaption,
			IdempotencyKey: req.IdempotencyKey,
			Status:         service.StatusQueued,
			CreatedAt:      time.Now().UTC(),
		}, nil
	}
	h := Handler{Tasks: tasks, Queue: q}

	reply, err := h.HandleText(context.Background(), 1, 1, "/forward https://t.me/c/1/2 https://t.me/channel_name --drop-caption")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reply, "Task ID:") {
		t.Fatalf("unexpected reply: %s", reply)
	}
	if capturedReq.TargetPeer != "channel_name" {
		t.Fatalf("expected normalized target peer, got %+v", capturedReq)
	}
	if !capturedReq.DropCaption {
		t.Fatalf("expected drop caption in create request, got %+v", capturedReq)
	}
	if len(q.messages) != 1 {
		t.Fatalf("expected one enqueue, got %d", len(q.messages))
	}
	if q.messages[0].TargetPeer != "channel_name" || !q.messages[0].DropCaption {
		t.Fatalf("unexpected enqueued message: %+v", q.messages[0])
	}
}

func TestHandlerForwardCommandRejectsPrivateInviteLink(t *testing.T) {
	h := Handler{Tasks: &fakeTaskQuery{}, Queue: &fakeQueue{}}

	reply, err := h.HandleText(context.Background(), 1, 1, "/forward https://t.me/c/1/2 https://t.me/+abcde")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reply, "私有邀请链接暂不支持") {
		t.Fatalf("unexpected reply: %s", reply)
	}
}

func TestHandlerForwardCommandRejectsInvalidSourceURL(t *testing.T) {
	h := Handler{Tasks: &fakeTaskQuery{}, Queue: &fakeQueue{}}

	reply, err := h.HandleText(context.Background(), 1, 1, "/forward https://example.com/post/1 channel_name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reply, "源链接必须是受支持的 Telegram 消息链接") {
		t.Fatalf("unexpected reply: %s", reply)
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
			TargetPeer:     req.TargetPeer,
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
			TargetPeer:     req.TargetPeer,
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

func TestHandlerRetryCommandRebuildsAndEnqueues(t *testing.T) {
	q := &fakeQueue{}
	tasks := &fakeTaskQuery{
		task: service.Task{
			TaskID:         "failed-task",
			ChatID:         1,
			UserID:         1,
			URL:            "https://t.me/c/1/2",
			Status:         service.StatusFailed,
			IdempotencyKey: "idem-key",
		},
	}
	tasks.createFn = func(req service.CreateQueuedTaskRequest) (service.Task, error) {
		return service.Task{
			TaskID:         req.TaskID,
			ChatID:         req.ChatID,
			UserID:         req.UserID,
			TargetPeer:     req.TargetPeer,
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

	h := Handler{Tasks: tasks, Queue: q}
	outcome, err := h.HandleTextWithOutcome(context.Background(), 1, 1, "/retry failed-task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(outcome.Reply, "失败") && !strings.Contains(outcome.Reply, "Task ID: failed-task") {
		t.Fatalf("unexpected retry reply: %s", outcome.Reply)
	}
	if outcome.ReplyMarkup == nil {
		t.Fatalf("expected retry task action keyboard, got %+v", outcome.ReplyMarkup)
	}
	if deleteCalls != 0 {
		t.Fatalf("expected no cleanup delete before callback retry, got %d", deleteCalls)
	}
	if len(q.messages) != 0 {
		t.Fatalf("expected no enqueue before callback retry, got %d", len(q.messages))
	}
}

func TestHandlerRetryCommandShowsRunningTaskMenu(t *testing.T) {
	h := Handler{
		Tasks: &fakeTaskQuery{
			task: service.Task{
				TaskID: "task-a",
				UserID: 1,
				URL:    "https://t.me/c/1/2",
				Status: service.StatusRunning,
			},
		},
		Queue: &fakeQueue{},
	}
	reply, err := h.HandleText(context.Background(), 1, 1, "/retry task-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reply, "不会停止旧的实际执行") {
		t.Fatalf("unexpected retry reply: %s", reply)
	}
}

func TestHandlerRetryCommandWithoutTaskIDShowsRetrySelection(t *testing.T) {
	h := Handler{
		Tasks: &fakeTaskQuery{queueTasks: []service.Task{
			{TaskID: "task-a", Status: service.StatusFailed, URL: "https://t.me/c/1/2"},
			{TaskID: "task-b", Status: service.StatusQueued, URL: "https://t.me/c/1/3"},
			{TaskID: "task-c", Status: service.StatusDeadLettered, URL: "https://t.me/c/1/4"},
			{TaskID: "task-d", Status: service.StatusRunning, URL: "https://t.me/c/1/5"},
		}},
		Queue: &fakeQueue{},
	}

	outcome, err := h.HandleTextWithOutcome(context.Background(), 1, 1, "/retry")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Reply != "请选择要重试的任务：" {
		t.Fatalf("unexpected retry selection reply: %s", outcome.Reply)
	}
	if outcome.ReplyMarkup == nil || len(outcome.ReplyMarkup.InlineKeyboard) != 4 {
		t.Fatalf("expected retry selection keyboard, got %+v", outcome.ReplyMarkup)
	}
}

func TestHandlerRetryCommandWithoutTaskIDShowsEmptyState(t *testing.T) {
	h := Handler{
		Tasks: &fakeTaskQuery{queueTasks: []service.Task{
			{TaskID: "task-a", Status: service.StatusDone, URL: "https://t.me/c/1/2"},
		}},
		Queue: &fakeQueue{},
	}

	outcome, err := h.HandleTextWithOutcome(context.Background(), 1, 1, "/retry")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Reply != "当前无可重试任务。" {
		t.Fatalf("unexpected retry empty reply: %s", outcome.Reply)
	}
	if outcome.ReplyMarkup != nil {
		t.Fatalf("expected no keyboard for empty retry list, got %+v", outcome.ReplyMarkup)
	}
}

func TestHandleCallbackDeleteFlow(t *testing.T) {
	const taskID = "task-123456"
	tasks := &fakeTaskQuery{
		queueTasks: []service.Task{{TaskID: "other-task", UserID: 1, Status: service.StatusQueued, URL: "https://t.me/c/1/3"}},
		task: service.Task{
			TaskID: taskID,
			UserID: 1,
			URL:    "https://t.me/c/1/2",
			Status: service.StatusQueued,
		},
		deletePendingResp: true,
	}
	h := Handler{Tasks: tasks}

	prompt, err := h.HandleCallback(context.Background(), 1, "cb-1", callbackDeletePrefix+encodeModeTask(taskMenuModeQueue, taskID))
	if err != nil {
		t.Fatalf("unexpected prompt error: %v", err)
	}
	if prompt.ReplyMarkup == nil || len(prompt.ReplyMarkup.InlineKeyboard) == 0 {
		t.Fatalf("expected delete confirmation keyboard, got %+v", prompt.ReplyMarkup)
	}

	confirmed, err := h.HandleCallback(context.Background(), 1, "cb-1", callbackDeleteOKPrefix+encodeModeTask(taskMenuModeQueue, taskID))
	if err != nil {
		t.Fatalf("unexpected confirm error: %v", err)
	}
	if !strings.Contains(confirmed.Reply, "任务已删除") {
		t.Fatalf("unexpected confirm reply: %s", confirmed.Reply)
	}
	if confirmed.ReplyMarkup == nil {
		t.Fatalf("expected refreshed queue keyboard after delete, got %+v", confirmed.ReplyMarkup)
	}
	if confirmed.AnswerText != "删除成功" {
		t.Fatalf("unexpected confirm answer: %s", confirmed.AnswerText)
	}
}

func TestHandleCallbackQueueTaskMenuShowsStatusSpecificActions(t *testing.T) {
	const taskID = "task-123456"
	h := Handler{Tasks: &fakeTaskQuery{
		task: service.Task{
			TaskID: taskID,
			UserID: 1,
			URL:    "https://t.me/c/1/2",
			Status: service.StatusPaused,
		},
	}}

	outcome, err := h.HandleCallback(context.Background(), 1, "cb-1", callbackQueueTaskPrefix+encodeModeTask(taskMenuModeQueue, taskID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(outcome.Reply, "已暂停") {
		t.Fatalf("unexpected queue task detail reply: %s", outcome.Reply)
	}
	if outcome.ReplyMarkup == nil || len(outcome.ReplyMarkup.InlineKeyboard) < 2 {
		t.Fatalf("expected action keyboard, got %+v", outcome.ReplyMarkup)
	}
	row := outcome.ReplyMarkup.InlineKeyboard[0]
	if row[0].Text != "重试" {
		t.Fatalf("unexpected paused action row: %+v", row)
	}
}

func TestHandleCallbackQueuePauseFlow(t *testing.T) {
	const taskID = "task-123456"
	tasks := &fakeTaskQuery{
		task: service.Task{
			TaskID: taskID,
			UserID: 1,
			URL:    "https://t.me/c/1/2",
			Status: service.StatusPaused,
		},
		pauseResp: true,
	}
	tasks.pauseFn = func(userID int64, gotTaskID string) (bool, error) {
		if userID != 1 || gotTaskID != taskID {
			t.Fatalf("unexpected pause request: %d %s", userID, gotTaskID)
		}
		return true, nil
	}

	h := Handler{Tasks: tasks}
	outcome, err := h.HandleCallback(context.Background(), 1, "cb-1", callbackQueuePausePrefix+encodeModeTask(taskMenuModeQueue, taskID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.AnswerText != "已暂停" {
		t.Fatalf("unexpected pause answer text: %s", outcome.AnswerText)
	}
	if !strings.Contains(outcome.Reply, "已暂停") {
		t.Fatalf("unexpected pause reply: %s", outcome.Reply)
	}
}

func TestHandleCallbackQueueCancelRefreshesList(t *testing.T) {
	const taskID = "task-123456"
	tasks := &fakeTaskQuery{
		queueTasks: []service.Task{{TaskID: "task-b", UserID: 1, Status: service.StatusQueued, URL: "https://t.me/c/1/3"}},
		cancelResp: true,
	}

	h := Handler{Tasks: tasks}
	outcome, err := h.HandleCallback(context.Background(), 1, "cb-1", callbackQueueCancelPrefix+encodeModeTask(taskMenuModeQueue, taskID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(outcome.Reply, "任务已取消") {
		t.Fatalf("unexpected cancel reply: %s", outcome.Reply)
	}
	if outcome.ReplyMarkup == nil || len(outcome.ReplyMarkup.InlineKeyboard) == 0 {
		t.Fatalf("expected refreshed queue keyboard, got %+v", outcome.ReplyMarkup)
	}
}

func TestHandleCallbackQueueRunningTaskShowsNoMutationActions(t *testing.T) {
	const taskID = "task-123456"
	h := Handler{Tasks: &fakeTaskQuery{
		task: service.Task{
			TaskID: taskID,
			UserID: 1,
			URL:    "https://t.me/c/1/2",
			Status: service.StatusRunning,
		},
	}}

	outcome, err := h.HandleCallback(context.Background(), 1, "cb-1", callbackQueueTaskPrefix+encodeModeTask(taskMenuModeQueue, taskID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(outcome.Reply, "可强制删除记录") {
		t.Fatalf("unexpected running task reply: %s", outcome.Reply)
	}
	if !strings.Contains(outcome.Reply, "不会停止旧的实际执行") {
		t.Fatalf("unexpected running retry hint: %s", outcome.Reply)
	}
	if outcome.ReplyMarkup == nil || len(outcome.ReplyMarkup.InlineKeyboard) != 2 || outcome.ReplyMarkup.InlineKeyboard[0][0].Text != "重试" || outcome.ReplyMarkup.InlineKeyboard[0][1].Text != "强制删除" {
		t.Fatalf("unexpected running task keyboard: %+v", outcome.ReplyMarkup)
	}
}

func TestHandleCallbackQueueForceDeleteRunningShowsConfirmation(t *testing.T) {
	const taskID = "task-123456"
	tasks := &fakeTaskQuery{
		task: service.Task{
			TaskID: taskID,
			UserID: 1,
			URL:    "https://t.me/c/1/2",
			Status: service.StatusRunning,
		},
	}

	h := Handler{Tasks: tasks}
	outcome, err := h.HandleCallback(context.Background(), 1, "cb-1", callbackQueueForcePrefix+encodeModeTask(taskMenuModeQueue, taskID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(outcome.Reply, "确认强制删除任务") {
		t.Fatalf("unexpected force delete callback reply: %s", outcome.Reply)
	}
	if outcome.AnswerText != "请确认强制删除" {
		t.Fatalf("unexpected force delete callback answer: %s", outcome.AnswerText)
	}
}

func TestHandleCallbackQueueForceDeleteConfirmRunning(t *testing.T) {
	const taskID = "task-123456"
	tasks := &fakeTaskQuery{
		task: service.Task{
			TaskID: taskID,
			UserID: 1,
			URL:    "https://t.me/c/1/2",
			Status: service.StatusRunning,
		},
		queueTasks:      []service.Task{{TaskID: "task-b", UserID: 1, Status: service.StatusQueued, URL: "https://t.me/c/1/3"}},
		forceDeleteResp: true,
	}

	h := Handler{Tasks: tasks}
	outcome, err := h.HandleCallback(context.Background(), 1, "cb-1", callbackQueueForceOKPrefix+encodeModeTask(taskMenuModeQueue, taskID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(outcome.Reply, "仅删除记录，不终止当前执行") {
		t.Fatalf("unexpected force delete callback reply: %s", outcome.Reply)
	}
	if outcome.AnswerText != "已强制删除" {
		t.Fatalf("unexpected force delete callback answer: %s", outcome.AnswerText)
	}
}

func TestHandleCallbackRetryFromRetryListRebuildsAndRefreshes(t *testing.T) {
	const taskID = "task-123456"
	q := &fakeQueue{}
	tasks := &fakeTaskQuery{
		task: service.Task{
			TaskID:         taskID,
			ChatID:         1,
			UserID:         1,
			URL:            "https://t.me/c/1/2",
			Status:         service.StatusFailed,
			IdempotencyKey: "idem-key",
		},
		queueTasks: []service.Task{{TaskID: "other-failed", UserID: 1, Status: service.StatusDeadLettered, URL: "https://t.me/c/1/3"}},
	}
	tasks.createFn = func(req service.CreateQueuedTaskRequest) (service.Task, error) {
		return service.Task{
			TaskID:         req.TaskID,
			ChatID:         req.ChatID,
			UserID:         req.UserID,
			URL:            req.URL,
			Status:         service.StatusQueued,
			CreatedAt:      time.Now().UTC(),
			IdempotencyKey: req.IdempotencyKey,
		}, nil
	}
	tasks.deleteFailedFn = func(string) (int64, error) { return 1, nil }

	h := Handler{Tasks: tasks, Queue: q}
	outcome, err := h.HandleCallback(context.Background(), 1, "cb-1", callbackQueueRetryPrefix+encodeModeTask(taskMenuModeRetry, taskID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(outcome.Reply, "任务已替换并重新入队") {
		t.Fatalf("unexpected retry callback reply: %s", outcome.Reply)
	}
	if outcome.ReplyMarkup == nil || len(outcome.ReplyMarkup.InlineKeyboard) != 1 {
		t.Fatalf("expected refreshed retry keyboard, got %+v", outcome.ReplyMarkup)
	}
	if len(q.messages) != 1 {
		t.Fatalf("expected one enqueue after retry callback, got %d", len(q.messages))
	}
}

func TestHandleCallbackRetryFromQueueRunningReplacesTask(t *testing.T) {
	const taskID = "task-running"
	q := &fakeQueue{}
	forceDeleteCalls := 0
	tasks := &fakeTaskQuery{
		task: service.Task{
			TaskID:         taskID,
			ChatID:         1,
			UserID:         1,
			URL:            "https://t.me/c/1/2",
			Status:         service.StatusRunning,
			IdempotencyKey: "idem-running",
		},
		queueTasks: []service.Task{{TaskID: "other-task", UserID: 1, Status: service.StatusQueued, URL: "https://t.me/c/1/3"}},
		forceDeleteFn: func(userID int64, gotTaskID string) (bool, error) {
			forceDeleteCalls++
			if userID != 1 || gotTaskID != taskID {
				t.Fatalf("unexpected force delete request: %d %s", userID, gotTaskID)
			}
			return true, nil
		},
	}
	tasks.createFn = func(req service.CreateQueuedTaskRequest) (service.Task, error) {
		return service.Task{
			TaskID:         req.TaskID,
			ChatID:         req.ChatID,
			UserID:         req.UserID,
			URL:            req.URL,
			Status:         service.StatusQueued,
			CreatedAt:      time.Now().UTC(),
			IdempotencyKey: req.IdempotencyKey,
		}, nil
	}

	h := Handler{Tasks: tasks, Queue: q}
	outcome, err := h.HandleCallback(context.Background(), 1, "cb-1", callbackQueueRetryPrefix+encodeModeTask(taskMenuModeQueue, taskID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(outcome.Reply, "任务已替换并重新入队") || !strings.Contains(outcome.Reply, "不会终止它") {
		t.Fatalf("unexpected running retry reply: %s", outcome.Reply)
	}
	if forceDeleteCalls != 1 {
		t.Fatalf("expected one force delete before retry, got %d", forceDeleteCalls)
	}
	if len(q.messages) != 1 {
		t.Fatalf("expected one enqueue after running retry, got %d", len(q.messages))
	}
}

func TestHandleCallbackRetryFromQueuePausedReplacesTask(t *testing.T) {
	const taskID = "task-paused"
	q := &fakeQueue{}
	deleteCalls := 0
	tasks := &fakeTaskQuery{
		task: service.Task{
			TaskID:         taskID,
			ChatID:         1,
			UserID:         1,
			URL:            "https://t.me/c/1/2",
			Status:         service.StatusPaused,
			IdempotencyKey: "idem-paused",
		},
		queueTasks: []service.Task{{TaskID: "other-task", UserID: 1, Status: service.StatusQueued, URL: "https://t.me/c/1/3"}},
		deleteNonRunningFn: func(userID int64, gotTaskID string) (bool, error) {
			deleteCalls++
			if userID != 1 || gotTaskID != taskID {
				t.Fatalf("unexpected non-running delete request: %d %s", userID, gotTaskID)
			}
			return true, nil
		},
	}
	tasks.createFn = func(req service.CreateQueuedTaskRequest) (service.Task, error) {
		return service.Task{
			TaskID:         req.TaskID,
			ChatID:         req.ChatID,
			UserID:         req.UserID,
			URL:            req.URL,
			Status:         service.StatusQueued,
			CreatedAt:      time.Now().UTC(),
			IdempotencyKey: req.IdempotencyKey,
		}, nil
	}

	h := Handler{Tasks: tasks, Queue: q}
	outcome, err := h.HandleCallback(context.Background(), 1, "cb-1", callbackQueueRetryPrefix+encodeModeTask(taskMenuModeQueue, taskID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(outcome.Reply, "任务已替换并重新入队") {
		t.Fatalf("unexpected paused retry reply: %s", outcome.Reply)
	}
	if deleteCalls != 1 {
		t.Fatalf("expected one non-running delete before retry, got %d", deleteCalls)
	}
	if len(q.messages) != 1 {
		t.Fatalf("expected one enqueue after paused retry, got %d", len(q.messages))
	}
}

func TestHandleCallbackRetryFromQueueQueuedReplacesTask(t *testing.T) {
	const taskID = "task-queued"
	q := &fakeQueue{}
	deleteCalls := 0
	tasks := &fakeTaskQuery{
		task: service.Task{
			TaskID:         taskID,
			ChatID:         1,
			UserID:         1,
			URL:            "https://t.me/c/1/2",
			Status:         service.StatusQueued,
			IdempotencyKey: "idem-queued",
		},
		queueTasks: []service.Task{{TaskID: "other-task", UserID: 1, Status: service.StatusPaused, URL: "https://t.me/c/1/3"}},
		deletePendingFn: func(userID int64, gotTaskID string) (bool, error) {
			deleteCalls++
			if userID != 1 || gotTaskID != taskID {
				t.Fatalf("unexpected pending delete request: %d %s", userID, gotTaskID)
			}
			return true, nil
		},
	}
	tasks.createFn = func(req service.CreateQueuedTaskRequest) (service.Task, error) {
		return service.Task{
			TaskID:         req.TaskID,
			ChatID:         req.ChatID,
			UserID:         req.UserID,
			URL:            req.URL,
			Status:         service.StatusQueued,
			CreatedAt:      time.Now().UTC(),
			IdempotencyKey: req.IdempotencyKey,
		}, nil
	}

	h := Handler{Tasks: tasks, Queue: q}
	outcome, err := h.HandleCallback(context.Background(), 1, "cb-1", callbackQueueRetryPrefix+encodeModeTask(taskMenuModeQueue, taskID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(outcome.Reply, "任务已替换并重新入队") {
		t.Fatalf("unexpected queued retry reply: %s", outcome.Reply)
	}
	if deleteCalls != 1 {
		t.Fatalf("expected one pending delete before retry, got %d", deleteCalls)
	}
	if len(q.messages) != 1 {
		t.Fatalf("expected one enqueue after queued retry, got %d", len(q.messages))
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
	if outcome.ReactionEmoji != "🤔" {
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
			TargetPeer:     req.TargetPeer,
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
