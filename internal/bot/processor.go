package bot

import (
	"context"

	"tgdl-bot/internal/telegram"
)

type UpdateOutcome struct {
	SendRequest     *telegram.SendMessageRequest
	ReactionRequest *telegram.SetMessageReactionRequest
	TaskID          string
	SourceMessageID int64
}

func (h Handler) HandleUpdate(ctx context.Context, update telegram.Update) (*UpdateOutcome, error) {
	if update.Message == nil || update.Message.From == nil {
		return nil, nil
	}

	outcome, err := h.HandleTextWithOutcome(ctx, update.Message.From.ID, update.Message.Chat.ID, update.Message.Text)
	if err != nil {
		return nil, err
	}
	if outcome.Reply == "" {
		return nil, nil
	}

	result := &UpdateOutcome{
		SendRequest: &telegram.SendMessageRequest{
			ChatID: update.Message.Chat.ID,
			Text:   outcome.Reply,
		},
	}
	if update.Message.MessageID > 0 {
		replyToMessageID := update.Message.MessageID
		result.SendRequest.ReplyToMessageID = &replyToMessageID
	}
	if outcome.ReactionEmoji != "" {
		result.ReactionRequest = &telegram.SetMessageReactionRequest{
			ChatID:    update.Message.Chat.ID,
			MessageID: update.Message.MessageID,
			Reaction: []telegram.ReactionTypeEmoji{
				{Type: "emoji", Emoji: outcome.ReactionEmoji},
			},
		}
	}
	if outcome.TaskID != "" && update.Message.MessageID > 0 {
		result.TaskID = outcome.TaskID
		result.SourceMessageID = update.Message.MessageID
	}
	return result, nil
}
