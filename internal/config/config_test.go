package config

import (
	"strings"
	"testing"
)

func TestLoadWebhookFallbackWhenURLMissing(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("TELEGRAM_USE_WEBHOOK", "true")
	t.Setenv("TELEGRAM_WEBHOOK_URL", "")
	t.Setenv("TELEGRAM_WEBHOOK_SECRET", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !cfg.Telegram.UseWebhook {
		t.Fatal("expected TELEGRAM_USE_WEBHOOK to be true")
	}
	if cfg.Telegram.WebhookURL != "" {
		t.Fatalf("expected empty webhook url, got %q", cfg.Telegram.WebhookURL)
	}
}

func TestLoadWebhookModeRequiresSecret(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("TELEGRAM_USE_WEBHOOK", "true")
	t.Setenv("TELEGRAM_WEBHOOK_URL", "https://example.com/bot-webhook")
	t.Setenv("TELEGRAM_WEBHOOK_SECRET", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "TELEGRAM_WEBHOOK_SECRET") {
		t.Fatalf("expected TELEGRAM_WEBHOOK_SECRET error, got %v", err)
	}
}

func TestLoadWebhookModeDefaultsListenAddr(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("TELEGRAM_USE_WEBHOOK", "true")
	t.Setenv("TELEGRAM_WEBHOOK_URL", "https://example.com/bot-webhook")
	t.Setenv("TELEGRAM_WEBHOOK_SECRET", "secret")
	t.Setenv("TELEGRAM_WEBHOOK_LISTEN_ADDR", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Telegram.WebhookListenAddr != ":8080" {
		t.Fatalf("expected default listen addr :8080, got %q", cfg.Telegram.WebhookListenAddr)
	}
}

func TestLoadRequiresD1DatabaseID(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("CF_D1_DATABASE_ID", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "CF_D1_DATABASE_ID") {
		t.Fatalf("expected CF_D1_DATABASE_ID error, got %v", err)
	}
}

func TestLoadForDownloaderDoesNotRequireTelegramToken(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("TELEGRAM_BOT_TOKEN", "")

	cfg, err := LoadForDownloader()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Telegram.BotToken != "" {
		t.Fatalf("expected empty bot token, got %q", cfg.Telegram.BotToken)
	}
}

func TestLoadRequiresDifferentStatusQueueID(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("CF_QUEUE_ID", "same-queue")
	t.Setenv("CF_STATUS_QUEUE_ID", "same-queue")

	_, err := Load()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "CF_STATUS_QUEUE_ID must be different from CF_QUEUE_ID") {
		t.Fatalf("expected queue id mismatch error, got %v", err)
	}
}

func setBaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_API_BASE", "https://api.telegram.org")
	t.Setenv("TELEGRAM_ALLOWED_USER_IDS", "")
	t.Setenv("TELEGRAM_USE_WEBHOOK", "false")
	t.Setenv("TELEGRAM_WEBHOOK_URL", "")
	t.Setenv("TELEGRAM_WEBHOOK_SECRET", "")
	t.Setenv("TELEGRAM_WEBHOOK_LISTEN_ADDR", ":8080")
	t.Setenv("CF_ACCOUNT_ID", "account")
	t.Setenv("CF_D1_DATABASE_ID", "d1-db-id")
	t.Setenv("CF_QUEUE_ID", "queue")
	t.Setenv("CF_STATUS_QUEUE_ID", "status-queue")
	t.Setenv("CF_API_TOKEN", "api-token")
	t.Setenv("CF_QUEUE_BATCH_SIZE", "5")
	t.Setenv("CF_QUEUE_VISIBILITY_TIMEOUT_MS", "900000")
	t.Setenv("CF_QUEUE_PULL_INTERVAL_MS", "3000")
	t.Setenv("TDL_BIN", "tdl")
	t.Setenv("TDL_NAMESPACE", "default")
	t.Setenv("TDL_STORAGE", "")
	t.Setenv("TDL_LOGIN_REQUIRED", "true")
	t.Setenv("TDL_LOGIN_CHECK_ON_START", "true")
	t.Setenv("DOWNLOADER_WORKERS", "2")
	t.Setenv("TASK_TIMEOUT_MINUTES", "180")
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("ENV", "test")
}
