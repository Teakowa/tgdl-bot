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

	handler := bot.Handler{
		AllowedUserIDs: cfg.Telegram.AllowedUserIDs,
	}

	logger.Info("bot entrypoint initialized",
		"env", cfg.Environment,
		"telegram_api_base", cfg.Telegram.APIBase,
		"webhook_enabled", cfg.Telegram.UseWebhook,
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
	}
	return runtime.Run(ctx)
}
