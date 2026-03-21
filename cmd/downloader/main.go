package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"tgdl-bot/internal/config"
	dl "tgdl-bot/internal/downloader"
	"tgdl-bot/internal/logging"
	"tgdl-bot/internal/queue"
	"tgdl-bot/internal/service"
	"tgdl-bot/internal/storage"

	_ "modernc.org/sqlite"
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
}

type taskService interface {
	GetTask(ctx context.Context, taskID string) (service.Task, error)
	UpdateTask(ctx context.Context, taskID string, update service.TaskUpdate) error
}

type queuePullLoop struct {
	logger      *slog.Logger
	queue       queueConsumer
	tasks       taskService
	runner      dl.Runner
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

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel)
	db, err := openSQLite(cfg.Storage.SQLitePath)
	if err != nil {
		logger.Error("downloader exited", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	store := storage.NewSQLiteStore(db)
	if err := store.ApplyMigrations(context.Background(), storage.DefaultMigrations()...); err != nil {
		logger.Error("downloader exited", "error", fmt.Errorf("apply sqlite migrations: %w", err))
		os.Exit(1)
	}

	taskService := service.NewTaskService(store.TaskRepository())
	queueClient := queue.NewCloudflareClient(cfg.Cloudflare.AccountID, cfg.Cloudflare.QueueID, cfg.Cloudflare.APIToken, 20*time.Second)
	runner := dl.DefaultRunner{PreflightChecker: dl.NewTDLPreflightChecker()}

	if err := run(context.Background(), cfg, logger, startupPreflightHook{logger: logger, runner: runner}, queuePullLoop{
		logger:      logger,
		queue:       queueClient,
		tasks:       taskService,
		runner:      runner,
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
			_ = l.queue.Ack(ctx, []string{message.LeaseID})
		}
		return
	}

	task, err := l.tasks.GetTask(ctx, message.Body.TaskID)
	if err != nil {
		if errors.Is(err, service.ErrTaskNotFound) {
			_ = l.queue.Ack(ctx, []string{message.LeaseID})
			return
		}
		l.logger.Error("load task failed", "task_id", message.Body.TaskID, "error", err)
		_ = l.queue.Retry(ctx, []string{message.LeaseID})
		return
	}

	if task.Status == service.StatusDone {
		_ = l.queue.Ack(ctx, []string{message.LeaseID})
		return
	}

	startedAt := time.Now().UTC()
	leaseID := message.LeaseID
	if err := l.tasks.UpdateTask(ctx, task.TaskID, service.TaskUpdate{
		Status:    service.StatusRunning,
		LeaseID:   &leaseID,
		StartedAt: &startedAt,
	}); err != nil {
		l.logger.Error("mark task running failed", "task_id", task.TaskID, "error", err)
		_ = l.queue.Retry(ctx, []string{message.LeaseID})
		return
	}

	result, runErr := l.executeTask(ctx, cfg, task)
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
		_ = l.queue.Ack(ctx, []string{message.LeaseID})
		return
	}

	retryCount := task.RetryCount + 1
	finishedAt := time.Now().UTC()
	errorMessage := runErr.Error()
	status := service.StatusFailed
	shouldRetry := false
	if dl.IsRetryableError(runErr) && retryCount < l.maxAttempts {
		status = service.StatusRetrying
		shouldRetry = true
	}
	if dl.IsRetryableError(runErr) && retryCount >= l.maxAttempts {
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

	if shouldRetry {
		_ = l.queue.Retry(ctx, []string{message.LeaseID})
		return
	}
	_ = l.queue.Ack(ctx, []string{message.LeaseID})
}

func (l queuePullLoop) executeTask(ctx context.Context, cfg config.Config, task service.Task) (dl.RunResult, error) {
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
		return dl.RunResult{}, errors.Join(dl.ErrNonRetryable, err)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	started := time.Now()
	runErr := cmd.Run()
	result := dl.RunResult{
		Command:  append([]string{cmd.Path}, cmd.Args[1:]...),
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: time.Since(started),
	}
	if runErr == nil {
		return result, nil
	}

	result.ExitCode = exitCodeFrom(runErr)
	if classifyTDLError(runCtx, result, runErr) == dl.ErrorClassRetryable {
		if runCtx.Err() != nil {
			return result, errors.Join(dl.ErrRetryable, runErr, runCtx.Err())
		}
		return result, errors.Join(dl.ErrRetryable, runErr)
	}
	return result, errors.Join(dl.ErrNonRetryable, runErr)
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

	if errors.Is(runCtx.Err(), context.DeadlineExceeded) || errors.Is(runCtx.Err(), context.Canceled) {
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

func openSQLite(path string) (*sql.DB, error) {
	if path == "" {
		return nil, errors.New("empty sqlite path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite dir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set sqlite busy_timeout: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return db, nil
}
