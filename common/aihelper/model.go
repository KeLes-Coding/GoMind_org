package aihelper

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
)

type StreamCallback func(msg *schema.Message)

// AIModel 定义 AIHelper 面向上层暴露的统一模型接口。
// 第一阶段收口后，这个接口的实现不再直接区分 RAG / MCP / 普通聊天，
// 而是统一由“底层 Provider + 上层 Capability”组合生成。
type AIModel interface {
	GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error)
	StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (*schema.Message, error)
	GenerateSummary(ctx context.Context, existingSummary string, messages []*schema.Message) (string, error)
	GetModelType() string
}

// ChatCapability 负责聊天模式编排。
// 它只关心“是否做 RAG / MCP 增强”，不关心底层 SDK 怎么调用。
type ChatCapability interface {
	GenerateResponse(ctx context.Context, provider ChatProvider, messages []*schema.Message) (*schema.Message, error)
	StreamResponse(ctx context.Context, provider ChatProvider, messages []*schema.Message, cb StreamCallback) (*schema.Message, error)
	GetChatMode() string
}

// CapabilityModel 把 Provider 和 Capability 组合成现有 AIHelper 可消费的统一模型对象。
type CapabilityModel struct {
	provider   ChatProvider
	capability ChatCapability
}

func NewCapabilityModel(provider ChatProvider, capability ChatCapability) *CapabilityModel {
	return &CapabilityModel{
		provider:   provider,
		capability: capability,
	}
}

func (m *CapabilityModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	return m.capability.GenerateResponse(ctx, m.provider, messages)
}

func (m *CapabilityModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (*schema.Message, error) {
	return m.capability.StreamResponse(ctx, m.provider, messages, cb)
}

// 摘要依然直接走底层 Provider。
// 这样摘要逻辑不会被 RAG / MCP 编排干扰，行为更加稳定。
func (m *CapabilityModel) GenerateSummary(ctx context.Context, existingSummary string, messages []*schema.Message) (string, error) {
	return m.provider.GenerateSummary(ctx, existingSummary, messages)
}

// GetModelType 继续返回底层 Provider 类型。
// 现有并发控制和观测体系都依赖这个字段，所以这里保持兼容。
func (m *CapabilityModel) GetModelType() string {
	return m.provider.GetModelType()
}

// buildSummaryRequestMessages 构造摘要请求，要求模型把较早历史压缩成稳定、精炼的中文摘要。
func buildSummaryRequestMessages(existingSummary string, messages []*schema.Message) []*schema.Message {
	var conversation strings.Builder
	for _, msg := range messages {
		role := "AI"
		if msg.Role == schema.User {
			role = "用户"
		}
		conversation.WriteString(role)
		conversation.WriteString(": ")
		conversation.WriteString(msg.Content)
		conversation.WriteString("\n")
	}

	prompt := "请把下面这段多轮对话压缩成中文摘要，用于后续继续对话时恢复上下文。" +
		"要求：保留用户目标、约束、关键事实、已做结论、未完成事项；避免寒暄；不要编造信息；输出尽量精炼。\n\n"

	if strings.TrimSpace(existingSummary) != "" {
		prompt += "已有历史摘要如下，请在此基础上合并更新，而不是重复抄写：\n" +
			existingSummary + "\n\n"
	}

	prompt += "需要新增吸收进摘要的对话如下：\n" + conversation.String()

	return []*schema.Message{
		{
			Role:    schema.System,
			Content: "你是一个会话摘要助手。你的任务是把较早历史压缩成稳定、准确、可继续复用的中文摘要。",
		},
		{
			Role:    schema.User,
			Content: prompt,
		},
	}
}

func unsupportedChatModeError(chatMode string) error {
	return fmt.Errorf("unsupported chat mode: %s", chatMode)
}
