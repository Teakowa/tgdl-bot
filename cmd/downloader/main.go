package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"tgdl-bot/internal/config"
	dl "tgdl-bot/internal/downloader"
	"tgdl-bot/internal/logging"
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
		DownloadDir:   cfg.Downloader.DownloadDir,
		Namespace:     cfg.Downloader.Namespace,
		Storage:       cfg.Downloader.Storage,
		LoginRequired: cfg.Downloader.LoginRequired && cfg.Downloader.LoginCheckOnStart,
	})
}

type noopPullLoop struct {
	logger *slog.Logger
}

func (l noopPullLoop) Run(ctx context.Context, cfg config.Config) error {
	if l.logger == nil {
		return errors.New("logger is required")
	}

	l.logger.Info("downloader pull loop placeholder started",
		"workers", cfg.Downloader.Workers,
		"queue_batch_size", cfg.Cloudflare.QueueBatchSize,
	)

	<-ctx.Done()
	return ctx.Err()
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel)
	if err := run(context.Background(), cfg, logger, startupPreflightHook{logger: logger, runner: dl.DefaultRunner{}}, noopPullLoop{logger: logger}); err != nil {
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
		"download_dir", cfg.Downloader.DownloadDir,
		"queue_id", cfg.Cloudflare.QueueID,
	)

	if err := preflight.Check(ctx, cfg); err != nil {
		return err
	}

	return loop.Run(ctx, cfg)
}
