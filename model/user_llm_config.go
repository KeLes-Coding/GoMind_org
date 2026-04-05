package model

import (
	"time"

	"gorm.io/gorm"
)

// UserLLMConfig 表示用户可管理的一条聊天模型配置。
// 第一阶段先聚焦 chat provider 相关字段，embedding 和 MCP server 配置暂不进入这里。
type UserLLMConfig struct {
	ID         int64          `gorm:"primaryKey" json:"id"`
	UserID     int64          `gorm:"index;not null" json:"user_id"`
	Name       string         `gorm:"type:varchar(100);not null" json:"name"`
	Provider   string         `gorm:"type:varchar(32);not null" json:"provider"`
	APIKey     string         `gorm:"type:varchar(512);not null" json:"-"`
	BaseURL    string         `gorm:"type:varchar(255)" json:"base_url"`
	Model      string         `gorm:"type:varchar(100);not null" json:"model"`
	IsDefault  bool           `gorm:"not null;default:false" json:"is_default"`
	IsEnabled  bool           `gorm:"not null;default:true" json:"is_enabled"`
	SourceType string         `gorm:"type:varchar(16);not null;default:'user'" json:"source_type"`
	ExtraJSON  string         `gorm:"type:json" json:"extra_json,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}
