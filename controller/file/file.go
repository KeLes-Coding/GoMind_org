package file

import (
	"GopherAI/common/code"
	"GopherAI/controller"
	"GopherAI/model"
	"GopherAI/service/file"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type (
	UploadFileResponse struct {
		FilePath string `json:"file_path,omitempty"`
		controller.Response
	}

	ListFilesResponse struct {
		Files []*model.FileAsset `json:"files"`
		controller.Response
	}

	DeleteFileResponse struct {
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

	username := c.GetString("userName")
	if username == "" {
		log.Println("Username not found in context")
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
		return
	}

	// 获取用户ID（从上下文中获取）
	userID := c.GetInt64("userID")
	if userID == 0 {
		log.Println("UserID not found in context")
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
		return
	}

	// 调用 service 层，传入 userID 和 username
	filePath, err := file.UploadRagFile(userID, username, uploadedFile)
	if err != nil {
		log.Println("UploadFile fail ", err)
		c.JSON(http.StatusOK, res.CodeOf(code.CodeServerBusy))
		return
	}

	res.Success()
	res.FilePath = filePath
	c.JSON(http.StatusOK, res)
}

// ListFiles 查询用户的所有文件
func ListFiles(c *gin.Context) {
	res := new(ListFilesResponse)

	userID := c.GetInt64("userID")
	if userID == 0 {
		log.Println("UserID not found in context")
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
		return
	}

	files, err := file.ListUserFiles(userID)
	if err != nil {
		log.Println("ListUserFiles fail ", err)
		c.JSON(http.StatusOK, res.CodeOf(code.CodeServerBusy))
		return
	}

	res.Success()
	res.Files = files
	c.JSON(http.StatusOK, res)
}

// DeleteFile 删除文件
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

	if err := file.DeleteFile(userID, fileID); err != nil {
		log.Println("DeleteFile fail ", err)
		c.JSON(http.StatusOK, res.CodeOf(code.CodeServerBusy))
		return
	}

	res.Success()
	c.JSON(http.StatusOK, res)
}
