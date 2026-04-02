package model

import (
	"time"

	"gorm.io/gorm"
)

type SessionFolder struct {
	ID        string         `gorm:"primaryKey;type:varchar(36)" json:"id"`
	UserID    int64          `gorm:"index;not null" json:"user_id"`
	UserName  string         `gorm:"index;not null" json:"username"`
	Name      string         `gorm:"type:varchar(100);not null" json:"name"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
