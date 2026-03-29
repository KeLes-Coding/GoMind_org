package file

import (
	"GopherAI/common/code"
	"GopherAI/controller"
	"GopherAI/model"
	fileService "GopherAI/service/file"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

type (
	UploadFileResponse struct {
		FileID     string `json:"file_id,omitempty"`
		FilePath   string `json:"file_path,omitempty"`
		StorageKey string `json:"storage_key,omitempty"`
		Status     string `json:"status,omitempty"`
		controller.Response
	}

	ListFilesResponse struct {
		Files []*model.FileAsset `json:"files"`
		controller.Response
	}

	DeleteFileResponse struct {
		controller.Response
	}

	DownloadFileResponse struct {
		controller.Response
	}
)

func UploadRagFile(c *gin.Context) {
	res := new(UploadFileResponse)
	uploadedFile, err := c.FormFile("file")
	if err != nil {
		log.Println("FormFile fail ", err)
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	userID := c.GetInt64("userID")
	if userID == 0 {
		log.Println("UserID not found in context")
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
		return
	}

	fileAsset, err := fileService.UploadRagFile(userID, uploadedFile)
	if err != nil {
		log.Println("UploadFile fail ", err)
		c.JSON(http.StatusOK, res.CodeOf(code.CodeServerBusy))
		return
	}

	res.Success()
	res.FileID = fileAsset.ID
	res.FilePath = fileAsset.StorageKey
	res.StorageKey = fileAsset.StorageKey
	res.Status = fileAsset.Status
	c.JSON(http.StatusOK, res)
}

func ListFiles(c *gin.Context) {
	res := new(ListFilesResponse)

	userID := c.GetInt64("userID")
	if userID == 0 {
		log.Println("UserID not found in context")
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
		return
	}

	files, err := fileService.ListUserFiles(userID)
	if err != nil {
		log.Println("ListUserFiles fail ", err)
		c.JSON(http.StatusOK, res.CodeOf(code.CodeServerBusy))
		return
	}

	res.Success()
	res.Files = files
	c.JSON(http.StatusOK, res)
}

func DeleteFile(c *gin.Context) {
	res := new(DeleteFileResponse)

	userID := c.GetInt64("userID")
	if userID == 0 {
		log.Println("UserID not found in context")
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
		return
	}

	fileID := c.Param("fileId")
	if fileID == "" {
		log.Println("FileID not provided")
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	if err := fileService.DeleteFile(userID, fileID); err != nil {
		log.Println("DeleteFile fail ", err)
		c.JSON(http.StatusOK, res.CodeOf(mapFileErrorToCode(err)))
		return
	}

	res.Success()
	c.JSON(http.StatusOK, res)
}

func DownloadFile(c *gin.Context) {
	res := new(DownloadFileResponse)

	userID := c.GetInt64("userID")
	if userID == 0 {
		log.Println("UserID not found in context")
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
		return
	}

	fileID := c.Param("fileId")
	if fileID == "" {
		log.Println("FileID not provided")
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	downloadResult, err := fileService.PrepareDownloadFile(userID, fileID)
	if err != nil {
		log.Println("DownloadFile fail ", err)
		c.JSON(http.StatusOK, res.CodeOf(mapFileErrorToCode(err)))
		return
	}

	// 对象存储场景优先返回短时预签名 URL。
	// 这样客户端直接向对象存储下载，应用节点不再承担大文件中转带宽。
	if downloadResult.PresignedURL != "" {
		c.Redirect(http.StatusFound, downloadResult.PresignedURL)
		return
	}

	reader := downloadResult.Reader
	fileAsset := downloadResult.FileAsset
	defer reader.Close()

	contentType := fileAsset.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(fileAsset.FileName))
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, reader); err != nil {
		log.Println("DownloadFile stream fail ", err)
	}
}

func mapFileErrorToCode(err error) code.Code {
	switch {
	case errors.Is(err, fileService.ErrFileNotFound):
		return code.CodeRecordNotFound
	case errors.Is(err, fileService.ErrPermissionDenied):
		return code.CodeForbidden
	default:
		return code.CodeServerBusy
	}
}
