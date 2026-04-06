package aihelper

import (
	"context"
	"testing"

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

func TestMCPChatCapabilityStreamResponseEmitsFallbackWhenNoToolCall(t *testing.T) {
	capability := &MCPChatCapability{}
	provider := &fakeChatProvider{
		generateResp: &schema.Message{Content: "普通回答"},
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
	if provider.streamInvoked {
		t.Fatalf("expected provider stream to be skipped when no tool call")
	}
}

func TestMCPChatCapabilityStreamResponseEmitsFallbackWhenToolCallFails(t *testing.T) {
	capability := &MCPChatCapability{
		mcpBaseURL: "http://127.0.0.1:1/mcp",
	}
	provider := &fakeChatProvider{
		generateResp: &schema.Message{Content: `{"isToolCall":true,"toolName":"get_weather","args":{"city":"佛山"}}`},
	}

	var chunks []string
	got, err := capability.StreamResponse(context.Background(), provider, []*schema.Message{
		{Role: schema.User, Content: "佛山天气怎么样"},
	}, func(msg string) {
		chunks = append(chunks, msg)
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got == "" {
		t.Fatalf("expected fallback content, got empty string")
	}
	if len(chunks) != 1 || chunks[0] != got {
		t.Fatalf("expected callback to receive fallback content, got %#v want %q", chunks, got)
	}
	if provider.streamInvoked {
		t.Fatalf("expected provider stream not to run when MCP setup fails")
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

var _ ChatProvider = (*fakeChatProvider)(nil)
