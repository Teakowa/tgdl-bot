package tasknotify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"tgdl-bot/internal/service"
	"tgdl-bot/internal/telegram"
)

type Client interface {
	EditMessageText(ctx context.Context, req telegram.EditMessageTextRequest) error
	SetMessageReaction(ctx context.Context, req telegram.SetMessageReactionRequest) error
}

type Notifier struct {
	Client Client
	Logger *slog.Logger
}

func (n Notifier) Notify(ctx context.Context, task service.Task) error {
	if n.Client == nil {
		return nil
	}

	var errs []error
	if task.StatusMessageID != nil {
		if err := n.Client.EditMessageText(ctx, telegram.EditMessageTextRequest{
			ChatID:    task.ChatID,
			MessageID: *task.StatusMessageID,
			Text:      FormatTaskStatusMessage(task),
		}); err != nil {
			n.logWarn("task status message update failed", "task_id", task.TaskID, "error", err)
			errs = append(errs, fmt.Errorf("edit status message: %w", err))
		}
	}

	if task.SourceMessageID != nil {
		emoji := SourceReactionEmoji(task.Status)
		if emoji != "" {
			if err := n.Client.SetMessageReaction(ctx, telegram.SetMessageReactionRequest{
				ChatID:    task.ChatID,
				MessageID: *task.SourceMessageID,
				Reaction:  []telegram.ReactionTypeEmoji{{Type: "emoji", Emoji: emoji}},
			}); err != nil {
				n.logWarn("task source reaction update failed", "task_id", task.TaskID, "error", err)
				errs = append(errs, fmt.Errorf("set source reaction: %w", err))
			}
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

func SourceReactionEmoji(status service.Status) string {
	switch status {
	case service.StatusQueued:
		return "👀"
	case service.StatusRunning:
		return "⚡"
	case service.StatusRetrying:
		return "🤔"
	case service.StatusDone:
		return "👍"
	case service.StatusFailed, service.StatusDeadLettered:
		return "👎"
	default:
		return ""
	}
}

func StatusMessageEmoji(status service.Status) string {
	switch status {
	case service.StatusQueued:
		return "⏳"
	case service.StatusRunning:
		return "⚡"
	case service.StatusRetrying:
		return "🔄"
	case service.StatusDone:
		return "✅"
	case service.StatusFailed, service.StatusDeadLettered:
		return "❌"
	default:
		return ""
	}
}

func FormatTaskStatusMessage(task service.Task) string {
	icon := StatusMessageEmoji(task.Status)
	if icon == "" {
		icon = "ℹ️"
	}
	lines := []string{
		fmt.Sprintf("%s %s", icon, statusLabel(task.Status)),
		fmt.Sprintf("Task ID: %s", task.TaskID),
		fmt.Sprintf("URL: %s", task.URL),
	}
	if task.RetryCount > 0 {
		lines = append(lines, fmt.Sprintf("重试次数: %d", task.RetryCount))
	}
	if task.StartedAt != nil {
		lines = append(lines, fmt.Sprintf("开始时间: %s", task.StartedAt.Format("2006-01-02 15:04:05")))
	}
	if task.FinishedAt != nil {
		lines = append(lines, fmt.Sprintf("完成时间: %s", task.FinishedAt.Format("2006-01-02 15:04:05")))
	}
	if task.ErrorMessage != nil && strings.TrimSpace(*task.ErrorMessage) != "" {
		lines = append(lines, fmt.Sprintf("原因: %s", summarizeError(*task.ErrorMessage)))
	}
	return strings.Join(lines, "\n")
}

func statusLabel(status service.Status) string {
	switch status {
	case service.StatusQueued:
		return "任务已入队"
	case service.StatusRunning:
		return "任务进行中"
	case service.StatusRetrying:
		return "任务重试中"
	case service.StatusDone:
		return "任务已完成"
	case service.StatusFailed:
		return "任务失败"
	case service.StatusDeadLettered:
		return "任务失败（停止重试）"
	default:
		return "任务状态更新"
	}
}

func summarizeError(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) <= 200 {
		return raw
	}
	return raw[:200] + "..."
}

func (n Notifier) logWarn(msg string, args ...any) {
	if n.Logger == nil {
		return
	}
	n.Logger.Warn(msg, args...)
}
