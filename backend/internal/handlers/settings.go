package handlers

import (
	"encoding/json"
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

// GetSystemPrompt 获取系统前置提示词
func GetSystemPrompt(c *gin.Context) {
	db := database.DB()
	var prompt, enabled string
	db.QueryRow("SELECT value FROM settings WHERE key = 'system_prompt'").Scan(&prompt)
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'system_prompt_enabled'").Scan(&enabled)
	if err != nil {
		enabled = "false"
	}
	c.JSON(200, gin.H{
		"prompt":  prompt,
		"enabled": enabled == "true",
	})
}

// SetSystemPrompt 设置系统前置提示词
func SetSystemPrompt(c *gin.Context) {
	var req models.SystemPromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}

	db := database.DB()
	
	// 保存提示词内容
	_, err := db.Exec(`
		INSERT INTO settings (key, value) VALUES ('system_prompt', ?)
		ON CONFLICT(key) DO UPDATE SET value = ?
	`, req.Prompt, req.Prompt)
	if err != nil {
		c.JSON(500, gin.H{"detail": "保存失败"})
		return
	}

	// 保存启用状态
	enabledStr := "false"
	if req.Enabled {
		enabledStr = "true"
	}
	_, err = db.Exec(`
		INSERT INTO settings (key, value) VALUES ('system_prompt_enabled', ?)
		ON CONFLICT(key) DO UPDATE SET value = ?
	`, enabledStr, enabledStr)
	if err != nil {
		c.JSON(500, gin.H{"detail": "保存失败"})
		return
	}

	logger.Info(fmt.Sprintf("%s | 更新系统提示词 | 启用=%v 长度=%d", c.ClientIP(), req.Enabled, len(req.Prompt)))
	c.JSON(200, gin.H{"message": "设置已更新"})
}

// GetCustomErrorResponse 获取自定义错误响应设置
func GetCustomErrorResponse(c *gin.Context) {
	db := database.DB()
	var enabled, rulesJSON string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'custom_error_enabled'").Scan(&enabled)
	if err != nil {
		enabled = "false"
	}
	err = db.QueryRow("SELECT value FROM settings WHERE key = 'custom_error_rules'").Scan(&rulesJSON)
	if err != nil {
		rulesJSON = "[]"
	}
	
	var rules []models.CustomErrorRule
	json.Unmarshal([]byte(rulesJSON), &rules)
	
	c.JSON(200, gin.H{
		"enabled": enabled == "true",
		"rules":   rules,
	})
}

// SetCustomErrorResponse 设置自定义错误响应
func SetCustomErrorResponse(c *gin.Context) {
	var req models.CustomErrorResponseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}

	db := database.DB()
	
	// 保存启用状态
	enabledStr := "false"
	if req.Enabled {
		enabledStr = "true"
	}
	_, err := db.Exec(`
		INSERT INTO settings (key, value) VALUES ('custom_error_enabled', ?)
		ON CONFLICT(key) DO UPDATE SET value = ?
	`, enabledStr, enabledStr)
	if err != nil {
		c.JSON(500, gin.H{"detail": "保存失败"})
		return
	}

	// 保存规则
	rulesJSON, _ := json.Marshal(req.Rules)
	_, err = db.Exec(`
		INSERT INTO settings (key, value) VALUES ('custom_error_rules', ?)
		ON CONFLICT(key) DO UPDATE SET value = ?
	`, string(rulesJSON), string(rulesJSON))
	if err != nil {
		c.JSON(500, gin.H{"detail": "保存失败"})
		return
	}

	logger.Info(fmt.Sprintf("%s | 更新自定义错误响应 | 启用=%v 规则数=%d", c.ClientIP(), req.Enabled, len(req.Rules)))
	c.JSON(200, gin.H{"message": "设置已更新"})
}

// GetRateLimitSettings 获取速率限制设置
func GetRateLimitSettings(c *gin.Context) {
	db := database.DB()
	var enabled, maxRequests, window string
	
	db.QueryRow("SELECT value FROM settings WHERE key = 'rate_limit_enabled'").Scan(&enabled)
	db.QueryRow("SELECT value FROM settings WHERE key = 'rate_limit_max_requests'").Scan(&maxRequests)
	db.QueryRow("SELECT value FROM settings WHERE key = 'rate_limit_window'").Scan(&window)
	
	maxReq, _ := strconv.Atoi(maxRequests)
	windowSec, _ := strconv.Atoi(window)
	
	if maxReq <= 0 {
		maxReq = 60
	}
	if windowSec <= 0 {
		windowSec = 60
	}
	
	c.JSON(200, gin.H{
		"enabled":      enabled == "true",
		"max_requests": maxReq,
		"window":       windowSec,
	})
}

// SetRateLimitSettings 设置速率限制
func SetRateLimitSettings(c *gin.Context) {
	var req models.RateLimitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}
	
	db := database.DB()
	
	enabledStr := "false"
	if req.Enabled {
		enabledStr = "true"
	}
	
	db.Exec(`INSERT INTO settings (key, value) VALUES ('rate_limit_enabled', ?) ON CONFLICT(key) DO UPDATE SET value = ?`, enabledStr, enabledStr)
	db.Exec(`INSERT INTO settings (key, value) VALUES ('rate_limit_max_requests', ?) ON CONFLICT(key) DO UPDATE SET value = ?`, strconv.Itoa(req.MaxRequests), strconv.Itoa(req.MaxRequests))
	db.Exec(`INSERT INTO settings (key, value) VALUES ('rate_limit_window', ?) ON CONFLICT(key) DO UPDATE SET value = ?`, strconv.Itoa(req.Window), strconv.Itoa(req.Window))
	
	logger.Info(fmt.Sprintf("%s | 更新速率限制 | 启用=%v 最大请求=%d 窗口=%ds", c.ClientIP(), req.Enabled, req.MaxRequests, req.Window))
	c.JSON(200, gin.H{"message": "设置已更新"})
}

// GetConcurrencySettings 获取并发限制设置
func GetConcurrencySettings(c *gin.Context) {
	db := database.DB()
	var enabled, limit string
	
	db.QueryRow("SELECT value FROM settings WHERE key = 'concurrency_enabled'").Scan(&enabled)
	db.QueryRow("SELECT value FROM settings WHERE key = 'concurrency_limit'").Scan(&limit)
	
	limitNum, _ := strconv.Atoi(limit)
	if limitNum <= 0 {
		limitNum = 10
	}
	
	c.JSON(200, gin.H{
		"enabled":     enabled == "true",
		"limit":       limitNum,
		"current":     GetCurrentConcurrency(),
	})
}

// SetConcurrencySettings 设置并发限制
func SetConcurrencySettings(c *gin.Context) {
	var req models.ConcurrencyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}
	
	db := database.DB()
	
	enabledStr := "false"
	if req.Enabled {
		enabledStr = "true"
	}
	
	db.Exec(`INSERT INTO settings (key, value) VALUES ('concurrency_enabled', ?) ON CONFLICT(key) DO UPDATE SET value = ?`, enabledStr, enabledStr)
	db.Exec(`INSERT INTO settings (key, value) VALUES ('concurrency_limit', ?) ON CONFLICT(key) DO UPDATE SET value = ?`, strconv.Itoa(req.Limit), strconv.Itoa(req.Limit))
	
	logger.Info(fmt.Sprintf("%s | 更新并发限制 | 启用=%v 限制=%d", c.ClientIP(), req.Enabled, req.Limit))
	c.JSON(200, gin.H{"message": "设置已更新"})
}

// GetCustomRateLimitRules 获取自定义速率限制规则
func GetCustomRateLimitRules(c *gin.Context) {
	db := database.DB()
	var rulesJSON string
	
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'custom_rate_limit_rules'").Scan(&rulesJSON)
	if err != nil || rulesJSON == "" {
		c.JSON(200, gin.H{"rules": []interface{}{}})
		return
	}
	
	var rules []map[string]interface{}
	json.Unmarshal([]byte(rulesJSON), &rules)
	
	// 补充提供商名称
	for i, rule := range rules {
		if providerID, ok := rule["provider_id"].(float64); ok && providerID > 0 {
			var name string
			db.QueryRow("SELECT name FROM providers WHERE id = ?", int(providerID)).Scan(&name)
			rules[i]["provider_name"] = name
		}
	}
	
	c.JSON(200, gin.H{"rules": rules})
}

// SetCustomRateLimitRules 设置自定义速率限制规则
func SetCustomRateLimitRules(c *gin.Context) {
	var req struct {
		Rules []struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			ProviderID  int    `json:"provider_id"`
			ModelName   string `json:"model_name"`
			MaxRequests int    `json:"max_requests"`
			Window      int    `json:"window"`
			Enabled     bool   `json:"enabled"`
		} `json:"rules"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}
	
	rulesJSON, _ := json.Marshal(req.Rules)
	
	db := database.DB()
	db.Exec(`INSERT INTO settings (key, value) VALUES ('custom_rate_limit_rules', ?) ON CONFLICT(key) DO UPDATE SET value = ?`, string(rulesJSON), string(rulesJSON))
	
	logger.Info(fmt.Sprintf("%s | 更新自定义速率限制规则 | %d条规则", c.ClientIP(), len(req.Rules)))
	c.JSON(200, gin.H{"message": "设置已更新"})
}