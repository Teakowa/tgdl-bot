package bot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	neturl "net/url"
	"strings"
	"time"

	"tgdl-bot/internal/queue"
	"tgdl-bot/internal/service"
)

func BuildCommandReply(cmd ParsedCommand) string {
	switch cmd.Name {
	case CommandStart:
		return "欢迎使用 TGDL Bot。发送 Telegram 消息链接即可创建转发任务。"
	case CommandHelp:
		return strings.Join([]string{
			"支持命令:",
			"/start",
			"/help",
			"/status <task_id>",
			"/last",
		}, "\n")
	case CommandStatus:
		if cmd.TaskID == "" {
			return "用法: /status <task_id>"
		}
		return fmt.Sprintf("任务状态查询已接收: %s", cmd.TaskID)
	case CommandLast:
		return "最近任务查询已接收（最多10条）。"
	default:
		return ""
	}
}

type TaskQueryService interface {
	GetTask(ctx context.Context, taskID string) (service.Task, error)
	ListRecentTasks(ctx context.Context, userID int64, limit int) ([]service.Task, error)
}

type Handler struct {
	AllowedUserIDs []int64
	Tasks          service.TaskService
	Queue          queue.Producer
	Logger         *slog.Logger
}

func (h Handler) HandleText(ctx context.Context, userID, chatID int64, text string) (string, error) {
	outcome, err := h.HandleTextWithOutcome(ctx, userID, chatID, text)
	if err != nil {
		return "", err
	}
	return outcome.Reply, nil
}

type HandleTextOutcome struct {
	Reply         string
	ReactionEmoji string
}

func (h Handler) HandleTextWithOutcome(ctx context.Context, userID, chatID int64, text string) (HandleTextOutcome, error) {
	if !IsAllowedUser(h.AllowedUserIDs, userID) {
		return HandleTextOutcome{Reply: "无权限使用该机器人。"}, nil
	}

	cmd := ParseCommand(text)
	switch cmd.Name {
	case CommandStart, CommandHelp:
		return HandleTextOutcome{Reply: BuildCommandReply(cmd)}, nil
	case CommandStatus:
		if cmd.TaskID == "" {
			return HandleTextOutcome{Reply: BuildCommandReply(cmd)}, nil
		}
		if h.Tasks == nil {
			return HandleTextOutcome{Reply: "状态查询暂未启用。"}, nil
		}
		task, err := h.Tasks.GetTask(ctx, cmd.TaskID)
		if err != nil {
			return HandleTextOutcome{}, err
		}
		return HandleTextOutcome{
			Reply:         formatStatus(task),
			ReactionEmoji: statusReaction(task.Status),
		}, nil
	case CommandLast:
		if h.Tasks == nil {
			return HandleTextOutcome{Reply: "最近任务查询暂未启用。"}, nil
		}
		tasks, err := h.Tasks.ListRecentTasks(ctx, userID, 10)
		if err != nil {
			return HandleTextOutcome{}, err
		}
		return HandleTextOutcome{Reply: formatLast(tasks)}, nil
	default:
		url, ok := ExtractTaskURL(text)
		if !ok {
			return HandleTextOutcome{}, nil
		}
		if h.Tasks == nil || h.Queue == nil {
			return HandleTextOutcome{Reply: "任务创建暂未启用。"}, nil
		}

		newTaskIDValue := newTaskID()
		idempotencyKey := service.NewIdempotencyKey(userID, url)
		h.logInfo("bot task request parsed",
			"chat_id", chatID,
			"user_id", userID,
			"url", redactURL(url),
			"idempotency_key_prefix", idempotencyKeyPrefix(idempotencyKey),
		)

		req := service.CreateQueuedTaskRequest{
			TaskID:         newTaskIDValue,
			ChatID:         chatID,
			UserID:         userID,
			TargetChatID:   chatID,
			URL:            url,
			IdempotencyKey: idempotencyKey,
		}
		task, err := h.Tasks.CreateQueuedTask(ctx, req)
		if err != nil {
			h.logError("bot task create failed",
				"chat_id", chatID,
				"user_id", userID,
				"url", redactURL(url),
				"error", err,
			)
			return HandleTextOutcome{Reply: "任务创建失败，请稍后重试。"}, nil
		}
		if task.TaskID != newTaskIDValue {
			h.logInfo("bot task existing hit",
				"task_id", task.TaskID,
				"status", task.Status,
				"rebuild", isRebuildableStatus(task.Status),
				"idempotency_key_prefix", idempotencyKeyPrefix(idempotencyKey),
			)
			if isRebuildableStatus(task.Status) {
				if _, err := h.Tasks.DeleteFailedByIdempotencyKey(ctx, idempotencyKey); err != nil {
					h.logError("bot task rebuild cleanup failed",
						"task_id", task.TaskID,
						"status", task.Status,
						"error", err,
					)
					return HandleTextOutcome{Reply: fmt.Sprintf("任务重建失败\nTask ID: %s", task.TaskID), ReactionEmoji: statusReaction(task.Status)}, nil
				}

				rebuildTaskID := newTaskID()
				req.TaskID = rebuildTaskID
				task, err = h.Tasks.CreateQueuedTask(ctx, req)
				if err != nil {
					h.logError("bot task rebuild create failed",
						"task_id", rebuildTaskID,
						"error", err,
					)
					return HandleTextOutcome{Reply: "任务重建失败，请稍后重试。", ReactionEmoji: statusReaction(service.StatusFailed)}, nil
				}
				if task.TaskID != rebuildTaskID {
					h.logInfo("bot task rebuild dedup hit",
						"task_id", task.TaskID,
						"status", task.Status,
					)
					return HandleTextOutcome{
						Reply:         fmt.Sprintf("任务已存在\nTask ID: %s\n状态: %s", task.TaskID, task.Status),
						ReactionEmoji: statusReaction(task.Status),
					}, nil
				}
				h.logInfo("bot task rebuilt",
					"task_id", task.TaskID,
					"status", task.Status,
					"idempotency_key_prefix", idempotencyKeyPrefix(task.IdempotencyKey),
				)
				if err := h.enqueueTask(ctx, task); err != nil {
					return HandleTextOutcome{Reply: fmt.Sprintf("任务重建后入队失败\nTask ID: %s", task.TaskID), ReactionEmoji: statusReaction(service.StatusFailed)}, nil
				}
				return HandleTextOutcome{
					Reply:         fmt.Sprintf("检测到历史失败任务，已重新创建并入队\nTask ID: %s\nURL: %s", task.TaskID, task.URL),
					ReactionEmoji: statusReaction(task.Status),
				}, nil
			}
			return HandleTextOutcome{
				Reply:         fmt.Sprintf("任务已存在\nTask ID: %s\n状态: %s", task.TaskID, task.Status),
				ReactionEmoji: statusReaction(task.Status),
			}, nil
		}
		h.logInfo("bot task created",
			"task_id", task.TaskID,
			"status", task.Status,
			"idempotency_key_prefix", idempotencyKeyPrefix(task.IdempotencyKey),
			"url", redactURL(task.URL),
		)

		if err := h.enqueueTask(ctx, task); err != nil {
			return HandleTextOutcome{Reply: fmt.Sprintf("任务入队失败\nTask ID: %s", task.TaskID), ReactionEmoji: statusReaction(service.StatusFailed)}, nil
		}
		return HandleTextOutcome{
			Reply:         fmt.Sprintf("转发任务已加入队列\nTask ID: %s\nURL: %s", task.TaskID, task.URL),
			ReactionEmoji: statusReaction(task.Status),
		}, nil
	}
}

func (h Handler) enqueueTask(ctx context.Context, task service.Task) error {
	if err := h.Queue.Enqueue(ctx, queue.Message{
		TaskID:       task.TaskID,
		ChatID:       task.ChatID,
		UserID:       task.UserID,
		TargetChatID: task.TargetChatID,
		URL:          task.URL,
		CreatedAt:    task.CreatedAt,
		Idempotency:  task.IdempotencyKey,
	}); err != nil {
		h.logError("bot queue enqueue failed",
			"task_id", task.TaskID,
			"status_to", service.StatusFailed,
			"error", err,
		)
		msg := err.Error()
		_ = h.Tasks.UpdateTask(ctx, task.TaskID, service.TaskUpdate{
			Status:       service.StatusFailed,
			ErrorMessage: &msg,
		})
		return err
	}
	h.logInfo("bot queue enqueue succeeded",
		"task_id", task.TaskID,
		"status_to", service.StatusQueued,
	)
	return nil
}

func (h Handler) logInfo(msg string, args ...any) {
	if h.Logger == nil {
		return
	}
	h.Logger.Info(msg, args...)
}

func (h Handler) logError(msg string, args ...any) {
	if h.Logger == nil {
		return
	}
	h.Logger.Error(msg, args...)
}

func idempotencyKeyPrefix(key string) string {
	key = strings.TrimSpace(key)
	if len(key) <= 8 {
		return key
	}
	return key[:8]
}

func redactURL(raw string) string {
	u, err := neturl.Parse(strings.TrimSpace(raw))
	if err != nil {
		return raw
	}

	path := u.Path
	if len(path) > 18 {
		path = path[:18] + "..."
	}
	return fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, path)
}

func isRebuildableStatus(status service.Status) bool {
	return status == service.StatusFailed || status == service.StatusDeadLettered
}

func statusReaction(status service.Status) string {
	switch status {
	case service.StatusQueued:
		return "👀"
	case service.StatusRunning:
		return "⚡"
	case service.StatusDone:
		return "👍"
	case service.StatusRetrying:
		return "🔥"
	case service.StatusFailed, service.StatusDeadLettered:
		return "👎"
	default:
		return ""
	}
}

func formatStatus(task service.Task) string {
	out := []string{
		fmt.Sprintf("Task ID: %s", task.TaskID),
		fmt.Sprintf("状态: %s", task.Status),
		fmt.Sprintf("URL: %s", task.URL),
		fmt.Sprintf("创建时间: %s", task.CreatedAt.Format("2006-01-02 15:04:05")),
	}
	if task.FinishedAt != nil {
		out = append(out, fmt.Sprintf("完成时间: %s", task.FinishedAt.Format("2006-01-02 15:04:05")))
	}
	if task.ErrorMessage != nil && *task.ErrorMessage != "" {
		out = append(out, fmt.Sprintf("最近错误: %s", *task.ErrorMessage))
	}
	return strings.Join(out, "\n")
}

func formatLast(tasks []service.Task) string {
	if len(tasks) == 0 {
		return "暂无任务记录。"
	}

	lines := make([]string, 0, len(tasks)+1)
	lines = append(lines, "最近任务:")
	for _, task := range tasks {
		lines = append(lines, fmt.Sprintf("- %s | %s | %s", task.TaskID, task.Status, task.URL))
	}
	return strings.Join(lines, "\n")
}

func newTaskID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("task-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
