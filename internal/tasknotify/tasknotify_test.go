package tasknotify

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"tgdl-bot/internal/service"
	"tgdl-bot/internal/telegram"
)

type fakeClient struct {
	editReq     *telegram.EditMessageTextRequest
	reactionReq *telegram.SetMessageReactionRequest
	editErr     error
	reactionErr error
}

func (f *fakeClient) EditMessageText(_ context.Context, req telegram.EditMessageTextRequest) error {
	f.editReq = &req
	return f.editErr
}

func (f *fakeClient) SetMessageReaction(_ context.Context, req telegram.SetMessageReactionRequest) error {
	f.reactionReq = &req
	return f.reactionErr
}

func TestNotifierNotifySendsEditAndReaction(t *testing.T) {
	sourceID := int64(10)
	statusID := int64(11)

	client := &fakeClient{}
	notifier := Notifier{Client: client}
	task := service.Task{
		TaskID:          "t1",
		ChatID:          100,
		URL:             "https://t.me/c/1/2",
		Status:          service.StatusRunning,
		SourceMessageID: &sourceID,
		StatusMessageID: &statusID,
	}

	if err := notifier.Notify(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.editReq == nil || client.editReq.MessageID != statusID {
		t.Fatalf("expected status message update, got %+v", client.editReq)
	}
	if client.reactionReq == nil || client.reactionReq.MessageID != sourceID {
		t.Fatalf("expected source reaction update, got %+v", client.reactionReq)
	}
	if got := client.reactionReq.Reaction[0].Emoji; got != "⚡" {
		t.Fatalf("expected running emoji, got %q", got)
	}
}

func TestNotifierNotifyReturnsJoinedError(t *testing.T) {
	sourceID := int64(10)
	statusID := int64(11)
	client := &fakeClient{
		editErr:     errors.New("edit failed"),
		reactionErr: errors.New("reaction failed"),
	}
	notifier := Notifier{Client: client}
	task := service.Task{
		TaskID:          "t1",
		ChatID:          100,
		URL:             "https://t.me/c/1/2",
		Status:          service.StatusFailed,
		SourceMessageID: &sourceID,
		StatusMessageID: &statusID,
	}

	err := notifier.Notify(context.Background(), task)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "edit status message") || !strings.Contains(err.Error(), "set source reaction") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFormatTaskStatusMessageIncludesErrorAndTimes(t *testing.T) {
	started := time.Date(2026, 3, 22, 4, 0, 0, 0, time.UTC)
	finished := started.Add(2 * time.Minute)
	msg := FormatTaskStatusMessage(service.Task{
		TaskID:       "t1",
		URL:          "https://t.me/c/1/2",
		Status:       service.StatusFailed,
		RetryCount:   1,
		StartedAt:    &started,
		FinishedAt:   &finished,
		ErrorMessage: ptr("boom"),
	})

	for _, want := range []string{"❌ 任务失败", "Task ID: t1", "重试次数: 1", "原因: boom"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected %q in message: %s", want, msg)
		}
	}
}

func ptr(s string) *string {
	return &s
}
