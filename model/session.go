package model

import (
	"time"

	"gorm.io/gorm"
)

type Session struct {
	ID                  string         `gorm:"primaryKey;type:varchar(36)" json:"id"`
	UserName            string         `gorm:"index;not null" json:"username"`
	UserID              int64          `gorm:"index;not null" json:"user_id"`
	FolderID            *string        `gorm:"index;type:varchar(36)" json:"folder_id,omitempty"`
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
