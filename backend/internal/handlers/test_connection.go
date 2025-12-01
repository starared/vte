package handlers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"vte/internal/database"
	"vte/internal/logger"
	"vte/internal/models"
	"vte/internal/proxy"
)

// TestConnection 测试提供商连接
func TestConnection(c *gin.Context) {
	providerID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"detail": "无效的提供商ID"})
		return
	}

	var req models.TestConnectionRequest
	c.ShouldBindJSON(&req)

	db := database.DB()

	// 获取提供商信息
	var provider struct {
		Name           string
		BaseURL        string
		APIKey         string
		ProviderType   string
		VertexProject  string
		VertexLocation string
		ExtraHeaders   *string
		ProxyURL       string
	}
	err = db.QueryRow(`
		SELECT name, base_url, api_key, provider_type, 
		       COALESCE(vertex_project, ''), COALESCE(vertex_location, 'global'),
		       extra_headers, COALESCE(proxy_url, '')
		FROM providers WHERE id = ?
	`, providerID).Scan(
		&provider.Name, &provider.BaseURL, &provider.APIKey, &provider.ProviderType,
		&provider.VertexProject, &provider.VertexLocation, &provider.ExtraHeaders, &provider.ProxyURL,
	)
	if err != nil {
		c.JSON(404, gin.H{"detail": "提供商不存在"})
		return
	}

	// 确定使用哪个 API Key
	apiKey := provider.APIKey
	apiKeyName := "默认密钥"
	if req.APIKeyID != nil && *req.APIKeyID > 0 {
		var keyInfo struct {
			APIKey string
			Name   string
		}
		err = db.QueryRow("SELECT api_key, name FROM provider_api_keys WHERE id = ? AND provider_id = ?",
			*req.APIKeyID, providerID).Scan(&keyInfo.APIKey, &keyInfo.Name)
		if err == nil {
			apiKey = keyInfo.APIKey
			apiKeyName = keyInfo.Name
		}
	}

	// 确定使用哪个模型
	var modelOriginalID, modelDisplayName string
	if req.ModelID != nil && *req.ModelID > 0 {
		err = db.QueryRow("SELECT original_id, display_name FROM models WHERE id = ? AND provider_id = ? AND is_active = 1",
			*req.ModelID, providerID).Scan(&modelOriginalID, &modelDisplayName)
		if err != nil {
			c.JSON(400, gin.H{"detail": "指定的模型不存在或未启用"})
			return
		}
	} else {
		// 使用第一个启用的模型
		err = db.QueryRow("SELECT original_id, display_name FROM models WHERE provider_id = ? AND is_active = 1 ORDER BY id LIMIT 1",
			providerID).Scan(&modelOriginalID, &modelDisplayName)
		if err != nil {
			c.JSON(400, gin.H{"detail": "没有启用的模型可供测试"})
			return
		}
	}

	// 构建配置
	cfg := &proxy.ProviderConfig{
		BaseURL:        provider.BaseURL,
		APIKey:         apiKey,
		ProviderType:   provider.ProviderType,
		VertexProject:  provider.VertexProject,
		VertexLocation: provider.VertexLocation,
		ProxyURL:       provider.ProxyURL,
	}

	if provider.ExtraHeaders != nil && *provider.ExtraHeaders != "" {
		json.Unmarshal([]byte(*provider.ExtraHeaders), &cfg.ExtraHeaders)
	}

	// 处理 Vertex Express 模型名
	testModelID := modelOriginalID
	if provider.ProviderType == "vertex_express" && len(testModelID) > 0 {
		if len(testModelID) < 7 || testModelID[:7] != "google/" {
			testModelID = "google/" + testModelID
		}
	}

	// 构建测试请求
	payload := map[string]interface{}{
		"model": testModelID,
		"messages": []map[string]string{
			{"role": "user", "content": "Hi"},
		},
		"max_tokens": 5,
	}

	startTime := time.Now()
	result, err := cfg.ChatCompletionWithRetry(payload, 1) // 只重试1次
	duration := time.Since(startTime).Milliseconds()

	if err != nil {
		logger.Error(fmt.Sprintf("%s | 测试连接失败 | %s | %s | %v", c.ClientIP(), provider.Name, modelDisplayName, err))
		c.JSON(200, gin.H{
			"success":      false,
			"message":      fmt.Sprintf("连接失败: %v", err),
			"duration_ms":  duration,
			"model":        modelDisplayName,
			"api_key_name": apiKeyName,
		})
		return
	}

	// 检查响应
	responseText := ""
	if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if message, ok := choice["message"].(map[string]interface{}); ok {
				if content, ok := message["content"].(string); ok {
					responseText = content
				}
			}
		}
	}

	logger.Info(fmt.Sprintf("%s | 测试连接成功 | %s | %s | %dms", c.ClientIP(), provider.Name, modelDisplayName, duration))
	c.JSON(200, gin.H{
		"success":      true,
		"message":      "连接成功",
		"duration_ms":  duration,
		"model":        modelDisplayName,
		"api_key_name": apiKeyName,
		"response":     responseText,
	})
}

// GetTestOptions 获取测试选项（可用的模型和密钥列表）
func GetTestOptions(c *gin.Context) {
	providerID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"detail": "无效的提供商ID"})
		return
	}

	db := database.DB()

	// 获取启用的模型
	modelRows, err := db.Query(`
		SELECT id, original_id, display_name FROM models 
		WHERE provider_id = ? AND is_active = 1 ORDER BY id
	`, providerID)
	if err != nil {
		c.JSON(500, gin.H{"detail": "查询失败"})
		return
	}
	defer modelRows.Close()

	var models []gin.H
	for modelRows.Next() {
		var id int
		var originalID, displayName string
		modelRows.Scan(&id, &originalID, &displayName)
		models = append(models, gin.H{
			"id":           id,
			"original_id":  originalID,
			"display_name": displayName,
		})
	}

	// 获取启用的密钥
	keyRows, err := db.Query(`
		SELECT id, name FROM provider_api_keys 
		WHERE provider_id = ? AND is_active = 1 ORDER BY id
	`, providerID)
	if err != nil {
		c.JSON(500, gin.H{"detail": "查询失败"})
		return
	}
	defer keyRows.Close()

	var keys []gin.H
	// 添加默认密钥选项
	keys = append(keys, gin.H{"id": 0, "name": "默认密钥"})
	for keyRows.Next() {
		var id int
		var name string
		keyRows.Scan(&id, &name)
		keys = append(keys, gin.H{"id": id, "name": name})
	}

	c.JSON(200, gin.H{
		"models":   models,
		"api_keys": keys,
	})
}
