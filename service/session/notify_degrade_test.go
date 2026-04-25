package session

import (
	"GopherAI/model"
	notifyservice "GopherAI/service/notify"
	"context"
	"errors"
	"testing"
)

func TestPublishAssistantReadyNotificationBestEffortPublishesCompletedAssistant(t *testing.T) {
	original := publishChatMessageReadyFunc
	defer func() {
		publishChatMessageReadyFunc = original
	}()

	var captured notifyservice.ChatMessageReadyParams
	called := 0
	publishChatMessageReadyFunc = func(ctx context.Context, params notifyservice.ChatMessageReadyParams) error {
		called++
		captured = params
		return nil
	}

	sess := &model.Session{ID: "session-1", UserID: 101}
	msg := &model.Message{
		MessageKey: "message-1",
		Content:    "assistant done",
		Status:     model.MessageStatusCompleted,
	}

	publishAssistantReadyNotificationBestEffort(context.Background(), sess, msg)

	if called != 1 {
		t.Fatalf("expected publish to be called once, got %d", called)
	}
	if captured.UserID != 101 || captured.SessionID != "session-1" || captured.MessageKey != "message-1" || captured.Content != "assistant done" {
		t.Fatalf("unexpected publish params: %+v", captured)
	}
}

func TestPublishAssistantReadyNotificationBestEffortSkipsNonCompletedAssistant(t *testing.T) {
	original := publishChatMessageReadyFunc
	defer func() {
		publishChatMessageReadyFunc = original
	}()

	called := 0
	publishChatMessageReadyFunc = func(ctx context.Context, params notifyservice.ChatMessageReadyParams) error {
		called++
		return errors.New("should not be called")
	}

	sess := &model.Session{ID: "session-1", UserID: 101}
	publishAssistantReadyNotificationBestEffort(context.Background(), sess, &model.Message{
		MessageKey: "message-user",
		Content:    "user message",
		IsUser:     true,
		Status:     model.MessageStatusCompleted,
	})
	publishAssistantReadyNotificationBestEffort(context.Background(), sess, &model.Message{
		MessageKey: "message-partial",
		Content:    "partial",
		Status:     model.MessageStatusPartial,
	})

	if called != 0 {
		t.Fatalf("expected publish not to be called, got %d", called)
	}
}
