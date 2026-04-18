package llm_config

import (
	"GopherAI/common/mysql"
	"GopherAI/model"

	"gorm.io/gorm"
)

// ListUserLLMConfigs 列出指定用户的全部未删除配置。
func ListUserLLMConfigs(userID int64) ([]model.UserLLMConfig, error) {
	var configs []model.UserLLMConfig
	err := mysql.DB.Where("user_id = ?", userID).Order("is_default desc, updated_at desc").Find(&configs).Error
	return configs, err
}

// GetUserLLMConfigByID 读取用户自己的单个配置。
func GetUserLLMConfigByID(userID int64, id int64) (*model.UserLLMConfig, error) {
	var config model.UserLLMConfig
	err := mysql.DB.Where("id = ? AND user_id = ?", id, userID).First(&config).Error
	return &config, err
}

func ListUserLLMConfigsByIDs(userID int64, ids []int64) ([]model.UserLLMConfig, error) {
	var configs []model.UserLLMConfig
	if len(ids) == 0 {
		return configs, nil
	}
	err := mysql.DB.Where("user_id = ? AND id IN ?", userID, ids).Find(&configs).Error
	return configs, err
}

// GetDefaultUserLLMConfig 获取用户默认配置。
func GetDefaultUserLLMConfig(userID int64) (*model.UserLLMConfig, error) {
	var config model.UserLLMConfig
	err := mysql.DB.Where("user_id = ? AND is_default = ?", userID, true).Order("updated_at desc").First(&config).Error
	return &config, err
}

// CreateUserLLMConfig 创建配置。
// 当新配置被设为默认时，这里会先清掉该用户的旧默认项。
func CreateUserLLMConfig(config *model.UserLLMConfig) (*model.UserLLMConfig, error) {
	err := mysql.DB.Transaction(func(tx *gorm.DB) error {
		if config.IsDefault {
			if err := tx.Model(&model.UserLLMConfig{}).
				Where("user_id = ?", config.UserID).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(config).Error
	})
	return config, err
}

// UpdateUserLLMConfig 更新配置。
// 若本次更新把配置设为默认，也会先撤销同用户下的其他默认项。
func UpdateUserLLMConfig(config *model.UserLLMConfig, updates map[string]interface{}) error {
	return mysql.DB.Transaction(func(tx *gorm.DB) error {
		if isDefault, ok := updates["is_default"].(bool); ok && isDefault {
			if err := tx.Model(&model.UserLLMConfig{}).
				Where("user_id = ? AND id <> ?", config.UserID, config.ID).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}

		return tx.Model(&model.UserLLMConfig{}).
			Where("id = ? AND user_id = ?", config.ID, config.UserID).
			Updates(updates).Error
	})
}

// SoftDeleteUserLLMConfig 对指定配置做软删除。
func SoftDeleteUserLLMConfig(userID int64, id int64) error {
	return mysql.DB.Where("id = ? AND user_id = ?", id, userID).Delete(&model.UserLLMConfig{}).Error
}

// SetDefaultUserLLMConfig 直接把某个配置设置为用户默认项。
func SetDefaultUserLLMConfig(userID int64, id int64) error {
	return mysql.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.UserLLMConfig{}).
			Where("user_id = ?", userID).
			Update("is_default", false).Error; err != nil {
			return err
		}

		return tx.Model(&model.UserLLMConfig{}).
			Where("id = ? AND user_id = ?", id, userID).
			Update("is_default", true).Error
	})
}
