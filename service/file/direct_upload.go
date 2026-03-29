package file

import (
	"GopherAI/common/metrics"
	"GopherAI/common/mysql"
	"GopherAI/common/storage"
	"GopherAI/config"
	"GopherAI/dao"
	"GopherAI/model"
	"GopherAI/utils"
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"
)

const (
	// UploadModeForm 表示当前环境不支持对象存储直传，前端应继续走传统 multipart 上传。
	UploadModeForm = "form"
	// UploadModeDirect 表示后端已经生成对象存储预签名上传地址，前端可以直传对象存储。
	UploadModeDirect = "direct"
	// UploadModeInstant 表示命中了同内容文件复用，无需再次实际上传。
	UploadModeInstant = "instant"
)

// DirectUploadInitRequest 描述“初始化直传”所需的最小文件元信息。
// 这里不直接接收文件流，只接收签发上传地址所需的元信息。
type DirectUploadInitRequest struct {
	FileName    string
	FileSize    int64
	ContentType string
	SHA256      string
}

// DirectUploadInitResult 把直传初始化结果统一抽象成三种模式：
// 1. form：当前 provider 不支持直传，回退到原有 /upload；
// 2. direct：已经生成预签名上传地址；
// 3. instant：命中秒传复用，无需重新上传。
type DirectUploadInitResult struct {
	Mode             string
	FileAsset        *model.FileAsset
	Upload           *storage.PresignedUpload
	ExpiresInSeconds int
}

// InitDirectUpload 为对象存储直传做初始化。
// 它会先创建 pending_upload 状态的 file_asset 记录，再返回客户端直传所需的签名地址。
func InitDirectUpload(userID int64, req *DirectUploadInitRequest) (*DirectUploadInitResult, error) {
	ctx := context.Background()

	if req == nil {
		return nil, fmt.Errorf("direct upload request is required")
	}
	if err := validateDirectUploadRequest(req); err != nil {
		return nil, err
	}

	fileStorage, err := storage.GetStorage()
	if err != nil {
		return nil, fmt.Errorf("get storage failed: %w", err)
	}

	uploadSigner, ok := fileStorage.(storage.UploadSigner)
	if !ok {
		// local provider 故意不支持直传签名。
		// 这样单机默认环境仍然保持最简单的表单上传路径，不强迫每个 provider 都实现直传能力。
		return &DirectUploadInitResult{Mode: UploadModeForm}, nil
	}

	fileDAO := dao.NewFileDAO(mysql.DB)
	normalizedSHA := strings.ToLower(strings.TrimSpace(req.SHA256))
	existingFile, err := fileDAO.FindFileByHash(ctx, userID, normalizedSHA)
	if err == nil && existingFile != nil {
		return &DirectUploadInitResult{
			Mode:      UploadModeInstant,
			FileAsset: existingFile,
		}, nil
	}

	fileID := utils.GenerateUUID()
	objectKey := buildObjectKey(userID, fileID, extensionFromName(req.FileName))
	fileAsset := &model.FileAsset{
		ID:               fileID,
		OwnerID:          userID,
		KBID:             "",
		FileName:         req.FileName,
		ContentType:      req.ContentType,
		Size:             req.FileSize,
		SHA256:           normalizedSHA,
		StorageKey:       objectKey,
		Status:           model.FileStatusPendingUpload,
		Version:          1,
		VectorTaskQueued: false,
	}

	if err := fileDAO.CreateFile(ctx, fileAsset); err != nil {
		return nil, fmt.Errorf("create pending file record failed: %w", err)
	}

	expiry := getUploadPresignExpiry()
	presignedUpload, err := uploadSigner.PresignUpload(ctx, objectKey, expiry)
	if err != nil {
		// 如果签名失败，需要把刚创建的 pending 记录清掉，避免留下无效资产。
		if deleteErr := fileDAO.DeleteFile(ctx, fileID); deleteErr != nil {
			return nil, fmt.Errorf("presign upload failed: %w; cleanup failed: %v", err, deleteErr)
		}
		return nil, fmt.Errorf("presign upload failed: %w", err)
	}

	return &DirectUploadInitResult{
		Mode:             UploadModeDirect,
		FileAsset:        fileAsset,
		Upload:           presignedUpload,
		ExpiresInSeconds: int(expiry / time.Second),
	}, nil
}

// CompleteDirectUpload 在客户端直传完成后收口元数据和任务投递。
// 这样上传链路可以拆成“初始化 -> 客户端直传 -> 完成确认”三个阶段。
func CompleteDirectUpload(userID int64, fileID string) (*model.FileAsset, error) {
	ctx := context.Background()
	fileDAO := dao.NewFileDAO(mysql.DB)
	fileStorage, err := storage.GetStorage()
	if err != nil {
		return nil, fmt.Errorf("get storage failed: %w", err)
	}

	if _, ok := fileStorage.(storage.UploadSigner); !ok {
		return nil, ErrDirectUploadUnsupported
	}

	fileAsset, err := getOwnedFile(ctx, fileDAO, userID, fileID)
	if err != nil {
		return nil, err
	}

	// complete 接口需要天然支持幂等。
	// 如果前端或网关因为超时重复调用，不应重复投递任务或把状态写乱。
	switch fileAsset.Status {
	case model.FileStatusUploaded, model.FileStatusParsing, model.FileStatusVectorizing, model.FileStatusReady:
		return fileAsset, nil
	}

	if fileAsset.Status != model.FileStatusPendingUpload {
		return nil, fmt.Errorf("file status does not allow complete upload: %s", fileAsset.Status)
	}

	exists, err := fileStorage.Exists(ctx, fileAsset.StorageKey)
	if err != nil {
		return nil, fmt.Errorf("check uploaded object failed: %w", err)
	}
	if !exists {
		return nil, ErrUploadNotCompleted
	}

	if err := fileDAO.UpdateFileStatus(ctx, fileID, model.FileStatusUploaded); err != nil {
		return nil, fmt.Errorf("mark file uploaded failed: %w", err)
	}
	fileAsset.Status = model.FileStatusUploaded
	metrics.File.IncrUploaded()

	// 直传完成后，文件本体已经存在于对象存储，但“进入异步处理队列”仍然是独立的一跳。
	// 因此这里也必须走统一的补偿封装，确保当前版本是否成功入队有明确落库状态。
	if err := publishVectorizeTaskWithCompensation(ctx, fileDAO, fileAsset); err != nil {
		log.Printf("Failed to publish vectorize task after direct upload complete: %v", err)
	}

	return fileAsset, nil
}

func validateDirectUploadRequest(req *DirectUploadInitRequest) error {
	// 直传初始化没有 multipart.FileHeader，所以需要单独做元信息校验。
	// 这里和普通上传保持同样的文件大小、扩展名约束，避免两条路径标准不一致。
	if err := utils.ValidateFileMeta(req.FileName, req.FileSize); err != nil {
		return err
	}
	if strings.TrimSpace(req.SHA256) == "" {
		return fmt.Errorf("sha256 is required")
	}
	return nil
}

func extensionFromName(fileName string) string {
	return strings.ToLower(strings.TrimSpace(filepath.Ext(fileName)))
}

func getUploadPresignExpiry() time.Duration {
	// 上传预签名也需要短时有效，避免长时间暴露可写入地址。
	seconds := config.GetConfig().StorageConfig.UploadPresignExpirySeconds
	if seconds <= 0 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}
