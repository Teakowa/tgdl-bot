package bot

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"tgdl-bot/internal/tasknotify"
	"tgdl-bot/internal/telegram"
)

type Runtime struct {
	Client         telegram.Client
	Handler        Handler
	Logger         *slog.Logger
	PollInterval   time.Duration
	PollLimit      int
	TimeoutSeconds int
}

func (r Runtime) Run(ctx context.Context) error {
	if r.Client == nil {
		return errors.New("bot runtime: telegram client is required")
	}

	pollInterval := r.PollInterval
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	pollLimit := r.PollLimit
	if pollLimit <= 0 {
		pollLimit = 50
	}
	timeoutSeconds := r.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}

	var offset int64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		updates, err := r.Client.GetUpdates(ctx, telegram.GetUpdatesRequest{
			Offset:         offset,
			Limit:          pollLimit,
			TimeoutSeconds: timeoutSeconds,
			AllowedUpdates: []string{"message"},
		})
		if err != nil {
			r.log("telegram getUpdates failed", "error", err)
			time.Sleep(pollInterval)
			continue
		}

		for _, update := range updates.Result {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			outcome, err := r.Handler.HandleUpdate(ctx, update)
			if err != nil {
				r.log("handle update failed", "update_id", update.UpdateID, "error", err)
				continue
			}
			if outcome == nil {
				continue
			}
			if outcome.ReactionRequest != nil {
				if err := r.Client.SetMessageReaction(ctx, *outcome.ReactionRequest); err != nil {
					r.log("set message reaction failed", "chat_id", outcome.ReactionRequest.ChatID, "message_id", outcome.ReactionRequest.MessageID, "error", err)
				}
			}
			if outcome.SendRequest == nil {
				continue
			}
			sentMessage, err := r.Client.SendMessage(ctx, *outcome.SendRequest)
			if err != nil {
				r.log("send message failed", "chat_id", outcome.SendRequest.ChatID, "error", err)
				continue
			}
			r.bindAndSyncTaskStatus(ctx, outcome, sentMessage)
		}
	}
}

func (r Runtime) bindAndSyncTaskStatus(ctx context.Context, outcome *UpdateOutcome, sentMessage telegram.Message) {
	if outcome == nil || outcome.TaskID == "" || outcome.SourceMessageID == 0 || sentMessage.MessageID == 0 {
		return
	}

	task, err := r.Handler.BindTaskMessageRefs(ctx, outcome.TaskID, outcome.SourceMessageID, sentMessage.MessageID)
	if err != nil {
		r.log("bind task message refs failed", "task_id", outcome.TaskID, "error", err)
		return
	}

	notifier := tasknotify.Notifier{Client: r.Client, Logger: r.Logger}
	if err := notifier.Notify(ctx, task); err != nil {
		r.log("sync task status message failed", "task_id", task.TaskID, "error", err)
	}
}

func (r Runtime) log(msg string, args ...any) {
	if r.Logger == nil {
		return
	}
	r.Logger.Error(msg, args...)
}
