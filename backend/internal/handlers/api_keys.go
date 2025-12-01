package handlers

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"vte/internal/database"
	"vte/internal/logger"
	"vte/internal/models"
)

// 轮询计数器，用于实现 Round-Robin
var (
	keyIndexMap = make(map[int]int) // provider_id -> current_index
	keyIndexMu  sync.Mutex
)

// GetNextAPIKey 获取下一个可用的 API Key（轮询）
func GetNextAPIKey(providerID int) (string, int, error) {
	db := database.DB()

	// 获取所有启用的密钥
	rows, err := db.Query(`
		SELECT id, api_key FROM provider_api_keys 
		WHERE provider_id = ? AND is_active = 1 
		ORDER BY id
	`, providerID)
	if err != nil {
		return "", 0, err
	}
	defer rows.Close()

	var keys []struct {
		ID     int
		APIKey string
	}
	for rows.Next() {
		var k struct {
			ID     int
			APIKey string
		}
		rows.Scan(&k.ID, &k.APIKey)
		keys = append(keys, k)
	}

	if len(keys) == 0 {
		return "", 0, fmt.Errorf("no active api keys")
	}

	if len(keys) == 1 {
		// 只有一个密钥，直接返回并更新统计
		go func() {
			db.Exec(`
				UPDATE provider_api_keys 
				SET usage_count = usage_count + 1, last_used_at = CURRENT_TIMESTAMP 
				WHERE id = ?
			`, keys[0].ID)
		}()
		return keys[0].APIKey, keys[0].ID, nil
	}

	// 轮询选择
	keyIndexMu.Lock()
	idx := keyIndexMap[providerID]
	if idx >= len(keys) {
		idx = 0
	}
	selected := keys[idx]
	keyIndexMap[providerID] = (idx + 1) % len(keys)
	keyIndexMu.Unlock()

	// 更新使用统计
	go func() {
		db.Exec(`
			UPDATE provider_api_keys 
			SET usage_count = usage_count + 1, last_used_at = CURRENT_TIMESTAMP 
			WHERE id = ?
		`, selected.ID)
	}()

	return selected.APIKey, selected.ID, nil
}

// ListAPIKeys 列出提供商的所有密钥
func ListAPIKeys(c *gin.Context) {
	providerID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		logger.Error(fmt.Sprintf("ListAPIKeys: 无效的提供商ID: %v", err))
		c.JSON(400, gin.H{"detail": "无效的提供商ID"})
		return
	}

	db := database.DB()

	// 检查提供商是否存在
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM providers WHERE id = ?", providerID).Scan(&count)
	if err != nil {
		logger.Error(fmt.Sprintf("ListAPIKeys: 查询提供商失败: %v", err))
		c.JSON(500, gin.H{"detail": "查询失败"})
		return
	}
	if count == 0 {
		logger.Error(fmt.Sprintf("ListAPIKeys: 提供商不存在: %d", providerID))
		c.JSON(404, gin.H{"detail": "提供商不存在"})
		return
	}

	// 获取所有密钥
	rows, err := db.Query(`
		SELECT id, provider_id, api_key, name, is_active, usage_count, last_used_at, created_at
		FROM provider_api_keys
		WHERE provider_id = ?
		ORDER BY id
	`, providerID)
	if err != nil {
		logger.Error(fmt.Sprintf("ListAPIKeys: 查询密钥失败: %v", err))
		c.JSON(500, gin.H{"detail": "查询失败"})
		return
	}
	defer rows.Close()

	keys := make([]models.ProviderAPIKey, 0)
	for rows.Next() {
		var k models.ProviderAPIKey
		var isActive int
		var lastUsedAt *time.Time
		var apiKey string
		err := rows.Scan(&k.ID, &k.ProviderID, &apiKey, &k.Name, &isActive, &k.UsageCount, &lastUsedAt, &k.CreatedAt)
		if err != nil {
			logger.Error(fmt.Sprintf("ListAPIKeys: 扫描行失败: %v", err))
			continue
		}
		k.IsActive = isActive == 1
		k.LastUsedAt = lastUsedAt
		k.APIKey = apiKey
		keys = append(keys, k)
	}

	logger.Info(fmt.Sprintf("ListAPIKeys: 返回 %d 个密钥", len(keys)))
	c.JSON(200, keys)
}

// AddAPIKey 添加新密钥
func AddAPIKey(c *gin.Context) {
	providerID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"detail": "无效的提供商ID"})
		return
	}

	var req models.APIKeyCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}

	db := database.DB()

	// 检查提供商是否存在
	var providerName string
	err = db.QueryRow("SELECT name FROM providers WHERE id = ?", providerID).Scan(&providerName)
	if err != nil {
		c.JSON(404, gin.H{"detail": "提供商不存在"})
		return
	}

	// 生成默认名称
	name := req.Name
	if name == "" {
		var count int
		db.QueryRow("SELECT COUNT(*) FROM provider_api_keys WHERE provider_id = ?", providerID).Scan(&count)
		name = fmt.Sprintf("密钥 %d", count+1)
	}

	result, err := db.Exec(`
		INSERT INTO provider_api_keys (provider_id, api_key, name)
		VALUES (?, ?, ?)
	`, providerID, req.APIKey, name)
	if err != nil {
		c.JSON(500, gin.H{"detail": "添加失败"})
		return
	}

	id, _ := result.LastInsertId()
	logger.Info(fmt.Sprintf("%s | 添加密钥 | %s | %s", c.ClientIP(), providerName, name))

	c.JSON(200, gin.H{
		"id":          id,
		"provider_id": providerID,
		"name":        name,
		"is_active":   true,
	})
}

// UpdateAPIKey 更新密钥
func UpdateAPIKey(c *gin.Context) {
	keyID, err := strconv.Atoi(c.Param("keyId"))
	if err != nil {
		c.JSON(400, gin.H{"detail": "无效的密钥ID"})
		return
	}

	var req models.APIKeyUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}

	db := database.DB()

	updates := []string{}
	args := []interface{}{}

	if req.Name != nil {
		updates = append(updates, "name = ?")
		args = append(args, *req.Name)
	}
	if req.IsActive != nil {
		active := 0
		if *req.IsActive {
			active = 1
		}
		updates = append(updates, "is_active = ?")
		args = append(args, active)
	}

	if len(updates) > 0 {
		query := "UPDATE provider_api_keys SET "
		for i, u := range updates {
			if i > 0 {
				query += ", "
			}
			query += u
		}
		query += " WHERE id = ?"
		args = append(args, keyID)

		_, err = db.Exec(query, args...)
		if err != nil {
			c.JSON(500, gin.H{"detail": "更新失败"})
			return
		}
	}

	c.JSON(200, gin.H{"message": "更新成功"})
}

// DeleteAPIKey 删除密钥
func DeleteAPIKey(c *gin.Context) {
	keyID, err := strconv.Atoi(c.Param("keyId"))
	if err != nil {
		c.JSON(400, gin.H{"detail": "无效的密钥ID"})
		return
	}

	db := database.DB()
	_, err = db.Exec("DELETE FROM provider_api_keys WHERE id = ?", keyID)
	if err != nil {
		c.JSON(500, gin.H{"detail": "删除失败"})
		return
	}

	c.JSON(200, gin.H{"message": "删除成功"})
}
