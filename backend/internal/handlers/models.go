package handlers

import (
	"database/sql"
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

	// 显示名称在新增模型、改名、修改提供商前缀时就已经同步好，这里只读取
	rows, err := db.Query(`
		SELECT m.id, m.provider_id, p.name, m.original_id, m.display_name, m.is_active,
		       COALESCE(m.custom_name, 0), COALESCE(m.source, ''), COALESCE(m.disabled_by_sync, 0)
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
		m, err := scanModelRow(rows)
		if err != nil {
			c.JSON(500, gin.H{"detail": "查询失败"})
			return
		}
		result = append(result, m)
	}

	c.JSON(200, result)
}

// scanModelRow 读取 id, provider_id, provider_name, original_id, display_name, is_active, custom_name, source, disabled_by_sync
func scanModelRow(rows *sql.Rows) (models.Model, error) {
	var m models.Model
	var isActive, customName, disabledBySync int
	var displayName *string
	if err := rows.Scan(&m.ID, &m.ProviderID, &m.ProviderName, &m.OriginalID, &displayName, &isActive, &customName, &m.Source, &disabledBySync); err != nil {
		return m, err
	}
	m.IsActive = isActive == 1
	m.CustomName = customName == 1
	m.DisabledBySync = disabledBySync == 1
	if displayName != nil {
		m.DisplayName = *displayName
	}
	return m, nil
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

	tx, err := database.DB().Begin()
	if err != nil {
		c.JSON(500, gin.H{"detail": "更新失败"})
		return
	}
	defer tx.Rollback()

	var displayName string
	err = tx.QueryRow("SELECT COALESCE(display_name, original_id) FROM models WHERE id = ?", id).Scan(&displayName)
	if err != nil {
		c.JSON(404, gin.H{"detail": "模型不存在"})
		return
	}

	renames := map[string]string{}

	// 更新 display_name（用户自定义名称）
	if req.DisplayName != nil {
		newDisplayName := strings.TrimSpace(*req.DisplayName)
		if newDisplayName != "" && newDisplayName != displayName {
			taken, err := displayNameTaken(tx, newDisplayName, id)
			if err != nil {
				c.JSON(500, gin.H{"detail": "更新失败"})
				return
			}
			if taken {
				c.JSON(409, gin.H{"detail": fmt.Sprintf("显示名称「%s」已被其他模型使用", newDisplayName)})
				return
			}
			if _, err := tx.Exec("UPDATE models SET display_name = ?, custom_name = 1 WHERE id = ?", newDisplayName, id); err != nil {
				c.JSON(500, gin.H{"detail": "更新失败"})
				return
			}
			renames[displayName] = newDisplayName
		}
	}

	// 更新 is_active
	if req.IsActive != nil {
		if *req.IsActive {
			conflicts, err := activeNameConflicts(tx, []int{id})
			if err != nil {
				c.JSON(500, gin.H{"detail": "更新失败"})
				return
			}
			if len(conflicts) > 0 {
				c.JSON(409, gin.H{"detail": duplicateNameError(conflicts).Error()})
				return
			}
		}
		active := 0
		if *req.IsActive {
			active = 1
		}
		// 手动开关后，不再由「拉取模型」自动恢复
		if _, err := tx.Exec("UPDATE models SET is_active = ?, disabled_by_sync = 0 WHERE id = ?", active, id); err != nil {
			c.JSON(500, gin.H{"detail": "更新失败"})
			return
		}
	}

	if err := renameModelReferences(tx, renames); err != nil {
		c.JSON(500, gin.H{"detail": "同步模型引用失败"})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(500, gin.H{"detail": "更新失败"})
		return
	}

	for oldName, newName := range renames {
		logger.Info(fmt.Sprintf("%s | 修改模型名称 | %s -> %s", c.ClientIP(), oldName, newName))
	}
	if req.IsActive != nil {
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

	tx, err := database.DB().Begin()
	if err != nil {
		c.JSON(500, gin.H{"detail": "重置失败"})
		return
	}
	defer tx.Rollback()

	// 获取模型和提供商信息
	var originalID, providerPrefix, currentName string
	err = tx.QueryRow(`
		SELECT m.original_id, COALESCE(p.model_prefix, ''), COALESCE(m.display_name, m.original_id)
		FROM models m
		JOIN providers p ON m.provider_id = p.id
		WHERE m.id = ?
	`, id).Scan(&originalID, &providerPrefix, &currentName)
	if err != nil {
		c.JSON(404, gin.H{"detail": "模型不存在"})
		return
	}

	// 生成自动名称
	autoDisplayName := originalID
	if providerPrefix != "" {
		autoDisplayName = providerPrefix + "/" + originalID
	}

	if autoDisplayName != currentName {
		taken, err := displayNameTaken(tx, autoDisplayName, id)
		if err != nil {
			c.JSON(500, gin.H{"detail": "重置失败"})
			return
		}
		if taken {
			c.JSON(409, gin.H{"detail": fmt.Sprintf("自动名称「%s」已被其他模型使用，无法重置", autoDisplayName)})
			return
		}
	}

	if _, err := tx.Exec("UPDATE models SET display_name = ?, custom_name = 0 WHERE id = ?", autoDisplayName, id); err != nil {
		c.JSON(500, gin.H{"detail": "重置失败"})
		return
	}
	if err := renameModelReferences(tx, map[string]string{currentName: autoDisplayName}); err != nil {
		c.JSON(500, gin.H{"detail": "同步模型引用失败"})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(500, gin.H{"detail": "重置失败"})
		return
	}

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

	if _, err := db.Exec("DELETE FROM models WHERE id = ?", id); err != nil {
		c.JSON(500, gin.H{"detail": "删除失败"})
		return
	}

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

	if req.IsActive {
		conflicts, err := activeNameConflicts(db, req.ModelIDs)
		if err != nil {
			c.JSON(500, gin.H{"detail": "更新失败"})
			return
		}
		if len(conflicts) > 0 {
			c.JSON(409, gin.H{"detail": duplicateNameError(conflicts).Error()})
			return
		}
	}

	active := 0
	if req.IsActive {
		active = 1
	}

	// 构建 IN 查询
	marks := make([]string, len(req.ModelIDs))
	args := make([]interface{}, len(req.ModelIDs)+1)
	args[0] = active
	for i, id := range req.ModelIDs {
		marks[i] = "?"
		args[i+1] = id
	}

	query := fmt.Sprintf("UPDATE models SET is_active = ?, disabled_by_sync = 0 WHERE id IN (%s)", strings.Join(marks, ","))
	if _, err := db.Exec(query, args...); err != nil {
		c.JSON(500, gin.H{"detail": "更新失败"})
		return
	}

	status := "禁用"
	if req.IsActive {
		status = "启用"
	}
	logger.Info(fmt.Sprintf("%s | 批量%s模型 | %d个", c.ClientIP(), status, len(req.ModelIDs)))
	c.JSON(200, gin.H{"message": fmt.Sprintf("已更新 %d 个模型", len(req.ModelIDs))})
}
