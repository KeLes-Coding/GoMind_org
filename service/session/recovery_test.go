package session

import (
	"GopherAI/common/aihelper"
	myredis "GopherAI/common/redis"
	"GopherAI/model"
	"context"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

type sessionStubModel struct{}

func (sessionStubModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	return &schema.Message{}, nil
}

func (sessionStubModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb aihelper.StreamCallback) (string, error) {
	return "", nil
}

func (sessionStubModel) GenerateSummary(ctx context.Context, existingSummary string, messages []*schema.Message) (string, error) {
	return existingSummary, nil
}

func (sessionStubModel) GetModelType() string {
	return "stub"
}

func TestGetReusableExecutionHelperMatchesSelection(t *testing.T) {
	manager := aihelper.GetGlobalManager()
	userName := "tester-stage2-match"
	sessionID := "session-stage2-match"
	helper := aihelper.NewAIHelper(sessionStubModel{}, "session-1", "selection-a")
	manager.SetAIHelper(userName, sessionID, helper)
	defer manager.RemoveAIHelper(userName, sessionID)

	got, reused := getReusableExecutionHelper(userName, sessionID, "selection-a")
	if !reused {
		t.Fatal("expected helper to be reused")
	}
	if got != helper {
		t.Fatal("expected reused helper instance to match cached helper")
	}
}

func TestGetReusableExecutionHelperRemovesMismatchedSelection(t *testing.T) {
	manager := aihelper.GetGlobalManager()
	userName := "tester-stage2-mismatch"
	sessionID := "session-stage2-mismatch"
	helper := aihelper.NewAIHelper(sessionStubModel{}, "session-1", "selection-a")
	manager.SetAIHelper(userName, sessionID, helper)

	got, reused := getReusableExecutionHelper(userName, sessionID, "selection-b")
	if reused {
		t.Fatal("expected mismatched helper not to be reused")
	}
	if got != nil {
		t.Fatal("expected nil helper when selection mismatches")
	}
	if _, exists := manager.GetAIHelper(userName, sessionID); exists {
		t.Fatal("expected mismatched helper to be removed from manager")
	}
}

func TestCanWarmResumeFromHotStateAcceptsTrustedRedisState(t *testing.T) {
	sess := &model.Session{
		ID:                  "session-1",
		Version:             6,
		PersistedVersion:    4,
		ContextSummary:      "db summary",
		SummaryMessageCount: 3,
	}
	hotState := &model.SessionHotState{
		SessionID:           "session-1",
		SelectionSignature:  "selection-a",
		Version:             6,
		PersistedVersion:    4,
		ContextSummary:      "redis summary",
		SummaryMessageCount: 3,
		RecentMessagesStart: 2,
		RecentMessages: []model.SessionHotMessage{
			{
				MessageKey: "msg-1",
				SessionID:  "session-1",
				UserName:   "tester",
				Content:    "hello",
				IsUser:     true,
				Status:     string(model.MessageStatusCompleted),
				CreatedAt:  time.Now(),
			},
		},
		FenceToken: 8,
	}
	lease := &myredis.SessionOwnerLease{
		SessionID:  "session-1",
		OwnerID:    "owner-a",
		FenceToken: 8,
	}

	if !canWarmResumeFromHotState(sess, hotState, lease, "selection-a") {
		t.Fatal("expected trusted redis hot state to be accepted")
	}
}

func TestCanWarmResumeFromHotStateRejectsMismatchedSelection(t *testing.T) {
	sess := &model.Session{
		ID:               "session-1",
		Version:          6,
		PersistedVersion: 6,
	}
	hotState := &model.SessionHotState{
		SessionID:           "session-1",
		SelectionSignature:  "selection-a",
		Version:             6,
		SummaryMessageCount: 1,
		RecentMessages: []model.SessionHotMessage{
			{MessageKey: "msg-1", SessionID: "session-1", Content: "hello"},
		},
	}

	if canWarmResumeFromHotState(sess, hotState, nil, "selection-b") {
		t.Fatal("expected mismatched selection signature to be rejected")
	}
}

func TestApplySessionMetadataToHelperIncludesPersistedVersion(t *testing.T) {
	helper := aihelper.NewAIHelper(sessionStubModel{}, "session-1", "selection-a")
	sess := &model.Session{
		ID:                  "session-1",
		Version:             9,
		PersistedVersion:    7,
		ContextSummary:      "summary",
		SummaryMessageCount: 5,
	}

	applySessionMetadataToHelper(sess, helper)

	if helper.GetVersion() != 9 {
		t.Fatalf("expected version 9, got %d", helper.GetVersion())
	}
	if helper.GetPersistedVersion() != 7 {
		t.Fatalf("expected persisted_version 7, got %d", helper.GetPersistedVersion())
	}
}
