package session

import (
	"GopherAI/common/code"
	"GopherAI/controller"
	"GopherAI/model"
	sessionService "GopherAI/service/session"
	"net/http"

	"github.com/gin-gonic/gin"
)

type (
	GetSessionTreeResponse struct {
		controller.Response
		Tree *model.SessionTree `json:"tree,omitempty"`
	}

	CreateFolderRequest struct {
		Name string `json:"name" binding:"required"`
	}

	CreateFolderResponse struct {
		controller.Response
		Folder *model.SessionFolder `json:"folder,omitempty"`
	}

	RenameFolderRequest struct {
		FolderID string `json:"folderId" binding:"required"`
		Name     string `json:"name" binding:"required"`
	}

	DeleteFolderRequest struct {
		FolderID string `json:"folderId" binding:"required"`
	}

	MoveSessionRequest struct {
		SessionID string `json:"sessionId" binding:"required"`
		FolderID  string `json:"folderId" binding:"required"`
	}

	RemoveSessionFromFolderRequest struct {
		SessionID string `json:"sessionId" binding:"required"`
	}

	RenameSessionRequest struct {
		SessionID string `json:"sessionId" binding:"required"`
		Title     string `json:"title" binding:"required"`
	}

	DeleteSessionRequest struct {
		SessionID string `json:"sessionId" binding:"required"`
	}
)

func GetSessionTree(c *gin.Context) {
	res := new(GetSessionTreeResponse)
	userID := c.GetInt64("userID")
	userName := c.GetString("userName")

	tree, code_ := sessionService.GetSessionTree(userID, userName)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	res.Tree = tree
	c.JSON(http.StatusOK, res)
}

func CreateFolder(c *gin.Context) {
	req := new(CreateFolderRequest)
	res := new(CreateFolderResponse)
	userID := c.GetInt64("userID")
	userName := c.GetString("userName")
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	folder, code_ := sessionService.CreateFolder(userID, userName, req.Name)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	res.Folder = folder
	c.JSON(http.StatusOK, res)
}

func RenameFolder(c *gin.Context) {
	req := new(RenameFolderRequest)
	res := new(controller.Response)
	userID := c.GetInt64("userID")
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	code_ := sessionService.RenameFolder(userID, req.FolderID, req.Name)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	c.JSON(http.StatusOK, res)
}

func DeleteFolder(c *gin.Context) {
	req := new(DeleteFolderRequest)
	res := new(controller.Response)
	userID := c.GetInt64("userID")
	userName := c.GetString("userName")
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	code_ := sessionService.DeleteFolder(userID, userName, req.FolderID)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	c.JSON(http.StatusOK, res)
}

func MoveSessionToFolder(c *gin.Context) {
	req := new(MoveSessionRequest)
	res := new(controller.Response)
	userID := c.GetInt64("userID")
	userName := c.GetString("userName")
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	code_ := sessionService.MoveSessionToFolder(userID, userName, req.SessionID, req.FolderID)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	c.JSON(http.StatusOK, res)
}

func RemoveSessionFromFolder(c *gin.Context) {
	req := new(RemoveSessionFromFolderRequest)
	res := new(controller.Response)
	userName := c.GetString("userName")
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	code_ := sessionService.RemoveSessionFromFolder(userName, req.SessionID)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	c.JSON(http.StatusOK, res)
}

func RenameSession(c *gin.Context) {
	req := new(RenameSessionRequest)
	res := new(controller.Response)
	userName := c.GetString("userName")
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	code_ := sessionService.RenameSession(userName, req.SessionID, req.Title)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	c.JSON(http.StatusOK, res)
}

func DeleteSession(c *gin.Context) {
	req := new(DeleteSessionRequest)
	res := new(controller.Response)
	userName := c.GetString("userName")
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	code_ := sessionService.DeleteSession(userName, req.SessionID)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	c.JSON(http.StatusOK, res)
}
