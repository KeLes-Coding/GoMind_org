package worker

import (
	"GopherAI/common/metrics"
	"GopherAI/common/mysql"
	"GopherAI/common/rag"
	"GopherAI/common/redis"
	"GopherAI/dao"
	"GopherAI/model"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

type VectorizeWorker struct {
	fileDAO *dao.FileDAO
}

func NewVectorizeWorker() *VectorizeWorker {
	return &VectorizeWorker{
		fileDAO: dao.NewFileDAO(mysql.DB),
	}
}

func (w *VectorizeWorker) ProcessTask(ctx context.Context, taskData []byte) error {
	var task model.VectorizeTask
	if err := json.Unmarshal(taskData, &task); err != nil {
		return fmt.Errorf("failed to unmarshal task: %w", err)
	}

	log.Printf("Processing vectorize task: fileID=%s, version=%d", task.FileID, task.Version)

	lockKey := fmt.Sprintf("lock:vectorize:%s:%d", task.FileID, task.Version)
	locked, err := redis.AcquireLock(ctx, lockKey, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !locked {
		log.Printf("Task already processing: fileID=%s", task.FileID)
		return nil
	}
	defer redis.ReleaseLock(ctx, lockKey)

	file, err := w.fileDAO.GetFileByID(ctx, task.FileID)
	if err != nil {
		return fmt.Errorf("failed to get file: %w", err)
	}
	if file.Status == model.FileStatusReady && file.Version == task.Version {
		log.Printf("File already ready: fileID=%s", task.FileID)
		return nil
	}

	claimed, err := w.fileDAO.ClaimFileForVectorize(ctx, file.ID, task.Version)
	if err != nil {
		return fmt.Errorf("failed to claim file: %w", err)
	}
	if !claimed {
		log.Printf("Skip vectorize task because file state changed: fileID=%s status=%s version=%d", file.ID, file.Status, file.Version)
		return nil
	}

	return w.vectorizeFile(ctx, file)
}

func (w *VectorizeWorker) vectorizeFile(ctx context.Context, file *model.FileAsset) error {
	if err := w.fileDAO.UpdateFileStatus(ctx, file.ID, model.FileStatusParsing); err != nil {
		return fmt.Errorf("failed to update status to parsing: %w", err)
	}

	if err := w.fileDAO.UpdateFileStatus(ctx, file.ID, model.FileStatusVectorizing); err != nil {
		return fmt.Errorf("failed to update status to vectorizing: %w", err)
	}

	// 真正的向量化写入也收口到统一同步函数里。
	// 这样 worker 正常处理、查询自愈迁移、后续批量迁移都会复用同一套：
	// 1. 先删同 file_id 的旧 chunk；
	// 2. 再写当前 version；
	// 3. 最后顺手清理历史按文件索引。
	if err := rag.SyncFileToUnifiedIndex(ctx, file); err != nil {
		w.fileDAO.UpdateFileStatus(ctx, file.ID, model.FileStatusFailed, err.Error())
		metrics.File.IncrFailed()
		return fmt.Errorf("failed to sync file to unified index: %w", err)
	}

	if err := w.fileDAO.UpdateFileStatus(ctx, file.ID, model.FileStatusReady); err != nil {
		return fmt.Errorf("failed to update status to ready: %w", err)
	}

	metrics.File.IncrVectorized()
	log.Printf("File vectorized successfully: fileID=%s", file.ID)
	return nil
}
