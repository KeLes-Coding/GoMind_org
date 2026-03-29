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

type FileManageResponse struct {
	FileID     string `json:"file_id,omitempty"`
	Status     string `json:"status,omitempty"`
	Version    int    `json:"version,omitempty"`
	StorageKey string `json:"storage_key,omitempty"`
	controller.Response
}

// RetryVectorizeFile 用于手动重试失败或未完成的向量化任务。
// 它不会新建文件版本，只是把同一版本重新投递给 worker。
func RetryVectorizeFile(c *gin.Context) {
	res := new(FileManageResponse)

	userID := c.GetInt64("userID")
	if userID == 0 {
		log.Println("UserID not found in context")
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
		return
	}

	fileID := c.Param("fileId")
	if fileID == "" {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	fileAsset, err := fileService.RetryVectorizeFile(userID, fileID)
	if err != nil {
		log.Println("RetryVectorizeFile fail ", err)
		c.JSON(http.StatusOK, res.CodeOf(mapManageFileErrorToCode(err)))
		return
	}

	res.Success()
	res.FileID = fileAsset.ID
	res.Status = fileAsset.Status
	res.Version = fileAsset.Version
	res.StorageKey = fileAsset.StorageKey
	c.JSON(http.StatusOK, res)
}

// ReindexFile 用于手动触发“重建索引”。
// 它会先删除旧索引，再把文件版本号加一，然后重新投递向量化任务。
func ReindexFile(c *gin.Context) {
	res := new(FileManageResponse)

	userID := c.GetInt64("userID")
	if userID == 0 {
		log.Println("UserID not found in context")
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
		return
	}

	fileID := c.Param("fileId")
	if fileID == "" {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	fileAsset, err := fileService.ReindexFile(userID, fileID)
	if err != nil {
		log.Println("ReindexFile fail ", err)
		c.JSON(http.StatusOK, res.CodeOf(mapManageFileErrorToCode(err)))
		return
	}

	res.Success()
	res.FileID = fileAsset.ID
	res.Status = fileAsset.Status
	res.Version = fileAsset.Version
	res.StorageKey = fileAsset.StorageKey
	c.JSON(http.StatusOK, res)
}

func mapManageFileErrorToCode(err error) code.Code {
	switch {
	case errors.Is(err, fileService.ErrFileNotFound):
		return code.CodeRecordNotFound
	case errors.Is(err, fileService.ErrPermissionDenied):
		return code.CodeForbidden
	case errors.Is(err, fileService.ErrRetryNotAllowed):
		return code.CodeInvalidParams
	case errors.Is(err, fileService.ErrReindexNotAllowed):
		return code.CodeInvalidParams
	default:
		return code.CodeServerBusy
	}
}
