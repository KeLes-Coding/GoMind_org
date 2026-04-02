package model

import (
	"time"

	"gorm.io/gorm"
)

type SessionFolder struct {
<<<<<<< HEAD
	ID        string         `gorm:"primaryKey;type:varchar(36)" json:"id"`
	UserID    int64          `gorm:"index;not null" json:"user_id"`
	UserName  string         `gorm:"index;not null" json:"username"`
=======
	ID        int64          `gorm:"primaryKey" json:"id"`
	UserID    int64          `gorm:"index;not null" json:"user_id"`
	UserName  string         `gorm:"index;not null" json:"user_name"`
>>>>>>> 8b8125bb7c712b316afa9e1ad7389df2e321a22f
	Name      string         `gorm:"type:varchar(100);not null" json:"name"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
<<<<<<< HEAD
=======

type SessionTreeItem struct {
	SessionID string `json:"sessionId"`
	Title     string `json:"name"`
}

type SessionFolderInfo struct {
	ID       int64             `json:"id"`
	Name     string            `json:"name"`
	Sessions []SessionTreeItem `json:"sessions"`
}

type SessionListTreeResponse struct {
	Folders           []SessionFolderInfo `json:"folders"`
	UngroupedSessions []SessionTreeItem   `json:"ungroupedSessions"`
}
>>>>>>> 8b8125bb7c712b316afa9e1ad7389df2e321a22f
