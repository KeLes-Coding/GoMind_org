package aihelper

import (
	"context"
	"fmt"
	"sync"
)

const (
	// ModelTypeOpenAI 保持现有对外协议不变，仍然使用原来的字符串值。
	ModelTypeOpenAI = "1"
	ModelTypeRAG    = "2"
	ModelTypeMCP    = "3"
	ModelTypeOllama = "4"
)

var supportedModelTypes = map[string]struct{}{
	ModelTypeOpenAI: {},
	ModelTypeRAG:    {},
	ModelTypeMCP:    {},
	ModelTypeOllama: {},
}

// ProviderCreator 定义底层 Provider 构造函数签名。
type ProviderCreator func(ctx context.Context, config RuntimeConfig) (ChatProvider, error)

// AIModelFactory 负责按照模型类型创建具体模型实例。
type AIModelFactory struct {
	creators map[string]ProviderCreator
}

var (
	globalFactory *AIModelFactory
	factoryOnce   sync.Once
)

// GetGlobalFactory 返回全局单例工厂。
func GetGlobalFactory() *AIModelFactory {
	factoryOnce.Do(func() {
		globalFactory = &AIModelFactory{
			creators: make(map[string]ProviderCreator),
		}
		globalFactory.registerCreators()
	})
	return globalFactory
}

// IsSupportedModelType 判断当前请求里的 modelType 是否受支持。
func IsSupportedModelType(modelType string) bool {
	_, ok := supportedModelTypes[modelType]
	return ok
}

// registerCreators 注册系统内置的 Provider 构造器。
func (f *AIModelFactory) registerCreators() {
	f.creators[ModelTypeOpenAI] = func(ctx context.Context, config RuntimeConfig) (ChatProvider, error) {
		return NewOpenAIProvider(ctx, config)
	}

	f.creators[ModelTypeOllama] = func(ctx context.Context, config RuntimeConfig) (ChatProvider, error) {
		if config.ModelName == "" {
			return nil, fmt.Errorf("Ollama model requires modelName")
		}
		return NewOllamaProvider(ctx, config)
	}
}

// CreateProvider 根据底层 Provider 类型创建 Provider 实例。
func (f *AIModelFactory) CreateProvider(ctx context.Context, modelType string, config RuntimeConfig) (ChatProvider, error) {
	creator, ok := f.creators[modelType]
	if !ok {
		return nil, fmt.Errorf("unsupported model type: %s", modelType)
	}
	return creator(ctx, config)
}

// CreateAIHelper 一键创建带有指定模型的 AIHelper。
func (f *AIModelFactory) CreateAIHelper(ctx context.Context, modelType string, sessionID string, config RuntimeConfig) (*AIHelper, error) {
	provider, err := f.CreateProvider(ctx, modelType, config)
	if err != nil {
		return nil, err
	}

	capability, err := f.CreateCapability(config)
	if err != nil {
		return nil, err
	}

	model := NewCapabilityModel(provider, capability)
	helper := NewAIHelper(model, sessionID, config.SelectionSignature(modelType))

	// 备用模型依然只绑定到底层 Provider。
	// 这样 Provider 故障时，可以在相同 capability 下回退到默认 OpenAI-compatible。
	if modelType != ModelTypeOpenAI {
		fallbackProvider, fallbackErr := f.CreateProvider(ctx, ModelTypeOpenAI, config)
		if fallbackErr == nil {
			helper.SetFallbackModel(NewCapabilityModel(fallbackProvider, capability))
		}
	}

	return helper, nil
}

// CreateCapability 根据 chat_mode 创建能力编排器。
func (f *AIModelFactory) CreateCapability(config RuntimeConfig) (ChatCapability, error) {
	switch config.ChatMode {
	case ChatModeChat:
		return &PlainChatCapability{}, nil
	case ChatModeRAG:
		if config.UserID == 0 {
			return nil, fmt.Errorf("RAG chat mode requires userID")
		}
		return NewRAGChatCapability(config.UserID), nil
	case ChatModeMCP:
		if config.Username == "" {
			return nil, fmt.Errorf("MCP chat mode requires username")
		}
		return NewMCPChatCapability(config.Username), nil
	default:
		return nil, unsupportedChatModeError(config.ChatMode)
	}
}

// RegisterModel 允许后续扩展注册新的底层 Provider。
func (f *AIModelFactory) RegisterModel(modelType string, creator ProviderCreator) {
	f.creators[modelType] = creator
	supportedModelTypes[modelType] = struct{}{}
}
