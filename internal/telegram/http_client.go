package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type HTTPClient struct {
	baseURL    string
	botToken   string
	httpClient *http.Client
}

func NewHTTPClient(baseURL, botToken string, timeout time.Duration) *HTTPClient {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &HTTPClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		botToken:   strings.TrimSpace(botToken),
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *HTTPClient) GetUpdates(ctx context.Context, req GetUpdatesRequest) (GetUpdatesResponse, error) {
	values := url.Values{}
	if req.Offset > 0 {
		values.Set("offset", strconv.FormatInt(req.Offset, 10))
	}
	if req.Limit > 0 {
		values.Set("limit", strconv.Itoa(req.Limit))
	}
	if req.TimeoutSeconds > 0 {
		values.Set("timeout", strconv.Itoa(req.TimeoutSeconds))
	}
	if len(req.AllowedUpdates) > 0 {
		b, _ := json.Marshal(req.AllowedUpdates)
		values.Set("allowed_updates", string(b))
	}

	endpoint := c.methodURL("getUpdates")
	if encoded := values.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return GetUpdatesResponse{}, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return GetUpdatesResponse{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return GetUpdatesResponse{}, err
	}

	var out GetUpdatesResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return GetUpdatesResponse{}, fmt.Errorf("telegram getUpdates decode: %w", err)
	}
	if !out.Ok && out.Error != nil {
		return out, fmt.Errorf("telegram getUpdates api error: %d %s", out.Error.Code, out.Error.Message)
	}
	return out, nil
}

func (c *HTTPClient) SendMessage(ctx context.Context, req SendMessageRequest) (Message, error) {
	payload := map[string]any{
		"chat_id": req.ChatID,
		"text":    req.Text,
	}
	if req.ParseMode != "" {
		payload["parse_mode"] = req.ParseMode
	}
	if req.DisableWebPagePreview {
		payload["disable_web_page_preview"] = true
	}
	if req.ReplyToMessageID != nil {
		payload["reply_to_message_id"] = *req.ReplyToMessageID
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return Message{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.methodURL("sendMessage"), bytes.NewReader(b))
	if err != nil {
		return Message{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return Message{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Message{}, err
	}

	var out struct {
		Ok     bool      `json:"ok"`
		Result Message   `json:"result"`
		Error  *APIError `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return Message{}, fmt.Errorf("telegram sendMessage decode: %w", err)
	}
	if !out.Ok && out.Error != nil {
		return Message{}, fmt.Errorf("telegram sendMessage api error: %d %s", out.Error.Code, out.Error.Message)
	}
	return out.Result, nil
}

func (c *HTTPClient) EditMessageText(ctx context.Context, req EditMessageTextRequest) error {
	payload := map[string]any{
		"chat_id":    req.ChatID,
		"message_id": req.MessageID,
		"text":       req.Text,
	}
	if req.ParseMode != "" {
		payload["parse_mode"] = req.ParseMode
	}
	if req.DisableWebPagePreview {
		payload["disable_web_page_preview"] = true
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.methodURL("editMessageText"), bytes.NewReader(b))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var out struct {
		Ok          bool   `json:"ok"`
		ErrorCode   int    `json:"error_code,omitempty"`
		Description string `json:"description,omitempty"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("telegram editMessageText decode: %w", err)
	}
	if !out.Ok {
		return fmt.Errorf("telegram editMessageText api error: %d %s", out.ErrorCode, out.Description)
	}
	return nil
}

func (c *HTTPClient) SetMessageReaction(ctx context.Context, req SetMessageReactionRequest) error {
	payload := map[string]any{
		"chat_id":    req.ChatID,
		"message_id": req.MessageID,
		"reaction":   req.Reaction,
	}
	if req.IsBig {
		payload["is_big"] = true
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.methodURL("setMessageReaction"), bytes.NewReader(b))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var out struct {
		Ok          bool   `json:"ok"`
		Result      bool   `json:"result"`
		ErrorCode   int    `json:"error_code,omitempty"`
		Description string `json:"description,omitempty"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("telegram setMessageReaction decode: %w", err)
	}
	if !out.Ok {
		return fmt.Errorf("telegram setMessageReaction api error: %d %s", out.ErrorCode, out.Description)
	}
	return nil
}

func (c *HTTPClient) methodURL(method string) string {
	return fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.botToken, method)
}
