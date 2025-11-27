package handlers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"vte/internal/database"
	"vte/internal/logger"
	"vte/internal/models"
	"vte/internal/proxy"
)

func ListProviders(c *gin.Context) {
	db := database.DB()
	rows, err := db.Query(`
		SELECT id, name, base_url, model_prefix, provider_type, 
		       vertex_project, vertex_location, is_active, created_at 
		FROM providers
	`)
	if err != nil {
		c.JSON(500, gin.H{"detail": "查询失败"})
		return
	}
	defer rows.Close()

	var providers []models.Provider
	for rows.Next() {
		var p models.Provider
		var isActive int
		var vertexProject, vertexLocation *string
		err := rows.Scan(&p.ID, &p.Name, &p.BaseURL, &p.ModelPrefix, &p.ProviderType,
			&vertexProject, &vertexLocation, &isActive, &p.CreatedAt)
		if err != nil {
			continue
		}
		p.IsActive = isActive == 1
		if vertexProject != nil {
			p.VertexProject = *vertexProject
		}
		if vertexLocation != nil {
			p.VertexLocation = *vertexLocation
		}
		providers = append(providers, p)
	}

	c.JSON(200, providers)
}

func CreateProvider(c *gin.Context) {
	var req models.ProviderCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}

	if req.ProviderType == "" {
		req.ProviderType = "standard"
	}
	if req.VertexLocation == "" {
		req.VertexLocation = "global"
	}

	db := database.DB()
	result, err := db.Exec(`
		INSERT INTO providers (name, base_url, api_key, model_prefix, provider_type, 
		                       vertex_project, vertex_location, extra_headers, proxy_url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, req.Name, req.BaseURL, req.APIKey, req.ModelPrefix, req.ProviderType,
		req.VertexProject, req.VertexLocation, req.ExtraHeaders, req.ProxyURL)

	if err != nil {
		c.JSON(500, gin.H{"detail": "创建失败"})
		return
	}

	id, _ := result.LastInsertId()
	logger.Info(fmt.Sprintf("%s | 添加提供商 | %s", c.ClientIP(), req.Name))

	c.JSON(200, gin.H{
		"id":              id,
		"name":            req.Name,
		"base_url":        req.BaseURL,
		"model_prefix":    req.ModelPrefix,
		"provider_type":   req.ProviderType,
		"vertex_project":  req.VertexProject,
		"vertex_location": req.VertexLocation,
		"is_active":       true,
	})
}

func UpdateProvider(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))

	var req models.ProviderUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}

	db := database.DB()

	// 获取当前提供商信息
	var oldPrefix, baseURL, proxyURL string
	var name string
	err := db.QueryRow("SELECT name, COALESCE(model_prefix, ''), base_url, COALESCE(proxy_url, '') FROM providers WHERE id = ?", id).
		Scan(&name, &oldPrefix, &baseURL, &proxyURL)
	if err != nil {
		c.JSON(404, gin.H{"detail": "提供商不存在"})
		return
	}

	// 构建更新语句
	updates := []string{}
	args := []interface{}{}

	if req.Name != nil {
		updates = append(updates, "name = ?")
		args = append(args, *req.Name)
	}
	if req.BaseURL != nil {
		updates = append(updates, "base_url = ?")
		args = append(args, *req.BaseURL)
	}
	if req.APIKey != nil && *req.APIKey != "" {
		updates = append(updates, "api_key = ?")
		args = append(args, *req.APIKey)
	}
	if req.ModelPrefix != nil {
		updates = append(updates, "model_prefix = ?")
		args = append(args, *req.ModelPrefix)
	}
	if req.ProviderType != nil {
		updates = append(updates, "provider_type = ?")
		args = append(args, *req.ProviderType)
	}
	if req.VertexProject != nil {
		updates = append(updates, "vertex_project = ?")
		args = append(args, *req.VertexProject)
	}
	if req.VertexLocation != nil {
		updates = append(updates, "vertex_location = ?")
		args = append(args, *req.VertexLocation)
	}
	if req.ExtraHeaders != nil {
		updates = append(updates, "extra_headers = ?")
		args = append(args, *req.ExtraHeaders)
	}
	if req.ProxyURL != nil {
		updates = append(updates, "proxy_url = ?")
		args = append(args, *req.ProxyURL)
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
		updates = append(updates, "updated_at = CURRENT_TIMESTAMP")
		query := "UPDATE providers SET "
		for i, u := range updates {
			if i > 0 {
				query += ", "
			}
			query += u
		}
		query += " WHERE id = ?"
		args = append(args, id)

		_, err = db.Exec(query, args...)
		if err != nil {
			c.JSON(500, gin.H{"detail": "更新失败"})
			return
		}
	}

	// 如果前缀改变，更新所有模型的 display_name
	if req.ModelPrefix != nil {
		newPrefix := *req.ModelPrefix
		// 只要前缀字段被提交，就同步更新所有模型
		if newPrefix != oldPrefix {
			rows, err := db.Query("SELECT id, original_id FROM models WHERE provider_id = ?", id)
			if err == nil {
				for rows.Next() {
					var modelID int
					var originalID string
					rows.Scan(&modelID, &originalID)

					displayName := originalID
					if newPrefix != "" {
						displayName = newPrefix + "/" + originalID
					}
					db.Exec("UPDATE models SET display_name = ? WHERE id = ?", displayName, modelID)
				}
				rows.Close()
			}
			logger.Info(fmt.Sprintf("%s | 同步前缀 | %s | %s -> %s", c.ClientIP(), name, oldPrefix, newPrefix))
		}
	}

	// 清理连接池
	proxy.InvalidateClient(proxyURL)

	logger.Info(fmt.Sprintf("%s | 更新提供商 | %s", c.ClientIP(), name))
	c.JSON(200, gin.H{"message": "更新成功"})
}

func DeleteProvider(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	db := database.DB()

	var name, baseURL, proxyURL string
	err := db.QueryRow("SELECT name, base_url, COALESCE(proxy_url, '') FROM providers WHERE id = ?", id).
		Scan(&name, &baseURL, &proxyURL)
	if err != nil {
		c.JSON(404, gin.H{"detail": "提供商不存在"})
		return
	}

	db.Exec("DELETE FROM models WHERE provider_id = ?", id)
	db.Exec("DELETE FROM providers WHERE id = ?", id)

	proxy.InvalidateClient(proxyURL)

	logger.Info(fmt.Sprintf("%s | 删除提供商 | %s", c.ClientIP(), name))
	c.JSON(200, gin.H{"message": "删除成功"})
}

func FetchModels(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	db := database.DB()

	var baseURL, apiKey, providerType, modelPrefix, proxyURL, name string
	var extraHeaders *string
	err := db.QueryRow(`
		SELECT name, base_url, api_key, provider_type, model_prefix, 
		       COALESCE(proxy_url, ''), extra_headers 
		FROM providers WHERE id = ?
	`, id).Scan(&name, &baseURL, &apiKey, &providerType, &modelPrefix, &proxyURL, &extraHeaders)
	if err != nil {
		c.JSON(404, gin.H{"detail": "提供商不存在"})
		return
	}

	cfg := &proxy.ProviderConfig{
		BaseURL:      baseURL,
		APIKey:       apiKey,
		ProviderType: providerType,
		ProxyURL:     proxyURL,
	}

	if extraHeaders != nil && *extraHeaders != "" {
		json.Unmarshal([]byte(*extraHeaders), &cfg.ExtraHeaders)
	}

	modelsData, err := cfg.ListModels()
	if err != nil {
		logger.Error(fmt.Sprintf("%s | 拉取模型失败 | %s | %v", c.ClientIP(), name, err))
		c.JSON(500, gin.H{"detail": fmt.Sprintf("拉取模型失败: %v", err)})
		return
	}

	// 获取现有模型
	existingModels := make(map[string]int)
	rows, _ := db.Query("SELECT id, original_id FROM models WHERE provider_id = ?", id)
	for rows.Next() {
		var modelID int
		var originalID string
		rows.Scan(&modelID, &originalID)
		existingModels[originalID] = modelID
	}
	rows.Close()

	fetchedIDs := make(map[string]bool)
	added, updated, deleted := 0, 0, 0

	for _, m := range modelsData {
		modelID, ok := m["id"].(string)
		if !ok || modelID == "" {
			continue
		}
		fetchedIDs[modelID] = true

		displayName := modelID
		if modelPrefix != "" {
			displayName = modelPrefix + "/" + modelID
		}

		if existingID, exists := existingModels[modelID]; exists {
			// 更新
			db.Exec("UPDATE models SET display_name = ? WHERE id = ?", displayName, existingID)
			updated++
		} else {
			// 新增
			db.Exec(`
				INSERT INTO models (provider_id, original_id, display_name, is_active) 
				VALUES (?, ?, ?, 0)
			`, id, modelID, displayName)
			added++
		}
	}

	// 删除已下线的模型
	for originalID, modelID := range existingModels {
		if !fetchedIDs[originalID] {
			db.Exec("DELETE FROM models WHERE id = ?", modelID)
			deleted++
		}
	}

	var messages []string
	if added > 0 {
		messages = append(messages, fmt.Sprintf("添加 %d 个新模型", added))
	}
	if updated > 0 {
		messages = append(messages, fmt.Sprintf("更新 %d 个模型", updated))
	}
	if deleted > 0 {
		messages = append(messages, fmt.Sprintf("删除 %d 个已下线模型", deleted))
	}
	if len(messages) == 0 {
		messages = append(messages, "没有变化")
	}

	logger.Info(fmt.Sprintf("%s | 拉取模型 | %s | %v", c.ClientIP(), name, messages))
	c.JSON(200, gin.H{"message": strings.Join(messages, "、"), "total_fetched": len(modelsData)})
}

func AddModel(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))

	var req models.AddModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}

	db := database.DB()

	var name, modelPrefix string
	err := db.QueryRow("SELECT name, model_prefix FROM providers WHERE id = ?", id).Scan(&name, &modelPrefix)
	if err != nil {
		c.JSON(404, gin.H{"detail": "提供商不存在"})
		return
	}

	// 检查是否已存在
	var count int
	db.QueryRow("SELECT COUNT(*) FROM models WHERE provider_id = ? AND original_id = ?", id, req.ModelID).Scan(&count)
	if count > 0 {
		c.JSON(400, gin.H{"detail": "模型已存在"})
		return
	}

	displayName := req.ModelID
	if modelPrefix != "" {
		displayName = modelPrefix + "/" + req.ModelID
	}

	_, err = db.Exec(`
		INSERT INTO models (provider_id, original_id, display_name, is_active) 
		VALUES (?, ?, ?, 1)
	`, id, req.ModelID, displayName)
	if err != nil {
		c.JSON(500, gin.H{"detail": "添加失败"})
		return
	}

	logger.Info(fmt.Sprintf("%s | 手动添加模型 | %s | %s", c.ClientIP(), name, req.ModelID))
	c.JSON(200, gin.H{"message": "添加成功"})
}

func ListProviderModels(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	db := database.DB()

	rows, err := db.Query(`
		SELECT m.id, m.provider_id, p.name, m.original_id, m.display_name, m.is_active
		FROM models m
		JOIN providers p ON m.provider_id = p.id
		WHERE m.provider_id = ?
	`, id)
	if err != nil {
		c.JSON(500, gin.H{"detail": "查询失败"})
		return
	}
	defer rows.Close()

	var result []models.Model
	for rows.Next() {
		var m models.Model
		var isActive int
		var displayName *string
		rows.Scan(&m.ID, &m.ProviderID, &m.ProviderName, &m.OriginalID, &displayName, &isActive)
		m.IsActive = isActive == 1
		if displayName != nil {
			m.DisplayName = *displayName
		}
		result = append(result, m)
	}

	c.JSON(200, result)
}


