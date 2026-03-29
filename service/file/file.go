package file

import (
	"GopherAI/common/metrics"
	"GopherAI/common/mysql"
	"GopherAI/common/rag"
	"GopherAI/common/storage"
	"GopherAI/config"
	"GopherAI/dao"
	"GopherAI/model"
	"GopherAI/utils"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"path/filepath"
	"time"

	"gorm.io/gorm"
)

// DownloadResult 统一封装下载结果。
// 控制层只需要根据这里返回的是“预签名 URL”还是“流式 reader”来决定响应方式，
// 不需要关心底层到底是本地存储还是对象存储。
type DownloadResult struct {
	FileAsset    *model.FileAsset
	Reader       io.ReadCloser
	PresignedURL string
}

// UploadRagFile 负责完成普通 multipart 上传主链路。
// 这条链路的职责边界很明确：
// 1. 校验文件是否合法；
// 2. 计算内容哈希，支持同用户秒传复用；
// 3. 把文件本体写入统一 Storage；
// 4. 把元数据写入 MySQL；
// 5. 尝试投递异步向量化任务，并维护补偿状态。
func UploadRagFile(userID int64, file *multipart.FileHeader) (*model.FileAsset, error) {
	ctx := context.Background()

	if err := utils.ValidateFile(file); err != nil {
		log.Printf("File validation failed: %v", err)
		return nil, err
	}

	fileStorage, err := storage.GetStorage()
	if err != nil {
		return nil, fmt.Errorf("get storage failed: %w", err)
	}

	fileID := utils.GenerateUUID()
	objectKey := buildObjectKey(userID, fileID, filepath.Ext(file.Filename))

	src, err := file.Open()
	if err != nil {
		log.Printf("Failed to open uploaded file: %v", err)
		return nil, err
	}
	defer src.Close()

	// 先计算 SHA256，再决定是否需要真正上传文件内容。
	// 这样可以把“重复内容复用”前置到最便宜的判断阶段。
	hash := sha256.New()
	fileSize, err := io.Copy(hash, src)
	if err != nil {
		log.Printf("Failed to calculate file hash: %v", err)
		return nil, err
	}
	fileSHA256 := fmt.Sprintf("%x", hash.Sum(nil))

	fileDAO := dao.NewFileDAO(mysql.DB)
	existingFile, err := fileDAO.FindFileByHash(ctx, userID, fileSHA256)
	if err == nil && existingFile != nil {
		log.Printf("File already exists (instant upload): %s", existingFile.ID)
		return existingFile, nil
	}

	if _, err := src.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("reset uploaded file stream failed: %w", err)
	}

	if err := fileStorage.Upload(ctx, objectKey, src); err != nil {
		return nil, fmt.Errorf("upload file content failed: %w", err)
	}

	fileAsset := &model.FileAsset{
		ID:               fileID,
		OwnerID:          userID,
		KBID:             "",
		FileName:         file.Filename,
		ContentType:      file.Header.Get("Content-Type"),
		Size:             fileSize,
		SHA256:           fileSHA256,
		StorageKey:       objectKey,
		Status:           model.FileStatusUploaded,
		Version:          1,
		VectorTaskQueued: false,
	}

	if err := fileDAO.CreateFile(ctx, fileAsset); err != nil {
		log.Printf("Failed to create file record: %v", err)
		// 如果 DB 写元数据失败，需要主动补偿删除刚上传的文件本体，避免留下脏对象。
		if cleanupErr := fileStorage.Delete(ctx, objectKey); cleanupErr != nil {
			log.Printf("Failed to rollback uploaded file %s: %v", objectKey, cleanupErr)
		}
		return nil, fmt.Errorf("create file record failed: %w", err)
	}

	metrics.File.IncrUploaded()

	// 上传主链路在这里不再“只记日志”，而是显式维护任务投递状态。
	// 如果 MQ 这一跳失败，文件仍然保留为 uploaded，但会被标记为“当前版本未成功入队”，
	// 后台补偿 worker 就能稳定扫到这条记录并重新投递。
	if err := publishVectorizeTaskWithCompensation(ctx, fileDAO, fileAsset); err != nil {
		log.Printf("Failed to publish vectorize task: %v", err)
	}

	log.Printf("File uploaded and task publish attempted: %s", fileID)
	return fileAsset, nil
}

func ListUserFiles(userID int64) ([]*model.FileAsset, error) {
	ctx := context.Background()
	fileDAO := dao.NewFileDAO(mysql.DB)
	return fileDAO.ListFilesByOwner(ctx, userID)
}

// DeleteFile 删除文件的顺序是经过刻意设计的：
// 1. 先验证文件存在且属于当前用户；
// 2. 再删除 RAG 索引；
// 3. 再删除文件本体；
// 4. 最后删 DB 记录。
// 这样做的目的是尽量避免“DB 已删但文件还在 / 索引还在”的悬空状态。
func DeleteFile(userID int64, fileID string) error {
	ctx := context.Background()
	fileDAO := dao.NewFileDAO(mysql.DB)
	fileStorage, err := storage.GetStorage()
	if err != nil {
		return fmt.Errorf("get storage failed: %w", err)
	}

	fileAsset, err := getOwnedFile(ctx, fileDAO, userID, fileID)
	if err != nil {
		return err
	}

	storageFileName := filepath.Base(fileAsset.StorageKey)
	// 共享索引模式下，同一份文件的 chunk 不再通过“删整份文件索引”来清理，
	// 而是需要先按 file_id 精确删除对应文档，避免后续统一检索把脏 chunk 继续召回。
	if err := rag.DeleteIndexedFileDocuments(ctx, fileAsset.ID); err != nil {
		log.Printf("Failed to delete unified indexed documents for %s: %v", fileAsset.ID, err)
	}
	if err := rag.DeleteIndex(ctx, storageFileName); err != nil {
		// 索引删除失败当前仍然只记日志，不中断整个删除链路的后续步骤。
		// 原因是“用户删除文件”优先级更高，索引脏数据可以后续再做治理和补偿。
		log.Printf("Failed to delete index for %s: %v", storageFileName, err)
	}

	exists, err := fileStorage.Exists(ctx, fileAsset.StorageKey)
	if err != nil {
		return fmt.Errorf("check file content failed: %w", err)
	}
	if exists {
		if err := fileStorage.Delete(ctx, fileAsset.StorageKey); err != nil {
			return fmt.Errorf("delete file content failed: %w", err)
		}
	} else {
		// 删除操作天然需要幂等一点。
		// 如果对象本体已经不存在，不应再把整个删除请求判成失败。
		log.Printf("Skip deleting missing file content: %s", fileAsset.StorageKey)
	}

	if err := fileDAO.DeleteFile(ctx, fileID); err != nil {
		return fmt.Errorf("delete file record failed: %w", err)
	}
	return nil
}

// PrepareDownloadFile 统一封装下载前的业务决策：
// 1. 先做文件存在性和权限校验；
// 2. 对支持预签名的对象存储优先生成直链；
// 3. 如果不支持或生成失败，则自动降级为流式下载。
func PrepareDownloadFile(userID int64, fileID string) (*DownloadResult, error) {
	ctx := context.Background()
	fileDAO := dao.NewFileDAO(mysql.DB)
	fileStorage, err := storage.GetStorage()
	if err != nil {
		return nil, fmt.Errorf("get storage failed: %w", err)
	}

	fileAsset, err := getOwnedFile(ctx, fileDAO, userID, fileID)
	if err != nil {
		return nil, err
	}

	if signer, ok := fileStorage.(storage.DownloadSigner); ok {
		expiry := getDownloadPresignExpiry()
		presignedURL, err := signer.PresignDownload(ctx, fileAsset.StorageKey, fileAsset.FileName, fileAsset.ContentType, expiry)
		if err != nil {
			// 预签名失败不直接报错，而是降级为应用节点流式转发。
			// 这样可以在对象存储短时异常时继续保证下载可用性。
			log.Printf("Failed to presign download for %s, fallback to stream: %v", fileAsset.ID, err)
		} else {
			return &DownloadResult{
				FileAsset:    fileAsset,
				PresignedURL: presignedURL,
			}, nil
		}
	}

	reader, err := fileStorage.Download(ctx, fileAsset.StorageKey)
	if err != nil {
		return nil, fmt.Errorf("download file failed: %w", err)
	}
	return &DownloadResult{
		FileAsset: fileAsset,
		Reader:    reader,
	}, nil
}

func buildObjectKey(userID int64, fileID string, ext string) string {
	// object key 保持“业务稳定、物理无关”的命名策略。
	// 这样 local / minio / s3 之间切换时，上层元数据语义不需要变化。
	cleanExt := filepath.Ext("placeholder" + ext)
	if cleanExt == "" {
		return fmt.Sprintf("user/%d/%s", userID, fileID)
	}
	return fmt.Sprintf("user/%d/%s%s", userID, fileID, cleanExt)
}

func getOwnedFile(ctx context.Context, fileDAO *dao.FileDAO, userID int64, fileID string) (*model.FileAsset, error) {
	fileAsset, err := fileDAO.GetFileByID(ctx, fileID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFileNotFound
		}
		return nil, fmt.Errorf("get file record failed: %w", err)
	}
	if fileAsset.OwnerID != userID {
		return nil, ErrPermissionDenied
	}
	return fileAsset, nil
}

func getDownloadPresignExpiry() time.Duration {
	// 预签名 URL 的有效期需要兼顾安全与体验：
	// 太短会导致用户刚点击就过期，太长又会放大凭证泄露窗口。
	// 这里默认 5 分钟，并允许通过配置覆盖。
	seconds := config.GetConfig().StorageConfig.PresignExpirySeconds
	if seconds <= 0 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}
