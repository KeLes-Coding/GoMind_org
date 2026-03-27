package model

import (
	"time"

	"gorm.io/gorm"
)

type Session struct {
	ID                  string         `gorm:"primaryKey;type:varchar(36)" json:"id"`
	UserName            string         `gorm:"index;not null" json:"username"`
	UserID              int64          `gorm:"index;not null" json:"user_id"` // 用户ID，用于关联查询
	Title               string         `gorm:"type:varchar(100)" json:"title"`
	ContextSummary      string         `gorm:"type:text" json:"-"`
	SummaryMessageCount int            `gorm:"not null;default:0" json:"-"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`
}

type SessionInfo struct {
	SessionID string `json:"sessionId"`
	Title     string `json:"name"`
}
