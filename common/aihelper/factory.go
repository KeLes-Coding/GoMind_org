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

// ModelCreator 定义模型构造函数签名。
type ModelCreator func(ctx context.Context, config map[string]interface{}) (AIModel, error)

// AIModelFactory 负责按照模型类型创建具体模型实例。
type AIModelFactory struct {
	creators map[string]ModelCreator
}

var (
	globalFactory *AIModelFactory
	factoryOnce   sync.Once
)

// GetGlobalFactory 返回全局单例工厂。
func GetGlobalFactory() *AIModelFactory {
	factoryOnce.Do(func() {
		globalFactory = &AIModelFactory{
			creators: make(map[string]ModelCreator),
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

// registerCreators 注册系统内置的模型构造器。
func (f *AIModelFactory) registerCreators() {
	f.creators[ModelTypeOpenAI] = func(ctx context.Context, config map[string]interface{}) (AIModel, error) {
		return NewOpenAIModel(ctx)
	}

	f.creators[ModelTypeRAG] = func(ctx context.Context, config map[string]interface{}) (AIModel, error) {
		userID, ok := config["userID"].(int64)
		if !ok {
			return nil, fmt.Errorf("RAG model requires userID")
		}
		return NewAliRAGModel(ctx, userID)
	}

	f.creators[ModelTypeMCP] = func(ctx context.Context, config map[string]interface{}) (AIModel, error) {
		username, ok := config["username"].(string)
		if !ok {
			return nil, fmt.Errorf("MCP model requires username")
		}
		return NewMCPModel(ctx, username)
	}

	f.creators[ModelTypeOllama] = func(ctx context.Context, config map[string]interface{}) (AIModel, error) {
		baseURL, _ := config["baseURL"].(string)
		modelName, ok := config["modelName"].(string)
		if !ok {
			return nil, fmt.Errorf("Ollama model requires modelName")
		}
		return NewOllamaModel(ctx, baseURL, modelName)
	}
}

// CreateAIModel 根据模型类型创建 AI 模型。
func (f *AIModelFactory) CreateAIModel(ctx context.Context, modelType string, config map[string]interface{}) (AIModel, error) {
	creator, ok := f.creators[modelType]
	if !ok {
		return nil, fmt.Errorf("unsupported model type: %s", modelType)
	}
	return creator(ctx, config)
}

// CreateAIHelper 一键创建带有指定模型的 AIHelper。
func (f *AIModelFactory) CreateAIHelper(ctx context.Context, modelType string, SessionID string, config map[string]interface{}) (*AIHelper, error) {
	model, err := f.CreateAIModel(ctx, modelType, config)
	if err != nil {
		return nil, err
	}

	helper := NewAIHelper(model, SessionID)

	// 这轮先不动 RAG 相关链路，所以这里只给“非 OpenAI 且非 RAG”的模型挂一个轻量备用模型。
	// 这样 MCP / Ollama 下游抖动时，至少还能退回普通聊天，保证主链路可用。
	if modelType != ModelTypeOpenAI && modelType != ModelTypeRAG {
		fallbackModel, fallbackErr := f.CreateAIModel(ctx, ModelTypeOpenAI, config)
		if fallbackErr == nil {
			helper.SetFallbackModel(fallbackModel)
		}
	}

	return helper, nil
}

// RegisterModel 允许后续扩展注册新的模型类型。
func (f *AIModelFactory) RegisterModel(modelType string, creator ModelCreator) {
	f.creators[modelType] = creator
	supportedModelTypes[modelType] = struct{}{}
}
