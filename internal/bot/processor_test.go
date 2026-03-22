package bot

import (
	"context"
	"testing"

	"tgdl-bot/internal/service"
	"tgdl-bot/internal/telegram"
)

func TestHandleUpdateBuildsReply(t *testing.T) {
	h := Handler{}
	outcome, err := h.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 99,
			Chat:      telegram.Chat{ID: 10},
			From:      &telegram.User{ID: 20},
			Text:      "/help",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome == nil || outcome.SendRequest == nil || outcome.SendRequest.ChatID != 10 || outcome.SendRequest.Text == "" {
		t.Fatalf("unexpected send request: %+v", outcome)
	}
	if outcome.SendRequest.ReplyToMessageID == nil || *outcome.SendRequest.ReplyToMessageID != 99 {
		t.Fatalf("expected reply_to_message_id=99, got %+v", outcome.SendRequest.ReplyToMessageID)
	}
}

func TestHandleUpdateBuildsReactionForTaskStatus(t *testing.T) {
	h := Handler{
		Tasks: &fakeTaskQuery{task: service.Task{
			TaskID: "existing-task",
			Status: service.StatusFailed,
		}},
		Queue: &fakeQueue{},
	}

	outcome, err := h.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 88,
			Chat:      telegram.Chat{ID: 10},
			From:      &telegram.User{ID: 20},
			Text:      "https://t.me/c/1/2",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome == nil || outcome.ReactionRequest == nil {
		t.Fatalf("expected reaction request, got %+v", outcome)
	}
	if len(outcome.ReactionRequest.Reaction) != 1 || outcome.ReactionRequest.Reaction[0].Emoji != "❌" {
		t.Fatalf("unexpected reaction payload: %+v", outcome.ReactionRequest.Reaction)
	}
}

func TestHandleUpdateCarriesTaskBindingMetadata(t *testing.T) {
	tasks := &fakeTaskQuery{}
	tasks.createFn = func(req service.CreateQueuedTaskRequest) (service.Task, error) {
		return service.Task{
			TaskID:         req.TaskID,
			ChatID:         req.ChatID,
			UserID:         req.UserID,
			TargetChatID:   req.TargetChatID,
			URL:            req.URL,
			IdempotencyKey: req.IdempotencyKey,
			Status:         service.StatusQueued,
		}, nil
	}

	h := Handler{
		Tasks: tasks,
		Queue: &fakeQueue{},
	}

	outcome, err := h.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 77,
			Chat:      telegram.Chat{ID: 10},
			From:      &telegram.User{ID: 20},
			Text:      "https://t.me/c/1/2",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome == nil {
		t.Fatal("expected outcome")
	}
	if outcome.TaskID == "" {
		t.Fatalf("expected task id in outcome, got %+v", outcome)
	}
	if outcome.SourceMessageID != 77 {
		t.Fatalf("expected source message id 77, got %d", outcome.SourceMessageID)
	}
}
