package aihelper

import (
	"context"
	"strings"
	"testing"
	"time"

	"GopherAI/common/mcpgateway"
	"github.com/cloudwego/eino/schema"
)

type fakeChatProvider struct {
	generateResp  *schema.Message
	generateErr   error
	streamResp    string
	streamErr     error
	streamInvoked bool
}

func (f *fakeChatProvider) Generate(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	if f.generateErr != nil {
		return nil, f.generateErr
	}
	return f.generateResp, nil
}

func (f *fakeChatProvider) Stream(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	f.streamInvoked = true
	if f.streamErr != nil {
		return "", f.streamErr
	}
	if cb != nil && f.streamResp != "" {
		cb(f.streamResp)
	}
	return f.streamResp, nil
}

func (f *fakeChatProvider) GenerateSummary(ctx context.Context, existingSummary string, messages []*schema.Message) (string, error) {
	return "", nil
}

func (f *fakeChatProvider) GetModelType() string {
	return "fake"
}

func (f *fakeChatProvider) GetProviderName() string {
	return "fake"
}

func TestMCPChatCapabilityStreamResponseFallsBackToProviderStreamWhenManagerDisabled(t *testing.T) {
	capability := &MCPChatCapability{}
	provider := &fakeChatProvider{
		streamResp: "普通回答",
	}

	var chunks []string
	got, err := capability.StreamResponse(context.Background(), provider, []*schema.Message{
		{Role: schema.User, Content: "你好"},
	}, func(msg string) {
		chunks = append(chunks, msg)
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "普通回答" {
		t.Fatalf("expected fallback content, got %q", got)
	}
	if len(chunks) != 1 || chunks[0] != "普通回答" {
		t.Fatalf("expected callback to receive fallback content, got %#v", chunks)
	}
	if !provider.streamInvoked {
		t.Fatalf("expected provider stream to run when MCP manager is unavailable")
	}
}

func TestRenderToolListIncludesQualifiedNameAndAlias(t *testing.T) {
	capability := &MCPChatCapability{}
	text := capability.renderToolList([]mcpgateway.ToolDefinition{
		{
			ServerName:    "local",
			ToolName:      "get_weather",
			QualifiedName: "local.get_weather",
			AliasName:     "get_weather",
			Description:   "查询天气",
			InputSchema:   `{"type":"object"}`,
		},
	})
	if text == "" {
		t.Fatal("expected rendered tool list to be non-empty")
	}
	if want := "local.get_weather"; !containsAll(text, want, "短名", "查询天气") {
		t.Fatalf("expected rendered tool list to contain qualified name and description, got %q", text)
	}
}

func TestEmitStreamFallbackIgnoresEmptyContent(t *testing.T) {
	capability := &MCPChatCapability{}
	called := false
	capability.emitStreamFallback("", func(msg string) {
		called = true
	})
	if called {
		t.Fatal("expected callback to remain untouched for empty fallback content")
	}
}

func TestAIHelperManagerExpiresExecutionCacheEntry(t *testing.T) {
	baseTime := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	currentTime := baseTime

	manager := NewAIHelperManager()
	manager.helperTTL = 2 * time.Second
	manager.nowFunc = func() time.Time {
		return currentTime
	}

	helper := NewAIHelper(stubAIModel{}, "session-ttl", "selection-ttl")
	manager.SetAIHelper("tester", "session-ttl", helper)

	if _, exists := manager.GetAIHelper("tester", "session-ttl"); !exists {
		t.Fatal("expected helper to exist before ttl expires")
	}

	currentTime = baseTime.Add(3 * time.Second)
	if _, exists := manager.GetAIHelper("tester", "session-ttl"); exists {
		t.Fatal("expected helper to be evicted after ttl expires")
	}
}

func TestAIHelperManagerRefreshesTTLOnAccess(t *testing.T) {
	baseTime := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	currentTime := baseTime

	manager := NewAIHelperManager()
	manager.helperTTL = 2 * time.Second
	manager.nowFunc = func() time.Time {
		return currentTime
	}

	helper := NewAIHelper(stubAIModel{}, "session-refresh", "selection-refresh")
	manager.SetAIHelper("tester", "session-refresh", helper)

	currentTime = baseTime.Add(1500 * time.Millisecond)
	if got, exists := manager.GetAIHelper("tester", "session-refresh"); !exists || got == nil {
		t.Fatal("expected helper to still exist on first refresh")
	}

	currentTime = baseTime.Add(3 * time.Second)
	if got, exists := manager.GetAIHelper("tester", "session-refresh"); !exists || got == nil {
		t.Fatal("expected helper access to refresh ttl")
	}
}

var _ ChatProvider = (*fakeChatProvider)(nil)

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
