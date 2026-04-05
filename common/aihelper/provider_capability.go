package aihelper

// ProviderCapability 描述某个 Provider 当前在后端已经真正落地的能力矩阵。
// 前端后续可以基于这个结果动态渲染模式选择，而不是继续写死固定枚举。
type ProviderCapability struct {
	Provider                 string
	DisplayName              string
	IsImplemented            bool
	SupportedChatModes       []string
	SupportsConfigTest       bool
	SupportsToolCalling      bool
	SupportsEmbedding        bool
	SupportsMultiModalFuture bool
}

var providerCapabilities = map[string]ProviderCapability{
	ProviderOpenAICompatible: {
		Provider:                 ProviderOpenAICompatible,
		DisplayName:              "OpenAI Compatible",
		IsImplemented:            true,
		SupportedChatModes:       []string{ChatModeChat, ChatModeRAG, ChatModeMCP, ChatModeRAGMCP},
		SupportsConfigTest:       true,
		SupportsToolCalling:      true,
		SupportsEmbedding:        true,
		SupportsMultiModalFuture: false,
	},
	ProviderOllama: {
		Provider:                 ProviderOllama,
		DisplayName:              "Ollama",
		IsImplemented:            true,
		SupportedChatModes:       []string{ChatModeChat, ChatModeRAG, ChatModeMCP, ChatModeRAGMCP},
		SupportsConfigTest:       true,
		SupportsToolCalling:      false,
		SupportsEmbedding:        false,
		SupportsMultiModalFuture: false,
	},
	ProviderClaude: {
		Provider:                 ProviderClaude,
		DisplayName:              "Claude",
		IsImplemented:            false,
		SupportedChatModes:       []string{},
		SupportsConfigTest:       false,
		SupportsToolCalling:      false,
		SupportsEmbedding:        false,
		SupportsMultiModalFuture: false,
	},
	ProviderGemini: {
		Provider:                 ProviderGemini,
		DisplayName:              "Gemini",
		IsImplemented:            false,
		SupportedChatModes:       []string{},
		SupportsConfigTest:       false,
		SupportsToolCalling:      false,
		SupportsEmbedding:        false,
		SupportsMultiModalFuture: false,
	},
}

var orderedProviders = []string{
	ProviderOpenAICompatible,
	ProviderClaude,
	ProviderGemini,
	ProviderOllama,
}

var orderedChatModes = []string{
	ChatModeChat,
	ChatModeRAG,
	ChatModeMCP,
	ChatModeRAGMCP,
}

// GetProviderCapability 返回指定 Provider 的能力矩阵。
func GetProviderCapability(provider string) (ProviderCapability, bool) {
	capability, ok := providerCapabilities[provider]
	return capability, ok
}

// ListProviderCapabilities 按固定顺序返回当前系统声明的 Provider 能力矩阵。
func ListProviderCapabilities() []ProviderCapability {
	items := make([]ProviderCapability, 0, len(orderedProviders))
	for _, provider := range orderedProviders {
		if capability, ok := providerCapabilities[provider]; ok {
			items = append(items, capability)
		}
	}
	return items
}

// ListSupportedChatModes 返回系统级 chat_mode 枚举，供接口和前端做统一展示。
func ListSupportedChatModes() []string {
	items := make([]string, 0, len(orderedChatModes))
	items = append(items, orderedChatModes...)
	return items
}

// SupportsChatModeForProvider 判断指定 Provider 当前是否真正支持某个聊天模式。
func SupportsChatModeForProvider(provider string, chatMode string) bool {
	capability, ok := GetProviderCapability(provider)
	if !ok || !capability.IsImplemented {
		return false
	}
	for _, item := range capability.SupportedChatModes {
		if item == chatMode {
			return true
		}
	}
	return false
}

// ResolveProviderModelType 把配置层 Provider 名称映射为当前系统内部仍在使用的底层 modelType。
func ResolveProviderModelType(provider string) (string, bool) {
	switch provider {
	case ProviderOpenAICompatible:
		return ModelTypeOpenAI, true
	case ProviderOllama:
		return ModelTypeOllama, true
	default:
		return "", false
	}
}
