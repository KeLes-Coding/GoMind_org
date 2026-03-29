package controller

import (
	"GopherAI/common/metrics"
	"GopherAI/common/observability"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetMetrics 获取监控指标
func GetMetrics(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"file": metrics.File.Snapshot(),
		"ai":   observability.SnapshotAI(),
	})
}
