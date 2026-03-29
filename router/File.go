package router

import (
	"GopherAI/controller/file"

	"github.com/gin-gonic/gin"
)

func FileRouter(r *gin.RouterGroup) {
	r.POST("/upload/init", file.InitDirectUpload)
	r.POST("/upload/complete", file.CompleteDirectUpload)
	r.POST("/upload", file.UploadRagFile)
	r.POST("/retry/:fileId", file.RetryVectorizeFile)
	r.POST("/reindex/:fileId", file.ReindexFile)
	r.GET("/list", file.ListFiles)
	r.DELETE("/:fileId", file.DeleteFile)
	r.GET("/download/:fileId", file.DownloadFile)
}
