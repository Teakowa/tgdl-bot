package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSetMessageReactionSendsExpectedPayload(t *testing.T) {
	var capturedPath string
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "token", time.Second)
	err := client.SetMessageReaction(context.Background(), SetMessageReactionRequest{
		ChatID:    123,
		MessageID: 456,
		Reaction: []ReactionTypeEmoji{
			{Type: "emoji", Emoji: "✅"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedPath != "/bottoken/setMessageReaction" {
		t.Fatalf("unexpected path: %s", capturedPath)
	}
	if int(captured["chat_id"].(float64)) != 123 {
		t.Fatalf("unexpected chat_id: %v", captured["chat_id"])
	}
	if int(captured["message_id"].(float64)) != 456 {
		t.Fatalf("unexpected message_id: %v", captured["message_id"])
	}
}

func TestEditMessageTextSendsExpectedPayload(t *testing.T) {
	var capturedPath string
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":456}}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "token", time.Second)
	err := client.EditMessageText(context.Background(), EditMessageTextRequest{
		ChatID:    123,
		MessageID: 456,
		Text:      "new text",
		ReplyMarkup: &InlineKeyboardMarkup{
			InlineKeyboard: [][]InlineKeyboardButton{
				{{Text: "Delete", CallbackData: "qdel:task123"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedPath != "/bottoken/editMessageText" {
		t.Fatalf("unexpected path: %s", capturedPath)
	}
	if int(captured["chat_id"].(float64)) != 123 {
		t.Fatalf("unexpected chat_id: %v", captured["chat_id"])
	}
	if int(captured["message_id"].(float64)) != 456 {
		t.Fatalf("unexpected message_id: %v", captured["message_id"])
	}
	if captured["text"] != "new text" {
		t.Fatalf("unexpected text: %v", captured["text"])
	}
	replyMarkup, ok := captured["reply_markup"].(map[string]any)
	if !ok {
		t.Fatalf("expected reply_markup object, got %T", captured["reply_markup"])
	}
	keyboard, ok := replyMarkup["inline_keyboard"].([]any)
	if !ok || len(keyboard) != 1 {
		t.Fatalf("unexpected inline keyboard payload: %v", replyMarkup["inline_keyboard"])
	}
}

func TestEditMessageTextReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"bad request"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "token", time.Second)
	err := client.EditMessageText(context.Background(), EditMessageTextRequest{
		ChatID:    1,
		MessageID: 2,
		Text:      "updated",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSetMessageReactionReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"bad request"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "token", time.Second)
	err := client.SetMessageReaction(context.Background(), SetMessageReactionRequest{
		ChatID:    1,
		MessageID: 2,
		Reaction: []ReactionTypeEmoji{
			{Type: "emoji", Emoji: "❌"},
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSendMessageIncludesReplyToMessageID(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"chat":{"id":123},"text":"ok"}}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "token", time.Second)
	replyTo := int64(42)
	if _, err := client.SendMessage(context.Background(), SendMessageRequest{
		ChatID:           123,
		Text:             "ok",
		ReplyToMessageID: &replyTo,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if int(captured["reply_to_message_id"].(float64)) != 42 {
		t.Fatalf("unexpected reply_to_message_id: %v", captured["reply_to_message_id"])
	}
}

func TestSendMessageIncludesReplyMarkup(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"chat":{"id":123},"text":"ok"}}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "token", time.Second)
	if _, err := client.SendMessage(context.Background(), SendMessageRequest{
		ChatID: 123,
		Text:   "ok",
		ReplyMarkup: &InlineKeyboardMarkup{
			InlineKeyboard: [][]InlineKeyboardButton{
				{{Text: "Delete", CallbackData: "qdel:task123"}},
			},
		},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	replyMarkup, ok := captured["reply_markup"].(map[string]any)
	if !ok {
		t.Fatalf("expected reply_markup object, got %T", captured["reply_markup"])
	}
	keyboard, ok := replyMarkup["inline_keyboard"].([]any)
	if !ok || len(keyboard) != 1 {
		t.Fatalf("unexpected inline keyboard payload: %v", replyMarkup["inline_keyboard"])
	}
}

func TestSetWebhookSendsExpectedPayload(t *testing.T) {
	var capturedPath string
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "token", time.Second)
	err := client.SetWebhook(context.Background(), SetWebhookRequest{
		URL:            "https://example.com/hook",
		SecretToken:    "secret",
		AllowedUpdates: []string{"message"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedPath != "/bottoken/setWebhook" {
		t.Fatalf("unexpected path: %s", capturedPath)
	}
	if captured["url"] != "https://example.com/hook" {
		t.Fatalf("unexpected webhook url: %v", captured["url"])
	}
	if captured["secret_token"] != "secret" {
		t.Fatalf("unexpected secret token: %v", captured["secret_token"])
	}
	allowed, ok := captured["allowed_updates"].([]any)
	if !ok || len(allowed) != 1 || allowed[0] != "message" {
		t.Fatalf("unexpected allowed updates: %v", captured["allowed_updates"])
	}
}

func TestDeleteWebhookSendsDropPendingFlag(t *testing.T) {
	var capturedPath string
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "token", time.Second)
	err := client.DeleteWebhook(context.Background(), DeleteWebhookRequest{
		DropPendingUpdates: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedPath != "/bottoken/deleteWebhook" {
		t.Fatalf("unexpected path: %s", capturedPath)
	}
	dropPending, ok := captured["drop_pending_updates"].(bool)
	if !ok {
		t.Fatalf("expected bool drop_pending_updates, got %T", captured["drop_pending_updates"])
	}
	if dropPending {
		t.Fatalf("expected drop_pending_updates=false, got true")
	}
}

func TestAnswerCallbackQuerySendsExpectedPayload(t *testing.T) {
	var capturedPath string
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "token", time.Second)
	err := client.AnswerCallbackQuery(context.Background(), AnswerCallbackQueryRequest{
		CallbackQueryID: "cb-1",
		Text:            "done",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedPath != "/bottoken/answerCallbackQuery" {
		t.Fatalf("unexpected path: %s", capturedPath)
	}
	if captured["callback_query_id"] != "cb-1" {
		t.Fatalf("unexpected callback query id: %v", captured["callback_query_id"])
	}
	if captured["text"] != "done" {
		t.Fatalf("unexpected callback text: %v", captured["text"])
	}
}

func TestGetUpdatesReturnsTypedAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"error_code":409,"description":"Conflict: webhook active"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "token", time.Second)
	_, err := client.GetUpdates(context.Background(), GetUpdatesRequest{
		Limit:          1,
		TimeoutSeconds: 1,
	})
	if err == nil {
		t.Fatal("expected error")
	}

	var apiErr *APIRequestError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIRequestError, got %T", err)
	}
	if apiErr.Code != 409 {
		t.Fatalf("expected code 409, got %d", apiErr.Code)
	}
	if !IsAPIErrorCode(err, ErrorCodeConflict) {
		t.Fatal("expected IsAPIErrorCode to match conflict code")
	}
}
