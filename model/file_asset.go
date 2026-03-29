package model

import (
	"time"

	"gorm.io/gorm"
)

// FileAsset 是文件资产表。
// 这次升级后，文件不再只是磁盘上的一个匿名对象，而是一个有主键、有状态、有版本号的业务资产。
// 这样上传、删除、重试、重建索引、补偿投递都可以围绕同一份元数据来做治理。
type FileAsset struct {
	ID          string `gorm:"type:char(36);primaryKey" json:"id"`
	OwnerID     int64  `gorm:"index" json:"owner_id"`
	KBID        string `gorm:"type:char(36);index" json:"kb_id"`
	FileName    string `gorm:"type:varchar(255)" json:"file_name"`
	ContentType string `gorm:"type:varchar(128)" json:"content_type"`
	Size        int64  `json:"size"`
	SHA256      string `gorm:"type:char(64);index" json:"sha256"`
	StorageKey  string `gorm:"type:varchar(512)" json:"storage_key"`
	Status      string `gorm:"type:varchar(32);index" json:"status"`
	Version     int    `gorm:"default:1" json:"version"`
	// VectorTaskQueued 表示“当前版本的向量化任务是否已经成功投递到 MQ”。
	// 这个字段不是文件业务状态本身，而是补充描述“uploaded -> worker”之间的投递状态：
	// 1. 上传成功但 MQ 发布失败时，这里会保持 false；
	// 2. 补偿 worker 会扫描 status=uploaded 且该字段为 false 的文件并重投；
	// 3. reindex 会把版本号加一并重置该字段，确保新版本重新走一次完整投递流程。
	VectorTaskQueued bool `gorm:"default:false;index" json:"vector_task_queued"`
	// VectorTaskErrMsg 记录最近一次任务投递失败原因。
	// 它和 ErrorMsg 的职责不同：
	// 1. ErrorMsg 主要描述 worker 真正处理文件时的失败；
	// 2. VectorTaskErrMsg 只描述“任务没成功进队列”的失败。
	VectorTaskErrMsg string         `gorm:"type:text" json:"vector_task_err_msg,omitempty"`
	ErrorMsg         string         `gorm:"type:text" json:"error_msg,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

const (
	FileStatusPendingUpload = "pending_upload"
	FileStatusUploaded      = "uploaded"
	FileStatusParsing       = "parsing"
	FileStatusVectorizing   = "vectorizing"
	FileStatusReady         = "ready"
	FileStatusFailed        = "failed"
)
