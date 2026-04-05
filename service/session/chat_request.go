package session

import (
	"GopherAI/common/aihelper"
	"GopherAI/common/code"
	"GopherAI/config"
	llmConfigDAO "GopherAI/dao/llm_config"
	sessionDAO "GopherAI/dao/session"
	"GopherAI/model"
	"strings"

	"gorm.io/gorm"
)

type ChatRequest struct {
	ModelType   string
	LLMConfigID *int64
	ChatMode    string
}

// resolvedChatRequest 表示把前端请求、会话绑定状态、系统默认值合并后的最终运行时选择。
type resolvedChatRequest struct {
	ModelType     string
	ChatMode      string
	RuntimeConfig aihelper.RuntimeConfig
}

// validateChatRequest 只校验字段值是否合法，不负责补默认值和兼容映射。
func validateChatRequest(req ChatRequest) bool {
	if strings.TrimSpace(req.ModelType) != "" && !aihelper.IsSupportedModelType(req.ModelType) {
		return false
	}
	if strings.TrimSpace(req.ChatMode) != "" && !aihelper.IsSupportedChatMode(req.ChatMode) {
		return false
	}
	return true
}

// resolveChatRequest 把新协议、旧 modelType、会话绑定配置、系统默认配置合并成一次明确的运行时选择。
func resolveChatRequest(userName string, userID int64, req ChatRequest, sess *model.Session) (*resolvedChatRequest, code.Code) {
	if !validateChatRequest(req) {
		return nil, code.CodeInvalidParams
	}

	chatMode := resolveRequestedChatMode(req, sess)
	if !aihelper.IsSupportedChatMode(chatMode) {
		return nil, code.CodeInvalidParams
	}

	runtimeConfig := aihelper.RuntimeConfig{
		Username: userName,
		UserID:   userID,
		ChatMode: chatMode,
	}

	configID := resolveRequestedConfigID(req, sess)
	if configID != nil {
		// 显式指定配置时，优先使用用户选中的数据库配置。
		entity, err := llmConfigDAO.GetUserLLMConfigByID(userID, *configID)
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil, code.CodeRecordNotFound
			}
			return nil, code.CodeServerBusy
		}
		if !entity.IsEnabled {
			return nil, code.CodeInvalidParams
		}
		runtimeConfig.Provider = entity.Provider
		runtimeConfig.APIKey = entity.APIKey
		runtimeConfig.BaseURL = entity.BaseURL
		runtimeConfig.ModelName = entity.Model
		runtimeConfig.LLMConfigID = &entity.ID
	} else if defaultConfig, err := llmConfigDAO.GetDefaultUserLLMConfig(userID); err == nil && defaultConfig.IsEnabled {
		// 没传配置时，优先回退用户默认配置。
		runtimeConfig.Provider = defaultConfig.Provider
		runtimeConfig.APIKey = defaultConfig.APIKey
		runtimeConfig.BaseURL = defaultConfig.BaseURL
		runtimeConfig.ModelName = defaultConfig.Model
		runtimeConfig.LLMConfigID = &defaultConfig.ID
	} else if err != nil && err != gorm.ErrRecordNotFound {
		return nil, code.CodeServerBusy
	}

	fillSystemDefaultRuntimeConfig(&runtimeConfig, req.ModelType)

	modelType, ok := mapChatModeToRuntimeModelType(chatMode, runtimeConfig.Provider)
	if !ok {
		// 这里的失败通常意味着“当前 provider 暂未支持这个能力模式”。
		return nil, code.CodeInvalidParams
	}

	return &resolvedChatRequest{
		ModelType:     modelType,
		ChatMode:      chatMode,
		RuntimeConfig: runtimeConfig,
	}, code.CodeSuccess
}

// resolveRequestedChatMode 的优先级是：本次请求 > 会话当前绑定 > 旧 modelType 兼容映射 > 默认 chat。
func resolveRequestedChatMode(req ChatRequest, sess *model.Session) string {
	if chatMode := strings.TrimSpace(req.ChatMode); chatMode != "" {
		return chatMode
	}
	if sess != nil && strings.TrimSpace(sess.ChatMode) != "" {
		return sess.ChatMode
	}
	if mapped := mapLegacyModelTypeToChatMode(strings.TrimSpace(req.ModelType)); mapped != "" {
		return mapped
	}
	return aihelper.ChatModeChat
}

// resolveRequestedConfigID 的优先级是：本次请求 > 会话当前绑定。
func resolveRequestedConfigID(req ChatRequest, sess *model.Session) *int64 {
	if req.LLMConfigID != nil {
		return req.LLMConfigID
	}
	if sess != nil && sess.LLMConfigID != nil {
		return sess.LLMConfigID
	}
	return nil
}

// fillSystemDefaultRuntimeConfig 用系统级 TOML 补齐运行时缺失字段。
// 它的职责是“兜底”，而不是覆盖用户显式配置。
func fillSystemDefaultRuntimeConfig(runtimeConfig *aihelper.RuntimeConfig, requestedModelType string) {
	if runtimeConfig == nil {
		return
	}

	conf := config.GetConfig()
	if runtimeConfig.Provider == "" {
		runtimeConfig.Provider = aihelper.ProviderOpenAICompatible
		if strings.TrimSpace(requestedModelType) == aihelper.ModelTypeOllama {
			runtimeConfig.Provider = aihelper.ProviderOllama
		}
	}
	if runtimeConfig.APIKey == "" && runtimeConfig.Provider != aihelper.ProviderOllama {
		runtimeConfig.APIKey = strings.TrimSpace(conf.OpenAIConfig.APIKey)
	}
	if runtimeConfig.BaseURL == "" {
		runtimeConfig.BaseURL = strings.TrimSpace(conf.OpenAIConfig.BaseURL)
	}
	if runtimeConfig.ModelName == "" {
		runtimeConfig.ModelName = strings.TrimSpace(conf.OpenAIConfig.ModelName)
	}
}

// mapLegacyModelTypeToChatMode 只用于兼容旧接口，不再代表真正的底层实现类型。
func mapLegacyModelTypeToChatMode(modelType string) string {
	switch modelType {
	case aihelper.ModelTypeOpenAI, aihelper.ModelTypeOllama:
		return aihelper.ChatModeChat
	case aihelper.ModelTypeRAG:
		return aihelper.ChatModeRAG
	case aihelper.ModelTypeMCP:
		return aihelper.ChatModeMCP
	default:
		return ""
	}
}

// mapChatModeToRuntimeModelType 把聊天模式映射到底层 Provider 类型。
// 第一阶段里，RAG/MCP 都已经改为 capability，因此这里统一回到 OpenAI-compatible Provider。
func mapChatModeToRuntimeModelType(chatMode string, provider string) (string, bool) {
	if !aihelper.SupportsChatModeForProvider(provider, chatMode) {
		return "", false
	}
	return aihelper.ResolveProviderModelType(provider)
}

// persistResolvedChatSelection 负责把“本次请求最终生效的配置和模式”回写到 session。
// 这样后续继续聊天时，即使前端不再显式传参，也能复现上次选择。
func persistResolvedChatSelection(sess *model.Session, resolved *resolvedChatRequest) code.Code {
	if sess == nil || resolved == nil {
		return code.CodeSuccess
	}

	needsUpdate := false
	if sess.ChatMode != resolved.ChatMode {
		needsUpdate = true
	}
	if (sess.LLMConfigID == nil) != (resolved.RuntimeConfig.LLMConfigID == nil) {
		needsUpdate = true
	}
	if sess.LLMConfigID != nil && resolved.RuntimeConfig.LLMConfigID != nil && *sess.LLMConfigID != *resolved.RuntimeConfig.LLMConfigID {
		needsUpdate = true
	}
	if !needsUpdate {
		return code.CodeSuccess
	}

	if err := sessionDAO.UpdateSessionChatSelection(sess.ID, resolved.RuntimeConfig.LLMConfigID, resolved.ChatMode); err != nil {
		return code.CodeServerBusy
	}
	sess.ChatMode = resolved.ChatMode
	sess.LLMConfigID = resolved.RuntimeConfig.LLMConfigID
	return code.CodeSuccess
}
