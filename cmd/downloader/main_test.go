package main

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"os/exec"
	"testing"
	"time"

	"tgdl-bot/internal/config"
	dl "tgdl-bot/internal/downloader"
	"tgdl-bot/internal/queue"
	"tgdl-bot/internal/service"
)

type fakePreflight struct {
	err error
}

func (f fakePreflight) Check(context.Context, config.Config) error {
	return f.err
}

type fakeLoop struct {
	called bool
}

func (l *fakeLoop) Run(context.Context, config.Config) error {
	l.called = true
	return nil
}

func TestRun_PreflightFailureSkipsLoop(t *testing.T) {
	cfg := config.Config{}
	logger := slog.Default()
	loop := &fakeLoop{}

	err := run(context.Background(), cfg, logger, fakePreflight{err: errors.New("preflight failed")}, loop)
	if err == nil {
		t.Fatal("expected preflight error")
	}
	if loop.called {
		t.Fatal("loop should not be called when preflight fails")
	}
}

type fakeQueue struct {
	acked   []string
	retried []string
}

func (q *fakeQueue) Pull(context.Context, int, int) ([]queue.ReceivedMessage, error) { return nil, nil }
func (q *fakeQueue) Ack(_ context.Context, leaseIDs []string) error {
	q.acked = append(q.acked, leaseIDs...)
	return nil
}
func (q *fakeQueue) Retry(_ context.Context, leaseIDs []string) error {
	q.retried = append(q.retried, leaseIDs...)
	return nil
}

type fakeTasks struct {
	task    service.Task
	updates []service.TaskUpdate
}

func (f *fakeTasks) GetTask(context.Context, string) (service.Task, error) { return f.task, nil }
func (f *fakeTasks) UpdateTask(_ context.Context, _ string, update service.TaskUpdate) error {
	f.updates = append(f.updates, update)
	return nil
}

type fakeRunnerImpl struct {
	build func(context.Context, dl.DownloadRequest) (*exec.Cmd, error)
}

func (f fakeRunnerImpl) Preflight(context.Context, dl.DownloadRequest) (dl.SessionState, error) {
	return dl.SessionStateReady, nil
}

func (f fakeRunnerImpl) BuildCommand(ctx context.Context, req dl.DownloadRequest) (*exec.Cmd, error) {
	return f.build(ctx, req)
}

func TestQueuePullLoopProcessMessageSuccessAcks(t *testing.T) {
	q := &fakeQueue{}
	tasks := &fakeTasks{
		task: service.Task{
			TaskID:       "t1",
			URL:          "https://t.me/c/1/2",
			TargetChatID: 100,
			Status:       service.StatusQueued,
		},
	}
	loop := queuePullLoop{
		logger: slog.Default(),
		queue:  q,
		tasks:  tasks,
		runner: fakeRunnerImpl{build: func(ctx context.Context, _ dl.DownloadRequest) (*exec.Cmd, error) {
			return exec.CommandContext(ctx, "sh", "-c", "echo ok"), nil
		}},
		maxAttempts: 3,
	}

	loop.processMessage(context.Background(), config.Config{Downloader: config.DownloaderConfig{TaskTimeoutMinutes: 1}}, queue.ReceivedMessage{
		LeaseID: "lease-1",
		Body:    queue.Message{TaskID: "t1"},
	})

	if len(q.acked) != 1 || q.acked[0] != "lease-1" {
		t.Fatalf("expected lease to be acked, got %+v", q.acked)
	}
	if len(q.retried) != 0 {
		t.Fatalf("expected no retries, got %+v", q.retried)
	}
}

func TestQueuePullLoopProcessMessageNonRetryableAcks(t *testing.T) {
	q := &fakeQueue{}
	tasks := &fakeTasks{
		task: service.Task{
			TaskID:       "t1",
			URL:          "https://t.me/c/1/2",
			TargetChatID: 100,
			Status:       service.StatusQueued,
			RetryCount:   0,
		},
	}
	loop := queuePullLoop{
		logger: slog.Default(),
		queue:  q,
		tasks:  tasks,
		runner: fakeRunnerImpl{build: func(ctx context.Context, _ dl.DownloadRequest) (*exec.Cmd, error) {
			return exec.CommandContext(ctx, "sh", "-c", "exit 2"), nil
		}},
		maxAttempts: 3,
	}

	loop.processMessage(context.Background(), config.Config{Downloader: config.DownloaderConfig{TaskTimeoutMinutes: 1}}, queue.ReceivedMessage{
		LeaseID: "lease-2",
		Body:    queue.Message{TaskID: "t1"},
	})

	if len(q.retried) != 0 {
		t.Fatalf("expected no retry, got %+v", q.retried)
	}
	if len(q.acked) != 1 || q.acked[0] != "lease-2" {
		t.Fatalf("expected lease to be acked, got %+v", q.acked)
	}
	if len(tasks.updates) < 2 {
		t.Fatalf("expected running and final update, got %d", len(tasks.updates))
	}
	if tasks.updates[len(tasks.updates)-1].Status != service.StatusFailed {
		t.Fatalf("expected failed status, got %s", tasks.updates[len(tasks.updates)-1].Status)
	}
}

func TestQueuePullLoopProcessMessageRetryableNetworkErrorRetries(t *testing.T) {
	q := &fakeQueue{}
	tasks := &fakeTasks{
		task: service.Task{
			TaskID:       "t1",
			URL:          "https://t.me/c/1/2",
			TargetChatID: 100,
			Status:       service.StatusQueued,
			RetryCount:   0,
		},
	}
	loop := queuePullLoop{
		logger: slog.Default(),
		queue:  q,
		tasks:  tasks,
		runner: fakeRunnerImpl{build: func(ctx context.Context, _ dl.DownloadRequest) (*exec.Cmd, error) {
			return exec.CommandContext(ctx, "sh", "-c", "echo 'connection reset by peer' 1>&2; exit 1"), nil
		}},
		maxAttempts: 3,
	}

	loop.processMessage(context.Background(), config.Config{Downloader: config.DownloaderConfig{TaskTimeoutMinutes: 1}}, queue.ReceivedMessage{
		LeaseID: "lease-network",
		Body:    queue.Message{TaskID: "t1"},
	})

	if len(q.retried) != 1 || q.retried[0] != "lease-network" {
		t.Fatalf("expected retry lease, got %+v", q.retried)
	}
	if len(q.acked) != 0 {
		t.Fatalf("expected no ack for retryable failure, got %+v", q.acked)
	}
	if len(tasks.updates) < 2 {
		t.Fatalf("expected running and final update, got %d", len(tasks.updates))
	}
	final := tasks.updates[len(tasks.updates)-1]
	if final.Status != service.StatusRetrying {
		t.Fatalf("expected retrying status, got %s", final.Status)
	}
	if final.RetryCount == nil || *final.RetryCount != 1 {
		t.Fatalf("expected retry_count=1, got %+v", final.RetryCount)
	}
}

func TestQueuePullLoopProcessMessageRetryableExhaustedDeadLettered(t *testing.T) {
	q := &fakeQueue{}
	tasks := &fakeTasks{
		task: service.Task{
			TaskID:       "t1",
			URL:          "https://t.me/c/1/2",
			TargetChatID: 100,
			Status:       service.StatusRetrying,
			RetryCount:   2,
		},
	}
	loop := queuePullLoop{
		logger: slog.Default(),
		queue:  q,
		tasks:  tasks,
		runner: fakeRunnerImpl{build: func(ctx context.Context, _ dl.DownloadRequest) (*exec.Cmd, error) {
			return exec.CommandContext(ctx, "sh", "-c", "echo 'i/o timeout' 1>&2; exit 1"), nil
		}},
		maxAttempts: 3,
	}

	loop.processMessage(context.Background(), config.Config{Downloader: config.DownloaderConfig{TaskTimeoutMinutes: 1}}, queue.ReceivedMessage{
		LeaseID: "lease-dead",
		Body:    queue.Message{TaskID: "t1"},
	})

	if len(q.acked) != 1 || q.acked[0] != "lease-dead" {
		t.Fatalf("expected ack on exhausted retries, got %+v", q.acked)
	}
	if len(q.retried) != 0 {
		t.Fatalf("expected no queue retry on exhausted attempts, got %+v", q.retried)
	}
	final := tasks.updates[len(tasks.updates)-1]
	if final.Status != service.StatusDeadLettered {
		t.Fatalf("expected dead_lettered status, got %s", final.Status)
	}
}

func TestClassifyTDLErrorTimeoutAndKeyword(t *testing.T) {
	t.Run("context deadline", func(t *testing.T) {
		runCtx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		time.Sleep(2 * time.Millisecond)
		got := classifyTDLError(runCtx, dl.RunResult{}, errors.New("exit status 1"))
		if got != dl.ErrorClassRetryable {
			t.Fatalf("expected retryable, got %s", got)
		}
	})

	t.Run("network keyword", func(t *testing.T) {
		got := classifyTDLError(context.Background(), dl.RunResult{
			Stderr: "rpc error: transport is closing",
		}, errors.New("exit status 1"))
		if got != dl.ErrorClassRetryable {
			t.Fatalf("expected retryable, got %s", got)
		}
	})

	t.Run("business error", func(t *testing.T) {
		got := classifyTDLError(context.Background(), dl.RunResult{
			Stderr: "chat not found",
		}, errors.New("exit status 1"))
		if got != dl.ErrorClassNonRetryable {
			t.Fatalf("expected non_retryable, got %s", got)
		}
	})
}

func TestTransientKeywordsContainEOF(t *testing.T) {
	joined := strings.Join(transientErrorKeywords, ",")
	if !strings.Contains(joined, "eof") {
		t.Fatalf("expected eof keyword in transient keyword list, got %s", joined)
	}
}
