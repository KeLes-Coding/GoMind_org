package llmconfig

import (
	"GopherAI/common/aihelper"
	"GopherAI/common/code"
	llmConfigDAO "GopherAI/dao/llm_config"
	"GopherAI/model"
	"context"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
)

type CreateConfigInput struct {
	Name      string
	Provider  string
	APIKey    string
	BaseURL   string
	Model     string
	IsDefault bool
}

// UpdateConfigInput 用于承接更新配置时的可变字段。
// IsEnabled 使用指针，是为了区分“显式传 false”和“本次不修改”。
type UpdateConfigInput struct {
	Name      string
	Provider  string
	APIKey    string
	BaseURL   string
	Model     string
	IsDefault bool
	IsEnabled *bool
}

// TestConfigInput 用于测试一组尚未保存或已保存的配置是否可用。
// 它不要求一定先落库，便于前端在保存前先做一次快速连通性验证。
type TestConfigInput struct {
	Provider string
	APIKey   string
	BaseURL  string
	Model    string
}

// ProviderCapabilityItem 表示返回给 controller 的 Provider 能力矩阵项。
// 这里单独定义 service 侧结构，避免 controller 直接依赖底层 aihelper 细节。
type ProviderCapabilityItem struct {
	Provider                 string
	DisplayName              string
	IsImplemented            bool
	SupportedChatModes       []string
	SupportsConfigTest       bool
	SupportsToolCalling      bool
	SupportsEmbedding        bool
	SupportsMultiModalFuture bool
}

// ListUserConfigs 查询当前用户的全部模型配置。
func ListUserConfigs(userID int64) ([]model.UserLLMConfig, code.Code) {
	configs, err := llmConfigDAO.ListUserLLMConfigs(userID)
	if err != nil {
		return nil, code.CodeServerBusy
	}
	return configs, code.CodeSuccess
}

// GetUserConfig 按用户维度读取单个配置，避免越权访问其他人的配置。
func GetUserConfig(userID int64, id int64) (*model.UserLLMConfig, code.Code) {
	config, err := llmConfigDAO.GetUserLLMConfigByID(userID, id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, code.CodeRecordNotFound
		}
		return nil, code.CodeServerBusy
	}
	return config, code.CodeSuccess
}

// CreateUserConfig 创建用户配置，并在需要时同步设置默认项。
func CreateUserConfig(userID int64, input CreateConfigInput) (*model.UserLLMConfig, code.Code) {
	config, ok := normalizeConfigInput(input.Name, input.Provider, input.APIKey, input.BaseURL, input.Model, true)
	if !ok {
		return nil, code.CodeInvalidParams
	}

	entity := &model.UserLLMConfig{
		UserID:     userID,
		Name:       config.Name,
		Provider:   config.Provider,
		APIKey:     config.APIKey,
		BaseURL:    config.BaseURL,
		Model:      config.Model,
		IsDefault:  input.IsDefault,
		IsEnabled:  true,
		SourceType: aihelper.SourceTypeUser,
		ExtraJSON:  "{}",
	}
	created, err := llmConfigDAO.CreateUserLLMConfig(entity)
	if err != nil {
		return nil, code.CodeServerBusy
	}
	return created, code.CodeSuccess
}

// UpdateUserConfig 更新用户配置。
// 如果这次没有传新 apiKey，则继续沿用数据库里已有的 key。
func UpdateUserConfig(userID int64, id int64, input UpdateConfigInput) (*model.UserLLMConfig, code.Code) {
	existing, code_ := GetUserConfig(userID, id)
	if code_ != code.CodeSuccess {
		return nil, code_
	}

	config, ok := normalizeConfigInput(input.Name, input.Provider, input.APIKey, input.BaseURL, input.Model, false)
	if !ok {
		return nil, code.CodeInvalidParams
	}

	updates := map[string]interface{}{
		"name":       config.Name,
		"provider":   config.Provider,
		"base_url":   config.BaseURL,
		"model":      config.Model,
		"is_default": input.IsDefault,
	}
	if input.IsEnabled != nil {
		updates["is_enabled"] = *input.IsEnabled
	}
	if config.APIKey != "" {
		updates["api_key"] = config.APIKey
	}

	if err := llmConfigDAO.UpdateUserLLMConfig(existing, updates); err != nil {
		return nil, code.CodeServerBusy
	}
	return GetUserConfig(userID, id)
}

// DeleteUserConfig 对配置执行软删除。
func DeleteUserConfig(userID int64, id int64) code.Code {
	if _, code_ := GetUserConfig(userID, id); code_ != code.CodeSuccess {
		return code_
	}
	if err := llmConfigDAO.SoftDeleteUserLLMConfig(userID, id); err != nil {
		return code.CodeServerBusy
	}
	return code.CodeSuccess
}

// SetDefaultUserConfig 设置用户默认配置。
func SetDefaultUserConfig(userID int64, id int64) code.Code {
	if _, code_ := GetUserConfig(userID, id); code_ != code.CodeSuccess {
		return code_
	}
	if err := llmConfigDAO.SetDefaultUserLLMConfig(userID, id); err != nil {
		return code.CodeServerBusy
	}
	return code.CodeSuccess
}

// ListProviderCapabilities 返回当前后端已声明的 Provider 能力矩阵。
func ListProviderCapabilities() []ProviderCapabilityItem {
	raw := aihelper.ListProviderCapabilities()
	items := make([]ProviderCapabilityItem, 0, len(raw))
	for _, item := range raw {
		items = append(items, ProviderCapabilityItem{
			Provider:                 item.Provider,
			DisplayName:              item.DisplayName,
			IsImplemented:            item.IsImplemented,
			SupportedChatModes:       append([]string(nil), item.SupportedChatModes...),
			SupportsConfigTest:       item.SupportsConfigTest,
			SupportsToolCalling:      item.SupportsToolCalling,
			SupportsEmbedding:        item.SupportsEmbedding,
			SupportsMultiModalFuture: item.SupportsMultiModalFuture,
		})
	}
	return items
}

// ListSupportedChatModes 返回系统级 chat_mode 列表，供前端做统一枚举展示。
func ListSupportedChatModes() []string {
	return aihelper.ListSupportedChatModes()
}

// TestConfigConnectivity 用一次最小模型调用验证配置是否可用。
// 当前阶段先支持已经落地 Provider 的连通性测试，不支持的 Provider 直接返回参数错误。
func TestConfigConnectivity(input TestConfigInput) code.Code {
	config, ok := normalizeConfigInput("connectivity-check", input.Provider, input.APIKey, input.BaseURL, input.Model, false)
	if !ok {
		return code.CodeInvalidParams
	}
	if config.Provider != aihelper.ProviderOllama && config.APIKey == "" {
		return code.CodeInvalidParams
	}

	modelType, ok := aihelper.ResolveProviderModelType(config.Provider)
	if !ok {
		return code.CodeInvalidParams
	}

	runtimeConfig := aihelper.RuntimeConfig{
		Provider:  config.Provider,
		APIKey:    config.APIKey,
		BaseURL:   config.BaseURL,
		ModelName: config.Model,
		ChatMode:  aihelper.ChatModeChat,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	provider, err := aihelper.GetGlobalFactory().CreateProvider(ctx, modelType, runtimeConfig)
	if err != nil {
		return code.CodeInvalidParams
	}

	_, err = provider.Generate(ctx, aihelperBuildConnectivityMessages())
	if err != nil {
		return code.AIModelFail
	}
	return code.CodeSuccess
}

// normalizeConfigInput 统一做名称、Provider、模型名和 key 的基础校验与去空格处理。
func normalizeConfigInput(name, provider, apiKey, baseURL, modelName string, requireAPIKey bool) (*model.UserLLMConfig, bool) {
	normalizedName := strings.TrimSpace(name)
	normalizedProvider := strings.TrimSpace(provider)
	normalizedAPIKey := strings.TrimSpace(apiKey)
	normalizedBaseURL := strings.TrimSpace(baseURL)
	normalizedModel := strings.TrimSpace(modelName)

	if normalizedName == "" || normalizedModel == "" || !aihelper.IsSupportedProvider(normalizedProvider) {
		return nil, false
	}
	if requireAPIKey && normalizedProvider != aihelper.ProviderOllama && normalizedAPIKey == "" {
		return nil, false
	}

	return &model.UserLLMConfig{
		Name:     normalizedName,
		Provider: normalizedProvider,
		APIKey:   normalizedAPIKey,
		BaseURL:  normalizedBaseURL,
		Model:    normalizedModel,
	}, true
}

// aihelperBuildConnectivityMessages 构造最小化的连通性探测请求，避免浪费过多 token。
func aihelperBuildConnectivityMessages() []*schema.Message {
	return []*schema.Message{
		{
			Role:    schema.User,
			Content: "请只回复 OK。",
		},
	}
}
