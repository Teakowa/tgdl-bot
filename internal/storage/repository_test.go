package storage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tgdl-bot/internal/service"
)

func TestD1TaskRepositoryClaimForExecutionSuccess(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	call := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		call++
		w.WriteHeader(http.StatusOK)

		switch call {
		case 1:
			_, _ = w.Write([]byte(`{
				"success": true,
				"result": [{"success": true, "results": [], "meta": {"changes": 1}}],
				"errors": []
			}`))
		case 2:
			_, _ = w.Write([]byte(`{
				"success": true,
				"result": [{
					"success": true,
					"results": [{
						"task_id":"t1",
						"chat_id":1,
						"user_id":2,
						"target_peer":"",
						"url":"https://t.me/c/1/2",
						"drop_caption":0,
						"status":"running",
						"idempotency_key":"idem",
						"retry_count":0,
						"source_message_id":null,
						"status_message_id":null,
						"lease_id":"lease-1",
						"output_summary":null,
						"error_message":null,
						"exit_code":null,
						"created_at":"` + now + `",
						"updated_at":"` + now + `",
						"started_at":"` + now + `",
						"finished_at":null
					}],
					"meta": {"changes": 0}
				}],
				"errors": []
			}`))
		default:
			t.Fatalf("unexpected call index %d", call)
		}
	}))
	defer server.Close()

	client := NewD1Client("acc", "db", "token", time.Second)
	client.baseURL = server.URL
	repo := NewD1TaskRepository(client)

	task, claimed, err := repo.ClaimForExecution(context.Background(), "t1", "lease-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if !claimed {
		t.Fatal("expected claimed=true")
	}
	if task.TaskID != "t1" || task.Status != service.StatusRunning {
		t.Fatalf("unexpected claimed task: %+v", task)
	}
}

func TestD1TaskRepositoryClaimForExecutionNotClaimable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"success": true,
			"result": [{"success": true, "results": [], "meta": {"changes": 0}}],
			"errors": []
		}`))
	}))
	defer server.Close()

	client := NewD1Client("acc", "db", "token", time.Second)
	client.baseURL = server.URL
	repo := NewD1TaskRepository(client)

	task, claimed, err := repo.ClaimForExecution(context.Background(), "t1", "lease-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if claimed {
		t.Fatalf("expected claimed=false, got task=%+v", task)
	}
}

func TestD1TaskRepositoryCreateUsesInsertSQL(t *testing.T) {
	var payload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"result":[{"success":true,"results":[],"meta":{"changes":1}}]}`))
	}))
	defer server.Close()

	client := NewD1Client("acc", "db", "token", time.Second)
	client.baseURL = server.URL
	repo := NewD1TaskRepository(client)

	task := service.Task{
		TaskID:         "t1",
		ChatID:         1,
		UserID:         2,
		TargetPeer:     "channel_name",
		URL:            "https://t.me/c/1/2",
		DropCaption:    true,
		Status:         service.StatusQueued,
		IdempotencyKey: "idem",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	if err := repo.Create(context.Background(), task); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	sqlValue, ok := payload["sql"].(string)
	if !ok || sqlValue == "" {
		t.Fatalf("unexpected sql payload: %#v", payload["sql"])
	}
}

func TestD1TaskRepositoryListActiveByUserUsesStatusPriorityOrder(t *testing.T) {
	var payload map[string]any
	now := time.Now().UTC().Format(time.RFC3339Nano)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"success": true,
			"result": [{
				"success": true,
				"results": [{
					"task_id":"t1",
					"chat_id":1,
					"user_id":2,
					"target_peer":"",
					"url":"https://t.me/c/1/2",
					"drop_caption":0,
					"status":"running",
					"idempotency_key":"idem",
					"retry_count":0,
					"source_message_id":null,
					"status_message_id":null,
					"lease_id":null,
					"output_summary":null,
					"error_message":null,
					"exit_code":null,
					"created_at":"` + now + `",
					"updated_at":"` + now + `",
					"started_at":null,
					"finished_at":null
				}],
				"meta": {"changes": 0}
			}],
			"errors": []
		}`))
	}))
	defer server.Close()

	client := NewD1Client("acc", "db", "token", time.Second)
	client.baseURL = server.URL
	repo := NewD1TaskRepository(client)

	tasks, err := repo.ListActiveByUser(context.Background(), 2, 20)
	if err != nil {
		t.Fatalf("list active failed: %v", err)
	}
	if len(tasks) != 1 || tasks[0].TaskID != "t1" {
		t.Fatalf("unexpected tasks: %+v", tasks)
	}

	sqlValue, ok := payload["sql"].(string)
	if !ok {
		t.Fatalf("unexpected sql payload: %#v", payload["sql"])
	}
	for _, want := range []string{"status IN", "CASE status", "created_at ASC"} {
		if !strings.Contains(sqlValue, want) {
			t.Fatalf("expected sql to contain %q, got: %s", want, sqlValue)
		}
	}
}

func TestD1TaskRepositoryListQueueByUserUsesQueueStatusPriorityOrder(t *testing.T) {
	var payload map[string]any
	now := time.Now().UTC().Format(time.RFC3339Nano)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"success": true,
			"result": [{
				"success": true,
				"results": [{
					"task_id":"t1",
					"chat_id":1,
					"user_id":2,
					"target_chat_id":0,
					"target_peer":"channel_name",
					"url":"https://t.me/channel_name/2",
					"drop_caption":0,
					"status":"paused",
					"idempotency_key":"idem",
					"retry_count":0,
					"source_message_id":null,
					"status_message_id":null,
					"lease_id":null,
					"output_summary":null,
					"error_message":null,
					"exit_code":null,
					"created_at":"` + now + `",
					"updated_at":"` + now + `",
					"started_at":null,
					"finished_at":null
				}],
				"meta": {"changes": 0}
			}],
			"errors": []
		}`))
	}))
	defer server.Close()

	client := NewD1Client("acc", "db", "token", time.Second)
	client.baseURL = server.URL
	repo := NewD1TaskRepository(client)

	tasks, err := repo.ListQueueByUser(context.Background(), 2, 20)
	if err != nil {
		t.Fatalf("list queue failed: %v", err)
	}
	if len(tasks) != 1 || tasks[0].TaskID != "t1" {
		t.Fatalf("unexpected tasks: %+v", tasks)
	}

	sqlValue, ok := payload["sql"].(string)
	if !ok {
		t.Fatalf("unexpected sql payload: %#v", payload["sql"])
	}
	for _, want := range []string{"status IN", "CASE status", "created_at ASC"} {
		if !strings.Contains(sqlValue, want) {
			t.Fatalf("expected sql to contain %q, got: %s", want, sqlValue)
		}
	}
}

func TestD1TaskRepositoryDeletePendingByUserTaskIDUsesPendingStatusFilter(t *testing.T) {
	var payload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"success": true,
			"result": [{"success": true, "results": [], "meta": {"changes": 1}}],
			"errors": []
		}`))
	}))
	defer server.Close()

	client := NewD1Client("acc", "db", "token", time.Second)
	client.baseURL = server.URL
	repo := NewD1TaskRepository(client)

	rows, err := repo.DeletePendingByUserTaskID(context.Background(), 2, "task-1")
	if err != nil {
		t.Fatalf("delete pending failed: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 deleted row, got %d", rows)
	}

	sqlValue, ok := payload["sql"].(string)
	if !ok {
		t.Fatalf("unexpected sql payload: %#v", payload["sql"])
	}
	if !strings.Contains(sqlValue, "status IN") {
		t.Fatalf("expected pending status filter in sql, got: %s", sqlValue)
	}
}

func TestD1TaskRepositoryDeleteNonRunningByUserTaskIDUsesNonRunningStatusFilter(t *testing.T) {
	var payload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"success": true,
			"result": [{"success": true, "results": [], "meta": {"changes": 1}}],
			"errors": []
		}`))
	}))
	defer server.Close()

	client := NewD1Client("acc", "db", "token", time.Second)
	client.baseURL = server.URL
	repo := NewD1TaskRepository(client)

	rows, err := repo.DeleteNonRunningByUserTaskID(context.Background(), 2, "task-1")
	if err != nil {
		t.Fatalf("delete non-running failed: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 deleted row, got %d", rows)
	}

	sqlValue, ok := payload["sql"].(string)
	if !ok {
		t.Fatalf("unexpected sql payload: %#v", payload["sql"])
	}
	if !strings.Contains(sqlValue, "status <>") {
		t.Fatalf("expected non-running status filter in sql, got: %s", sqlValue)
	}
}

func TestD1TaskRepositoryPauseByUserTaskIDUsesPendingStatusFilter(t *testing.T) {
	var payload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"success": true,
			"result": [{"success": true, "results": [], "meta": {"changes": 1}}],
			"errors": []
		}`))
	}))
	defer server.Close()

	client := NewD1Client("acc", "db", "token", time.Second)
	client.baseURL = server.URL
	repo := NewD1TaskRepository(client)

	rows, err := repo.PauseByUserTaskID(context.Background(), 2, "task-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("pause failed: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 changed row, got %d", rows)
	}

	sqlValue, ok := payload["sql"].(string)
	if !ok {
		t.Fatalf("unexpected sql payload: %#v", payload["sql"])
	}
	if !strings.Contains(sqlValue, "status IN") {
		t.Fatalf("expected pending status guard in sql, got: %s", sqlValue)
	}
}

func TestD1TaskRepositoryResumeByUserTaskIDUsesPausedStatusFilter(t *testing.T) {
	var payload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"success": true,
			"result": [{"success": true, "results": [], "meta": {"changes": 1}}],
			"errors": []
		}`))
	}))
	defer server.Close()

	client := NewD1Client("acc", "db", "token", time.Second)
	client.baseURL = server.URL
	repo := NewD1TaskRepository(client)

	rows, err := repo.ResumeByUserTaskID(context.Background(), 2, "task-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 changed row, got %d", rows)
	}

	sqlValue, ok := payload["sql"].(string)
	if !ok {
		t.Fatalf("unexpected sql payload: %#v", payload["sql"])
	}
	if !strings.Contains(sqlValue, "status = ?") {
		t.Fatalf("expected paused status guard in sql, got: %s", sqlValue)
	}
}

func TestD1TaskRepositoryCancelByUserTaskIDUsesCancelableStatusFilter(t *testing.T) {
	var payload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"success": true,
			"result": [{"success": true, "results": [], "meta": {"changes": 1}}],
			"errors": []
		}`))
	}))
	defer server.Close()

	client := NewD1Client("acc", "db", "token", time.Second)
	client.baseURL = server.URL
	repo := NewD1TaskRepository(client)

	rows, err := repo.CancelByUserTaskID(context.Background(), 2, "task-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 changed row, got %d", rows)
	}

	sqlValue, ok := payload["sql"].(string)
	if !ok {
		t.Fatalf("unexpected sql payload: %#v", payload["sql"])
	}
	if !strings.Contains(sqlValue, "finished_at") || !strings.Contains(sqlValue, "status IN") {
		t.Fatalf("expected cancel status guard in sql, got: %s", sqlValue)
	}
}

func TestTaskFromResultRowReadsTargetPeerAndDropCaption(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task, err := taskFromResultRow(map[string]any{
		"task_id":           "t1",
		"chat_id":           int64(1),
		"user_id":           int64(2),
		"target_peer":       "channel_name",
		"url":               "https://t.me/c/1/2",
		"drop_caption":      int64(1),
		"status":            "queued",
		"idempotency_key":   "idem",
		"retry_count":       int64(0),
		"source_message_id": nil,
		"status_message_id": nil,
		"lease_id":          nil,
		"output_summary":    nil,
		"error_message":     nil,
		"exit_code":         nil,
		"created_at":        now,
		"updated_at":        now,
		"started_at":        nil,
		"finished_at":       nil,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.TargetPeer != "channel_name" {
		t.Fatalf("expected target peer, got %+v", task)
	}
	if !task.DropCaption {
		t.Fatalf("expected drop caption true, got %+v", task)
	}
}
