package handlers

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"vte/internal/database"
	"vte/internal/logger"
	"vte/internal/models"
)

func ListAllModels(c *gin.Context) {
	db := database.DB()

	// 只同步非自定义名称的模型的 display_name（根据提供商前缀自动生成）
	db.Exec(`
		UPDATE models SET display_name = 
			CASE 
				WHEN (SELECT COALESCE(model_prefix, '') FROM providers WHERE id = models.provider_id) != ''
				THEN (SELECT model_prefix FROM providers WHERE id = models.provider_id) || '/' || original_id
				ELSE original_id
			END
		WHERE custom_name = 0 OR custom_name IS NULL
	`)

	rows, err := db.Query(`
		SELECT m.id, m.provider_id, p.name, m.original_id, m.display_name, m.is_active, COALESCE(m.custom_name, 0)
		FROM models m
		JOIN providers p ON m.provider_id = p.id
	`)
	if err != nil {
		c.JSON(500, gin.H{"detail": "查询失败"})
		return
	}
	defer rows.Close()

	result := []models.Model{}
	for rows.Next() {
		var m models.Model
		var isActive, customName int
		var displayName *string
		rows.Scan(&m.ID, &m.ProviderID, &m.ProviderName, &m.OriginalID, &displayName, &isActive, &customName)
		m.IsActive = isActive == 1
		m.CustomName = customName == 1
		if displayName != nil {
			m.DisplayName = *displayName
		}
		result = append(result, m)
	}

	c.JSON(200, result)
}

func UpdateModel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"detail": "无效的模型ID"})
		return
	}

	var req models.ModelUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}

	db := database.DB()

	var displayName string
	err = db.QueryRow("SELECT COALESCE(display_name, original_id) FROM models WHERE id = ?", id).Scan(&displayName)
	if err != nil {
		c.JSON(404, gin.H{"detail": "模型不存在"})
		return
	}

	// 更新 display_name（用户自定义名称）
	if req.DisplayName != nil {
		newDisplayName := *req.DisplayName
		if newDisplayName != "" {
			// 设置自定义名称
			db.Exec("UPDATE models SET display_name = ?, custom_name = 1 WHERE id = ?", newDisplayName, id)
			logger.Info(fmt.Sprintf("%s | 修改模型名称 | %s -> %s", c.ClientIP(), displayName, newDisplayName))
		}
	}

	// 更新 is_active
	if req.IsActive != nil {
		active := 0
		if *req.IsActive {
			active = 1
		}
		db.Exec("UPDATE models SET is_active = ? WHERE id = ?", active, id)

		status := "禁用"
		if *req.IsActive {
			status = "启用"
		}
		logger.Info(fmt.Sprintf("%s | %s模型 | %s", c.ClientIP(), status, displayName))
	}

	c.JSON(200, gin.H{"message": "更新成功"})
}

// ResetModelDisplayName 重置模型显示名称为自动生成
func ResetModelDisplayName(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"detail": "无效的模型ID"})
		return
	}

	db := database.DB()

	// 获取模型和提供商信息
	var originalID, providerPrefix string
	err = db.QueryRow(`
		SELECT m.original_id, COALESCE(p.model_prefix, '')
		FROM models m
		JOIN providers p ON m.provider_id = p.id
		WHERE m.id = ?
	`, id).Scan(&originalID, &providerPrefix)
	if err != nil {
		c.JSON(404, gin.H{"detail": "模型不存在"})
		return
	}

	// 生成自动名称
	autoDisplayName := originalID
	if providerPrefix != "" {
		autoDisplayName = providerPrefix + "/" + originalID
	}

	// 重置为自动生成的名称
	db.Exec("UPDATE models SET display_name = ?, custom_name = 0 WHERE id = ?", autoDisplayName, id)

	logger.Info(fmt.Sprintf("%s | 重置模型名称 | %s", c.ClientIP(), autoDisplayName))
	c.JSON(200, gin.H{"message": "重置成功", "display_name": autoDisplayName})
}

func DeleteModel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"detail": "无效的模型ID"})
		return
	}
	db := database.DB()

	var displayName string
	err = db.QueryRow("SELECT COALESCE(display_name, original_id) FROM models WHERE id = ?", id).Scan(&displayName)
	if err != nil {
		c.JSON(404, gin.H{"detail": "模型不存在"})
		return
	}

	db.Exec("DELETE FROM models WHERE id = ?", id)

	logger.Info(fmt.Sprintf("%s | 删除模型 | %s", c.ClientIP(), displayName))
	c.JSON(200, gin.H{"message": "删除成功"})
}

func BatchToggleModels(c *gin.Context) {
	var req models.BatchToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}

	if len(req.ModelIDs) == 0 {
		c.JSON(400, gin.H{"detail": "没有选择模型"})
		return
	}

	db := database.DB()

	active := 0
	if req.IsActive {
		active = 1
	}

	// 构建 IN 查询
	placeholders := make([]string, len(req.ModelIDs))
	args := make([]interface{}, len(req.ModelIDs)+1)
	args[0] = active
	for i, id := range req.ModelIDs {
		placeholders[i] = "?"
		args[i+1] = id
	}

	query := fmt.Sprintf("UPDATE models SET is_active = ? WHERE id IN (%s)", strings.Join(placeholders, ","))
	db.Exec(query, args...)

	status := "禁用"
	if req.IsActive {
		status = "启用"
	}
	logger.Info(fmt.Sprintf("%s | 批量%s模型 | %d个", c.ClientIP(), status, len(req.ModelIDs)))
	c.JSON(200, gin.H{"message": fmt.Sprintf("已更新 %d 个模型", len(req.ModelIDs))})
}
