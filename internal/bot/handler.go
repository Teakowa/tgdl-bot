package bot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
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
}

func (h Handler) HandleText(ctx context.Context, userID, chatID int64, text string) (string, error) {
	if !IsAllowedUser(h.AllowedUserIDs, userID) {
		return "无权限使用该机器人。", nil
	}

	cmd := ParseCommand(text)
	switch cmd.Name {
	case CommandStart, CommandHelp:
		return BuildCommandReply(cmd), nil
	case CommandStatus:
		if cmd.TaskID == "" {
			return BuildCommandReply(cmd), nil
		}
		if h.Tasks == nil {
			return "状态查询暂未启用。", nil
		}
		task, err := h.Tasks.GetTask(ctx, cmd.TaskID)
		if err != nil {
			return "", err
		}
		return formatStatus(task), nil
	case CommandLast:
		if h.Tasks == nil {
			return "最近任务查询暂未启用。", nil
		}
		tasks, err := h.Tasks.ListRecentTasks(ctx, userID, 10)
		if err != nil {
			return "", err
		}
		return formatLast(tasks), nil
	default:
		url, ok := ExtractTaskURL(text)
		if !ok {
			return "", nil
		}
		if h.Tasks == nil || h.Queue == nil {
			return "任务创建暂未启用。", nil
		}

		newTaskIDValue := newTaskID()
		idempotencyKey := service.NewIdempotencyKey(userID, url)
		task, err := h.Tasks.CreateQueuedTask(ctx, service.CreateQueuedTaskRequest{
			TaskID:         newTaskIDValue,
			ChatID:         chatID,
			UserID:         userID,
			TargetChatID:   chatID,
			URL:            url,
			IdempotencyKey: idempotencyKey,
		})
		if err != nil {
			return "任务创建失败，请稍后重试。", nil
		}
		if task.TaskID != newTaskIDValue {
			return fmt.Sprintf("任务已存在\nTask ID: %s\n状态: %s", task.TaskID, task.Status), nil
		}

		if err := h.Queue.Enqueue(ctx, queue.Message{
			TaskID:       task.TaskID,
			ChatID:       task.ChatID,
			UserID:       task.UserID,
			TargetChatID: task.TargetChatID,
			URL:          task.URL,
			CreatedAt:    task.CreatedAt,
			Idempotency:  task.IdempotencyKey,
		}); err != nil {
			msg := err.Error()
			_ = h.Tasks.UpdateTask(ctx, task.TaskID, service.TaskUpdate{
				Status:       service.StatusFailed,
				ErrorMessage: &msg,
			})
			return fmt.Sprintf("任务入队失败\nTask ID: %s", task.TaskID), nil
		}
		return fmt.Sprintf("转发任务已加入队列\nTask ID: %s\nURL: %s", task.TaskID, task.URL), nil
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
