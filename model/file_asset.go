package model

import (
	"time"

	"gorm.io/gorm"
)

// FileAsset 文件资产表 - 把文件从"目录里的临时对象"升级成"带主键和状态的业务资产"
type FileAsset struct {
	ID          string         `gorm:"type:char(36);primaryKey" json:"id"`           // 文件唯一标识（UUID）
	OwnerID     int64          `gorm:"index" json:"owner_id"`                        // 所属用户ID
	KBID        string         `gorm:"type:char(36);index" json:"kb_id"`             // 所属知识库ID（暂时可为空，后续扩展）
	FileName    string         `gorm:"type:varchar(255)" json:"file_name"`           // 原始文件名
	ContentType string         `gorm:"type:varchar(128)" json:"content_type"`        // MIME类型
	Size        int64          `json:"size"`                                         // 文件大小（字节）
	SHA256      string         `gorm:"type:char(64);index" json:"sha256"`            // 内容哈希（用于去重和秒传）
	StorageKey  string         `gorm:"type:varchar(512)" json:"storage_key"`         // 存储路径或对象存储key
	Status      string         `gorm:"type:varchar(32);index" json:"status"`         // 文件状态：pending_upload/uploaded/parsing/vectorizing/ready/failed
	Version     int            `gorm:"default:1" json:"version"`                     // 文件版本号（用于重建索引）
	ErrorMsg    string         `gorm:"type:text" json:"error_msg,omitempty"`         // 失败时的错误信息
	CreatedAt   time.Time      `json:"created_at"`                                   // 创建时间
	UpdatedAt   time.Time      `json:"updated_at"`                                   // 更新时间
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`                               // 软删除
}

// 文件状态常量
const (
	FileStatusPendingUpload = "pending_upload" // 等待上传
	FileStatusUploaded      = "uploaded"       // 已上传（但未向量化）
	FileStatusParsing       = "parsing"        // 解析中
	FileStatusVectorizing   = "vectorizing"    // 向量化中
	FileStatusReady         = "ready"          // 已就绪（可检索）
	FileStatusFailed        = "failed"         // 处理失败
)
