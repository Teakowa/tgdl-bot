package queue

import (
	"bytes"
	"context"
	"encoding/base64"
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
	type queuePushMessage struct {
		Body Message `json:"body"`
	}
	return c.postJSON(ctx, "/messages", queuePushMessage{Body: message}, nil)
}

func (c *CloudflareClient) EnqueueBatch(ctx context.Context, messages []Message) error {
	if len(messages) == 0 {
		return nil
	}
	type queuePushMessage struct {
		Body Message `json:"body"`
	}
	pushMessages := make([]queuePushMessage, 0, len(messages))
	for _, message := range messages {
		pushMessages = append(pushMessages, queuePushMessage{Body: message})
	}
	payload := map[string]any{"messages": pushMessages}
	return c.postJSON(ctx, "/messages/batch", payload, nil)
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
		body, err := decodeMessageBody(msg.Body)
		if err != nil {
			received = append(received, ReceivedMessage{
				LeaseID: msg.LeaseID,
				RawBody: append([]byte(nil), msg.Body...),
			})
			continue
		}
		received = append(received, ReceivedMessage{
			LeaseID: msg.LeaseID,
			Body:    body,
			RawBody: append([]byte(nil), msg.Body...),
		})
	}
	return received, nil
}

func decodeMessageBody(raw json.RawMessage) (Message, error) {
	var body Message
	if err := json.Unmarshal(raw, &body); err == nil {
		return body, nil
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err != nil {
		return Message{}, err
	}
	if err := json.Unmarshal([]byte(asString), &body); err != nil {
		decoded, decodeErr := decodeBase64String(asString)
		if decodeErr != nil {
			return Message{}, err
		}
		if err := json.Unmarshal(decoded, &body); err != nil {
			return Message{}, err
		}
	}
	return body, nil
}

func decodeBase64String(input string) ([]byte, error) {
	if b, err := base64.StdEncoding.DecodeString(input); err == nil {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(input); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(input); err == nil {
		return b, nil
	}
	return base64.RawURLEncoding.DecodeString(input)
}

func (c *CloudflareClient) Ack(ctx context.Context, leaseIDs []string) error {
	if len(leaseIDs) == 0 {
		return nil
	}
	acks := make([]map[string]string, 0, len(leaseIDs))
	for _, leaseID := range leaseIDs {
		acks = append(acks, map[string]string{"lease_id": leaseID})
	}
	return c.postJSON(ctx, "/messages/ack", map[string]any{"acks": acks}, nil)
}

func (c *CloudflareClient) Retry(ctx context.Context, leaseIDs []string) error {
	if len(leaseIDs) == 0 {
		return nil
	}
	retries := make([]map[string]string, 0, len(leaseIDs))
	for _, leaseID := range leaseIDs {
		retries = append(retries, map[string]string{"lease_id": leaseID})
	}
	return c.postJSON(ctx, "/messages/ack", map[string]any{"retries": retries}, nil)
}

func (c *CloudflareClient) AckAndRetry(ctx context.Context, ackLeaseIDs, retryLeaseIDs []string) error {
	payload := map[string]any{}
	if len(ackLeaseIDs) > 0 {
		acks := make([]map[string]string, 0, len(ackLeaseIDs))
		for _, leaseID := range ackLeaseIDs {
			acks = append(acks, map[string]string{"lease_id": leaseID})
		}
		payload["acks"] = acks
	}
	if len(retryLeaseIDs) > 0 {
		retries := make([]map[string]string, 0, len(retryLeaseIDs))
		for _, leaseID := range retryLeaseIDs {
			retries = append(retries, map[string]string{"lease_id": leaseID})
		}
		payload["retries"] = retries
	}
	if len(payload) == 0 {
		return nil
	}
	return c.postJSON(ctx, "/messages/ack", payload, nil)
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
