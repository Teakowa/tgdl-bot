package storage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestD1ClientQueryUsesExpectedEndpointAndAuth(t *testing.T) {
	var capturedPath string
	var capturedAuth string
	var capturedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"result":[{"success":true,"results":[],"meta":{"changes":0}}]}`))
	}))
	defer server.Close()

	client := NewD1Client("acc", "db", "token", time.Second)
	client.baseURL = server.URL

	if _, err := client.Query(context.Background(), "SELECT * FROM tasks WHERE task_id = ?", "t1"); err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if capturedPath != "/accounts/acc/d1/database/db/query" {
		t.Fatalf("unexpected request path: %s", capturedPath)
	}
	if capturedAuth != "Bearer token" {
		t.Fatalf("unexpected auth header: %q", capturedAuth)
	}
	if got, ok := capturedBody["sql"].(string); !ok || got != "SELECT * FROM tasks WHERE task_id = ?" {
		t.Fatalf("unexpected sql payload: %#v", capturedBody["sql"])
	}
	params, ok := capturedBody["params"].([]any)
	if !ok || len(params) != 1 || params[0] != "t1" {
		t.Fatalf("unexpected params payload: %#v", capturedBody["params"])
	}
}

func TestD1ClientQueryReturnsCloudflareError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":7003,"message":"invalid account id"}]}`))
	}))
	defer server.Close()

	client := NewD1Client("acc", "db", "token", time.Second)
	client.baseURL = server.URL

	_, err := client.Query(context.Background(), "SELECT 1")
	if err == nil {
		t.Fatal("expected query error")
	}
	if !strings.Contains(err.Error(), "7003") {
		t.Fatalf("expected cloudflare error details, got %v", err)
	}
}
