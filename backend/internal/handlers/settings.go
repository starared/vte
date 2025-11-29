package handlers

import (
	"fmt"
	"strconv"

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

// GetRetrySettings 获取重试设置
func GetRetrySettings(c *gin.Context) {
	db := database.DB()
	var maxRetries string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'max_retries'").Scan(&maxRetries)
	if err != nil {
		maxRetries = "3" // 默认3次
	}
	retries, _ := strconv.Atoi(maxRetries)
	c.JSON(200, gin.H{"max_retries": retries})
}

// SetRetrySettings 设置重试次数
func SetRetrySettings(c *gin.Context) {
	var req models.RetrySettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}

	// 验证范围 0-10
	if req.MaxRetries < 0 || req.MaxRetries > 10 {
		c.JSON(400, gin.H{"detail": "重试次数必须在 0-10 之间"})
		return
	}

	db := database.DB()
	_, err := db.Exec(`
		INSERT INTO settings (key, value) VALUES ('max_retries', ?)
		ON CONFLICT(key) DO UPDATE SET value = ?
	`, strconv.Itoa(req.MaxRetries), strconv.Itoa(req.MaxRetries))

	if err != nil {
		c.JSON(500, gin.H{"detail": "保存失败"})
		return
	}

	logger.Info(fmt.Sprintf("%s | 更新重试次数 | %d", c.ClientIP(), req.MaxRetries))
	c.JSON(200, gin.H{"message": "设置已更新"})
}

// GetThemeSettings 获取主题设置
func GetThemeSettings(c *gin.Context) {
	db := database.DB()
	var theme string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'theme'").Scan(&theme)
	if err != nil {
		theme = "light" // 默认亮色
	}
	c.JSON(200, gin.H{"theme": theme})
}

// SetThemeSettings 设置主题
func SetThemeSettings(c *gin.Context) {
	var req models.ThemeSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}

	// 验证主题
	validThemes := map[string]bool{"light": true, "dark": true, "auto": true}
	if !validThemes[req.Theme] {
		c.JSON(400, gin.H{"detail": "无效的主题"})
		return
	}

	db := database.DB()
	_, err := db.Exec(`
		INSERT INTO settings (key, value) VALUES ('theme', ?)
		ON CONFLICT(key) DO UPDATE SET value = ?
	`, req.Theme, req.Theme)

	if err != nil {
		c.JSON(500, gin.H{"detail": "保存失败"})
		return
	}

	logger.Info(fmt.Sprintf("%s | 更新主题 | %s", c.ClientIP(), req.Theme))
	c.JSON(200, gin.H{"message": "设置已更新"})
}
