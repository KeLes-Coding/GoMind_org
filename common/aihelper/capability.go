package aihelper

import (
	"GopherAI/common/applog"
	"GopherAI/common/mcpgateway"
	"GopherAI/common/observability"
	"GopherAI/common/rag"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// PlainChatCapability 表示普通聊天模式。
type PlainChatCapability struct{}

// GenerateResponse 普通聊天模式不做任何增强，直接透传给底层 Provider。
func (c *PlainChatCapability) GenerateResponse(ctx context.Context, provider ChatProvider, messages []*schema.Message) (*schema.Message, error) {
	return provider.Generate(ctx, messages)
}

// StreamResponse 普通聊天模式下的流式行为同样直接透传。
func (c *PlainChatCapability) StreamResponse(ctx context.Context, provider ChatProvider, messages []*schema.Message, cb StreamCallback) (*schema.Message, error) {
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
func (c *RAGChatCapability) StreamResponse(ctx context.Context, provider ChatProvider, messages []*schema.Message, cb StreamCallback) (*schema.Message, error) {
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
		applog.Userf("RAG fallback: failed to create query user_id=%d err=%v", c.userID, err)
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
		applog.Userf("RAG fallback: retrieve failed user_id=%d query=%q err=%v", c.userID, query, err)
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
	mcpManager *mcpgateway.Manager
}

// RAGMCPChatCapability 表示“先检索增强，再做 MCP 工具编排”的组合模式。
// 这样第三阶段里，RAG 与 MCP 就不再是互斥选择，而是可以复用同一套编排链路组合运行。
type RAGMCPChatCapability struct {
	rag *RAGChatCapability
	mcp *MCPChatCapability
}

// NewMCPChatCapability 创建 MCP 工具编排能力。
// 当前先沿用系统级 MCP server 地址，不把 server 配置放进用户数据库。
func NewMCPChatCapability(username string) *MCPChatCapability {
	return &MCPChatCapability{
		username:   username,
		mcpManager: mcpgateway.GetGlobalManager(),
	}
}

// NewRAGMCPChatCapability 创建 RAG + MCP 组合能力。
func NewRAGMCPChatCapability(userID int64, username string) *RAGMCPChatCapability {
	return &RAGMCPChatCapability{
		rag: NewRAGChatCapability(userID),
		mcp: NewMCPChatCapability(username),
	}
}

// GenerateResponse 保留现有“两段式决策”流程：
// 先判断是否要调工具，再把工具结果回注给模型生成最终回答。
func (c *MCPChatCapability) GenerateResponse(ctx context.Context, provider ChatProvider, messages []*schema.Message) (*schema.Message, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}
	if c.mcpManager == nil || !c.mcpManager.IsEnabled() {
		return provider.Generate(ctx, messages)
	}

	query := messages[len(messages)-1].Content
	tools, err := c.mcpManager.ListTools(ctx)
	if err != nil || len(tools) == 0 {
		if err != nil {
			applog.Categoryf(applog.CategoryMCP, "MCP tools unavailable user=%s err=%v", c.username, err)
		}
		return provider.Generate(ctx, messages)
	}

	firstMessages := c.buildPromptMessages(messages, c.buildFirstPrompt(query, tools))
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

	toolResult, err := c.callMCPTool(ctx, toolCall.ToolName, toolCall.Args)
	if err != nil {
		applog.Categoryf(applog.CategoryMCP, "MCP tool call failed user=%s tool=%s args=%v err=%v", c.username, toolCall.ToolName, toolCall.Args, err)
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
func (c *MCPChatCapability) StreamResponse(ctx context.Context, provider ChatProvider, messages []*schema.Message, cb StreamCallback) (*schema.Message, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}
	if c.mcpManager == nil || !c.mcpManager.IsEnabled() {
		return provider.Stream(ctx, messages, cb)
	}

	query := messages[len(messages)-1].Content
	tools, err := c.mcpManager.ListTools(ctx)
	if err != nil || len(tools) == 0 {
		if err != nil {
			applog.Categoryf(applog.CategoryMCP, "MCP tools unavailable user=%s err=%v", c.username, err)
		}
		return provider.Stream(ctx, messages, cb)
	}

	firstMessages := c.buildPromptMessages(messages, c.buildFirstPrompt(query, tools))
	firstResp, err := provider.Generate(ctx, firstMessages)
	if err != nil {
		return nil, fmt.Errorf("mcp first generate failed: %v", err)
	}

	toolCall, err := c.parseAIResponse(firstResp.Content)
	if err != nil || !toolCall.IsToolCall {
		if err != nil {
			log.Printf("Failed to parse AI response: %v", err)
		}
		c.emitStreamFallback(firstResp.Content, cb)
		return firstResp, nil
	}

	toolResult, err := c.callMCPTool(ctx, toolCall.ToolName, toolCall.Args)
	if err != nil {
		applog.Categoryf(applog.CategoryMCP, "MCP tool call failed user=%s tool=%s args=%v err=%v", c.username, toolCall.ToolName, toolCall.Args, err)
		c.emitStreamFallback(firstResp.Content, cb)
		return firstResp, nil
	}

	secondMessages := c.buildPromptMessages(messages, c.buildSecondPrompt(query, toolCall.ToolName, toolCall.Args, toolResult))
	return provider.Stream(ctx, secondMessages, cb)
}

func (c *MCPChatCapability) GetChatMode() string { return ChatModeMCP }

// GenerateResponse 先尽量做 RAG 增强，再进入 MCP 工具编排。
// 如果检索链路临时不可用，则自动降级为“只走 MCP”，避免把整个请求直接打失败。
func (c *RAGMCPChatCapability) GenerateResponse(ctx context.Context, provider ChatProvider, messages []*schema.Message) (*schema.Message, error) {
	ragMessages, err := c.rag.buildRAGMessages(ctx, messages)
	if err != nil {
		return c.mcp.GenerateResponse(ctx, provider, messages)
	}
	return c.mcp.GenerateResponse(ctx, provider, ragMessages)
}

// StreamResponse 与同步模式保持一致：优先做 RAG 增强，失败时降级为纯 MCP。
func (c *RAGMCPChatCapability) StreamResponse(ctx context.Context, provider ChatProvider, messages []*schema.Message, cb StreamCallback) (*schema.Message, error) {
	ragMessages, err := c.rag.buildRAGMessages(ctx, messages)
	if err != nil {
		return c.mcp.StreamResponse(ctx, provider, messages, cb)
	}
	return c.mcp.StreamResponse(ctx, provider, ragMessages, cb)
}

func (c *RAGMCPChatCapability) GetChatMode() string { return ChatModeRAGMCP }

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

// buildFirstPrompt 要求模型先输出“是否调用工具”的结构化结果。
func (c *MCPChatCapability) buildFirstPrompt(query string, tools []mcpgateway.ToolDefinition) string {
	return fmt.Sprintf(`你是一个智能助手，可以调用MCP工具来获取信息。

可用工具:
%s

重要规则:
1. 如果需要调用工具，必须严格返回以下JSON格式：
{
  "isToolCall": true,
  "toolName": "工具全名或唯一短名",
  "args": {"参数名": "参数值"}
}
2. 如果不需要调用工具，直接返回自然语言回答
3. 仅当工具确实能帮助回答时才调用
4. 如果工具存在同名冲突，优先使用 server.tool 这种全名
5. 不要编造不存在的工具或参数

用户问题: %s

请根据需要调用适当的工具，然后给出综合的回答。`, c.renderToolList(tools), query)
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
	if err := json.Unmarshal([]byte(strings.TrimSpace(response)), &toolCall); err == nil {
		return &toolCall, nil
	}

	if jsonText := extractFirstJSONObject(response); jsonText != "" {
		if err := json.Unmarshal([]byte(jsonText), &toolCall); err == nil {
			return &toolCall, nil
		}
	}

	return &AIToolCall{IsToolCall: false}, nil
}

func extractFirstJSONObject(response string) string {
	trimmed := strings.TrimSpace(response)
	if trimmed == "" {
		return ""
	}

	if strings.Contains(trimmed, "```") {
		parts := strings.Split(trimmed, "```")
		for _, part := range parts {
			block := strings.TrimSpace(part)
			if block == "" {
				continue
			}
			block = strings.TrimPrefix(block, "json")
			block = strings.TrimSpace(block)
			if strings.HasPrefix(block, "{") && strings.HasSuffix(block, "}") {
				return block
			}
		}
	}

	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end > start {
		return strings.TrimSpace(trimmed[start : end+1])
	}
	return ""
}

// callMCPTool 统一封装一次工具调用，并把多段文本结果聚合成字符串。
func (c *MCPChatCapability) callMCPTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	if c.mcpManager == nil {
		return "", fmt.Errorf("mcp manager is nil")
	}
	return c.mcpManager.CallTool(ctx, toolName, args)
}

func (c *MCPChatCapability) emitStreamFallback(content string, cb StreamCallback) {
	if cb == nil || content == "" {
		return
	}
	cb(&schema.Message{
		Role:    schema.Assistant,
		Content: content,
	})
}

func (c *MCPChatCapability) renderToolList(tools []mcpgateway.ToolDefinition) string {
	if len(tools) == 0 {
		return "- 当前没有可用工具"
	}

	lines := make([]string, 0, len(tools))
	sort.Slice(tools, func(i, j int) bool {
		if tools[i].ServerName == tools[j].ServerName {
			return tools[i].QualifiedName < tools[j].QualifiedName
		}
		return tools[i].ServerName < tools[j].ServerName
	})

	for _, tool := range tools {
		displayName := tool.QualifiedName
		if tool.AliasName != "" {
			displayName = fmt.Sprintf("%s（短名: %s）", tool.QualifiedName, tool.AliasName)
		}
		line := fmt.Sprintf("- %s: %s", displayName, strings.TrimSpace(tool.Description))
		if schema := strings.TrimSpace(tool.InputSchema); schema != "" {
			line += fmt.Sprintf("；参数Schema: %s", schema)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
