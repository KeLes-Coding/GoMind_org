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
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Provider     string `json:"provider"`
	BaseURL      string `json:"baseUrl,omitempty"`
	Model        string `json:"model"`
	IsDefault    bool   `json:"isDefault"`
	IsEnabled    bool   `json:"isEnabled"`
	SourceType   string `json:"sourceType"`
	MaskedAPIKey string `json:"maskedApiKey,omitempty"`
	HasAPIKey    bool   `json:"hasApiKey,omitempty"`
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
)

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
	return view
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
