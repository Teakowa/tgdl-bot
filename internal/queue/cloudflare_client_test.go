package queue

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestEnqueueOmitsTargetChatIDWhenUnset(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		TaskID:    "t-no-target",
		ChatID:    1,
		UserID:    2,
		URL:       "https://t.me/c/1/2",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, ok := captured["body"].(map[string]any)
	if !ok {
		t.Fatalf("expected body payload, got: %#v", captured["body"])
	}
	if _, exists := body["target_chat_id"]; exists {
		t.Fatalf("expected target_chat_id to be omitted when unset, got: %#v", body)
	}
}

func TestPullSkipsInvalidBodiesAndParsesJSONStringBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/messages/pull") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"result": {
				"messages": [
					{"lease_id":"l1","body":"not-json-object"},
					{"lease_id":"l2","body":"{\"task_id\":\"t2\",\"chat_id\":1,\"user_id\":2,\"target_chat_id\":3,\"url\":\"https://t.me/c/1/2\",\"created_at\":\"2026-03-21T18:00:00Z\"}"},
					{"lease_id":"l3","body":"eyJ0YXNrX2lkIjoidDMiLCJjaGF0X2lkIjoxLCJ1c2VyX2lkIjoyLCJ0YXJnZXRfY2hhdF9pZCI6MywidXJsIjoiaHR0cHM6Ly90Lm1lL2MvMS8zIiwiY3JlYXRlZF9hdCI6IjIwMjYtMDMtMjFUMTg6MDA6MDBaIn0="}
				]
			}
		}`))
	}))
	defer server.Close()

	client := NewCloudflareClient("acc", "queue", "token", time.Second)
	client.baseURL = server.URL

	got, err := client.Pull(context.Background(), 5, 30_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(got))
	}
	if got[0].LeaseID != "l1" || got[0].Body.TaskID != "" {
		t.Fatalf("expected first message invalid/empty body, got %+v", got[0])
	}
	if got[1].Body.TaskID != "t2" {
		t.Fatalf("expected json-string body to parse, got %+v", got[1].Body)
	}
	if got[2].Body.TaskID != "t3" {
		t.Fatalf("expected base64 string body to parse, got %+v", got[2].Body)
	}
}

func TestDecodeMessageBodyParsesBase64JSON(t *testing.T) {
	rawJSON := `{"task_id":"t4","chat_id":1,"user_id":2,"target_chat_id":3,"url":"https://t.me/c/1/4","created_at":"2026-03-21T18:00:00Z"}`
	encoded := base64.StdEncoding.EncodeToString([]byte(rawJSON))
	input, _ := json.Marshal(encoded)

	got, err := decodeMessageBody(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.TaskID != "t4" {
		t.Fatalf("unexpected decoded task id: %+v", got)
	}
}

func TestAckUsesAckEndpointContract(t *testing.T) {
	var capturedPath string
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	client := NewCloudflareClient("acc", "queue", "token", time.Second)
	client.baseURL = server.URL

	if err := client.Ack(context.Background(), []string{"l1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPath != "/accounts/acc/queues/queue/messages/ack" {
		t.Fatalf("unexpected path: %s", capturedPath)
	}
	acks, ok := captured["acks"].([]any)
	if !ok || len(acks) != 1 {
		t.Fatalf("unexpected ack payload: %#v", captured)
	}
}

func TestRetryUsesAckEndpointContract(t *testing.T) {
	var capturedPath string
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	client := NewCloudflareClient("acc", "queue", "token", time.Second)
	client.baseURL = server.URL

	if err := client.Retry(context.Background(), []string{"l2"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPath != "/accounts/acc/queues/queue/messages/ack" {
		t.Fatalf("unexpected path: %s", capturedPath)
	}
	retries, ok := captured["retries"].([]any)
	if !ok || len(retries) != 1 {
		t.Fatalf("unexpected retry payload: %#v", captured)
	}
}
