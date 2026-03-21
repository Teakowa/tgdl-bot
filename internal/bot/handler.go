package bot

import (
	"context"
	"fmt"
	"strings"

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
	Tasks          TaskQueryService
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
		return fmt.Sprintf("转发任务已加入队列\nURL: %s", url), nil
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
