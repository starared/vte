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

	// 同步所有模型的 display_name（根据提供商前缀自动生成）
	db.Exec(`
		UPDATE models SET display_name = 
			CASE 
				WHEN (SELECT COALESCE(model_prefix, '') FROM providers WHERE id = models.provider_id) != ''
				THEN (SELECT model_prefix FROM providers WHERE id = models.provider_id) || '/' || original_id
				ELSE original_id
			END
	`)

	rows, err := db.Query(`
		SELECT m.id, m.provider_id, p.name, m.original_id, m.display_name, m.is_active
		FROM models m
		JOIN providers p ON m.provider_id = p.id
	`)
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

	// 只允许更新 is_active，display_name 由提供商前缀自动生成
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
