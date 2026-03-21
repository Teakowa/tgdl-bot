package bot

import (
	"context"

	"tgdl-bot/internal/telegram"
)

func (h Handler) HandleUpdate(ctx context.Context, update telegram.Update) (*telegram.SendMessageRequest, error) {
	if update.Message == nil || update.Message.From == nil {
		return nil, nil
	}

	reply, err := h.HandleText(ctx, update.Message.From.ID, update.Message.Chat.ID, update.Message.Text)
	if err != nil {
		return nil, err
	}
	if reply == "" {
		return nil, nil
	}

	return &telegram.SendMessageRequest{
		ChatID: update.Message.Chat.ID,
		Text:   reply,
	}, nil
}
