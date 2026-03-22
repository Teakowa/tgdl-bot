package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type D1Client struct {
	accountID  string
	databaseID string
	apiToken   string
	baseURL    string
	http       *http.Client
}

type D1QueryMeta struct {
	ChangedDB bool    `json:"changed_db"`
	Changes   int64   `json:"changes"`
	Duration  float64 `json:"duration"`
	LastRowID int64   `json:"last_row_id"`
	RowsRead  int64   `json:"rows_read"`
	RowsWrite int64   `json:"rows_written"`
}

type D1QueryResult struct {
	Meta    D1QueryMeta      `json:"meta"`
	Results []map[string]any `json:"results"`
	Success bool             `json:"success"`
	Error   string           `json:"error"`
}

func NewD1Client(accountID, databaseID, apiToken string, timeout time.Duration) *D1Client {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	return &D1Client{
		accountID:  strings.TrimSpace(accountID),
		databaseID: strings.TrimSpace(databaseID),
		apiToken:   strings.TrimSpace(apiToken),
		baseURL:    "https://api.cloudflare.com/client/v4",
		http:       &http.Client{Timeout: timeout},
	}
}

func (c *D1Client) Query(ctx context.Context, sql string, params ...any) (D1QueryResult, error) {
	if c == nil {
		return D1QueryResult{}, errors.New("storage: nil d1 client")
	}
	if c.accountID == "" || c.databaseID == "" || c.apiToken == "" {
		return D1QueryResult{}, errors.New("storage: account id, d1 database id, and api token are required")
	}

	payload := map[string]any{"sql": strings.TrimSpace(sql)}
	if len(params) > 0 {
		payload["params"] = params
	}

	var out struct {
		Success  bool            `json:"success"`
		Result   []D1QueryResult `json:"result"`
		Messages []cfAPIMessage  `json:"messages"`
		Errors   []cfAPIMessage  `json:"errors"`
	}
	if err := c.postJSON(ctx, "/query", payload, &out); err != nil {
		return D1QueryResult{}, err
	}

	if !out.Success {
		return D1QueryResult{}, fmt.Errorf("storage: d1 query failed: %s", formatCloudflareMessages(out.Errors))
	}
	if len(out.Errors) > 0 {
		return D1QueryResult{}, fmt.Errorf("storage: d1 query error: %s", formatCloudflareMessages(out.Errors))
	}
	if len(out.Result) == 0 {
		return D1QueryResult{Success: true}, nil
	}

	result := out.Result[0]
	if !result.Success {
		return D1QueryResult{}, fmt.Errorf("storage: d1 query result failed: %s", result.Error)
	}
	return result, nil
}

func (c *D1Client) postJSON(ctx context.Context, path string, payload any, decodeTarget any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("storage: encode d1 request: %w", err)
	}

	url := fmt.Sprintf("%s/accounts/%s/d1/database/%s%s", strings.TrimRight(c.baseURL, "/"), c.accountID, c.databaseID, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("storage: build d1 request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("storage: d1 request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("storage: read d1 response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("storage: d1 unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if decodeTarget == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, decodeTarget); err != nil {
		return fmt.Errorf("storage: decode d1 response: %w", err)
	}
	return nil
}

type cfAPIMessage struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func formatCloudflareMessages(messages []cfAPIMessage) string {
	if len(messages) == 0 {
		return "unknown cloudflare api error"
	}

	parts := make([]string, 0, len(messages))
	for _, msg := range messages {
		if msg.Message == "" {
			continue
		}
		if msg.Code == 0 {
			parts = append(parts, msg.Message)
			continue
		}
		parts = append(parts, fmt.Sprintf("%d: %s", msg.Code, msg.Message))
	}
	if len(parts) == 0 {
		return "unknown cloudflare api error"
	}
	return strings.Join(parts, "; ")
}
