package notify

import (
	"GopherAI/common/observability"
	"GopherAI/model"
	"context"
	"testing"
)

func TestPublishChatMessageReadyValidatesParams(t *testing.T) {
	tests := []ChatMessageReadyParams{
		{UserID: 0, SessionID: "s1", MessageKey: "m1", Content: "hello"},
		{UserID: 1, SessionID: "", MessageKey: "m1", Content: "hello"},
		{UserID: 1, SessionID: "s1", MessageKey: "", Content: "hello"},
	}

	for _, tc := range tests {
		if err := PublishChatMessageReady(context.Background(), tc); err == nil {
			t.Fatalf("expected validation error for %+v", tc)
		}
	}
}

func TestPublishChatMessageReadyBuildsNotificationTask(t *testing.T) {
	original := publishTaskFunc
	defer func() {
		publishTaskFunc = original
	}()

	var captured *model.NotificationTask
	publishTaskFunc = func(ctx context.Context, task *model.NotificationTask) error {
		captured = task
		return nil
	}

	longContent := " 第一行\n\n第二行   第三行 " +
		"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"
	err := PublishChatMessageReady(context.Background(), ChatMessageReadyParams{
		UserID:     7,
		SessionID:  "session-1",
		MessageKey: "message-1",
		Content:    longContent,
	})
	if err != nil {
		t.Fatalf("PublishChatMessageReady returned error: %v", err)
	}
	if captured == nil {
		t.Fatal("expected notification task to be published")
	}
	if captured.EventType != model.NotificationEventChatMessageReady {
		t.Fatalf("unexpected event type: %s", captured.EventType)
	}
	if captured.UserID != 7 || captured.SessionID != "session-1" || captured.MessageKey != "message-1" {
		t.Fatalf("unexpected task identity: %+v", captured)
	}
	if captured.Summary == "" {
		t.Fatal("expected summary to be generated")
	}
	if len(captured.Summary) > notificationSummaryLimit {
		t.Fatalf("summary should be truncated to %d chars, got %d", notificationSummaryLimit, len(captured.Summary))
	}
	if captured.Summary[0] == ' ' {
		t.Fatalf("summary should be trimmed, got %q", captured.Summary)
	}
}

func TestPublishChatMessageReadyRecordsPublishFailMetric(t *testing.T) {
	original := publishTaskFunc
	defer func() {
		publishTaskFunc = original
	}()

	before := observability.SnapshotAI()

	publishTaskFunc = func(ctx context.Context, task *model.NotificationTask) error {
		return context.DeadlineExceeded
	}

	err := PublishChatMessageReady(context.Background(), ChatMessageReadyParams{
		UserID:     9,
		SessionID:  "session-9",
		MessageKey: "message-9",
		Content:    "hello",
	})
	if err == nil {
		t.Fatal("expected publish error")
	}

	snapshot := observability.SnapshotAI()
	if snapshot.NotificationPublishFail != before.NotificationPublishFail+1 {
		t.Fatalf("expected notification_publish_fail to increment by 1, before=%d after=%d", before.NotificationPublishFail, snapshot.NotificationPublishFail)
	}
}
