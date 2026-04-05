package session

import (
	"GopherAI/common/code"
	"GopherAI/controller"
	"GopherAI/model"
	llmconfig "GopherAI/service/llmconfig"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type LLMConfigView struct {
	ID                 int64                   `json:"id"`
	Name               string                  `json:"name"`
	Provider           string                  `json:"provider"`
	BaseURL            string                  `json:"baseUrl,omitempty"`
	Model              string                  `json:"model"`
	IsDefault          bool                    `json:"isDefault"`
	IsEnabled          bool                    `json:"isEnabled"`
	SourceType         string                  `json:"sourceType"`
	MaskedAPIKey       string                  `json:"maskedApiKey,omitempty"`
	HasAPIKey          bool                    `json:"hasApiKey,omitempty"`
	ProviderCapability *ProviderCapabilityView `json:"providerCapability,omitempty"`
}

type ProviderCapabilityView struct {
	Provider                 string   `json:"provider"`
	DisplayName              string   `json:"displayName"`
	IsImplemented            bool     `json:"isImplemented"`
	SupportedChatModes       []string `json:"supportedChatModes"`
	SupportsConfigTest       bool     `json:"supportsConfigTest"`
	SupportsToolCalling      bool     `json:"supportsToolCalling"`
	SupportsEmbedding        bool     `json:"supportsEmbedding"`
	SupportsMultiModalFuture bool     `json:"supportsMultiModalFuture"`
}

type (
	// ListLLMConfigsResponse 返回当前用户可见的模型配置列表。
	ListLLMConfigsResponse struct {
		controller.Response
		Configs []LLMConfigView `json:"configs,omitempty"`
	}

	GetLLMConfigResponse struct {
		controller.Response
		Config *LLMConfigView `json:"config,omitempty"`
	}

	GetLLMConfigMetaResponse struct {
		controller.Response
		Providers []ProviderCapabilityView `json:"providers,omitempty"`
		ChatModes []string                 `json:"chatModes,omitempty"`
	}

	CreateLLMConfigRequest struct {
		Name      string `json:"name" binding:"required"`
		Provider  string `json:"provider" binding:"required"`
		APIKey    string `json:"apiKey,omitempty"`
		BaseURL   string `json:"baseUrl,omitempty"`
		Model     string `json:"model" binding:"required"`
		IsDefault bool   `json:"isDefault"`
	}

	UpdateLLMConfigRequest struct {
		Name      string `json:"name" binding:"required"`
		Provider  string `json:"provider" binding:"required"`
		APIKey    string `json:"apiKey,omitempty"`
		BaseURL   string `json:"baseUrl,omitempty"`
		Model     string `json:"model" binding:"required"`
		IsDefault bool   `json:"isDefault"`
		IsEnabled *bool  `json:"isEnabled,omitempty"`
	}

	TestLLMConfigRequest struct {
		Provider string `json:"provider" binding:"required"`
		APIKey   string `json:"apiKey,omitempty"`
		BaseURL  string `json:"baseUrl,omitempty"`
		Model    string `json:"model" binding:"required"`
	}
)

// GetLLMConfigMeta 返回后端当前声明的 Provider 能力矩阵和 chat_mode 枚举。
// 前端后续可以据此动态渲染模式选择，而不是继续把能力写死在页面里。
func GetLLMConfigMeta(c *gin.Context) {
	res := new(GetLLMConfigMetaResponse)

	items := llmconfig.ListProviderCapabilities()
	views := make([]ProviderCapabilityView, 0, len(items))
	for _, item := range items {
		views = append(views, toProviderCapabilityView(item))
	}

	res.Success()
	res.Providers = views
	res.ChatModes = llmconfig.ListSupportedChatModes()
	c.JSON(http.StatusOK, res)
}

// ListLLMConfigs 返回用户自己的模型配置列表。
// 列表接口只返回掩码后的 key，不暴露真实敏感信息。
func ListLLMConfigs(c *gin.Context) {
	res := new(ListLLMConfigsResponse)
	userID := c.GetInt64("userID")

	configs, code_ := llmconfig.ListUserConfigs(userID)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	items := make([]LLMConfigView, 0, len(configs))
	for _, item := range configs {
		items = append(items, toLLMConfigView(item, false))
	}

	res.Success()
	res.Configs = items
	c.JSON(http.StatusOK, res)
}

// GetLLMConfig 返回单个配置详情。
// 详情接口同样不回传明文 key，只补一个 hasApiKey 方便前端做回显控制。
func GetLLMConfig(c *gin.Context) {
	res := new(GetLLMConfigResponse)
	userID := c.GetInt64("userID")

	id, ok := parseConfigIDParam(c)
	if !ok {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	config, code_ := llmconfig.GetUserConfig(userID, id)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	view := toLLMConfigView(*config, true)
	res.Success()
	res.Config = &view
	c.JSON(http.StatusOK, res)
}

// CreateLLMConfig 创建一条新的用户模型配置。
func CreateLLMConfig(c *gin.Context) {
	req := new(CreateLLMConfigRequest)
	res := new(GetLLMConfigResponse)
	userID := c.GetInt64("userID")
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	config, code_ := llmconfig.CreateUserConfig(userID, llmconfig.CreateConfigInput{
		Name:      req.Name,
		Provider:  req.Provider,
		APIKey:    req.APIKey,
		BaseURL:   req.BaseURL,
		Model:     req.Model,
		IsDefault: req.IsDefault,
	})
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	view := toLLMConfigView(*config, true)
	res.Success()
	res.Config = &view
	c.JSON(http.StatusOK, res)
}

// UpdateLLMConfig 更新已有配置。
// 当前约定里，apiKey 为空字符串表示“不修改原 key”。
func UpdateLLMConfig(c *gin.Context) {
	req := new(UpdateLLMConfigRequest)
	res := new(GetLLMConfigResponse)
	userID := c.GetInt64("userID")
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	id, ok := parseConfigIDParam(c)
	if !ok {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	config, code_ := llmconfig.UpdateUserConfig(userID, id, llmconfig.UpdateConfigInput{
		Name:      req.Name,
		Provider:  req.Provider,
		APIKey:    req.APIKey,
		BaseURL:   req.BaseURL,
		Model:     req.Model,
		IsDefault: req.IsDefault,
		IsEnabled: req.IsEnabled,
	})
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	view := toLLMConfigView(*config, true)
	res.Success()
	res.Config = &view
	c.JSON(http.StatusOK, res)
}

// DeleteLLMConfig 对配置做软删除。
func DeleteLLMConfig(c *gin.Context) {
	res := new(controller.Response)
	userID := c.GetInt64("userID")

	id, ok := parseConfigIDParam(c)
	if !ok {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	code_ := llmconfig.DeleteUserConfig(userID, id)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	c.JSON(http.StatusOK, res)
}

// SetDefaultLLMConfig 设置用户默认配置。
// service 层会负责清掉该用户其他配置上的默认标记。
func SetDefaultLLMConfig(c *gin.Context) {
	res := new(controller.Response)
	userID := c.GetInt64("userID")

	id, ok := parseConfigIDParam(c)
	if !ok {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	code_ := llmconfig.SetDefaultUserConfig(userID, id)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	c.JSON(http.StatusOK, res)
}

// TestLLMConfig 在用户保存配置前先做一次连通性验证。
// 这个接口不会持久化任何数据，只返回当前参数组合是否可用。
func TestLLMConfig(c *gin.Context) {
	req := new(TestLLMConfigRequest)
	res := new(controller.Response)
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	code_ := llmconfig.TestConfigConnectivity(llmconfig.TestConfigInput{
		Provider: req.Provider,
		APIKey:   req.APIKey,
		BaseURL:  req.BaseURL,
		Model:    req.Model,
	})
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	c.JSON(http.StatusOK, res)
}

// parseConfigIDParam 统一解析路径参数中的配置 ID。
func parseConfigIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// toLLMConfigView 负责把数据库实体转换成接口视图，并执行敏感字段掩码。
func toLLMConfigView(config model.UserLLMConfig, includeHasAPIKey bool) LLMConfigView {
	view := LLMConfigView{
		ID:           config.ID,
		Name:         config.Name,
		Provider:     config.Provider,
		BaseURL:      config.BaseURL,
		Model:        config.Model,
		IsDefault:    config.IsDefault,
		IsEnabled:    config.IsEnabled,
		SourceType:   config.SourceType,
		MaskedAPIKey: maskAPIKey(config.APIKey),
	}
	if includeHasAPIKey {
		view.HasAPIKey = strings.TrimSpace(config.APIKey) != ""
	}
	if capability, ok := lookupProviderCapabilityView(config.Provider); ok {
		view.ProviderCapability = &capability
	}
	return view
}

// lookupProviderCapabilityView 返回某个 provider 在当前后端里的能力矩阵。
func lookupProviderCapabilityView(provider string) (ProviderCapabilityView, bool) {
	for _, item := range llmconfig.ListProviderCapabilities() {
		if item.Provider == provider {
			return toProviderCapabilityView(item), true
		}
	}
	return ProviderCapabilityView{}, false
}

func toProviderCapabilityView(item llmconfig.ProviderCapabilityItem) ProviderCapabilityView {
	return ProviderCapabilityView{
		Provider:                 item.Provider,
		DisplayName:              item.DisplayName,
		IsImplemented:            item.IsImplemented,
		SupportedChatModes:       append([]string(nil), item.SupportedChatModes...),
		SupportsConfigTest:       item.SupportsConfigTest,
		SupportsToolCalling:      item.SupportsToolCalling,
		SupportsEmbedding:        item.SupportsEmbedding,
		SupportsMultiModalFuture: item.SupportsMultiModalFuture,
	}
}

// maskAPIKey 只保留后四位，避免列表和详情接口泄漏真实 key。
func maskAPIKey(apiKey string) string {
	normalized := strings.TrimSpace(apiKey)
	if normalized == "" {
		return ""
	}
	if len(normalized) <= 4 {
		return "****"
	}
	return "****" + normalized[len(normalized)-4:]
}
