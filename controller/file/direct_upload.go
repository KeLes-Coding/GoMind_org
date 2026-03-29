package file

import (
	"GopherAI/common/code"
	"GopherAI/controller"
	fileService "GopherAI/service/file"
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// DirectUploadInitRequest 对应“初始化对象存储直传”的 HTTP 请求体。
// 前端先传元信息，后端根据当前 storage provider 决定返回直传地址还是要求回退到普通上传。
type DirectUploadInitRequest struct {
	FileName    string `json:"file_name"`
	FileSize    int64  `json:"file_size"`
	ContentType string `json:"content_type"`
	SHA256      string `json:"sha256"`
}

type DirectUploadInitResponse struct {
	Mode             string            `json:"mode,omitempty"`
	FileID           string            `json:"file_id,omitempty"`
	StorageKey       string            `json:"storage_key,omitempty"`
	Status           string            `json:"status,omitempty"`
	UploadURL        string            `json:"upload_url,omitempty"`
	UploadMethod     string            `json:"upload_method,omitempty"`
	UploadHeaders    map[string]string `json:"upload_headers,omitempty"`
	ExpiresInSeconds int               `json:"expires_in_seconds,omitempty"`
	controller.Response
}

type DirectUploadCompleteRequest struct {
	FileID string `json:"file_id"`
}

type DirectUploadCompleteResponse struct {
	FileID     string `json:"file_id,omitempty"`
	StorageKey string `json:"storage_key,omitempty"`
	Status     string `json:"status,omitempty"`
	controller.Response
}

// InitDirectUpload 是对象存储直传的控制面入口。
// 它不接收文件流，只接收文件元信息，并根据当前 provider 返回：
// 1. form：继续走原有 multipart 上传；
// 2. direct：返回对象存储预签名上传地址；
// 3. instant：命中同内容文件复用。
func InitDirectUpload(c *gin.Context) {
	res := new(DirectUploadInitResponse)

	userID := c.GetInt64("userID")
	if userID == 0 {
		log.Println("UserID not found in context")
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
		return
	}

	req := new(DirectUploadInitRequest)
	if err := c.ShouldBindJSON(req); err != nil {
		log.Println("InitDirectUpload bind fail ", err)
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	result, err := fileService.InitDirectUpload(userID, &fileService.DirectUploadInitRequest{
		FileName:    req.FileName,
		FileSize:    req.FileSize,
		ContentType: req.ContentType,
		SHA256:      req.SHA256,
	})
	if err != nil {
		log.Println("InitDirectUpload fail ", err)
		c.JSON(http.StatusOK, res.CodeOf(code.CodeServerBusy))
		return
	}

	res.Success()
	res.Mode = result.Mode
	if result.FileAsset != nil {
		res.FileID = result.FileAsset.ID
		res.StorageKey = result.FileAsset.StorageKey
		res.Status = result.FileAsset.Status
	}
	if result.Upload != nil {
		res.UploadURL = result.Upload.URL
		res.UploadMethod = result.Upload.Method
		res.UploadHeaders = result.Upload.Headers
		res.ExpiresInSeconds = result.ExpiresInSeconds
	}
	c.JSON(http.StatusOK, res)
}

// CompleteDirectUpload 用于在客户端直传完成后收口状态。
// 后端会再次确认对象是否已经存在，只有确认存在后才把文件标记为 uploaded。
func CompleteDirectUpload(c *gin.Context) {
	res := new(DirectUploadCompleteResponse)

	userID := c.GetInt64("userID")
	if userID == 0 {
		log.Println("UserID not found in context")
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
		return
	}

	req := new(DirectUploadCompleteRequest)
	if err := c.ShouldBindJSON(req); err != nil {
		log.Println("CompleteDirectUpload bind fail ", err)
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}
	if req.FileID == "" {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	fileAsset, err := fileService.CompleteDirectUpload(userID, req.FileID)
	if err != nil {
		log.Println("CompleteDirectUpload fail ", err)
		c.JSON(http.StatusOK, res.CodeOf(mapDirectUploadErrorToCode(err)))
		return
	}

	res.Success()
	res.FileID = fileAsset.ID
	res.StorageKey = fileAsset.StorageKey
	res.Status = fileAsset.Status
	c.JSON(http.StatusOK, res)
}

func mapDirectUploadErrorToCode(err error) code.Code {
	switch {
	case errors.Is(err, fileService.ErrFileNotFound):
		return code.CodeRecordNotFound
	case errors.Is(err, fileService.ErrPermissionDenied):
		return code.CodeForbidden
	case errors.Is(err, fileService.ErrUploadNotCompleted):
		return code.CodeInvalidParams
	default:
		return code.CodeServerBusy
	}
}
