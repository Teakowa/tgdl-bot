package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultTelegramAPIBase          = "https://api.telegram.org"
	defaultWebhookListenAddr        = ":8080"
	defaultCloudflareQueueBatchSize = 5
	defaultQueueVisibilityTimeoutMS = 15 * 60 * 1000
	defaultQueuePullIntervalMS      = 3000
	defaultTDLBin                   = "tdl"
	defaultTDLNamespace             = "default"
	defaultDownloaderWorkers        = 2
	defaultTaskTimeoutMinutes       = 180
	defaultLogLevel                 = "info"
	defaultEnvironment              = "dev"
)

type Config struct {
	Environment string
	LogLevel    string

	Telegram   TelegramConfig
	Cloudflare CloudflareConfig
	Downloader DownloaderConfig
}

type TelegramConfig struct {
	BotToken          string
	APIBase           string
	UseWebhook        bool
	WebhookURL        string
	WebhookSecret     string
	WebhookListenAddr string
	AllowedUserIDs    []int64
}

type CloudflareConfig struct {
	AccountID                string
	D1DatabaseID             string
	QueueID                  string
	StatusQueueID            string
	APIToken                 string
	QueueBatchSize           int
	QueueVisibilityTimeoutMS int
	QueuePullIntervalMS      int
}

type DownloaderConfig struct {
	Bin                string
	Namespace          string
	Storage            string
	LoginRequired      bool
	LoginCheckOnStart  bool
	Workers            int
	TaskTimeoutMinutes int
}

func Load() (Config, error) {
	return LoadForBot()
}

func LoadForBot() (Config, error) {
	return load(loadTargetBot)
}

func LoadForDownloader() (Config, error) {
	return load(loadTargetDownloader)
}

type loadTarget int

const (
	loadTargetBot loadTarget = iota + 1
	loadTargetDownloader
)

func load(target loadTarget) (Config, error) {
	_ = loadDotEnv(".env")

	cfg := Config{
		Environment: getEnvOrDefault("ENV", defaultEnvironment),
		LogLevel:    getEnvOrDefault("LOG_LEVEL", defaultLogLevel),
		Telegram: TelegramConfig{
			BotToken:          strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
			APIBase:           normalizeURL(getEnvOrDefault("TELEGRAM_API_BASE", defaultTelegramAPIBase)),
			UseWebhook:        getBoolEnv("TELEGRAM_USE_WEBHOOK", false),
			WebhookURL:        strings.TrimSpace(os.Getenv("TELEGRAM_WEBHOOK_URL")),
			WebhookSecret:     strings.TrimSpace(os.Getenv("TELEGRAM_WEBHOOK_SECRET")),
			WebhookListenAddr: strings.TrimSpace(getEnvOrDefault("TELEGRAM_WEBHOOK_LISTEN_ADDR", defaultWebhookListenAddr)),
		},
		Cloudflare: CloudflareConfig{
			AccountID:                strings.TrimSpace(os.Getenv("CF_ACCOUNT_ID")),
			D1DatabaseID:             strings.TrimSpace(os.Getenv("CF_D1_DATABASE_ID")),
			QueueID:                  strings.TrimSpace(os.Getenv("CF_QUEUE_ID")),
			StatusQueueID:            strings.TrimSpace(os.Getenv("CF_STATUS_QUEUE_ID")),
			APIToken:                 strings.TrimSpace(os.Getenv("CF_API_TOKEN")),
			QueueBatchSize:           getIntEnv("CF_QUEUE_BATCH_SIZE", defaultCloudflareQueueBatchSize),
			QueueVisibilityTimeoutMS: getIntEnv("CF_QUEUE_VISIBILITY_TIMEOUT_MS", defaultQueueVisibilityTimeoutMS),
			QueuePullIntervalMS:      getIntEnv("CF_QUEUE_PULL_INTERVAL_MS", defaultQueuePullIntervalMS),
		},
		Downloader: DownloaderConfig{
			Bin:                getEnvOrDefault("TDL_BIN", defaultTDLBin),
			Namespace:          getEnvOrDefault("TDL_NAMESPACE", defaultTDLNamespace),
			Storage:            strings.TrimSpace(os.Getenv("TDL_STORAGE")),
			LoginRequired:      getBoolEnv("TDL_LOGIN_REQUIRED", true),
			LoginCheckOnStart:  getBoolEnv("TDL_LOGIN_CHECK_ON_START", true),
			Workers:            getIntEnv("DOWNLOADER_WORKERS", defaultDownloaderWorkers),
			TaskTimeoutMinutes: getIntEnv("TASK_TIMEOUT_MINUTES", defaultTaskTimeoutMinutes),
		},
	}

	cfg.Telegram.AllowedUserIDs = parseInt64List(os.Getenv("TELEGRAM_ALLOWED_USER_IDS"))

	var errs []error
	if target == loadTargetBot {
		validateRequired(&errs, "TELEGRAM_BOT_TOKEN", cfg.Telegram.BotToken)
	}
	validateRequired(&errs, "CF_ACCOUNT_ID", cfg.Cloudflare.AccountID)
	validateRequired(&errs, "CF_D1_DATABASE_ID", cfg.Cloudflare.D1DatabaseID)
	validateRequired(&errs, "CF_QUEUE_ID", cfg.Cloudflare.QueueID)
	validateRequired(&errs, "CF_STATUS_QUEUE_ID", cfg.Cloudflare.StatusQueueID)
	validateRequired(&errs, "CF_API_TOKEN", cfg.Cloudflare.APIToken)
	if cfg.Cloudflare.QueueID != "" && cfg.Cloudflare.StatusQueueID != "" && cfg.Cloudflare.QueueID == cfg.Cloudflare.StatusQueueID {
		errs = append(errs, fmt.Errorf("CF_STATUS_QUEUE_ID must be different from CF_QUEUE_ID"))
	}

	webhookMode := cfg.Telegram.UseWebhook && cfg.Telegram.WebhookURL != ""
	if webhookMode && cfg.Telegram.WebhookSecret == "" {
		errs = append(errs, fmt.Errorf("TELEGRAM_WEBHOOK_SECRET is required when webhook mode is enabled"))
	}
	if webhookMode && cfg.Telegram.WebhookListenAddr == "" {
		errs = append(errs, fmt.Errorf("TELEGRAM_WEBHOOK_LISTEN_ADDR is required when webhook mode is enabled"))
	}
	if cfg.Cloudflare.QueueBatchSize <= 0 {
		errs = append(errs, fmt.Errorf("CF_QUEUE_BATCH_SIZE must be greater than zero"))
	}
	if cfg.Cloudflare.QueueVisibilityTimeoutMS <= 0 {
		errs = append(errs, fmt.Errorf("CF_QUEUE_VISIBILITY_TIMEOUT_MS must be greater than zero"))
	}
	if cfg.Cloudflare.QueuePullIntervalMS <= 0 {
		errs = append(errs, fmt.Errorf("CF_QUEUE_PULL_INTERVAL_MS must be greater than zero"))
	}
	if cfg.Downloader.Workers <= 0 {
		errs = append(errs, fmt.Errorf("DOWNLOADER_WORKERS must be greater than zero"))
	}
	if cfg.Downloader.TaskTimeoutMinutes <= 0 {
		errs = append(errs, fmt.Errorf("TASK_TIMEOUT_MINUTES must be greater than zero"))
	}
	if cfg.Downloader.Bin == "" {
		errs = append(errs, fmt.Errorf("TDL_BIN cannot be empty"))
	}
	if cfg.Downloader.Namespace == "" {
		errs = append(errs, fmt.Errorf("TDL_NAMESPACE cannot be empty"))
	}

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}

	return cfg, nil
}

func validateRequired(errs *[]error, name, value string) {
	if strings.TrimSpace(value) == "" {
		*errs = append(*errs, fmt.Errorf("%s is required", name))
	}
}

func getEnvOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getBoolEnv(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getIntEnv(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseInt64List(value string) []int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

func normalizeURL(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		os.Setenv(key, trimQuotes(value))
	}
	return scanner.Err()
}

func trimQuotes(value string) string {
	if len(value) < 2 {
		return value
	}
	if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) || (strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
		return value[1 : len(value)-1]
	}
	return value
}
