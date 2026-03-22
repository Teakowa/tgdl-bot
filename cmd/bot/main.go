package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"tgdl-bot/internal/bot"
	"tgdl-bot/internal/config"
	"tgdl-bot/internal/logging"
	"tgdl-bot/internal/queue"
	"tgdl-bot/internal/service"
	"tgdl-bot/internal/storage"
	"tgdl-bot/internal/telegram"

	_ "modernc.org/sqlite"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel)
	if err := run(context.Background(), cfg, logger); err != nil {
		logger.Error("bot exited", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	if logger == nil {
		return errors.New("logger is required")
	}

	db, err := openSQLite(cfg.Storage.SQLitePath)
	if err != nil {
		return err
	}
	defer db.Close()

	store := storage.NewSQLiteStore(db)
	if err := store.ApplyMigrations(ctx, storage.DefaultMigrations()...); err != nil {
		return fmt.Errorf("apply sqlite migrations: %w", err)
	}
	if err := storage.EnsureTaskColumns(ctx, db); err != nil {
		return fmt.Errorf("ensure sqlite task columns: %w", err)
	}
	taskService := service.NewTaskService(store.TaskRepository())
	queueClient := queue.NewCloudflareClient(cfg.Cloudflare.AccountID, cfg.Cloudflare.QueueID, cfg.Cloudflare.APIToken, 20*time.Second)

	handler := bot.Handler{
		AllowedUserIDs: cfg.Telegram.AllowedUserIDs,
		Tasks:          taskService,
		Queue:          queueClient,
		Logger:         logger,
	}

	logger.Info("bot entrypoint initialized",
		"env", cfg.Environment,
		"telegram_api_base", cfg.Telegram.APIBase,
		"webhook_enabled", cfg.Telegram.UseWebhook,
		"webhook_configured", cfg.Telegram.WebhookURL != "",
		"webhook_addr", cfg.Telegram.WebhookListenAddr,
		"commands", []string{"/start", "/help", "/status", "/last"},
		"allowlist_size", len(handler.AllowedUserIDs),
	)
	client := telegram.NewHTTPClient(cfg.Telegram.APIBase, cfg.Telegram.BotToken, 35*time.Second)
	runtime := bot.Runtime{
		Client:         client,
		Handler:        handler,
		Logger:         logger,
		PollInterval:   1200 * time.Millisecond,
		PollLimit:      50,
		TimeoutSeconds: 30,
		UseWebhook:     cfg.Telegram.UseWebhook,
		WebhookURL:     cfg.Telegram.WebhookURL,
		WebhookSecret:  cfg.Telegram.WebhookSecret,
		WebhookAddr:    cfg.Telegram.WebhookListenAddr,
	}
	return runtime.Run(ctx)
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
