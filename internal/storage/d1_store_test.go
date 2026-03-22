package storage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
)

func TestParseConditionalAddColumnStatement(t *testing.T) {
	stmt, ok := parseConditionalAddColumnStatement("ALTER TABLE tasks ADD COLUMN IF NOT EXISTS drop_caption INTEGER NOT NULL DEFAULT 0")
	if !ok {
		t.Fatal("expected statement to parse")
	}
	if stmt.TableName != "tasks" {
		t.Fatalf("unexpected table name: %q", stmt.TableName)
	}
	if stmt.ColumnName != "drop_caption" {
		t.Fatalf("unexpected column name: %q", stmt.ColumnName)
	}
	if stmt.AlterSQL != "ALTER TABLE tasks ADD COLUMN drop_caption INTEGER NOT NULL DEFAULT 0" {
		t.Fatalf("unexpected alter sql: %q", stmt.AlterSQL)
	}
}

func TestD1StoreApplyMigrationsSkipsExistingColumns(t *testing.T) {
	var queries []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		var body struct {
			SQL string `json:"sql"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		queries = append(queries, body.SQL)

		response := `{"success":true,"result":[{"success":true,"results":[],"meta":{"changes":0}}]}`
		switch body.SQL {
		case "PRAGMA table_info(tasks)":
			response = `{"success":true,"result":[{"success":true,"results":[{"name":"task_id"},{"name":"target_peer"}],"meta":{"changes":0}}]}`
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	client := NewD1Client("acc", "db", "token", time.Second)
	client.baseURL = server.URL
	store := NewD1Store(client)

	err := store.ApplyMigrations(context.Background(), Migration{
		Name: "002_target_peer.sql",
		SQL: `ALTER TABLE tasks ADD COLUMN IF NOT EXISTS target_peer TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS drop_caption INTEGER NOT NULL DEFAULT 0;
UPDATE tasks
SET target_peer = ''
WHERE target_peer IS NULL;`,
	})
	if err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	want := []string{
		"PRAGMA table_info(tasks)",
		"PRAGMA table_info(tasks)",
		"ALTER TABLE tasks ADD COLUMN drop_caption INTEGER NOT NULL DEFAULT 0",
		"UPDATE tasks\nSET target_peer = ''\nWHERE target_peer IS NULL",
	}
	if !slices.Equal(queries, want) {
		t.Fatalf("unexpected query order:\n got: %#v\nwant: %#v", queries, want)
	}
}
