package handlers

import (
	"github.com/gin-gonic/gin"
	"vte/internal/logger"
)

func GetLogs(c *gin.Context) {
	logs := logger.GetLogs()
	c.JSON(200, gin.H{"logs": logs})
}

func ClearLogs(c *gin.Context) {
	logger.ClearLogs()
	c.JSON(200, gin.H{"message": "日志已清空"})
}

func GetStats(c *gin.Context) {
	stats := logger.GetStats()
	c.JSON(200, stats)
}

func ResetStats(c *gin.Context) {
	logger.ResetStats()
	c.JSON(200, gin.H{"message": "统计已重置"})
}
