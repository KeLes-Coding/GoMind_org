package aihelper

import (
	"fmt"
	"strings"
)

const (
	ProviderOpenAICompatible = "openai_compatible"
	ProviderClaude           = "claude"
	ProviderGemini           = "gemini"
	ProviderOllama           = "ollama"

	ChatModeChat     = "chat"
	ChatModeRAG      = "chat_rag"
	ChatModeMCP      = "chat_mcp"
	ChatModeRAGMCP   = "chat_rag_mcp"
	SourceTypeUser   = "user"
	SourceTypeSystem = "system"
)

var supportedProviders = map[string]struct{}{
	ProviderOpenAICompatible: {},
	ProviderClaude:           {},
	ProviderGemini:           {},
	ProviderOllama:           {},
}

var supportedChatModes = map[string]struct{}{
	ChatModeChat:   {},
	ChatModeRAG:    {},
	ChatModeMCP:    {},
	ChatModeRAGMCP: {},
}

type RuntimeConfig struct {
	Provider    string
	APIKey      string
	BaseURL     string
	ModelName   string
	Username    string
	UserID      int64
	LLMConfigID *int64
	ChatMode    string
}

// IsSupportedProvider 判断 provider 是否在当前系统允许的范围内。
func IsSupportedProvider(provider string) bool {
	_, ok := supportedProviders[strings.TrimSpace(provider)]
	return ok
}

// IsSupportedChatMode 判断 chat_mode 是否在当前系统允许的范围内。
func IsSupportedChatMode(chatMode string) bool {
	_, ok := supportedChatModes[strings.TrimSpace(chatMode)]
	return ok
}

// SelectionSignature 把会话当前的底层 Provider 选择和能力模式拼成一个稳定签名。
// 当 session 中途切换配置或模式时，AIHelperManager 会用它判断是否必须重建 helper。
func (c RuntimeConfig) SelectionSignature(modelType string) string {
	configID := "system"
	if c.LLMConfigID != nil {
		configID = fmt.Sprintf("%d", *c.LLMConfigID)
	}

	return strings.Join([]string{
		modelType,
		c.ChatMode,
		c.Provider,
		c.BaseURL,
		c.ModelName,
		c.APIKey,
		configID,
	}, "|")
}
