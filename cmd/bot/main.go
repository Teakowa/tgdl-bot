package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"tgdl-bot/internal/bot"
	"tgdl-bot/internal/config"
	"tgdl-bot/internal/logging"
	"tgdl-bot/internal/queue"
	"tgdl-bot/internal/service"
	"tgdl-bot/internal/storage"
	"tgdl-bot/internal/telegram"
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

	d1Client := storage.NewD1Client(
		cfg.Cloudflare.AccountID,
		cfg.Cloudflare.D1DatabaseID,
		cfg.Cloudflare.APIToken,
		20*time.Second,
	)
	store := storage.NewD1Store(d1Client)
	if err := store.ApplyMigrations(ctx, storage.DefaultMigrations()...); err != nil {
		return fmt.Errorf("apply d1 migrations: %w", err)
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
		"commands", []string{"/start", "/help", "/status", "/last", "/queue", "/delete", "/retry"},
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
