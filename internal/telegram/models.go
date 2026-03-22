package telegram

import (
	"context"
)

type Client interface {
	GetUpdates(ctx context.Context, req GetUpdatesRequest) (GetUpdatesResponse, error)
	SendMessage(ctx context.Context, req SendMessageRequest) (Message, error)
	EditMessageText(ctx context.Context, req EditMessageTextRequest) error
	SetMessageReaction(ctx context.Context, req SetMessageReactionRequest) error
}

type GetUpdatesRequest struct {
	Offset         int64
	Limit          int
	TimeoutSeconds int
	AllowedUpdates []string
}

type GetUpdatesResponse struct {
	Ok     bool      `json:"ok"`
	Result []Update  `json:"result"`
	Error  *APIError `json:"error,omitempty"`
}

type SendMessageRequest struct {
	ChatID                int64
	Text                  string
	ParseMode             string
	DisableWebPagePreview bool
	ReplyToMessageID      *int64
}

type EditMessageTextRequest struct {
	ChatID                int64
	MessageID             int64
	Text                  string
	ParseMode             string
	DisableWebPagePreview bool
}

type ReactionTypeEmoji struct {
	Type  string `json:"type"`
	Emoji string `json:"emoji"`
}

type SetMessageReactionRequest struct {
	ChatID    int64               `json:"chat_id"`
	MessageID int64               `json:"message_id"`
	Reaction  []ReactionTypeEmoji `json:"reaction"`
	IsBig     bool                `json:"is_big,omitempty"`
}

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message,omitempty"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	Date      int64  `json:"date"`
	Chat      Chat   `json:"chat"`
	From      *User  `json:"from,omitempty"`
	Text      string `json:"text,omitempty"`
}

type Chat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title,omitempty"`
	Username string `json:"username,omitempty"`
}

type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
