package queue

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEnqueueBatchUsesBodyWrapper(t *testing.T) {
	var capturedPath string
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	client := NewCloudflareClient("acc", "queue", "token", time.Second)
	client.baseURL = server.URL

	err := client.EnqueueBatch(context.Background(), []Message{
		{
			TaskID:       "t1",
			ChatID:       1,
			UserID:       2,
			TargetChatID: 3,
			URL:          "https://t.me/c/1/2",
			CreatedAt:    time.Now().UTC(),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPath != "/accounts/acc/queues/queue/messages/batch" {
		t.Fatalf("unexpected path: %s", capturedPath)
	}

	msgs, ok := captured["messages"].([]any)
	if !ok || len(msgs) != 1 {
		t.Fatalf("unexpected messages payload: %#v", captured["messages"])
	}
	first, ok := msgs[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected first message: %#v", msgs[0])
	}
	if _, hasBody := first["body"]; !hasBody {
		t.Fatalf("expected body wrapper, got: %#v", first)
	}
}

func TestEnqueueUsesSingleBodyPayload(t *testing.T) {
	var capturedPath string
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	client := NewCloudflareClient("acc", "queue", "token", time.Second)
	client.baseURL = server.URL

	err := client.Enqueue(context.Background(), Message{
		TaskID:       "t1",
		ChatID:       1,
		UserID:       2,
		TargetChatID: 3,
		URL:          "https://t.me/c/1/2",
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedPath != "/accounts/acc/queues/queue/messages" {
		t.Fatalf("unexpected path: %s", capturedPath)
	}
	if _, hasBody := captured["body"]; !hasBody {
		t.Fatalf("expected single body payload, got: %#v", captured)
	}
}
