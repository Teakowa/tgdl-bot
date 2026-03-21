package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"tgdl-bot/internal/config"
	"tgdl-bot/internal/logging"
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

	logger.Info("bot entrypoint initialized",
		"env", cfg.Environment,
		"telegram_api_base", cfg.Telegram.APIBase,
		"webhook_enabled", cfg.Telegram.UseWebhook,
	)

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return nil
}
