package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tgdl-bot/internal/service"
	"tgdl-bot/internal/telegram"
)

type fakeTelegramClient struct {
	updates           []telegram.GetUpdatesResponse
	getUpdatesErrs    []error
	getUpdatesCalls   int
	getUpdatesReqs    []telegram.GetUpdatesRequest
	sentMessages      []telegram.SendMessageRequest
	editedMessages    []telegram.EditMessageTextRequest
	setReactionCalled int
	answeredCallbacks []telegram.AnswerCallbackQueryRequest
	setWebhookReqs    []telegram.SetWebhookRequest
	deleteWebhookReqs []telegram.DeleteWebhookRequest
	setWebhookErr     error
	deleteWebhookErr  error
}

func (f *fakeTelegramClient) GetUpdates(_ context.Context, req telegram.GetUpdatesRequest) (telegram.GetUpdatesResponse, error) {
	f.getUpdatesReqs = append(f.getUpdatesReqs, req)
	call := f.getUpdatesCalls
	f.getUpdatesCalls++
	if call < len(f.getUpdatesErrs) && f.getUpdatesErrs[call] != nil {
		return telegram.GetUpdatesResponse{}, f.getUpdatesErrs[call]
	}
	if call < len(f.updates) {
		return f.updates[call], nil
	}
	return telegram.GetUpdatesResponse{Ok: true}, nil
}

func (f *fakeTelegramClient) SetWebhook(_ context.Context, req telegram.SetWebhookRequest) error {
	f.setWebhookReqs = append(f.setWebhookReqs, req)
	return f.setWebhookErr
}

func (f *fakeTelegramClient) DeleteWebhook(_ context.Context, req telegram.DeleteWebhookRequest) error {
	f.deleteWebhookReqs = append(f.deleteWebhookReqs, req)
	return f.deleteWebhookErr
}

func (f *fakeTelegramClient) SendMessage(_ context.Context, req telegram.SendMessageRequest) (telegram.Message, error) {
	f.sentMessages = append(f.sentMessages, req)
	return telegram.Message{MessageID: 1, Chat: telegram.Chat{ID: req.ChatID}, Text: req.Text}, nil
}

func (f *fakeTelegramClient) EditMessageText(_ context.Context, req telegram.EditMessageTextRequest) error {
	f.editedMessages = append(f.editedMessages, req)
	return nil
}

func (f *fakeTelegramClient) SetMessageReaction(_ context.Context, _ telegram.SetMessageReactionRequest) error {
	f.setReactionCalled++
	return nil
}

func (f *fakeTelegramClient) AnswerCallbackQuery(_ context.Context, req telegram.AnswerCallbackQueryRequest) error {
	f.answeredCallbacks = append(f.answeredCallbacks, req)
	return nil
}

func TestRuntimeProcessesUpdate(t *testing.T) {
	client := &fakeTelegramClient{
		updates: []telegram.GetUpdatesResponse{
			{
				Ok: true,
				Result: []telegram.Update{
					{
						UpdateID: 1,
						Message: &telegram.Message{
							Chat: telegram.Chat{ID: 101},
							From: &telegram.User{ID: 202},
							Text: "/help",
						},
					},
				},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runtime := Runtime{
		Client:         client,
		Handler:        Handler{},
		PollInterval:   time.Millisecond,
		PollLimit:      1,
		TimeoutSeconds: 1,
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_ = runtime.Run(ctx)
	if len(client.sentMessages) == 0 {
		t.Fatal("expected at least one message to be sent")
	}
	if len(client.deleteWebhookReqs) == 0 {
		t.Fatal("expected polling startup to call deleteWebhook")
	}
	if client.deleteWebhookReqs[0].DropPendingUpdates {
		t.Fatal("expected polling startup deleteWebhook with drop_pending_updates=false")
	}
	if len(client.getUpdatesReqs) == 0 {
		t.Fatal("expected getUpdates request")
	}
	foundCallback := false
	for _, typ := range client.getUpdatesReqs[0].AllowedUpdates {
		if typ == "callback_query" {
			foundCallback = true
			break
		}
	}
	if !foundCallback {
		t.Fatalf("expected polling allowed updates to include callback_query, got %v", client.getUpdatesReqs[0].AllowedUpdates)
	}
}

func TestRuntimeBindsTaskMessageRefsForCreatedTask(t *testing.T) {
	client := &fakeTelegramClient{
		updates: []telegram.GetUpdatesResponse{
			{
				Ok: true,
				Result: []telegram.Update{
					{
						UpdateID: 1,
						Message: &telegram.Message{
							MessageID: 77,
							Chat:      telegram.Chat{ID: 101},
							From:      &telegram.User{ID: 202},
							Text:      "https://t.me/c/1/2",
						},
					},
				},
			},
		},
	}

	tasks := &fakeTaskQuery{}
	tasks.createFn = func(req service.CreateQueuedTaskRequest) (service.Task, error) {
		tasks.task = service.Task{
			TaskID:         req.TaskID,
			ChatID:         req.ChatID,
			UserID:         req.UserID,
			TargetChatID:   req.TargetChatID,
			URL:            req.URL,
			IdempotencyKey: req.IdempotencyKey,
			Status:         service.StatusQueued,
			CreatedAt:      time.Now().UTC(),
		}
		return tasks.task, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runtime := Runtime{
		Client: client,
		Handler: Handler{
			Tasks: tasks,
			Queue: &fakeQueue{},
		},
		PollInterval:   time.Millisecond,
		PollLimit:      1,
		TimeoutSeconds: 1,
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_ = runtime.Run(ctx)
	if tasks.updateTaskCalls == 0 {
		t.Fatal("expected task metadata update call")
	}
	if tasks.lastUpdate == nil || tasks.lastUpdate.SourceMessageID == nil || *tasks.lastUpdate.SourceMessageID != 77 {
		t.Fatalf("unexpected source message update: %+v", tasks.lastUpdate)
	}
}

func TestRuntimePollingConflictRecoveryDeletesWebhook(t *testing.T) {
	client := &fakeTelegramClient{
		getUpdatesErrs: []error{
			&telegram.APIRequestError{
				Method:      "getUpdates",
				Code:        telegram.ErrorCodeConflict,
				Description: "Conflict: terminated by other getUpdates request",
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runtime := Runtime{
		Client:         client,
		Handler:        Handler{},
		PollInterval:   time.Millisecond,
		PollLimit:      1,
		TimeoutSeconds: 1,
	}

	go func() {
		time.Sleep(12 * time.Millisecond)
		cancel()
	}()

	_ = runtime.Run(ctx)
	if len(client.deleteWebhookReqs) < 2 {
		t.Fatalf("expected startup and conflict recovery deleteWebhook calls, got %d", len(client.deleteWebhookReqs))
	}
	for _, req := range client.deleteWebhookReqs {
		if req.DropPendingUpdates {
			t.Fatal("expected deleteWebhook to keep pending updates")
		}
	}
}

func TestRuntimeWebhookModeSetsWebhookAndStartsServer(t *testing.T) {
	client := &fakeTelegramClient{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	runtime := Runtime{
		Client:        client,
		Handler:       Handler{},
		UseWebhook:    true,
		WebhookURL:    "https://example.com/bot-webhook",
		WebhookSecret: "secret-token",
		WebhookAddr:   "127.0.0.1:0",
	}

	err := runtime.Run(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
	if len(client.setWebhookReqs) != 1 {
		t.Fatalf("expected one setWebhook call, got %d", len(client.setWebhookReqs))
	}
	if client.setWebhookReqs[0].URL != "https://example.com/bot-webhook" {
		t.Fatalf("unexpected webhook url: %s", client.setWebhookReqs[0].URL)
	}
	if len(client.setWebhookReqs[0].AllowedUpdates) != 2 {
		t.Fatalf("unexpected webhook allowed updates: %+v", client.setWebhookReqs[0].AllowedUpdates)
	}
	if len(client.deleteWebhookReqs) != 0 {
		t.Fatalf("expected no deleteWebhook in webhook mode, got %d", len(client.deleteWebhookReqs))
	}
}

func TestRuntimeWebhookHandlerRejectsWrongSecret(t *testing.T) {
	runtime := Runtime{
		Client:        &fakeTelegramClient{},
		Handler:       Handler{},
		WebhookSecret: "correct-secret",
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader([]byte(`{}`)))
	req.Header.Set(telegram.WebhookSecretHeader, "wrong-secret")
	runtime.webhookHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRuntimeWebhookHandlerProcessesUpdate(t *testing.T) {
	client := &fakeTelegramClient{}
	runtime := Runtime{
		Client:        client,
		Handler:       Handler{},
		WebhookSecret: "correct-secret",
	}

	update := telegram.Update{
		UpdateID: 10,
		Message: &telegram.Message{
			MessageID: 1,
			Chat:      telegram.Chat{ID: 1001},
			From:      &telegram.User{ID: 2002},
			Text:      "/help",
		},
	}
	b, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("marshal update: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(b))
	req.Header.Set(telegram.WebhookSecretHeader, "correct-secret")
	runtime.webhookHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(client.sentMessages) == 0 {
		t.Fatal("expected webhook update to trigger reply")
	}
}

func TestRuntimeAnswersCallbackQuery(t *testing.T) {
	client := &fakeTelegramClient{
		updates: []telegram.GetUpdatesResponse{
			{
				Ok: true,
				Result: []telegram.Update{
					{
						UpdateID: 1,
						CallbackQuery: &telegram.CallbackQuery{
							ID:   "cb-1",
							From: telegram.User{ID: 202},
							Message: &telegram.Message{
								MessageID: 77,
								Chat:      telegram.Chat{ID: 101},
							},
							Data: callbackDeleteNoPrefix + "task123456",
						},
					},
				},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runtime := Runtime{
		Client:         client,
		Handler:        Handler{},
		PollInterval:   time.Millisecond,
		PollLimit:      1,
		TimeoutSeconds: 1,
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_ = runtime.Run(ctx)
	if len(client.answeredCallbacks) == 0 {
		t.Fatal("expected callback answer to be sent")
	}
	if client.answeredCallbacks[0].CallbackQueryID != "cb-1" {
		t.Fatalf("unexpected callback query id: %+v", client.answeredCallbacks[0])
	}
}
