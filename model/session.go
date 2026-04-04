package model

import (
	"time"

	"gorm.io/gorm"
)

type Session struct {
	ID                  string  `gorm:"primaryKey;type:varchar(36)" json:"id"`
	UserName            string  `gorm:"index;not null" json:"username"`
	UserID              int64   `gorm:"index;not null" json:"user_id"`
	FolderID            *string `gorm:"index;type:varchar(36)" json:"folder_id,omitempty"`
	Title               string  `gorm:"type:varchar(100)" json:"title"`
	ContextSummary      string  `gorm:"type:text" json:"-"`
	SummaryMessageCount int     `gorm:"not null;default:0" json:"-"`
	// Version 是会话一致性的正式版本号，用来约束 Redis 热状态恢复和后续状态推进。
	Version   int64          `gorm:"not null;default:1" json:"version"`
	// PersistedVersion 表示 MySQL 已经可靠追平到的会话版本水位。
	// 它和 Version 分开维护，用于识别“会话正式状态已推进，但异步落库尚未追平”的场景。
	PersistedVersion int64          `gorm:"not null;default:0" json:"persisted_version"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type SessionInfo struct {
	SessionID string `json:"sessionId"`
	Title     string `json:"name"`
	FolderID  string `json:"folderId,omitempty"`
}

type SessionFolderDetail struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Sessions  []SessionInfo `json:"sessions"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

type SessionTree struct {
	Folders           []SessionFolderDetail `json:"folders"`
	UngroupedSessions []SessionInfo         `json:"ungrouped_sessions"`
}
