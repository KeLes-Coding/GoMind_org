package file

import (
	"GopherAI/common/mysql"
	"GopherAI/common/rag"
	"GopherAI/dao"
	"GopherAI/model"
	"context"
	"fmt"
	"path/filepath"
)

// RetryVectorizeFile 处理“失败任务重试”。
// 它只针对 uploaded / failed 两类状态开放，避免在文件正在处理中时重复入队。
func RetryVectorizeFile(userID int64, fileID string) (*model.FileAsset, error) {
	ctx := context.Background()
	fileDAO := dao.NewFileDAO(mysql.DB)

	fileAsset, err := getOwnedFile(ctx, fileDAO, userID, fileID)
	if err != nil {
		return nil, err
	}

	switch fileAsset.Status {
	case model.FileStatusFailed:
		// 对失败文件，先显式改回 uploaded。
		// 这样列表页和管理接口都能看出“已经重新排队”，而不是仍然停留在失败态。
		if err := fileDAO.ResetFileToUploaded(ctx, fileID); err != nil {
			return nil, fmt.Errorf("reset failed file to uploaded failed: %w", err)
		}
		fileAsset.Status = model.FileStatusUploaded
		fileAsset.VectorTaskQueued = false
	case model.FileStatusUploaded:
		// uploaded 本身就表示“尚未完成向量化”，可以直接重新投递任务。
		fileAsset.VectorTaskQueued = false
	default:
		return nil, ErrRetryNotAllowed
	}

	// retry 不是直接把状态改完就结束，真正让文件重新进入处理链路还依赖任务重新入队。
	// 这里统一走补偿封装，避免 retry 时再次遇到 MQ 抖动后无处补发。
	if err := publishVectorizeTaskWithCompensation(ctx, fileDAO, fileAsset); err != nil {
		return nil, fmt.Errorf("publish retry vectorize task failed: %w", err)
	}

	return fileAsset, nil
}

// ReindexFile 处理“重建索引”。
// 它和 retry 的区别在于：retry 复用同一版本重新尝试，reindex 会明确进入新版本。
func ReindexFile(userID int64, fileID string) (*model.FileAsset, error) {
	ctx := context.Background()
	fileDAO := dao.NewFileDAO(mysql.DB)

	fileAsset, err := getOwnedFile(ctx, fileDAO, userID, fileID)
	if err != nil {
		return nil, err
	}

	switch fileAsset.Status {
	case model.FileStatusPendingUpload, model.FileStatusParsing, model.FileStatusVectorizing:
		// 这些状态下文件还没稳定到可重建阶段。
		// 如果允许直接 reindex，很容易和正在运行的 worker 打架。
		return nil, ErrReindexNotAllowed
	}

	// 当前 RAG 索引名仍然以 storageFileName 语义构建，因此重建前需要先删旧索引。
	// 这样可以避免新一轮 upsert 和旧索引内容混在一起。
	storageFileName := filepath.Base(fileAsset.StorageKey)
	// 新增的统一共享索引不再按文件拆索引，因此 reindex 前必须补一次按 file_id 删 chunk。
	// 否则同一份文件的旧版本 chunk 会继续留在共享索引里，和新版本结果一起被召回。
	rag.InvalidateRetrievalScope(ctx, fileAsset.OwnerID, model.FileStatusReady, fileAsset.KBID)
	if err := rag.DeleteIndexedFileDocuments(ctx, fileAsset.ID); err != nil {
		return nil, fmt.Errorf("delete unified indexed documents before reindex failed: %w", err)
	}
	if err := rag.DeleteIndex(ctx, storageFileName); err != nil {
		return nil, fmt.Errorf("delete old index before reindex failed: %w", err)
	}

	if err := fileDAO.PrepareFileForReindex(ctx, fileID); err != nil {
		return nil, fmt.Errorf("prepare file for reindex failed: %w", err)
	}

	refreshedFile, err := fileDAO.GetFileByID(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("reload file after reindex prepare failed: %w", err)
	}

	// reindex 会进入新的版本，因此这里必须按“新版本重新入队”的语义维护投递状态。
	// 只要当前版本没成功发布到 MQ，补偿 worker 之后仍然可以继续把它补进去。
	if err := publishVectorizeTaskWithCompensation(ctx, fileDAO, refreshedFile); err != nil {
		return nil, fmt.Errorf("publish reindex task failed: %w", err)
	}

	return refreshedFile, nil
}
