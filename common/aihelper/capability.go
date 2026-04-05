package aihelper

import (
	"GopherAI/common/observability"
	"GopherAI/common/rag"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// PlainChatCapability 表示普通聊天模式。
type PlainChatCapability struct{}

// GenerateResponse 普通聊天模式不做任何增强，直接透传给底层 Provider。
func (c *PlainChatCapability) GenerateResponse(ctx context.Context, provider ChatProvider, messages []*schema.Message) (*schema.Message, error) {
	return provider.Generate(ctx, messages)
}

// StreamResponse 普通聊天模式下的流式行为同样直接透传。
func (c *PlainChatCapability) StreamResponse(ctx context.Context, provider ChatProvider, messages []*schema.Message, cb StreamCallback) (string, error) {
	return provider.Stream(ctx, messages, cb)
}

func (c *PlainChatCapability) GetChatMode() string { return ChatModeChat }

// RAGChatCapability 负责检索增强。
// 它会在最后一条用户消息上叠加检索上下文，然后再交给底层 Provider。
type RAGChatCapability struct {
	userID int64
}

// NewRAGChatCapability 创建基于用户维度的检索增强能力。
// 这里保留 userID，是为了继续复用现有“按用户文档空间检索”的实现。
func NewRAGChatCapability(userID int64) *RAGChatCapability {
	return &RAGChatCapability{userID: userID}
}

// GenerateResponse 先尝试构造 RAG 增强消息；如果检索链路不可用，则自动回退普通聊天。
func (c *RAGChatCapability) GenerateResponse(ctx context.Context, provider ChatProvider, messages []*schema.Message) (*schema.Message, error) {
	ragMessages, err := c.buildRAGMessages(ctx, messages)
	if err != nil {
		return provider.Generate(ctx, messages)
	}
	return provider.Generate(ctx, ragMessages)
}

// StreamResponse 与同步模式保持同一套检索增强和回退口径。
func (c *RAGChatCapability) StreamResponse(ctx context.Context, provider ChatProvider, messages []*schema.Message, cb StreamCallback) (string, error) {
	ragMessages, err := c.buildRAGMessages(ctx, messages)
	if err != nil {
		return provider.Stream(ctx, messages, cb)
	}
	return provider.Stream(ctx, ragMessages, cb)
}

func (c *RAGChatCapability) GetChatMode() string { return ChatModeRAG }

// buildRAGMessages 只改写最后一条用户消息，避免破坏已有的多轮上下文结构。
func (c *RAGChatCapability) buildRAGMessages(ctx context.Context, messages []*schema.Message) ([]*schema.Message, error) {
	ragQuery, err := rag.NewRAGQuery(ctx, c.userID)
	if err != nil {
		log.Printf("Failed to create RAG query (user may not have uploaded file): %v", err)
		observability.RecordRAGFallback()
		return nil, err
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	lastMessage := messages[len(messages)-1]
	query := lastMessage.Content
	docs, err := ragQuery.RetrieveDocuments(ctx, query)
	if err != nil {
		log.Printf("Failed to retrieve documents: %v", err)
		observability.RecordRAGFallback()
		return nil, err
	}

	ragPrompt := rag.BuildRAGPromptWithReferences(query, docs)
	ragMessages := make([]*schema.Message, len(messages))
	copy(ragMessages, messages)
	ragMessages[len(ragMessages)-1] = &schema.Message{
		Role:    schema.User,
		Content: ragPrompt,
	}
	return ragMessages, nil
}

// MCPChatCapability 负责 MCP 工具编排。
// 它保留现有“两段式提示词 + 工具结果回注”流程，但不再自己持有 LLM。
type MCPChatCapability struct {
	username   string
	mcpBaseURL string
	mcpClient  *client.Client
}

// NewMCPChatCapability 创建 MCP 工具编排能力。
// 当前先沿用系统级 MCP server 地址，不把 server 配置放进用户数据库。
func NewMCPChatCapability(username string) *MCPChatCapability {
	return &MCPChatCapability{
		username:   username,
		mcpBaseURL: "http://localhost:8081/mcp",
	}
}

// GenerateResponse 保留现有“两段式决策”流程：
// 先判断是否要调工具，再把工具结果回注给模型生成最终回答。
func (c *MCPChatCapability) GenerateResponse(ctx context.Context, provider ChatProvider, messages []*schema.Message) (*schema.Message, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	query := messages[len(messages)-1].Content
	firstMessages := c.buildPromptMessages(messages, c.buildFirstPrompt(query))
	firstResp, err := provider.Generate(ctx, firstMessages)
	if err != nil {
		return nil, fmt.Errorf("mcp first generate failed: %v", err)
	}

	toolCall, err := c.parseAIResponse(firstResp.Content)
	if err != nil || !toolCall.IsToolCall {
		if err != nil {
			log.Printf("Failed to parse AI response: %v", err)
		}
		return firstResp, nil
	}

	mcpClient, err := c.getMCPClient(ctx)
	if err != nil {
		log.Printf("MCP client error: %v", err)
		return firstResp, nil
	}

	toolResult, err := c.callMCPTool(ctx, mcpClient, toolCall.ToolName, toolCall.Args)
	if err != nil {
		log.Printf("MCP tool call failed: %v", err)
		return firstResp, nil
	}

	secondMessages := c.buildPromptMessages(messages, c.buildSecondPrompt(query, toolCall.ToolName, toolCall.Args, toolResult))
	finalResp, err := provider.Generate(ctx, secondMessages)
	if err != nil {
		return nil, fmt.Errorf("mcp second generate failed: %v", err)
	}
	return finalResp, nil
}

// StreamResponse 的首轮工具决策仍走同步调用，只有最终回答阶段走流式输出。
func (c *MCPChatCapability) StreamResponse(ctx context.Context, provider ChatProvider, messages []*schema.Message, cb StreamCallback) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("no messages provided")
	}

	query := messages[len(messages)-1].Content
	firstMessages := c.buildPromptMessages(messages, c.buildFirstPrompt(query))
	firstResp, err := provider.Generate(ctx, firstMessages)
	if err != nil {
		return "", fmt.Errorf("mcp first generate failed: %v", err)
	}

	toolCall, err := c.parseAIResponse(firstResp.Content)
	if err != nil || !toolCall.IsToolCall {
		if err != nil {
			log.Printf("Failed to parse AI response: %v", err)
		}
		return firstResp.Content, nil
	}

	mcpClient, err := c.getMCPClient(ctx)
	if err != nil {
		log.Printf("MCP client error: %v", err)
		return firstResp.Content, nil
	}

	toolResult, err := c.callMCPTool(ctx, mcpClient, toolCall.ToolName, toolCall.Args)
	if err != nil {
		log.Printf("MCP tool call failed: %v", err)
		return firstResp.Content, nil
	}

	secondMessages := c.buildPromptMessages(messages, c.buildSecondPrompt(query, toolCall.ToolName, toolCall.Args, toolResult))
	return provider.Stream(ctx, secondMessages, cb)
}

func (c *MCPChatCapability) GetChatMode() string { return ChatModeMCP }

// buildPromptMessages 用“替换最后一条用户消息”的方式构造编排提示词，
// 这样前面的多轮上下文和摘要消息都能保持不变。
func (c *MCPChatCapability) buildPromptMessages(messages []*schema.Message, prompt string) []*schema.Message {
	promptMessages := make([]*schema.Message, len(messages))
	copy(promptMessages, messages)
	promptMessages[len(promptMessages)-1] = &schema.Message{
		Role:    schema.User,
		Content: prompt,
	}
	return promptMessages
}

// getMCPClient 惰性初始化 MCP 客户端。
// 这样普通聊天请求不会无谓地建立 MCP 连接。
func (c *MCPChatCapability) getMCPClient(ctx context.Context) (*client.Client, error) {
	if c.mcpClient == nil {
		httpTransport, err := transport.NewStreamableHTTP(c.mcpBaseURL)
		if err != nil {
			return nil, fmt.Errorf("create mcp transport failed: %v", err)
		}

		c.mcpClient = client.NewClient(httpTransport)
		initRequest := mcp.InitializeRequest{}
		initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initRequest.Params.ClientInfo = mcp.Implementation{
			Name:    "MCP-Go AIHelper Client",
			Version: "1.0.0",
		}
		initRequest.Params.Capabilities = mcp.ClientCapabilities{}

		if _, err := c.mcpClient.Initialize(ctx, initRequest); err != nil {
			return nil, fmt.Errorf("mcp client initialize failed: %v", err)
		}
	}
	return c.mcpClient, nil
}

// buildFirstPrompt 要求模型先输出“是否调用工具”的结构化结果。
func (c *MCPChatCapability) buildFirstPrompt(query string) string {
	return fmt.Sprintf(`你是一个智能助手，可以调用MCP工具来获取信息。

可用工具:
- get_weather: 获取指定城市的天气信息，参数: city（城市名称，支持中文和英文，如北京、Shanghai等）

重要规则:
1. 如果需要调用工具，必须严格返回以下JSON格式：
{
  "isToolCall": true,
  "toolName": "工具名称",
  "args": {"参数名": "参数值"}
}
2. 如果不需要调用工具，直接返回自然语言回答
3. 请根据用户问题决定是否需要调用工具

用户问题: %s

请根据需要调用适当的工具，然后给出综合的回答。`, query)
}

// buildSecondPrompt 在工具执行后，把结果重新交给模型组织最终自然语言回答。
func (c *MCPChatCapability) buildSecondPrompt(query, toolName string, args map[string]interface{}, toolResult string) string {
	return fmt.Sprintf(`你是一个智能助手，可以调用MCP工具来获取信息。

工具执行结果:
工具名称: %s
工具参数: %v
工具结果: %s

用户问题: %s

请根据工具结果和用户问题，给出最终的综合回答。`, toolName, args, toolResult, query)
}

// AIToolCall 表示第一轮模型输出的工具决策结构。
type AIToolCall struct {
	IsToolCall bool                   `json:"isToolCall"`
	ToolName   string                 `json:"toolName"`
	Args       map[string]interface{} `json:"args"`
}

// parseAIResponse 先按 JSON 解析；如果模型没完全遵守格式，再做一次轻量关键词兜底。
func (c *MCPChatCapability) parseAIResponse(response string) (*AIToolCall, error) {
	var toolCall AIToolCall
	if err := json.Unmarshal([]byte(response), &toolCall); err == nil {
		return &toolCall, nil
	}

	if strings.Contains(response, "get_weather") {
		city := c.extractCityFromResponse(response)
		if city != "" {
			return &AIToolCall{
				IsToolCall: true,
				ToolName:   "get_weather",
				Args:       map[string]interface{}{"city": city},
			}, nil
		}
	}

	return &AIToolCall{IsToolCall: false}, nil
}

// callMCPTool 统一封装一次工具调用，并把多段文本结果聚合成字符串。
func (c *MCPChatCapability) callMCPTool(ctx context.Context, client *client.Client, toolName string, args map[string]interface{}) (string, error) {
	callToolRequest := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: args,
		},
	}

	result, err := client.CallTool(ctx, callToolRequest)
	if err != nil {
		return "", fmt.Errorf("mcp tool call failed: %v", err)
	}

	var text string
	for _, content := range result.Content {
		if textContent, ok := content.(mcp.TextContent); ok {
			text += textContent.Text + "\n"
		}
	}

	return text, nil
}

// extractCityFromResponse 是当前 demo 级 weather 工具的参数提取兜底逻辑。
func (c *MCPChatCapability) extractCityFromResponse(response string) string {
	var toolCall AIToolCall
	if err := json.Unmarshal([]byte(response), &toolCall); err == nil {
		if args, ok := toolCall.Args["city"].(string); ok {
			return args
		}
	}

	return ""
}
