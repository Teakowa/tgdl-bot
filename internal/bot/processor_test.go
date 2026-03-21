package bot

import (
	"context"
	"testing"

	"tgdl-bot/internal/telegram"
)

func TestHandleUpdateBuildsReply(t *testing.T) {
	h := Handler{}
	req, err := h.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			Chat: telegram.Chat{ID: 10},
			From: &telegram.User{ID: 20},
			Text: "/help",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req == nil || req.ChatID != 10 || req.Text == "" {
		t.Fatalf("unexpected send request: %+v", req)
	}
}
