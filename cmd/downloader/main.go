package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	neturl "net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"tgdl-bot/internal/config"
	dl "tgdl-bot/internal/downloader"
	"tgdl-bot/internal/logging"
	"tgdl-bot/internal/queue"
	"tgdl-bot/internal/service"
	"tgdl-bot/internal/storage"
	"tgdl-bot/internal/tasknotify"
	"tgdl-bot/internal/telegram"
)

type preflightHook interface {
	Check(context.Context, config.Config) error
}

type pullLoop interface {
	Run(context.Context, config.Config) error
}

type startupPreflightHook struct {
	logger *slog.Logger
	runner dl.Runner
}

func (h startupPreflightHook) Check(ctx context.Context, cfg config.Config) error {
	if h.logger == nil {
		return errors.New("logger is required")
	}

	h.logger.Info("running downloader preflight",
		"env", cfg.Environment,
		"login_required", cfg.Downloader.LoginRequired,
		"login_check_on_start", cfg.Downloader.LoginCheckOnStart,
	)

	checker := dl.StartupPreflight{Runner: h.runner}
	return checker.Check(ctx, dl.StartupConfig{
		Binary:        cfg.Downloader.Bin,
		Namespace:     cfg.Downloader.Namespace,
		Storage:       cfg.Downloader.Storage,
		LoginRequired: cfg.Downloader.LoginRequired && cfg.Downloader.LoginCheckOnStart,
	})
}

type queueConsumer interface {
	Pull(ctx context.Context, batchSize, visibilityTimeoutMs int) ([]queue.ReceivedMessage, error)
	Ack(ctx context.Context, leaseIDs []string) error
	Retry(ctx context.Context, leaseIDs []string) error
	Enqueue(ctx context.Context, message queue.Message) error
}

type taskService interface {
	UpdateTask(ctx context.Context, taskID string, update service.TaskUpdate) error
	ListFailedTasksForRetry(ctx context.Context, maxRetryCount int, limit int) ([]service.Task, error)
	ClaimTaskForExecution(ctx context.Context, req service.ClaimTaskExecutionRequest) (service.Task, bool, error)
	GetTask(ctx context.Context, taskID string) (service.Task, error)
}

type taskStatusNotifier interface {
	Notify(ctx context.Context, task service.Task) error
}

type queuePullLoop struct {
	logger      *slog.Logger
	queue       queueConsumer
	tasks       taskService
	runner      dl.Runner
	notifier    taskStatusNotifier
	maxAttempts int
}

func (l queuePullLoop) Run(ctx context.Context, cfg config.Config) error {
	if l.logger == nil {
		return errors.New("logger is required")
	}
	if l.queue == nil {
		return errors.New("queue client is required")
	}
	if l.tasks == nil {
		return errors.New("task service is required")
	}

	pullInterval := time.Duration(cfg.Cloudflare.QueuePullIntervalMS) * time.Millisecond
	if pullInterval <= 0 {
		pullInterval = 3 * time.Second
	}

	l.logger.Info("downloader pull loop started",
		"workers", cfg.Downloader.Workers,
		"queue_batch_size", cfg.Cloudflare.QueueBatchSize,
	)
	l.requeueFailedTasksOnStartup(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		messages, err := l.queue.Pull(ctx, cfg.Cloudflare.QueueBatchSize, cfg.Cloudflare.QueueVisibilityTimeoutMS)
		if err != nil {
			l.logger.Error("queue pull failed", "error", err)
			time.Sleep(pullInterval)
			continue
		}
		if len(messages) == 0 {
			time.Sleep(pullInterval)
			continue
		}

		for _, message := range messages {
			l.processMessage(ctx, cfg, message)
		}
	}
}

func (l queuePullLoop) requeueFailedTasksOnStartup(ctx context.Context) {
	const retryScanLimit = 200

	if l.maxAttempts <= 0 {
		return
	}

	tasks, err := l.tasks.ListFailedTasksForRetry(ctx, l.maxAttempts, retryScanLimit)
	if err != nil {
		l.logger.Error("downloader startup retry scan failed", "error", err)
		return
	}
	if len(tasks) == 0 {
		return
	}

	for _, task := range tasks {
		requeueMessage := queue.Message{
			TaskID:       task.TaskID,
			ChatID:       task.ChatID,
			UserID:       task.UserID,
			TargetChatID: task.TargetChatID,
			URL:          task.URL,
			CreatedAt:    time.Now().UTC(),
			Idempotency:  task.IdempotencyKey,
		}
		if err := l.queue.Enqueue(ctx, requeueMessage); err != nil {
			l.logger.Error("downloader startup retry enqueue failed",
				"task_id", task.TaskID,
				"retry_count", task.RetryCount,
				"error", err,
			)
			continue
		}

		status := service.StatusRetrying
		if err := l.tasks.UpdateTask(ctx, task.TaskID, service.TaskUpdate{Status: status}); err != nil {
			l.logger.Error("downloader startup retry status update failed",
				"task_id", task.TaskID,
				"status_to", status,
				"error", err,
			)
			continue
		}

		l.logger.Info("downloader startup retry enqueued",
			"task_id", task.TaskID,
			"retry_count", task.RetryCount,
			"status_to", status,
		)
		task.Status = status
		l.notifyTaskStatus(ctx, task)
	}
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel)
	d1Client := storage.NewD1Client(
		cfg.Cloudflare.AccountID,
		cfg.Cloudflare.D1DatabaseID,
		cfg.Cloudflare.APIToken,
		20*time.Second,
	)
	store := storage.NewD1Store(d1Client)
	if err := store.ApplyMigrations(context.Background(), storage.DefaultMigrations()...); err != nil {
		logger.Error("downloader exited", "error", fmt.Errorf("apply d1 migrations: %w", err))
		os.Exit(1)
	}

	taskService := service.NewTaskService(store.TaskRepository())
	queueClient := queue.NewCloudflareClient(cfg.Cloudflare.AccountID, cfg.Cloudflare.QueueID, cfg.Cloudflare.APIToken, 20*time.Second)
	runner := dl.DefaultRunner{PreflightChecker: dl.NewTDLPreflightChecker()}
	notifyClient := telegram.NewHTTPClient(cfg.Telegram.APIBase, cfg.Telegram.BotToken, 15*time.Second)
	statusNotifier := tasknotify.Notifier{Client: notifyClient, Logger: logger}

	if err := run(context.Background(), cfg, logger, startupPreflightHook{logger: logger, runner: runner}, queuePullLoop{
		logger:      logger,
		queue:       queueClient,
		tasks:       taskService,
		runner:      runner,
		notifier:    statusNotifier,
		maxAttempts: 3,
	}); err != nil {
		logger.Error("downloader exited", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config, logger *slog.Logger, preflight preflightHook, loop pullLoop) error {
	if logger == nil {
		return errors.New("logger is required")
	}
	if preflight == nil {
		return errors.New("preflight hook is required")
	}
	if loop == nil {
		return errors.New("pull loop is required")
	}

	logger.Info("downloader entrypoint initialized",
		"env", cfg.Environment,
		"queue_id", cfg.Cloudflare.QueueID,
	)

	if err := preflight.Check(ctx, cfg); err != nil {
		return err
	}

	return loop.Run(ctx, cfg)
}

func (l queuePullLoop) processMessage(ctx context.Context, cfg config.Config, message queue.ReceivedMessage) {
	if message.LeaseID == "" || message.Body.TaskID == "" {
		if message.LeaseID != "" {
			l.logger.Warn("downloader invalid message acked",
				"lease_id", message.LeaseID,
				"has_task_id", message.Body.TaskID != "",
			)
			_ = l.queue.Ack(ctx, []string{message.LeaseID})
		}
		return
	}
	l.logger.Info("downloader message pulled",
		"task_id", message.Body.TaskID,
		"lease_id", message.LeaseID,
	)

	task, claimed, err := l.tasks.ClaimTaskForExecution(ctx, service.ClaimTaskExecutionRequest{
		TaskID:    message.Body.TaskID,
		LeaseID:   message.LeaseID,
		StartedAt: time.Now().UTC(),
	})
	if err != nil {
		l.logger.Error("claim task failed", "task_id", message.Body.TaskID, "error", err)
		_ = l.queue.Retry(ctx, []string{message.LeaseID})
		return
	}
	if !claimed {
		l.logger.Info("downloader task not claimable, ack lease",
			"task_id", message.Body.TaskID,
			"lease_id", message.LeaseID,
		)
		_ = l.queue.Ack(ctx, []string{message.LeaseID})
		return
	}

	l.logger.Info("downloader task state updated",
		"task_id", task.TaskID,
		"lease_id", message.LeaseID,
		"status_to", service.StatusRunning,
	)
	task.Status = service.StatusRunning
	task.ErrorMessage = nil
	l.notifyTaskStatus(ctx, task)

	result, errorClass, runErr := l.executeTask(ctx, cfg, task)
	if runErr == nil {
		finishedAt := time.Now().UTC()
		out := strings.TrimSpace(strings.TrimSpace(result.Stdout + "\n" + result.Stderr))
		if err := l.tasks.UpdateTask(ctx, task.TaskID, service.TaskUpdate{
			Status:        service.StatusDone,
			OutputSummary: &out,
			FinishedAt:    &finishedAt,
		}); err != nil {
			l.logger.Error("mark task done failed", "task_id", task.TaskID, "error", err)
			_ = l.queue.Retry(ctx, []string{message.LeaseID})
			return
		}
		l.logger.Info("downloader task state updated",
			"task_id", task.TaskID,
			"lease_id", message.LeaseID,
			"status_from", service.StatusRunning,
			"status_to", service.StatusDone,
			"duration_ms", result.Duration.Milliseconds(),
		)
		task.Status = service.StatusDone
		task.OutputSummary = &out
		task.FinishedAt = &finishedAt
		task.ErrorMessage = nil
		l.notifyFinalTaskStatus(ctx, task)
		l.logger.Info("downloader queue action",
			"task_id", task.TaskID,
			"lease_id", message.LeaseID,
			"action", "ack",
		)
		_ = l.queue.Ack(ctx, []string{message.LeaseID})
		return
	}

	retryCount := task.RetryCount + 1
	finishedAt := time.Now().UTC()
	errorMessage := runErr.Error()
	status := service.StatusFailed
	shouldRetry := false
	if errorClass == dl.ErrorClassRetryable && retryCount < l.maxAttempts {
		status = service.StatusRetrying
		shouldRetry = true
	}
	if errorClass == dl.ErrorClassRetryable && retryCount >= l.maxAttempts {
		status = service.StatusDeadLettered
	}

	update := service.TaskUpdate{
		Status:       status,
		RetryCount:   &retryCount,
		ErrorMessage: &errorMessage,
		FinishedAt:   &finishedAt,
	}
	if result.ExitCode != 0 {
		exitCode := result.ExitCode
		update.ExitCode = &exitCode
	}
	if err := l.tasks.UpdateTask(ctx, task.TaskID, update); err != nil {
		l.logger.Error("update failed task state failed", "task_id", task.TaskID, "error", err)
		_ = l.queue.Retry(ctx, []string{message.LeaseID})
		return
	}
	l.logger.Info("downloader task state updated",
		"task_id", task.TaskID,
		"lease_id", message.LeaseID,
		"status_from", service.StatusRunning,
		"status_to", status,
		"retry_count", retryCount,
		"exit_code", result.ExitCode,
		"error_class", errorClass,
	)
	task.Status = status
	task.RetryCount = retryCount
	task.ErrorMessage = &errorMessage
	task.FinishedAt = &finishedAt
	if result.ExitCode != 0 {
		exitCode := result.ExitCode
		task.ExitCode = &exitCode
	}
	l.notifyFinalTaskStatus(ctx, task)

	if shouldRetry {
		l.logger.Info("downloader queue action",
			"task_id", task.TaskID,
			"lease_id", message.LeaseID,
			"action", "retry",
			"error_class", errorClass,
		)
		_ = l.queue.Retry(ctx, []string{message.LeaseID})
		return
	}
	l.logger.Info("downloader queue action",
		"task_id", task.TaskID,
		"lease_id", message.LeaseID,
		"action", "ack",
		"error_class", errorClass,
	)
	_ = l.queue.Ack(ctx, []string{message.LeaseID})
}

func (l queuePullLoop) notifyTaskStatus(ctx context.Context, task service.Task) {
	if l.notifier == nil {
		return
	}

	notifyCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := l.notifier.Notify(notifyCtx, task); err != nil {
		l.logger.Warn("downloader task status notification failed",
			"task_id", task.TaskID,
			"status", task.Status,
			"error", err,
		)
	}
}

func (l queuePullLoop) notifyFinalTaskStatus(ctx context.Context, fallbackTask service.Task) {
	if l.tasks == nil {
		l.notifyTaskStatus(ctx, fallbackTask)
		return
	}

	freshTask, err := l.tasks.GetTask(ctx, fallbackTask.TaskID)
	if err != nil {
		l.logger.Warn("downloader refresh task before notification failed",
			"task_id", fallbackTask.TaskID,
			"status", fallbackTask.Status,
			"error", err,
		)
		l.notifyTaskStatus(ctx, fallbackTask)
		return
	}
	l.notifyTaskStatus(ctx, freshTask)
}

func (l queuePullLoop) executeTask(ctx context.Context, cfg config.Config, task service.Task) (dl.RunResult, dl.ErrorClass, error) {
	timeout := time.Duration(cfg.Downloader.TaskTimeoutMinutes) * time.Minute
	if timeout <= 0 {
		timeout = 60 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd, err := l.runner.BuildCommand(runCtx, dl.DownloadRequest{
		URL:          task.URL,
		TargetChatID: task.TargetChatID,
		Binary:       cfg.Downloader.Bin,
		Namespace:    cfg.Downloader.Namespace,
		Storage:      cfg.Downloader.Storage,
	})
	if err != nil {
		return dl.RunResult{}, dl.ErrorClassNonRetryable, errors.Join(dl.ErrNonRetryable, err)
	}
	l.logger.Info("downloader tdl execution started",
		"task_id", task.TaskID,
		"command", sanitizeCommand(resultCommand(cmd)),
	)

	var stdout, stderr bytes.Buffer
	stdoutLogWriter := newTDLStreamLineWriter(l.logger, slog.LevelInfo, task.TaskID, "stdout")
	stderrLogWriter := newTDLStreamLineWriter(l.logger, slog.LevelWarn, task.TaskID, "stderr")
	cmd.Stdout = io.MultiWriter(&stdout, stdoutLogWriter)
	cmd.Stderr = io.MultiWriter(&stderr, stderrLogWriter)

	started := time.Now()
	runErr := cmd.Run()
	stdoutLogWriter.Flush()
	stderrLogWriter.Flush()
	result := dl.RunResult{
		Command:  append([]string{cmd.Path}, cmd.Args[1:]...),
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: time.Since(started),
	}
	if runErr == nil {
		l.logger.Info("downloader tdl execution finished",
			"task_id", task.TaskID,
			"duration_ms", result.Duration.Milliseconds(),
			"exit_code", 0,
			"error_class", "none",
		)
		return result, dl.ErrorClassUnknown, nil
	}

	result.ExitCode = exitCodeFrom(runErr)
	errorClass := classifyTDLError(runCtx, result, runErr)
	l.logger.Info("downloader tdl execution finished",
		"task_id", task.TaskID,
		"duration_ms", result.Duration.Milliseconds(),
		"exit_code", result.ExitCode,
		"error_class", errorClass,
	)
	if errorClass == dl.ErrorClassRetryable {
		if runCtx.Err() != nil {
			return result, errorClass, errors.Join(dl.ErrRetryable, runErr, runCtx.Err())
		}
		return result, errorClass, errors.Join(dl.ErrRetryable, runErr)
	}
	return result, errorClass, errors.Join(dl.ErrNonRetryable, runErr)
}

func exitCodeFrom(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 0
}

func classifyTDLError(runCtx context.Context, result dl.RunResult, runErr error) dl.ErrorClass {
	if runErr == nil {
		return dl.ErrorClassUnknown
	}

	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return dl.ErrorClassNonRetryable
	}
	if errors.Is(runCtx.Err(), context.Canceled) {
		return dl.ErrorClassRetryable
	}

	var netErr net.Error
	if errors.As(runErr, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return dl.ErrorClassRetryable
	}

	text := strings.ToLower(strings.Join([]string{
		runErr.Error(),
		result.Stderr,
		result.Stdout,
	}, "\n"))
	for _, kw := range nonRetryableCLIErrorKeywords {
		if strings.Contains(text, kw) {
			return dl.ErrorClassNonRetryable
		}
	}
	for _, kw := range transientErrorKeywords {
		if strings.Contains(text, kw) {
			return dl.ErrorClassRetryable
		}
	}

	return dl.ErrorClassNonRetryable
}

var transientErrorKeywords = []string{
	"timeout",
	"i/o timeout",
	"connection reset",
	"connection aborted",
	"connection refused",
	"broken pipe",
	"network is unreachable",
	"transport is closing",
	"tls handshake timeout",
	"eof",
}

var nonRetryableCLIErrorKeywords = []string{
	"unknown shorthand flag",
	"unknown flag",
	"usage:",
	"flag needs an argument",
}

func resultCommand(cmd *exec.Cmd) []string {
	return append([]string{cmd.Path}, cmd.Args[1:]...)
}

type tdlStreamLineWriter struct {
	logger *slog.Logger
	level  slog.Level
	taskID string
	stream string

	mu              sync.Mutex
	buf             bytes.Buffer
	pendingProgress string
}

func newTDLStreamLineWriter(logger *slog.Logger, level slog.Level, taskID, stream string) *tdlStreamLineWriter {
	return &tdlStreamLineWriter{
		logger: logger,
		level:  level,
		taskID: taskID,
		stream: stream,
	}
}

func (w *tdlStreamLineWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	written := len(p)
	remaining := p
	for len(remaining) > 0 {
		index, separator, width := nextSeparator(remaining)
		if index < 0 {
			_, _ = w.buf.Write(remaining)
			break
		}

		_, _ = w.buf.Write(remaining[:index])
		w.emitCompletedLineLocked(separator)
		remaining = remaining[index+width:]
	}
	return written, nil
}

func (w *tdlStreamLineWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()

	remaining := normalizeStreamLine(w.buf.String())
	w.buf.Reset()
	if remaining != "" && isLikelyProgressLine(remaining) {
		w.pendingProgress = remaining
	}
	w.flushPendingProgressLocked()
	if remaining != "" && !isLikelyProgressLine(remaining) {
		w.emitLocked(remaining)
	}
}

func (w *tdlStreamLineWriter) emitLocked(line string) {
	if w.logger == nil || line == "" {
		return
	}

	w.logger.Log(context.Background(), w.level, "downloader tdl stream output",
		"task_id", w.taskID,
		"stream", w.stream,
		"line", line,
	)
}

func nextSeparator(raw []byte) (index int, separator byte, width int) {
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '\n':
			return i, '\n', 1
		case '\r':
			if i+1 < len(raw) && raw[i+1] == '\n' {
				return i, '\n', 2
			}
			return i, '\r', 1
		}
	}
	return -1, 0, 0
}

func (w *tdlStreamLineWriter) emitCompletedLineLocked(separator byte) {
	line := normalizeStreamLine(w.buf.String())
	w.buf.Reset()
	w.handleLineLocked(line, separator == '\r')
}

func (w *tdlStreamLineWriter) handleLineLocked(line string, isCarriageReturn bool) {
	if line == "" {
		return
	}
	if isCarriageReturn || isLikelyProgressLine(line) {
		w.pendingProgress = line
		return
	}
	w.emitLocked(line)
}

func (w *tdlStreamLineWriter) flushPendingProgressLocked() {
	if w.pendingProgress == "" {
		return
	}
	w.emitLocked(w.pendingProgress)
	w.pendingProgress = ""
}

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
var percentTokenPattern = regexp.MustCompile(`\b\d{1,3}(?:\.\d+)?%`)
var percentOnlyPattern = regexp.MustCompile(`^\d{1,3}(?:\.\d+)?%$`)
var byteTokenPattern = regexp.MustCompile(`\b\d+(?:\.\d+)?\s*(?:[kmgt]?i?b)\b`)

func isLikelyProgressLine(line string) bool {
	clean := strings.TrimSpace(strings.ToLower(stripANSIEscapes(line)))
	if clean == "" {
		return false
	}

	if isPercentProgressLine(clean) {
		return true
	}

	byteTokens := byteTokenPattern.FindAllString(clean, -1)
	if len(byteTokens) >= 2 && strings.Contains(clean, "/") {
		return true
	}
	if len(byteTokens) >= 1 && strings.Contains(clean, "/s") {
		for _, marker := range []string{"eta", "remaining", "elapsed", "download", "progress"} {
			if strings.Contains(clean, marker) {
				return true
			}
		}
	}
	return false
}

func isPercentProgressLine(clean string) bool {
	if !strings.Contains(clean, "%") {
		return false
	}
	if !percentTokenPattern.MatchString(clean) {
		return false
	}
	if percentOnlyPattern.MatchString(clean) {
		return true
	}

	for _, marker := range []string{
		"eta",
		"/s",
		"kb",
		"mb",
		"gb",
		"ib",
		"progress",
		"download",
		"remaining",
		"elapsed",
		" of ",
		"[",
		"]",
		"(",
		")",
	} {
		if strings.Contains(clean, marker) {
			return true
		}
	}
	return false
}

func stripANSIEscapes(line string) string {
	return ansiEscapePattern.ReplaceAllString(line, "")
}

func normalizeStreamLine(line string) string {
	return strings.TrimSuffix(line, "\r")
}

func sanitizeCommand(command []string) []string {
	if len(command) == 0 {
		return nil
	}

	safe := append([]string(nil), command...)
	for i := range safe {
		if i > 0 && (safe[i-1] == "-u" || safe[i-1] == "--from") {
			safe[i] = redactURL(safe[i])
			continue
		}
		if i > 0 && safe[i-1] == "--storage" {
			safe[i] = "[redacted]"
			continue
		}
		if strings.HasPrefix(safe[i], "--storage=") {
			safe[i] = "--storage=[redacted]"
		}
	}
	return safe
}

func redactURL(raw string) string {
	u, err := neturl.Parse(strings.TrimSpace(raw))
	if err != nil {
		return raw
	}

	path := u.Path
	if len(path) > 18 {
		path = path[:18] + "..."
	}
	return fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, path)
}
