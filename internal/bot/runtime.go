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

	"tgdl-bot/internal/queue"
	"tgdl-bot/internal/service"
	"tgdl-bot/internal/taskevent"
	"tgdl-bot/internal/tasknotify"
	"tgdl-bot/internal/telegram"
)

type statusQueueConsumer interface {
	Pull(ctx context.Context, batchSize, visibilityTimeoutMs int) ([]queue.ReceivedMessage, error)
	Ack(ctx context.Context, leaseIDs []string) error
	Retry(ctx context.Context, leaseIDs []string) error
}

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

	StatusQueue               statusQueueConsumer
	StatusQueueBatchSize      int
	StatusQueuePullInterval   time.Duration
	StatusQueueVisibilityMs   int
	StatusNotificationTimeout time.Duration
}

func (r Runtime) Run(ctx context.Context) error {
	if r.Client == nil {
		return errors.New("bot runtime: telegram client is required")
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	workers := 2
	if r.statusSyncEnabled() {
		workers++
	}

	errCh := make(chan error, workers)
	go func() {
		errCh <- r.runHTTPServer(runCtx)
	}()
	if r.statusSyncEnabled() {
		go func() {
			errCh <- r.runStatusSync(runCtx)
		}()
	}
	go func() {
		if r.useWebhookMode() {
			errCh <- r.runWebhook(runCtx)
			return
		}
		errCh <- r.runPolling(runCtx)
	}()

	errs := make([]error, 0, workers)
	firstErr := <-errCh
	errs = append(errs, firstErr)
	cancel()
	for len(errs) < workers {
		errs = append(errs, <-errCh)
	}
	return chooseRunError(errs...)
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
			AllowedUpdates: []string{"message", "callback_query"},
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
		AllowedUpdates: []string{"message", "callback_query"},
	}); err != nil {
		return fmt.Errorf("bot runtime: setWebhook failed: %w", err)
	}

	<-ctx.Done()
	return ctx.Err()
}

func (r Runtime) runHTTPServer(ctx context.Context) error {
	addr := strings.TrimSpace(r.WebhookAddr)
	if addr == "" {
		addr = ":8080"
	}

	server := &http.Server{
		Addr:    addr,
		Handler: r.httpHandler(),
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
			return fmt.Errorf("bot runtime: http server failed: %w", err)
		}
		return nil
	}
}

func (r Runtime) httpHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/ping", http.HandlerFunc(r.handlePing))
	mux.Handle("/webhook", http.HandlerFunc(r.handleWebhook))
	return mux
}

func (r Runtime) handlePing(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

func (r Runtime) handleWebhook(w http.ResponseWriter, req *http.Request) {
	if !r.useWebhookMode() {
		http.NotFound(w, req)
		return
	}
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
	if outcome.AnswerCallbackRequest != nil {
		if err := r.Client.AnswerCallbackQuery(ctx, *outcome.AnswerCallbackRequest); err != nil {
			r.log("answer callback query failed", "callback_query_id", outcome.AnswerCallbackRequest.CallbackQueryID, "error", err)
		}
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

func (r Runtime) statusSyncEnabled() bool {
	return r.StatusQueue != nil && r.Handler.Tasks != nil
}

func (r Runtime) runStatusSync(ctx context.Context) error {
	pullInterval := r.StatusQueuePullInterval
	if pullInterval <= 0 {
		pullInterval = 1200 * time.Millisecond
	}
	batchSize := r.StatusQueueBatchSize
	if batchSize <= 0 {
		batchSize = 5
	}
	visibilityTimeout := r.StatusQueueVisibilityMs
	if visibilityTimeout <= 0 {
		visibilityTimeout = 15 * 60 * 1000
	}

	r.log("status sync loop started", "batch_size", batchSize, "visibility_timeout_ms", visibilityTimeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		messages, err := r.StatusQueue.Pull(ctx, batchSize, visibilityTimeout)
		if err != nil {
			r.log("status sync queue pull failed", "error", err)
			time.Sleep(pullInterval)
			continue
		}
		if len(messages) == 0 {
			time.Sleep(pullInterval)
			continue
		}
		for _, message := range messages {
			r.processStatusMessage(ctx, message)
		}
	}
}

func (r Runtime) processStatusMessage(ctx context.Context, message queue.ReceivedMessage) {
	if message.LeaseID == "" {
		return
	}

	event, ok := taskevent.FromQueueMessage(message.Body)
	if !ok {
		r.log("status sync invalid message acked", "lease_id", message.LeaseID)
		_ = r.StatusQueue.Ack(ctx, []string{message.LeaseID})
		return
	}

	task, err := r.Handler.Tasks.GetTask(ctx, event.TaskID)
	if err != nil {
		if errors.Is(err, service.ErrTaskNotFound) {
			r.log("status sync task not found acked", "lease_id", message.LeaseID, "task_id", event.TaskID)
			_ = r.StatusQueue.Ack(ctx, []string{message.LeaseID})
			return
		}
		r.log("status sync task load failed, lease retried", "lease_id", message.LeaseID, "task_id", event.TaskID, "error", err)
		_ = r.StatusQueue.Retry(ctx, []string{message.LeaseID})
		return
	}

	if task.StatusMessageID == nil && task.SourceMessageID == nil {
		r.log("status sync task message refs missing, lease retried", "lease_id", message.LeaseID, "task_id", event.TaskID)
		_ = r.StatusQueue.Retry(ctx, []string{message.LeaseID})
		return
	}

	notifier := tasknotify.Notifier{Client: r.Client, Logger: r.Logger}
	timeout := r.StatusNotificationTimeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	notifyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := notifier.Notify(notifyCtx, task); err != nil {
		if shouldRetryStatusNotify(err) {
			r.log("status sync notify failed, lease retried", "lease_id", message.LeaseID, "task_id", event.TaskID, "error", err)
			_ = r.StatusQueue.Retry(ctx, []string{message.LeaseID})
			return
		}
		r.log("status sync notify failed, lease acked", "lease_id", message.LeaseID, "task_id", event.TaskID, "error", err)
		_ = r.StatusQueue.Ack(ctx, []string{message.LeaseID})
		return
	}

	_ = r.StatusQueue.Ack(ctx, []string{message.LeaseID})
}

func shouldRetryStatusNotify(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var apiErr *telegram.APIRequestError
	if errors.As(err, &apiErr) {
		if apiErr.Code == 429 {
			return true
		}
		return apiErr.Code >= 500
	}
	return true
}

func chooseRunError(errs ...error) error {
	for _, candidate := range errs {
		if candidate == nil {
			continue
		}
		if errors.Is(candidate, context.Canceled) || errors.Is(candidate, context.DeadlineExceeded) {
			continue
		}
		return candidate
	}
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
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

	initialText := ""
	if outcome.SendRequest != nil {
		initialText = strings.TrimSpace(outcome.SendRequest.Text)
	}
	currentText := strings.TrimSpace(tasknotify.FormatTaskStatusMessage(task))
	if initialText != "" && initialText == currentText {
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
