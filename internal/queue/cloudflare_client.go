package queue

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

type CloudflareClient struct {
	accountID string
	queueID   string
	apiToken  string
	baseURL   string
	http      *http.Client
}

func NewCloudflareClient(accountID, queueID, apiToken string, timeout time.Duration) *CloudflareClient {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &CloudflareClient{
		accountID: strings.TrimSpace(accountID),
		queueID:   strings.TrimSpace(queueID),
		apiToken:  strings.TrimSpace(apiToken),
		baseURL:   "https://api.cloudflare.com/client/v4",
		http:      &http.Client{Timeout: timeout},
	}
}

func (c *CloudflareClient) Enqueue(ctx context.Context, message Message) error {
	return c.EnqueueBatch(ctx, []Message{message})
}

func (c *CloudflareClient) EnqueueBatch(ctx context.Context, messages []Message) error {
	if len(messages) == 0 {
		return nil
	}
	payload := map[string]any{"messages": messages}
	return c.postJSON(ctx, "/messages", payload, nil)
}

func (c *CloudflareClient) Pull(ctx context.Context, batchSize, visibilityTimeoutMs int) ([]ReceivedMessage, error) {
	if batchSize <= 0 {
		batchSize = 1
	}
	if visibilityTimeoutMs <= 0 {
		visibilityTimeoutMs = 30_000
	}

	payload := map[string]any{
		"batch_size":            batchSize,
		"visibility_timeout_ms": visibilityTimeoutMs,
	}
	var out struct {
		Result struct {
			Messages []struct {
				LeaseID string          `json:"lease_id"`
				Body    json.RawMessage `json:"body"`
			} `json:"messages"`
		} `json:"result"`
	}
	if err := c.postJSON(ctx, "/messages/pull", payload, &out); err != nil {
		return nil, err
	}

	received := make([]ReceivedMessage, 0, len(out.Result.Messages))
	for _, msg := range out.Result.Messages {
		var body Message
		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return nil, fmt.Errorf("queue: decode pulled message body: %w", err)
		}
		received = append(received, ReceivedMessage{
			LeaseID: msg.LeaseID,
			Body:    body,
			RawBody: append([]byte(nil), msg.Body...),
		})
	}
	return received, nil
}

func (c *CloudflareClient) Ack(ctx context.Context, leaseIDs []string) error {
	if len(leaseIDs) == 0 {
		return nil
	}
	return c.postJSON(ctx, "/messages/ack", map[string]any{"leases": leaseIDs}, nil)
}

func (c *CloudflareClient) Retry(ctx context.Context, leaseIDs []string) error {
	if len(leaseIDs) == 0 {
		return nil
	}
	return c.postJSON(ctx, "/messages/retry", map[string]any{"leases": leaseIDs}, nil)
}

func (c *CloudflareClient) AckAndRetry(ctx context.Context, ackLeaseIDs, retryLeaseIDs []string) error {
	if err := c.Ack(ctx, ackLeaseIDs); err != nil {
		return err
	}
	if err := c.Retry(ctx, retryLeaseIDs); err != nil {
		return err
	}
	return nil
}

func (c *CloudflareClient) postJSON(ctx context.Context, path string, payload any, decodeTarget any) error {
	if c == nil {
		return errors.New("queue: nil cloudflare client")
	}
	if c.accountID == "" || c.queueID == "" || c.apiToken == "" {
		return errors.New("queue: account id, queue id, and api token are required")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("queue: encode request: %w", err)
	}

	url := fmt.Sprintf("%s/accounts/%s/queues/%s%s", strings.TrimRight(c.baseURL, "/"), c.accountID, c.queueID, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("queue: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("queue: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("queue: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("queue: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	if decodeTarget == nil {
		return nil
	}
	if len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, decodeTarget); err != nil {
		return fmt.Errorf("queue: decode response: %w", err)
	}
	return nil
}
