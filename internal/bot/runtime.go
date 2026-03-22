package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
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
	UseWebhook     bool
	WebhookURL     string
	WebhookSecret  string
	WebhookAddr    string
}

func (r Runtime) Run(ctx context.Context) error {
	if r.Client == nil {
		return errors.New("bot runtime: telegram client is required")
	}
	if r.useWebhookMode() {
		return r.runWebhook(ctx)
	}
	return r.runPolling(ctx)
}

func (r Runtime) runPolling(ctx context.Context) error {
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

	if err := r.Client.DeleteWebhook(ctx, telegram.DeleteWebhookRequest{DropPendingUpdates: false}); err != nil {
		r.log("telegram deleteWebhook before polling failed", "error", err)
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
			if telegram.IsAPIErrorCode(err, telegram.ErrorCodeConflict) {
				r.log("telegram getUpdates conflict detected, deleting webhook", "error", err)
				if clearErr := r.Client.DeleteWebhook(ctx, telegram.DeleteWebhookRequest{DropPendingUpdates: false}); clearErr != nil {
					r.log("telegram deleteWebhook conflict recovery failed", "error", clearErr)
				}
			} else {
				r.log("telegram getUpdates failed", "error", err)
			}
			time.Sleep(pollInterval)
			continue
		}

		for _, update := range updates.Result {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			r.processUpdate(ctx, update)
		}
	}
}

func (r Runtime) runWebhook(ctx context.Context) error {
	webhookURL := strings.TrimSpace(r.WebhookURL)
	if webhookURL == "" {
		return errors.New("bot runtime: webhook url is required")
	}

	if err := r.Client.SetWebhook(ctx, telegram.SetWebhookRequest{
		URL:            webhookURL,
		SecretToken:    strings.TrimSpace(r.WebhookSecret),
		AllowedUpdates: []string{"message"},
	}); err != nil {
		return fmt.Errorf("bot runtime: setWebhook failed: %w", err)
	}

	addr := strings.TrimSpace(r.WebhookAddr)
	if addr == "" {
		addr = ":8080"
	}

	server := &http.Server{
		Addr:    addr,
		Handler: r.webhookHandler(),
	}
	errCh := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return ctx.Err()
	case err, ok := <-errCh:
		if ok && err != nil {
			return fmt.Errorf("bot runtime: webhook server failed: %w", err)
		}
		return nil
	}
}

func (r Runtime) webhookHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if req.Header.Get(telegram.WebhookSecretHeader) != strings.TrimSpace(r.WebhookSecret) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		defer req.Body.Close()
		var update telegram.Update
		if err := json.NewDecoder(req.Body).Decode(&update); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		r.processUpdate(req.Context(), update)
		w.WriteHeader(http.StatusOK)
	})
}

func (r Runtime) processUpdate(ctx context.Context, update telegram.Update) {
	outcome, err := r.Handler.HandleUpdate(ctx, update)
	if err != nil {
		r.log("handle update failed", "update_id", update.UpdateID, "error", err)
		return
	}
	if outcome == nil {
		return
	}
	if outcome.ReactionRequest != nil {
		if err := r.Client.SetMessageReaction(ctx, *outcome.ReactionRequest); err != nil {
			r.log("set message reaction failed", "chat_id", outcome.ReactionRequest.ChatID, "message_id", outcome.ReactionRequest.MessageID, "error", err)
		}
	}
	if outcome.SendRequest == nil {
		return
	}
	sentMessage, err := r.Client.SendMessage(ctx, *outcome.SendRequest)
	if err != nil {
		r.log("send message failed", "chat_id", outcome.SendRequest.ChatID, "error", err)
		return
	}
	r.bindAndSyncTaskStatus(ctx, outcome, sentMessage)
}

func (r Runtime) useWebhookMode() bool {
	return r.UseWebhook && strings.TrimSpace(r.WebhookURL) != ""
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
