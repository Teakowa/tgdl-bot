package telegram

import (
	"context"
	"time"
)

type Client interface {
	GetUpdates(ctx context.Context, req GetUpdatesRequest) (GetUpdatesResponse, error)
	SendMessage(ctx context.Context, req SendMessageRequest) (Message, error)
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

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message,omitempty"`
}

type Message struct {
	MessageID int64     `json:"message_id"`
	Date      time.Time `json:"date"`
	Chat      Chat      `json:"chat"`
	From      *User     `json:"from,omitempty"`
	Text      string    `json:"text,omitempty"`
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
