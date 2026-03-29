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

// CreateFile 创建文件记录。
func (d *FileDAO) CreateFile(ctx context.Context, file *model.FileAsset) error {
	return d.db.WithContext(ctx).Create(file).Error
}

// GetFileByID 根据 ID 查询文件。
func (d *FileDAO) GetFileByID(ctx context.Context, fileID string) (*model.FileAsset, error) {
	var file model.FileAsset
	err := d.db.WithContext(ctx).Where("id = ?", fileID).First(&file).Error
	return &file, err
}

// UpdateFileStatus 更新文件状态。
// 这里继续保留可选 errorMsg 参数，方便 worker 在处理失败时把错误原因一并回写。
func (d *FileDAO) UpdateFileStatus(ctx context.Context, fileID, status string, errorMsg ...string) error {
	updates := map[string]interface{}{"status": status}
	if len(errorMsg) > 0 {
		updates["error_msg"] = errorMsg[0]
	}
	return d.db.WithContext(ctx).Model(&model.FileAsset{}).Where("id = ?", fileID).Updates(updates).Error
}

// MarkVectorTaskQueued 标记“当前版本的向量化任务已经成功投递”。
// 注意这里带 version 条件，是为了避免旧版本任务回写覆盖新版本状态。
func (d *FileDAO) MarkVectorTaskQueued(ctx context.Context, fileID string, version int) error {
	return d.db.WithContext(ctx).
		Model(&model.FileAsset{}).
		Where("id = ? AND version = ?", fileID, version).
		Updates(map[string]interface{}{
			"vector_task_queued":  true,
			"vector_task_err_msg": "",
		}).Error
}

// MarkVectorTaskPending 标记“当前版本任务尚未成功投递”，并记录失败原因。
// 这个状态会被补偿 worker 周期性扫描并重试投递。
func (d *FileDAO) MarkVectorTaskPending(ctx context.Context, fileID string, version int, errMsg string) error {
	return d.db.WithContext(ctx).
		Model(&model.FileAsset{}).
		Where("id = ? AND version = ?", fileID, version).
		Updates(map[string]interface{}{
			"vector_task_queued":  false,
			"vector_task_err_msg": errMsg,
		}).Error
}

// ListFilesPendingVectorTask 查询“文件已上传成功，但当前版本任务还没成功入队”的资产。
// 这批记录是后台补偿 worker 的主要处理对象。
func (d *FileDAO) ListFilesPendingVectorTask(ctx context.Context, limit int) ([]*model.FileAsset, error) {
	if limit <= 0 {
		limit = 100
	}

	var files []*model.FileAsset
	err := d.db.WithContext(ctx).
		Where("status = ? AND vector_task_queued = ?", model.FileStatusUploaded, false).
		Order("updated_at ASC").
		Limit(limit).
		Find(&files).Error
	return files, err
}

// ClaimFileForVectorize 把文件从 uploaded/failed 原子地切到 parsing，避免同一版本被重复消费。
func (d *FileDAO) ClaimFileForVectorize(ctx context.Context, fileID string, version int) (bool, error) {
	result := d.db.WithContext(ctx).
		Model(&model.FileAsset{}).
		Where("id = ? AND version = ? AND status IN ?", fileID, version, []string{model.FileStatusUploaded, model.FileStatusFailed}).
		Updates(map[string]interface{}{
			"status":    model.FileStatusParsing,
			"error_msg": "",
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// GetReadyFilesByOwner 查询用户所有 ready 状态的文件。
func (d *FileDAO) GetReadyFilesByOwner(ctx context.Context, ownerID int64) ([]*model.FileAsset, error) {
	var files []*model.FileAsset
	err := d.db.WithContext(ctx).
		Where("owner_id = ? AND status = ?", ownerID, model.FileStatusReady).
		Find(&files).Error
	return files, err
}

// FindFileByHash 按内容哈希查询文件，用于同用户秒传复用。
func (d *FileDAO) FindFileByHash(ctx context.Context, ownerID int64, sha256 string) (*model.FileAsset, error) {
	var file model.FileAsset
	err := d.db.WithContext(ctx).
		Where("owner_id = ? AND sha256 = ? AND status IN ?", ownerID, sha256, []string{model.FileStatusUploaded, model.FileStatusReady}).
		First(&file).Error
	return &file, err
}

// ListFilesByOwner 查询用户所有文件。
func (d *FileDAO) ListFilesByOwner(ctx context.Context, ownerID int64) ([]*model.FileAsset, error) {
	var files []*model.FileAsset
	err := d.db.WithContext(ctx).Where("owner_id = ?", ownerID).Order("created_at DESC").Find(&files).Error
	return files, err
}

func (d *FileDAO) DeleteFile(ctx context.Context, fileID string) error {
	return d.db.WithContext(ctx).Delete(&model.FileAsset{}, "id = ?", fileID).Error
}

// ResetFileToUploaded 把失败文件重新放回待向量化状态。
// 除了重置业务状态外，也会把“已入队标记”清掉，确保重新走一次可靠投递流程。
func (d *FileDAO) ResetFileToUploaded(ctx context.Context, fileID string) error {
	return d.db.WithContext(ctx).
		Model(&model.FileAsset{}).
		Where("id = ?", fileID).
		Updates(map[string]interface{}{
			"status":              model.FileStatusUploaded,
			"error_msg":           "",
			"vector_task_queued":  false,
			"vector_task_err_msg": "",
		}).Error
}

// PrepareFileForReindex 为重建索引做准备。
// 它会把版本号加一，并把“当前版本是否已入队”重置为 false，
// 这样新的版本一定会重新经历一次投递与补偿流程。
func (d *FileDAO) PrepareFileForReindex(ctx context.Context, fileID string) error {
	return d.db.WithContext(ctx).
		Model(&model.FileAsset{}).
		Where("id = ?", fileID).
		Updates(map[string]interface{}{
			"status":              model.FileStatusUploaded,
			"error_msg":           "",
			"version":             gorm.Expr("version + 1"),
			"vector_task_queued":  false,
			"vector_task_err_msg": "",
		}).Error
}
