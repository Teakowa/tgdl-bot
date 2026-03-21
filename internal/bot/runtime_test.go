package bot

import (
	"context"
	"testing"
	"time"

	"tgdl-bot/internal/telegram"
)

type fakeTelegramClient struct {
	updatesResp       telegram.GetUpdatesResponse
	sentMessages      []telegram.SendMessageRequest
	setReactionCalled int
}

func (f *fakeTelegramClient) GetUpdates(context.Context, telegram.GetUpdatesRequest) (telegram.GetUpdatesResponse, error) {
	return f.updatesResp, nil
}

func (f *fakeTelegramClient) SendMessage(_ context.Context, req telegram.SendMessageRequest) (telegram.Message, error) {
	f.sentMessages = append(f.sentMessages, req)
	return telegram.Message{Chat: telegram.Chat{ID: req.ChatID}, Text: req.Text}, nil
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
