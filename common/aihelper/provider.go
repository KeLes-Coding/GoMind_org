package aihelper

import (
	"GopherAI/config"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// ChatProvider 负责底层模型 SDK / API 调用。
// 它不参与 RAG 检索和 MCP 工具编排，只负责真正的模型请求。
type ChatProvider interface {
	Generate(ctx context.Context, messages []*schema.Message) (*schema.Message, error)
	Stream(ctx context.Context, messages []*schema.Message, cb StreamCallback) (*schema.Message, error)
	GenerateSummary(ctx context.Context, existingSummary string, messages []*schema.Message) (string, error)
	GetModelType() string
	GetProviderName() string
}

// BaseChatProvider 复用底层通用聊天模型的基础能力。
// 这样 OpenAI-compatible 和 Ollama 可以共享摘要与流式读写逻辑。
type BaseChatProvider struct {
	llm          einomodel.ToolCallingChatModel
	modelType    string
	providerName string
}

// NewOpenAIProvider 创建 OpenAI-compatible Provider。
// 当前系统里的 DeepSeek / 自建兼容网关都统一落到这条链路上。
func NewOpenAIProvider(ctx context.Context, runtimeConfig RuntimeConfig) (*BaseChatProvider, error) {
	key := resolveOpenAIAPIKey(runtimeConfig)
	modelName := resolveOpenAIModelName(runtimeConfig)
	baseURL := resolveOpenAIBaseURL(runtimeConfig)

	llm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: baseURL,
		Model:   modelName,
		APIKey:  key,
	})
	if err != nil {
		return nil, fmt.Errorf("create openai provider failed: %v", err)
	}

	return &BaseChatProvider{
		llm:          llm,
		modelType:    ModelTypeOpenAI,
		providerName: ProviderOpenAICompatible,
	}, nil
}

// NewOllamaProvider 创建本地 Ollama Provider。
// 这里仍然复用同一套 BaseChatProvider，只是底层 SDK 不同。
func NewOllamaProvider(ctx context.Context, runtimeConfig RuntimeConfig) (*BaseChatProvider, error) {
	baseURL := resolveOllamaBaseURL(runtimeConfig)
	modelName := resolveOllamaModelName(runtimeConfig)

	llm, err := ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
		BaseURL: baseURL,
		Model:   modelName,
	})
	if err != nil {
		return nil, fmt.Errorf("create ollama provider failed: %v", err)
	}

	return &BaseChatProvider{
		llm:          llm,
		modelType:    ModelTypeOllama,
		providerName: ProviderOllama,
	}, nil
}

// Generate 直接调用底层 SDK 的同步生成接口。
func (p *BaseChatProvider) Generate(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	resp, err := p.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("%s generate failed: %v", p.providerName, err)
	}
	return resp, nil
}

// Stream 负责把底层流式响应聚合成完整文本，同时逐段回调给上层。
func (p *BaseChatProvider) Stream(ctx context.Context, messages []*schema.Message, cb StreamCallback) (*schema.Message, error) {
	stream, err := p.llm.Stream(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("%s stream failed: %v", p.providerName, err)
	}
	defer stream.Close()

	var fullResp strings.Builder
	var fullReasoning strings.Builder
	var responseMeta *schema.ResponseMeta
	extra := make(map[string]any)
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s stream recv failed: %v", p.providerName, err)
		}
		if msg == nil {
			continue
		}
		if msg.Content != "" {
			fullResp.WriteString(msg.Content)
		}
		if msg.ReasoningContent != "" {
			fullReasoning.WriteString(msg.ReasoningContent)
		}
		if msg.ResponseMeta != nil {
			responseMeta = msg.ResponseMeta
		}
		for key, value := range msg.Extra {
			extra[key] = value
		}
		if cb != nil {
			cb(msg)
		}
	}

	finalExtra := extra
	if len(finalExtra) == 0 {
		finalExtra = nil
	}
	return &schema.Message{
		Role:             schema.Assistant,
		Content:          fullResp.String(),
		ReasoningContent: fullReasoning.String(),
		ResponseMeta:     responseMeta,
		Extra:            finalExtra,
	}, nil
}

// GenerateSummary 始终只走底层 Provider。
// 这样摘要刷新不会混入 RAG / MCP 的能力编排，结果更稳定。
func (p *BaseChatProvider) GenerateSummary(ctx context.Context, existingSummary string, messages []*schema.Message) (string, error) {
	if len(messages) == 0 {
		return strings.TrimSpace(existingSummary), nil
	}

	resp, err := p.llm.Generate(ctx, buildSummaryRequestMessages(existingSummary, messages))
	if err != nil {
		return "", fmt.Errorf("generate summary failed: %v", err)
	}

	return strings.TrimSpace(resp.Content), nil
}

// GetModelType 返回当前 Provider 在现有系统中的“底层模型类型”标识。
// 这里继续兼容旧的并发控制和观测统计维度。
func (p *BaseChatProvider) GetModelType() string {
	return p.modelType
}

// GetProviderName 返回配置层使用的 Provider 名称。
func (p *BaseChatProvider) GetProviderName() string {
	return p.providerName
}

// resolveOpenAIAPIKey 优先使用运行时配置，其次回退到系统默认配置和环境变量。
func resolveOpenAIAPIKey(runtimeConfig RuntimeConfig) string {
	if key := strings.TrimSpace(runtimeConfig.APIKey); key != "" {
		return key
	}

	cfg := config.GetConfig()
	if key := strings.TrimSpace(cfg.OpenAIConfig.APIKey); key != "" {
		return key
	}
	return os.Getenv("OPENAI_API_KEY")
}

// resolveOpenAIModelName 用于解析 OpenAI-compatible Provider 的模型名。
func resolveOpenAIModelName(runtimeConfig RuntimeConfig) string {
	if modelName := strings.TrimSpace(runtimeConfig.ModelName); modelName != "" {
		return modelName
	}

	cfg := config.GetConfig()
	if modelName := strings.TrimSpace(cfg.OpenAIConfig.ModelName); modelName != "" {
		return modelName
	}
	return os.Getenv("OPENAI_MODEL_NAME")
}

// resolveOpenAIBaseURL 用于解析 OpenAI-compatible Provider 的请求入口地址。
func resolveOpenAIBaseURL(runtimeConfig RuntimeConfig) string {
	if baseURL := strings.TrimSpace(runtimeConfig.BaseURL); baseURL != "" {
		return baseURL
	}

	cfg := config.GetConfig()
	if baseURL := strings.TrimSpace(cfg.OpenAIConfig.BaseURL); baseURL != "" {
		return baseURL
	}
	return os.Getenv("OPENAI_BASE_URL")
}

// resolveOllamaBaseURL 允许 Ollama 单独覆盖地址；未显式配置时继续复用通用 baseURL。
func resolveOllamaBaseURL(runtimeConfig RuntimeConfig) string {
	if baseURL := strings.TrimSpace(runtimeConfig.BaseURL); baseURL != "" {
		return baseURL
	}
	return resolveOpenAIBaseURL(runtimeConfig)
}

// resolveOllamaModelName 允许 Ollama 单独覆盖模型名；未显式配置时复用通用模型名。
func resolveOllamaModelName(runtimeConfig RuntimeConfig) string {
	if modelName := strings.TrimSpace(runtimeConfig.ModelName); modelName != "" {
		return modelName
	}
	return resolveOpenAIModelName(runtimeConfig)
}
