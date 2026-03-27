package dao

import (
	"GopherAI/model"
	"context"

	"gorm.io/gorm"
)

type FileDAO struct {
	db *gorm.DB
}

func NewFileDAO(db *gorm.DB) *FileDAO {
	return &FileDAO{db: db}
}

// CreateFile 创建文件记录
func (d *FileDAO) CreateFile(ctx context.Context, file *model.FileAsset) error {
	return d.db.WithContext(ctx).Create(file).Error
}

// GetFileByID 根据ID查询文件
func (d *FileDAO) GetFileByID(ctx context.Context, fileID string) (*model.FileAsset, error) {
	var file model.FileAsset
	err := d.db.WithContext(ctx).Where("id = ?", fileID).First(&file).Error
	return &file, err
}

// UpdateFileStatus 更新文件状态
func (d *FileDAO) UpdateFileStatus(ctx context.Context, fileID, status string, errorMsg ...string) error {
	updates := map[string]interface{}{"status": status}
	if len(errorMsg) > 0 {
		updates["error_msg"] = errorMsg[0]
	}
	return d.db.WithContext(ctx).Model(&model.FileAsset{}).Where("id = ?", fileID).Updates(updates).Error
}

// GetReadyFilesByOwner 查询用户所有已就绪的文件（用于 RAG 检索）
func (d *FileDAO) GetReadyFilesByOwner(ctx context.Context, ownerID int64) ([]*model.FileAsset, error) {
	var files []*model.FileAsset
	err := d.db.WithContext(ctx).
		Where("owner_id = ? AND status = ?", ownerID, model.FileStatusReady).
		Find(&files).Error
	return files, err
}

// FindFileByHash 根据内容哈希查找文件（用于秒传去重）
func (d *FileDAO) FindFileByHash(ctx context.Context, ownerID int64, sha256 string) (*model.FileAsset, error) {
	var file model.FileAsset
	err := d.db.WithContext(ctx).
		Where("owner_id = ? AND sha256 = ? AND status IN ?",
			ownerID, sha256, []string{model.FileStatusUploaded, model.FileStatusReady}).
		First(&file).Error
	return &file, err
}

// ListFilesByOwner 查询用户所有文件
func (d *FileDAO) ListFilesByOwner(ctx context.Context, ownerID int64) ([]*model.FileAsset, error) {
	var files []*model.FileAsset
	err := d.db.WithContext(ctx).Where("owner_id = ?", ownerID).Order("created_at DESC").Find(&files).Error
	return files, err
}
