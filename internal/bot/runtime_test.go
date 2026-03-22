package bot

import (
	"context"
	"testing"
	"time"

	"tgdl-bot/internal/service"
	"tgdl-bot/internal/telegram"
)

type fakeTelegramClient struct {
	updatesResp       telegram.GetUpdatesResponse
	sentMessages      []telegram.SendMessageRequest
	editedMessages    []telegram.EditMessageTextRequest
	setReactionCalled int
}

func (f *fakeTelegramClient) GetUpdates(context.Context, telegram.GetUpdatesRequest) (telegram.GetUpdatesResponse, error) {
	return f.updatesResp, nil
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

func TestRuntimeProcessesUpdate(t *testing.T) {
	client := &fakeTelegramClient{
		updatesResp: telegram.GetUpdatesResponse{
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
}

func TestRuntimeBindsTaskMessageRefsForCreatedTask(t *testing.T) {
	client := &fakeTelegramClient{
		updatesResp: telegram.GetUpdatesResponse{
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
