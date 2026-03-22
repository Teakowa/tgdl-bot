package bot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	neturl "net/url"
	"strings"
	"time"

	"tgdl-bot/internal/queue"
	"tgdl-bot/internal/service"
	"tgdl-bot/internal/tasknotify"
	"tgdl-bot/internal/telegram"
)

const (
	activeTaskListLimit     = 20
	callbackDeletePrefix    = "qdel:"
	callbackDeleteOKPrefix  = "qdelok:"
	callbackDeleteNoPrefix  = "qdelno:"
	callbackDeleteMinTaskID = 8
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
			"/queue",
			"/delete <task_id>",
			"/retry <task_id>",
		}, "\n")
	case CommandStatus:
		if cmd.TaskID == "" {
			return "用法: /status <task_id>"
		}
		return fmt.Sprintf("任务状态查询已接收: %s", cmd.TaskID)
	case CommandLast:
		return "最近任务查询已接收（最多10条）。"
	case CommandQueue:
		return "活跃任务查询已接收（running/queued/retrying）。"
	case CommandDelete:
		return "用法: /delete <task_id>"
	case CommandRetry:
		return "用法: /retry <task_id>"
	default:
		return ""
	}
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
	ReplyMarkup   *telegram.InlineKeyboardMarkup
	ReactionEmoji string
	TaskID        string
}

type HandleCallbackOutcome struct {
	Reply       string
	ReplyMarkup *telegram.InlineKeyboardMarkup
	AnswerText  string
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
	case CommandQueue:
		if h.Tasks == nil {
			return HandleTextOutcome{Reply: "任务队列查询暂未启用。"}, nil
		}
		tasks, err := h.Tasks.ListActiveTasks(ctx, userID, activeTaskListLimit)
		if err != nil {
			return HandleTextOutcome{}, err
		}
		reply := formatQueue(tasks)
		if len(tasks) == 0 {
			return HandleTextOutcome{Reply: reply}, nil
		}
		return HandleTextOutcome{Reply: reply, ReplyMarkup: buildQueueDeleteKeyboard(tasks)}, nil
	case CommandDelete:
		if cmd.TaskID == "" {
			return HandleTextOutcome{Reply: BuildCommandReply(cmd)}, nil
		}
		return h.handleDeleteByTaskID(ctx, userID, cmd.TaskID)
	case CommandRetry:
		if cmd.TaskID == "" {
			return HandleTextOutcome{Reply: BuildCommandReply(cmd)}, nil
		}
		return h.retryTaskByTaskID(ctx, userID, cmd.TaskID)
	default:
		url, ok := ExtractTaskURL(text)
		if !ok {
			return HandleTextOutcome{}, nil
		}
		return h.createTaskFromURL(ctx, userID, chatID, url)
	}
}

func (h Handler) HandleCallback(ctx context.Context, userID int64, callbackID, data string) (HandleCallbackOutcome, error) {
	if !IsAllowedUser(h.AllowedUserIDs, userID) {
		return HandleCallbackOutcome{AnswerText: "无权限"}, nil
	}
	if h.Tasks == nil {
		return HandleCallbackOutcome{Reply: "任务操作暂未启用。", AnswerText: "暂不可用"}, nil
	}

	switch {
	case strings.HasPrefix(data, callbackDeletePrefix):
		taskID := parseCallbackTaskID(data, callbackDeletePrefix)
		if taskID == "" {
			return HandleCallbackOutcome{Reply: "无效任务 ID。", AnswerText: "参数错误"}, nil
		}
		task, err := h.Tasks.GetTask(ctx, taskID)
		if err != nil {
			if errors.Is(err, service.ErrTaskNotFound) {
				return HandleCallbackOutcome{Reply: "任务不存在或已处理。", AnswerText: "任务不存在"}, nil
			}
			return HandleCallbackOutcome{}, err
		}
		if task.UserID != userID {
			return HandleCallbackOutcome{Reply: "无权限删除该任务。", AnswerText: "无权限"}, nil
		}
		switch task.Status {
		case service.StatusRunning:
			return HandleCallbackOutcome{Reply: "任务正在执行中，无法删除。", AnswerText: "无法删除"}, nil
		case service.StatusQueued, service.StatusRetrying:
			return HandleCallbackOutcome{
				Reply:       fmt.Sprintf("确认删除任务？\nTask ID: %s\nURL: %s", task.TaskID, task.URL),
				ReplyMarkup: buildDeleteConfirmKeyboard(task.TaskID),
				AnswerText:  "请确认删除",
			}, nil
		default:
			return HandleCallbackOutcome{Reply: "仅可删除 queued/retrying 任务。", AnswerText: "状态不支持"}, nil
		}
	case strings.HasPrefix(data, callbackDeleteOKPrefix):
		taskID := parseCallbackTaskID(data, callbackDeleteOKPrefix)
		if taskID == "" {
			return HandleCallbackOutcome{Reply: "无效任务 ID。", AnswerText: "参数错误"}, nil
		}
		outcome, err := h.handleDeleteByTaskID(ctx, userID, taskID)
		if err != nil {
			return HandleCallbackOutcome{}, err
		}
		answer := "删除失败"
		if strings.Contains(outcome.Reply, "任务已删除") {
			answer = "删除成功"
		}
		return HandleCallbackOutcome{Reply: outcome.Reply, AnswerText: answer}, nil
	case strings.HasPrefix(data, callbackDeleteNoPrefix):
		return HandleCallbackOutcome{Reply: "已取消删除。", AnswerText: "已取消"}, nil
	default:
		return HandleCallbackOutcome{AnswerText: "不支持的操作"}, nil
	}
}

func (h Handler) createTaskFromURL(ctx context.Context, userID, chatID int64, url string) (HandleTextOutcome, error) {
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
			return h.rebuildTaskFromRequest(ctx, req, task)
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
		Reply:         tasknotify.FormatTaskStatusMessage(task),
		ReactionEmoji: statusReaction(task.Status),
		TaskID:        task.TaskID,
	}, nil
}

func (h Handler) handleDeleteByTaskID(ctx context.Context, userID int64, taskID string) (HandleTextOutcome, error) {
	if h.Tasks == nil {
		return HandleTextOutcome{Reply: "任务删除暂未启用。"}, nil
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return HandleTextOutcome{Reply: "用法: /delete <task_id>"}, nil
	}

	task, err := h.Tasks.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, service.ErrTaskNotFound) {
			return HandleTextOutcome{Reply: "任务不存在或已处理。"}, nil
		}
		return HandleTextOutcome{}, err
	}
	if task.UserID != userID {
		return HandleTextOutcome{Reply: "无权限删除该任务。"}, nil
	}
	switch task.Status {
	case service.StatusRunning:
		return HandleTextOutcome{Reply: "任务正在执行中，无法删除。"}, nil
	case service.StatusQueued, service.StatusRetrying:
	default:
		return HandleTextOutcome{Reply: "仅可删除 queued/retrying 任务。"}, nil
	}

	deleted, err := h.Tasks.DeletePendingTask(ctx, userID, taskID)
	if err != nil {
		return HandleTextOutcome{}, err
	}
	if !deleted {
		return HandleTextOutcome{Reply: "任务状态已变化，请刷新 /queue。"}, nil
	}
	return HandleTextOutcome{Reply: fmt.Sprintf("任务已删除\nTask ID: %s", taskID)}, nil
}

func (h Handler) retryTaskByTaskID(ctx context.Context, userID int64, taskID string) (HandleTextOutcome, error) {
	if h.Tasks == nil || h.Queue == nil {
		return HandleTextOutcome{Reply: "任务重试暂未启用。"}, nil
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return HandleTextOutcome{Reply: "用法: /retry <task_id>"}, nil
	}

	task, err := h.Tasks.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, service.ErrTaskNotFound) {
			return HandleTextOutcome{Reply: "任务不存在。"}, nil
		}
		return HandleTextOutcome{}, err
	}
	if task.UserID != userID {
		return HandleTextOutcome{Reply: "无权限重试该任务。"}, nil
	}
	if !isRebuildableStatus(task.Status) {
		return HandleTextOutcome{
			Reply:         fmt.Sprintf("任务当前状态不支持重试: %s", task.Status),
			ReactionEmoji: statusReaction(task.Status),
		}, nil
	}

	req := service.CreateQueuedTaskRequest{
		TaskID:         newTaskID(),
		ChatID:         task.ChatID,
		UserID:         task.UserID,
		TargetChatID:   task.TargetChatID,
		URL:            task.URL,
		IdempotencyKey: task.IdempotencyKey,
	}
	return h.rebuildTaskFromRequest(ctx, req, task)
}

func (h Handler) rebuildTaskFromRequest(ctx context.Context, req service.CreateQueuedTaskRequest, existing service.Task) (HandleTextOutcome, error) {
	if _, err := h.Tasks.DeleteFailedByIdempotencyKey(ctx, req.IdempotencyKey); err != nil {
		h.logError("bot task rebuild cleanup failed",
			"task_id", existing.TaskID,
			"status", existing.Status,
			"error", err,
		)
		return HandleTextOutcome{Reply: fmt.Sprintf("任务重建失败\nTask ID: %s", existing.TaskID), ReactionEmoji: statusReaction(existing.Status)}, nil
	}

	rebuildTaskID := req.TaskID
	task, err := h.Tasks.CreateQueuedTask(ctx, req)
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
		Reply:         tasknotify.FormatTaskStatusMessage(task),
		ReactionEmoji: statusReaction(task.Status),
		TaskID:        task.TaskID,
	}, nil
}

func (h Handler) BindTaskMessageRefs(ctx context.Context, taskID string, sourceMessageID, statusMessageID int64) (service.Task, error) {
	if h.Tasks == nil {
		return service.Task{}, errors.New("task service is required")
	}

	if err := h.Tasks.UpdateTask(ctx, taskID, service.TaskUpdate{
		SourceMessageID: &sourceMessageID,
		StatusMessageID: &statusMessageID,
	}); err != nil {
		return service.Task{}, err
	}

	return h.Tasks.GetTask(ctx, taskID)
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
	return tasknotify.SourceReactionEmoji(status)
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

func formatQueue(tasks []service.Task) string {
	if len(tasks) == 0 {
		return "当前无执行中或排队中的任务。"
	}

	lines := make([]string, 0, len(tasks)+1)
	lines = append(lines, "当前任务队列:")
	for i, task := range tasks {
		lines = append(lines, fmt.Sprintf("%d. %s | %s | %s", i+1, task.URL, task.TaskID, task.Status))
	}
	return strings.Join(lines, "\n")
}

func buildQueueDeleteKeyboard(tasks []service.Task) *telegram.InlineKeyboardMarkup {
	if len(tasks) == 0 {
		return nil
	}
	rows := make([][]telegram.InlineKeyboardButton, 0, len(tasks))
	for i, task := range tasks {
		rows = append(rows, []telegram.InlineKeyboardButton{{
			Text:         fmt.Sprintf("删除 %d (%s)", i+1, shortTaskID(task.TaskID)),
			CallbackData: callbackDeletePrefix + task.TaskID,
		}})
	}
	return &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buildDeleteConfirmKeyboard(taskID string) *telegram.InlineKeyboardMarkup {
	return &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{
				{Text: "确认删除", CallbackData: callbackDeleteOKPrefix + taskID},
				{Text: "取消", CallbackData: callbackDeleteNoPrefix + taskID},
			},
		},
	}
}

func parseCallbackTaskID(data, prefix string) string {
	taskID := strings.TrimSpace(strings.TrimPrefix(data, prefix))
	if len(taskID) < callbackDeleteMinTaskID {
		return ""
	}
	return taskID
}

func shortTaskID(taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if len(taskID) <= 8 {
		return taskID
	}
	return taskID[:8]
}

func newTaskID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("task-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
