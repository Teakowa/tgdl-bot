package telegram

import (
	"context"
	"encoding/json"
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
