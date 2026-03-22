package telegram

import (
	"context"
	"errors"
	"fmt"
)

type Client interface {
	GetUpdates(ctx context.Context, req GetUpdatesRequest) (GetUpdatesResponse, error)
	SetWebhook(ctx context.Context, req SetWebhookRequest) error
	DeleteWebhook(ctx context.Context, req DeleteWebhookRequest) error
	SendMessage(ctx context.Context, req SendMessageRequest) (Message, error)
	EditMessageText(ctx context.Context, req EditMessageTextRequest) error
	SetMessageReaction(ctx context.Context, req SetMessageReactionRequest) error
}

const (
	WebhookSecretHeader = "X-Telegram-Bot-Api-Secret-Token"
	ErrorCodeConflict   = 409
)

type GetUpdatesRequest struct {
	Offset         int64
	Limit          int
	TimeoutSeconds int
	AllowedUpdates []string
}

type GetUpdatesResponse struct {
	Ok          bool     `json:"ok"`
	Result      []Update `json:"result"`
	ErrorCode   int      `json:"error_code,omitempty"`
	Description string   `json:"description,omitempty"`
}

type SetWebhookRequest struct {
	URL            string
	SecretToken    string
	AllowedUpdates []string
}

type DeleteWebhookRequest struct {
	DropPendingUpdates bool
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

type APIRequestError struct {
	Method      string
	Code        int
	Description string
}

func (e *APIRequestError) Error() string {
	method := e.Method
	if method == "" {
		method = "request"
	}
	return fmt.Sprintf("telegram %s api error: %d %s", method, e.Code, e.Description)
}

func IsAPIErrorCode(err error, code int) bool {
	var apiErr *APIRequestError
	return errors.As(err, &apiErr) && apiErr.Code == code
}
