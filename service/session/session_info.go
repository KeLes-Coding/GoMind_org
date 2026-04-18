package session

import (
	"GopherAI/common/aihelper"
	llmConfigDAO "GopherAI/dao/llm_config"
	"GopherAI/model"

	"gorm.io/gorm"
)

// buildSessionInfo 把数据库 Session 转成前端展示结构，并尽量补齐绑定的模型配置信息。
// 如果配置已经被删除或暂时不可读，这里只保留 session 侧的基础字段，不把整个接口打失败。
func buildSessionInfo(sess model.Session) model.SessionInfo {
	info := buildSessionInfoWithConfig(sess, nil)
	if sess.LLMConfigID == nil {
		return info
	}

	config, err := llmConfigDAO.GetUserLLMConfigByID(sess.UserID, *sess.LLMConfigID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return info
		}
		return info
	}

	return buildSessionInfoWithConfig(sess, config)
}

func buildSessionInfoWithConfig(sess model.Session, config *model.UserLLMConfig) model.SessionInfo {
	info := model.SessionInfo{
		SessionID:   sess.ID,
		Title:       sess.Title,
		LLMConfigID: sess.LLMConfigID,
		ChatMode:    sess.ChatMode,
	}
	if sess.FolderID != nil {
		info.FolderID = *sess.FolderID
	}

	if config == nil {
		return info
	}

	info.LLMConfigName = config.Name
	info.Provider = config.Provider
	info.Model = config.Model
	if capability, ok := aihelper.GetProviderCapability(config.Provider); ok {
		info.ProviderCapability = &model.SessionProviderCapability{
			Provider:                 capability.Provider,
			DisplayName:              capability.DisplayName,
			IsImplemented:            capability.IsImplemented,
			SupportedChatModes:       append([]string(nil), capability.SupportedChatModes...),
			SupportsConfigTest:       capability.SupportsConfigTest,
			SupportsToolCalling:      capability.SupportsToolCalling,
			SupportsEmbedding:        capability.SupportsEmbedding,
			SupportsMultiModalFuture: capability.SupportsMultiModalFuture,
		}
	}
	return info
}
