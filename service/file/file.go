package file

import (
	"GopherAI/common/mysql"
	"GopherAI/common/rag"
	"GopherAI/config"
	"GopherAI/dao"
	"GopherAI/model"
	"GopherAI/utils"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
)

// UploadRagFile 上传RAG文件 - 已升级支持多文件模型
// 核心改动：
// 1. 不再删除用户目录中的旧文件（支持多文件）
// 2. 文件元数据写入 file_asset 表
// 3. 计算文件哈希用于后续去重
// 4. 文件状态从 uploaded 开始（为异步向量化做准备）
func UploadRagFile(userID int64, username string, file *multipart.FileHeader) (string, error) {
	ctx := context.Background()

	// 校验文件类型和文件名
	if err := utils.ValidateFile(file); err != nil {
		log.Printf("File validation failed: %v", err)
		return "", err
	}

	// 创建用户目录（暂时保留本地存储，后续可迁移到对象存储）
	userDir := filepath.Join("uploads", username)
	if err := os.MkdirAll(userDir, 0755); err != nil {
		log.Printf("Failed to create user directory %s: %v", userDir, err)
		return "", err
	}

	// 生成UUID作为文件唯一标识
	fileID := utils.GenerateUUID()
	ext := filepath.Ext(file.Filename)
	storageFileName := fileID + ext
	filePath := filepath.Join(userDir, storageFileName)

	// 打开上传的文件并计算哈希
	src, err := file.Open()
	if err != nil {
		log.Printf("Failed to open uploaded file: %v", err)
		return "", err
	}
	defer src.Close()

	// 计算文件SHA256哈希（用于去重）
	hash := sha256.New()
	fileSize, err := io.Copy(hash, src)
	if err != nil {
		log.Printf("Failed to calculate file hash: %v", err)
		return "", err
	}
	fileSHA256 := fmt.Sprintf("%x", hash.Sum(nil))

	// 秒传逻辑：检查是否已存在相同内容的文件
	fileDAO := dao.NewFileDAO(mysql.DB)
	existingFile, err := fileDAO.FindFileByHash(ctx, userID, fileSHA256)
	if err == nil && existingFile != nil {
		// 文件已存在，直接返回（秒传成功）
		log.Printf("File already exists (instant upload): %s", existingFile.ID)
		return existingFile.StorageKey, nil
	}

	// 重置文件指针以便后续保存
	src.Seek(0, 0)

	// 创建目标文件
	dst, err := os.Create(filePath)
	if err != nil {
		log.Printf("Failed to create destination file %s: %v", filePath, err)
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		log.Printf("Failed to copy file content: %v", err)
		return "", err
	}

	log.Printf("File uploaded successfully: %s", filePath)

	// 创建文件资产记录
	fileAsset := &model.FileAsset{
		ID:          fileID,
		OwnerID:     userID,
		KBID:        "", // 暂时为空，后续可扩展知识库功能
		FileName:    file.Filename,
		ContentType: file.Header.Get("Content-Type"),
		Size:        fileSize,
		SHA256:      fileSHA256,
		StorageKey:  filePath,
		Status:      model.FileStatusUploaded, // 标记为已上传（但未向量化）
		Version:     1,
	}

	// 写入数据库
	fileDAO = dao.NewFileDAO(mysql.DB)
	if err := fileDAO.CreateFile(ctx, fileAsset); err != nil {
		log.Printf("Failed to create file record: %v", err)
		os.Remove(filePath) // 回滚：删除已上传的文件
		return "", err
	}

	// 同步向量化（第一阶段暂时保留同步逻辑，后续改为异步任务）
	if err := vectorizeFile(ctx, fileID, storageFileName, filePath, fileDAO); err != nil {
		log.Printf("Failed to vectorize file: %v", err)
		fileDAO.UpdateFileStatus(ctx, fileID, model.FileStatusFailed, err.Error())
		return "", err
	}

	log.Printf("File indexed successfully: %s", fileID)
	return filePath, nil
}

// vectorizeFile 向量化文件（内部函数）
func vectorizeFile(ctx context.Context, fileID, storageFileName, filePath string, fileDAO *dao.FileDAO) error {
	// 更新状态为向量化中
	fileDAO.UpdateFileStatus(ctx, fileID, model.FileStatusVectorizing)

	// 创建 RAG 索引器
	indexer, err := rag.NewRAGIndexer(storageFileName, config.GetConfig().RagModelConfig.RagEmbeddingModel)
	if err != nil {
		return fmt.Errorf("failed to create RAG indexer: %w", err)
	}

	// 读取文件内容并创建向量索引
	if err := indexer.IndexFile(ctx, filePath); err != nil {
		return fmt.Errorf("failed to index file: %w", err)
	}

	// 更新状态为就绪
	return fileDAO.UpdateFileStatus(ctx, fileID, model.FileStatusReady)
}

// ListUserFiles 查询用户的所有文件
func ListUserFiles(userID int64) ([]*model.FileAsset, error) {
	ctx := context.Background()
	fileDAO := dao.NewFileDAO(mysql.DB)
	return fileDAO.ListFilesByOwner(ctx, userID)
}

// DeleteFile 删除文件（包括数据库记录和 Redis 索引）
func DeleteFile(userID int64, fileID string) error {
	ctx := context.Background()
	fileDAO := dao.NewFileDAO(mysql.DB)

	// 查询文件信息
	file, err := fileDAO.GetFileByID(ctx, fileID)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}

	// 验证所有权
	if file.OwnerID != userID {
		return fmt.Errorf("permission denied")
	}

	// 删除 Redis 索引（从 StorageKey 提取文件名）
	storageFileName := filepath.Base(file.StorageKey)
	if err := rag.DeleteIndex(ctx, storageFileName); err != nil {
		log.Printf("Failed to delete index for %s: %v", storageFileName, err)
		// 继续执行，不因索引删除失败而中断
	}

	// 删除本地文件
	if err := os.Remove(file.StorageKey); err != nil {
		log.Printf("Failed to delete file %s: %v", file.StorageKey, err)
	}

	// 软删除数据库记录
	return mysql.DB.Delete(&model.FileAsset{}, "id = ?", fileID).Error
}
