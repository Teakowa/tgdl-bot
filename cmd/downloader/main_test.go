package main

import (
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
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
	acked    []string
	retried  []string
	enqueued []queue.Message
}

func (q *fakeQueue) Pull(context.Context, int, int) ([]queue.ReceivedMessage, error) { return nil, nil }
func (q *fakeQueue) Enqueue(_ context.Context, message queue.Message) error {
	q.enqueued = append(q.enqueued, message)
	return nil
}
func (q *fakeQueue) Ack(_ context.Context, leaseIDs []string) error {
	q.acked = append(q.acked, leaseIDs...)
	return nil
}
func (q *fakeQueue) Retry(_ context.Context, leaseIDs []string) error {
	q.retried = append(q.retried, leaseIDs...)
	return nil
}

type fakeTasks struct {
	task           service.Task
	updates        []service.TaskUpdate
	failedForRetry []service.Task
}

func (f *fakeTasks) GetTask(context.Context, string) (service.Task, error) { return f.task, nil }
func (f *fakeTasks) UpdateTask(_ context.Context, _ string, update service.TaskUpdate) error {
	f.updates = append(f.updates, update)
	return nil
}
func (f *fakeTasks) ListFailedTasksForRetry(context.Context, int, int) ([]service.Task, error) {
	return f.failedForRetry, nil
}

type fakeRunnerImpl struct {
	build func(context.Context, dl.DownloadRequest) (*exec.Cmd, error)
}

type fakeNotifier struct {
	err   error
	tasks []service.Task
}

func (f *fakeNotifier) Notify(_ context.Context, task service.Task) error {
	f.tasks = append(f.tasks, task)
	return f.err
}

func (f fakeRunnerImpl) Preflight(context.Context, dl.DownloadRequest) (dl.SessionState, error) {
	return dl.SessionStateReady, nil
}

func (f fakeRunnerImpl) BuildCommand(ctx context.Context, req dl.DownloadRequest) (*exec.Cmd, error) {
	return f.build(ctx, req)
}

func TestQueuePullLoopProcessMessageSuccessAcks(t *testing.T) {
	q := &fakeQueue{}
	notifier := &fakeNotifier{}
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
		notifier:    notifier,
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
	if len(notifier.tasks) < 2 {
		t.Fatalf("expected running and done notifications, got %d", len(notifier.tasks))
	}
	if notifier.tasks[len(notifier.tasks)-1].Status != service.StatusDone {
		t.Fatalf("expected done notification, got %s", notifier.tasks[len(notifier.tasks)-1].Status)
	}
}

func TestQueuePullLoopNotifierFailureDoesNotAffectAck(t *testing.T) {
	q := &fakeQueue{}
	notifier := &fakeNotifier{err: errors.New("notify failed")}
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
		notifier:    notifier,
		maxAttempts: 3,
	}

	loop.processMessage(context.Background(), config.Config{Downloader: config.DownloaderConfig{TaskTimeoutMinutes: 1}}, queue.ReceivedMessage{
		LeaseID: "lease-1",
		Body:    queue.Message{TaskID: "t1"},
	})

	if len(q.acked) != 1 || q.acked[0] != "lease-1" {
		t.Fatalf("expected lease to be acked, got %+v", q.acked)
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
		if got != dl.ErrorClassNonRetryable {
			t.Fatalf("expected non_retryable, got %s", got)
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

	t.Run("unknown flag error", func(t *testing.T) {
		got := classifyTDLError(context.Background(), dl.RunResult{
			Stderr: "Error: unknown shorthand flag: 'u' in -u\nUsage:\n  tdl forward [flags]",
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

func TestQueuePullLoopRunRequeuesFailedTasksOnStartup(t *testing.T) {
	q := &fakeQueue{}
	tasks := &fakeTasks{
		failedForRetry: []service.Task{
			{
				TaskID:         "retry-task",
				ChatID:         1,
				UserID:         2,
				TargetChatID:   3,
				URL:            "https://t.me/c/1/2",
				IdempotencyKey: "idem-1",
				RetryCount:     1,
				Status:         service.StatusFailed,
			},
		},
	}

	loop := queuePullLoop{
		logger:      slog.Default(),
		queue:       q,
		tasks:       tasks,
		runner:      fakeRunnerImpl{build: func(context.Context, dl.DownloadRequest) (*exec.Cmd, error) { return nil, errors.New("unused") }},
		maxAttempts: 3,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_ = loop.Run(ctx, config.Config{
		Cloudflare: config.CloudflareConfig{
			QueuePullIntervalMS: 10,
			QueueBatchSize:      1,
		},
		Downloader: config.DownloaderConfig{
			Workers: 1,
		},
	})

	if len(q.enqueued) != 1 || q.enqueued[0].TaskID != "retry-task" {
		t.Fatalf("expected startup retry enqueue, got %+v", q.enqueued)
	}
	if len(tasks.updates) != 1 || tasks.updates[0].Status != service.StatusRetrying {
		t.Fatalf("expected retrying status update, got %+v", tasks.updates)
	}
}

func TestQueuePullLoopEmitsLifecycleLogs(t *testing.T) {
	q := &fakeQueue{}
	tasks := &fakeTasks{
		task: service.Task{
			TaskID:       "t-log",
			URL:          "https://t.me/c/1/2",
			TargetChatID: 100,
			Status:       service.StatusQueued,
		},
	}
	capture := newLogCapture()
	loop := queuePullLoop{
		logger: capture.Logger(),
		queue:  q,
		tasks:  tasks,
		runner: fakeRunnerImpl{build: func(ctx context.Context, _ dl.DownloadRequest) (*exec.Cmd, error) {
			return exec.CommandContext(ctx, "sh", "-c", "echo ok"), nil
		}},
		maxAttempts: 3,
	}

	loop.processMessage(context.Background(), config.Config{Downloader: config.DownloaderConfig{TaskTimeoutMinutes: 1}}, queue.ReceivedMessage{
		LeaseID: "lease-log",
		Body:    queue.Message{TaskID: "t-log"},
	})

	messages := capture.Messages()
	for _, want := range []string{
		"downloader message pulled",
		"downloader tdl execution started",
		"downloader tdl execution finished",
		"downloader task state updated",
		"downloader queue action",
	} {
		if !containsMessage(messages, want) {
			t.Fatalf("expected log %q, got %v", want, messages)
		}
	}
}

func TestQueuePullLoopEmitsTDLStreamLogs(t *testing.T) {
	q := &fakeQueue{}
	tasks := &fakeTasks{
		task: service.Task{
			TaskID:       "t-stream",
			URL:          "https://t.me/c/1/2",
			TargetChatID: 100,
			Status:       service.StatusQueued,
		},
	}
	capture := newLogCapture()
	loop := queuePullLoop{
		logger: capture.Logger(),
		queue:  q,
		tasks:  tasks,
		runner: fakeRunnerImpl{build: func(ctx context.Context, _ dl.DownloadRequest) (*exec.Cmd, error) {
			return exec.CommandContext(ctx, "sh", "-c", "printf 'out-line\\n'; printf 'err-line\\n' 1>&2"), nil
		}},
		maxAttempts: 3,
	}

	loop.processMessage(context.Background(), config.Config{Downloader: config.DownloaderConfig{TaskTimeoutMinutes: 1}}, queue.ReceivedMessage{
		LeaseID: "lease-stream",
		Body:    queue.Message{TaskID: "t-stream"},
	})

	records := capture.Records()
	if !containsTDLStreamLog(records, slog.LevelInfo, "stdout", "out-line") {
		t.Fatalf("expected stdout stream log, got %+v", records)
	}
	if !containsTDLStreamLog(records, slog.LevelWarn, "stderr", "err-line") {
		t.Fatalf("expected stderr stream log, got %+v", records)
	}
}

func TestTDLStreamLineWriterCRProgressLogsOnlyFinalLine(t *testing.T) {
	capture := newLogCapture()
	writer := newTDLStreamLineWriter(capture.Logger(), slog.LevelInfo, "t-cr", "stdout")

	_, _ = writer.Write([]byte("10%\r20%\r100%\r"))
	if got := collectTDLStreamLogs(capture.Records()); len(got) != 0 {
		t.Fatalf("expected no immediate progress logs, got %+v", got)
	}

	writer.Flush()

	logs := collectTDLStreamLogs(capture.Records())
	if len(logs) != 1 {
		t.Fatalf("expected one final progress log, got %+v", logs)
	}
	if logs[0].level != slog.LevelInfo || logs[0].stream != "stdout" || logs[0].line != "100%" {
		t.Fatalf("unexpected final progress log: %+v", logs[0])
	}
}

func TestTDLStreamLineWriterPercentProgressLogsOnlyFinalLine(t *testing.T) {
	capture := newLogCapture()
	writer := newTDLStreamLineWriter(capture.Logger(), slog.LevelInfo, "t-percent", "stdout")

	_, _ = writer.Write([]byte("progress 10% eta 5s\nprogress 60% eta 2s\nprogress 100% eta 0s\n"))
	if got := collectTDLStreamLogs(capture.Records()); len(got) != 0 {
		t.Fatalf("expected no immediate progress logs, got %+v", got)
	}

	writer.Flush()

	logs := collectTDLStreamLogs(capture.Records())
	if len(logs) != 1 {
		t.Fatalf("expected one final progress log, got %+v", logs)
	}
	if logs[0].line != "progress 100% eta 0s" {
		t.Fatalf("expected final percent progress log, got %+v", logs[0])
	}
}

func TestTDLStreamLineWriterByteProgressWithoutPercentLogsOnlyFinalLine(t *testing.T) {
	capture := newLogCapture()
	writer := newTDLStreamLineWriter(capture.Logger(), slog.LevelInfo, "t-byte", "stdout")

	_, _ = writer.Write([]byte("12.3 MiB / 100.0 MiB 2.1 MiB/s\n64.0 MiB / 100.0 MiB 2.1 MiB/s\n"))
	if got := collectTDLStreamLogs(capture.Records()); len(got) != 0 {
		t.Fatalf("expected no immediate byte-progress logs, got %+v", got)
	}

	writer.Flush()

	logs := collectTDLStreamLogs(capture.Records())
	if len(logs) != 1 {
		t.Fatalf("expected one final byte-progress log, got %+v", logs)
	}
	if logs[0].line != "64.0 MiB / 100.0 MiB 2.1 MiB/s" {
		t.Fatalf("expected final byte-progress log, got %+v", logs[0])
	}
}

func TestTDLStreamLineWriterRegularPercentLineNotSuppressed(t *testing.T) {
	capture := newLogCapture()
	writer := newTDLStreamLineWriter(capture.Logger(), slog.LevelWarn, "t-normal", "stderr")

	_, _ = writer.Write([]byte("CPU usage 95% exceeds threshold\n"))
	logs := collectTDLStreamLogs(capture.Records())
	if len(logs) != 1 {
		t.Fatalf("expected immediate stderr log, got %+v", logs)
	}
	if logs[0].level != slog.LevelWarn || logs[0].stream != "stderr" || logs[0].line != "CPU usage 95% exceeds threshold" {
		t.Fatalf("unexpected stderr log: %+v", logs[0])
	}

	writer.Flush()
	logs = collectTDLStreamLogs(capture.Records())
	if len(logs) != 1 {
		t.Fatalf("expected no extra log on flush, got %+v", logs)
	}
}

func TestQueuePullLoopLogsInvalidMessageAck(t *testing.T) {
	q := &fakeQueue{}
	capture := newLogCapture()
	loop := queuePullLoop{
		logger: capture.Logger(),
		queue:  q,
		tasks:  &fakeTasks{},
		runner: fakeRunnerImpl{build: func(context.Context, dl.DownloadRequest) (*exec.Cmd, error) {
			return nil, errors.New("not used")
		}},
		maxAttempts: 3,
	}

	loop.processMessage(context.Background(), config.Config{}, queue.ReceivedMessage{
		LeaseID: "lease-invalid",
		Body:    queue.Message{},
	})

	if len(q.acked) != 1 || q.acked[0] != "lease-invalid" {
		t.Fatalf("expected invalid lease to be acked, got %+v", q.acked)
	}
	if !containsMessage(capture.Messages(), "downloader invalid message acked") {
		t.Fatalf("expected invalid-message log, got %v", capture.Messages())
	}
}

type logCapture struct {
	handler *logCaptureHandler
}

func newLogCapture() *logCapture {
	return &logCapture{handler: &logCaptureHandler{}}
}

func (c *logCapture) Logger() *slog.Logger {
	return slog.New(c.handler)
}

func (c *logCapture) Messages() []string {
	c.handler.mu.Lock()
	defer c.handler.mu.Unlock()

	out := make([]string, 0, len(c.handler.records))
	for _, record := range c.handler.records {
		out = append(out, record.Message)
	}
	return out
}

func (c *logCapture) Records() []slog.Record {
	c.handler.mu.Lock()
	defer c.handler.mu.Unlock()

	out := make([]slog.Record, 0, len(c.handler.records))
	for _, record := range c.handler.records {
		out = append(out, record.Clone())
	}
	return out
}

type logCaptureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *logCaptureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *logCaptureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}
func (h *logCaptureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *logCaptureHandler) WithGroup(string) slog.Handler      { return h }

func containsMessage(messages []string, want string) bool {
	for _, msg := range messages {
		if msg == want {
			return true
		}
	}
	return false
}

func containsTDLStreamLog(records []slog.Record, level slog.Level, stream, line string) bool {
	for _, record := range records {
		if record.Message != "downloader tdl stream output" || record.Level != level {
			continue
		}
		if recordAttrEquals(record, "stream", stream) && recordAttrEquals(record, "line", line) {
			return true
		}
	}
	return false
}

type tdlStreamLogEntry struct {
	level  slog.Level
	stream string
	line   string
}

func collectTDLStreamLogs(records []slog.Record) []tdlStreamLogEntry {
	out := make([]tdlStreamLogEntry, 0)
	for _, record := range records {
		if record.Message != "downloader tdl stream output" {
			continue
		}
		out = append(out, tdlStreamLogEntry{
			level:  record.Level,
			stream: recordAttrString(record, "stream"),
			line:   recordAttrString(record, "line"),
		})
	}
	return out
}

func recordAttrEquals(record slog.Record, key, want string) bool {
	match := false
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == key && attr.Value.String() == want {
			match = true
			return false
		}
		return true
	})
	return match
}

func recordAttrString(record slog.Record, key string) string {
	value := ""
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == key {
			value = attr.Value.String()
			return false
		}
		return true
	})
	return value
}
