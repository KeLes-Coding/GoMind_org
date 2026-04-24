package aihelper

import (
	"GopherAI/model"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

type stubAIModel struct{}

func (stubAIModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	return &schema.Message{}, nil
}

func (stubAIModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	return "", nil
}

func (stubAIModel) GenerateSummary(ctx context.Context, existingSummary string, messages []*schema.Message) (string, error) {
	return existingSummary, nil
}

func (stubAIModel) GetModelType() string {
	return "stub"
}

func TestExportHotStateIncludesPersistedVersion(t *testing.T) {
	helper := NewAIHelper(stubAIModel{}, "session-1", "selection-1")
	helper.SetVersion(6)
	helper.SetPersistedVersion(4)
	helper.SetSummaryState("摘要", 8)
	helper.AddMessage("用户消息", "tester", true, false)
	helper.AddMessage("助手消息", "tester", false, false)

	state := helper.ExportHotState()
	if state.PersistedVersion != 4 {
		t.Fatalf("expected persisted_version 4, got %d", state.PersistedVersion)
	}
	if state.Version != 6 {
		t.Fatalf("expected version 6, got %d", state.Version)
	}
	if len(state.RecentMessages) != 2 {
		t.Fatalf("expected 2 recent messages, got %d", len(state.RecentMessages))
	}
}

func TestLoadHotStateRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	helper := NewAIHelper(stubAIModel{}, "session-1", "selection-1")

	helper.LoadHotState(&model.SessionHotState{
		SessionID:           "session-1",
		SelectionSignature:  "selection-1",
		Version:             7,
		PersistedVersion:    5,
		ContextSummary:      "历史摘要",
		SummaryMessageCount: 10,
		RecentMessagesStart: 6,
		RecentMessages: []model.SessionHotMessage{
			{
				ID:         1,
				MessageKey: "msg-1",
				SessionID:  "session-1",
				UserName:   "tester",
				Content:    "hello",
				IsUser:     true,
				Status:     string(model.MessageStatusCompleted),
				CreatedAt:  now,
			},
		},
	})

	exported := helper.ExportHotState()
	if exported.PersistedVersion != 5 {
		t.Fatalf("expected persisted_version 5, got %d", exported.PersistedVersion)
	}
	if exported.Version != 7 {
		t.Fatalf("expected version 7, got %d", exported.Version)
	}
	if exported.SummaryMessageCount != 10 {
		t.Fatalf("expected summary_message_count 10, got %d", exported.SummaryMessageCount)
	}
	if exported.RecentMessagesStart != 6 {
		t.Fatalf("expected recent_messages_start 6, got %d", exported.RecentMessagesStart)
	}
	if len(exported.RecentMessages) != 1 {
		t.Fatalf("expected 1 recent message, got %d", len(exported.RecentMessages))
	}
}

func TestSessionHotStateBackwardCompatibility(t *testing.T) {
	legacyPayload := `{
		"session_id":"session-legacy",
		"selection_signature":"selection-legacy",
		"version":3,
		"updated_at":"2026-04-23T00:00:00Z",
		"context_summary":"旧摘要",
		"summary_message_count":2,
		"recent_messages_start":0,
		"recent_messages":[]
	}`

	var state model.SessionHotState
	if err := json.Unmarshal([]byte(legacyPayload), &state); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if state.PersistedVersion != 0 {
		t.Fatalf("expected default persisted_version 0, got %d", state.PersistedVersion)
	}
	if state.PendingPersist {
		t.Fatal("expected pending_persist default false")
	}
	if state.HotStateDirty {
		t.Fatal("expected hot_state_dirty default false")
	}
}

func TestGenerateResponseForPreparedUserMessageAppendsAssistantOnlyOnce(t *testing.T) {
	helper := NewAIHelper(stubAIModel{}, "session-2", "selection-2")
	saveCalls := 0
	helper.SetSaveFunc(func(message *model.Message) (*model.Message, error) {
		saveCalls++
		return message, nil
	})
	helper.AddMessage("用户先写入的问题", "tester", true, false)

	resp, err := helper.GenerateResponseForPreparedUserMessage("tester", context.Background())
	if err != nil {
		t.Fatalf("unexpected generate error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	messages := helper.GetMessages()
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages after prepared generate, got %d", len(messages))
	}
	if !messages[0].IsUser {
		t.Fatal("expected first message to remain user message")
	}
	if messages[1].IsUser {
		t.Fatal("expected second message to be assistant message")
	}
	if saveCalls != 0 {
		t.Fatalf("expected prepared generate path not to invoke saveFunc, got %d calls", saveCalls)
	}
}
