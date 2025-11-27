package handlers

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"vte/internal/database"
	"vte/internal/logger"
	"vte/internal/models"
)

func GetStreamMode(c *gin.Context) {
	db := database.DB()
	var mode string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'stream_mode'").Scan(&mode)
	if err != nil {
		mode = "auto"
	}
	c.JSON(200, gin.H{"mode": mode})
}

func SetStreamMode(c *gin.Context) {
	var req models.StreamModeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}

	// 验证模式
	validModes := map[string]bool{"auto": true, "force_stream": true, "force_non_stream": true}
	if !validModes[req.Mode] {
		c.JSON(400, gin.H{"detail": "无效的模式"})
		return
	}

	db := database.DB()
	_, err := db.Exec(`
		INSERT INTO settings (key, value) VALUES ('stream_mode', ?)
		ON CONFLICT(key) DO UPDATE SET value = ?
	`, req.Mode, req.Mode)

	if err != nil {
		c.JSON(500, gin.H{"detail": "保存失败"})
		return
	}

	logger.Info(fmt.Sprintf("%s | 更新流式模式 | %s", c.ClientIP(), req.Mode))
	c.JSON(200, gin.H{"message": "设置已更新"})
}
