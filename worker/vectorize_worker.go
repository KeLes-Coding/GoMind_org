package worker

import (
	"GopherAI/common/metrics"
	"GopherAI/common/mysql"
	"GopherAI/common/rag"
	"GopherAI/common/redis"
	"GopherAI/config"
	"GopherAI/dao"
	"GopherAI/model"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"time"
)

// VectorizeWorker 向量化任务 Worker
type VectorizeWorker struct {
	fileDAO *dao.FileDAO
}

func NewVectorizeWorker() *VectorizeWorker {
	return &VectorizeWorker{
		fileDAO: dao.NewFileDAO(mysql.DB),
	}
}

// ProcessTask 处理向量化任务（核心逻辑）
func (w *VectorizeWorker) ProcessTask(ctx context.Context, taskData []byte) error {
	// 1. 解析任务
	var task model.VectorizeTask
	if err := json.Unmarshal(taskData, &task); err != nil {
		return fmt.Errorf("failed to unmarshal task: %w", err)
	}

	log.Printf("Processing vectorize task: fileID=%s, version=%d", task.FileID, task.Version)

	// 2. 获取分布式锁（防止重复消费）
	lockKey := fmt.Sprintf("lock:vectorize:%s:%d", task.FileID, task.Version)
	locked, err := redis.AcquireLock(ctx, lockKey, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !locked {
		log.Printf("Task already processing: fileID=%s", task.FileID)
		return nil // 已有其他 worker 在处理，直接返回
	}
	defer redis.ReleaseLock(ctx, lockKey)

	// 3. 检查幂等性：如果文件已经是 ready 状态，直接返回
	file, err := w.fileDAO.GetFileByID(ctx, task.FileID)
	if err != nil {
		return fmt.Errorf("failed to get file: %w", err)
	}
	if file.Status == model.FileStatusReady && file.Version == task.Version {
		log.Printf("File already ready: fileID=%s", task.FileID)
		return nil
	}

	// 4. 执行向量化
	return w.vectorizeFile(ctx, file)
}

// vectorizeFile 向量化文件（内部方法）
func (w *VectorizeWorker) vectorizeFile(ctx context.Context, file *model.FileAsset) error {
	// 1. 更新状态为 parsing
	if err := w.fileDAO.UpdateFileStatus(ctx, file.ID, model.FileStatusParsing); err != nil {
		return fmt.Errorf("failed to update status to parsing: %w", err)
	}

	// 2. 提取文件名（用于创建索引）
	storageFileName := filepath.Base(file.StorageKey)

	// 3. 更新状态为 vectorizing
	if err := w.fileDAO.UpdateFileStatus(ctx, file.ID, model.FileStatusVectorizing); err != nil {
		return fmt.Errorf("failed to update status to vectorizing: %w", err)
	}

	// 4. 创建 RAG 索引器（传入权限信息）
	indexer, err := rag.NewRAGIndexerWithPermission(storageFileName, config.GetConfig().RagModelConfig.RagEmbeddingModel, file.OwnerID, file.KBID)
	if err != nil {
		w.fileDAO.UpdateFileStatus(ctx, file.ID, model.FileStatusFailed, err.Error())
		metrics.File.IncrFailed()
		return fmt.Errorf("failed to create indexer: %w", err)
	}

	// 5. 读取文件并创建向量索引（包含 chunk 切分）
	if err := indexer.IndexFile(ctx, file.StorageKey); err != nil {
		w.fileDAO.UpdateFileStatus(ctx, file.ID, model.FileStatusFailed, err.Error())
		metrics.File.IncrFailed()
		return fmt.Errorf("failed to index file: %w", err)
	}

	// 6. 更新状态为 ready
	if err := w.fileDAO.UpdateFileStatus(ctx, file.ID, model.FileStatusReady); err != nil {
		return fmt.Errorf("failed to update status to ready: %w", err)
	}

	// 记录向量化成功指标
	metrics.File.IncrVectorized()

	log.Printf("File vectorized successfully: fileID=%s", file.ID)
	return nil
}
