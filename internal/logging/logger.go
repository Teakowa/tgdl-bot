package logging

import (
	"log/slog"
	"os"
	"strings"
)

func New(level string) *slog.Logger {
	var lvl slog.LevelVar
	lvl.Set(parseLevel(level))

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: &lvl,
	})

	logger := slog.New(handler).With("service", "tgdl-bot")
	slog.SetDefault(logger)
	return logger
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
