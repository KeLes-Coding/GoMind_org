package file

import (
	"GopherAI/common/metrics"
	"GopherAI/common/mysql"
	"GopherAI/common/rag"
	"GopherAI/dao"
	"GopherAI/model"
	"GopherAI/service/task"
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

	// 记录上传指标
	metrics.File.IncrUploaded()

	// 异步向量化：投递任务到 RabbitMQ（第二阶段改造）
	if err := task.PublishVectorizeTask(ctx, fileID, 1); err != nil {
		log.Printf("Failed to publish vectorize task: %v", err)
		// 任务投递失败不影响上传成功，后续可以手动重试
	}

	log.Printf("File uploaded and task published: %s", fileID)
	return filePath, nil
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
