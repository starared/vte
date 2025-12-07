package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"vte/internal/database"
	"vte/internal/models"
)

func validateAllowedModels(names []string) (map[string]bool, error) {
	if len(names) == 0 {
		return nil, nil
	}

	db := database.DB()
	rows, err := db.Query(`
        SELECT COALESCE(display_name, ''), original_id
        FROM models
        WHERE is_active = 1
    `)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()

	available := make(map[string]bool)
	for rows.Next() {
		var displayName, originalID string
		if err := rows.Scan(&displayName, &originalID); err != nil {
			continue
		}
		if displayName != "" {
			available[displayName] = true
		}
		if originalID != "" {
			available[originalID] = true
		}
	}

	missing := make([]string, 0)
	for _, name := range names {
		if !available[name] {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		return nil, nil // 忽略不存在的模型
	}

	allowedSet := make(map[string]bool)
	for _, n := range names {
		allowedSet[n] = true
	}
	return allowedSet, nil
}

// ListTempAPIKeys 获取临时密钥列表
func ListTempAPIKeys(c *gin.Context) {
	keys, err := database.ListTempAPIKeys()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "查询失败"})
		return
	}
	// 确保返回空数组而不是 null
	if keys == nil {
		keys = []*models.TempAPIKey{}
	}
	c.JSON(http.StatusOK, keys)
}

// CreateTempAPIKey 创建新的临时密钥
func CreateTempAPIKey(c *gin.Context) {
	var req models.TempAPIKeyCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效的请求体"})
		return
	}

	// 校验必填项
	if len(req.AllowedModels) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "请至少选择一个模型"})
		return
	}

	// 校验有效期设置
	if req.ExpireDuration > 0 && req.ExpireUnit == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "请选择有效期单位"})
		return
	}
	if req.ExpireUnit != "" && req.ExpireUnit != "minutes" && req.ExpireUnit != "hours" && req.ExpireUnit != "days" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效的有效期单位"})
		return
	}

	// 校验速率限制
	if req.RateLimitCount > 0 && req.RateLimitWindow <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "请设置速率限制的时间窗口"})
		return
	}
	if req.RateLimitUnit != "" && req.RateLimitUnit != "seconds" && req.RateLimitUnit != "minutes" && req.RateLimitUnit != "hours" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效的速率限制单位"})
		return
	}

	allowedSet, _ := validateAllowedModels(req.AllowedModels)

	if req.ModelLimits == nil {
		req.ModelLimits = map[string]int{}
	}
	// 移除未在允许列表中的配额
	if allowedSet != nil {
		for k := range req.ModelLimits {
			if !allowedSet[k] {
				delete(req.ModelLimits, k)
			}
			if req.ModelLimits[k] < 0 {
				req.ModelLimits[k] = 0
			}
		}
	}

	if req.IsActive == nil {
		b := true
		req.IsActive = &b
	}

	key, err := database.CreateTempAPIKey(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "创建失败"})
		return
	}

	c.JSON(http.StatusOK, key)
}

// UpdateTempAPIKey 更新临时密钥
func UpdateTempAPIKey(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效的 ID"})
		return
	}

	var req models.TempAPIKeyUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效的请求体"})
		return
	}

	// 校验有效期设置
	if req.ExpireDuration != nil && *req.ExpireDuration > 0 {
		if req.ExpireUnit == nil || *req.ExpireUnit == "" {
			c.JSON(http.StatusBadRequest, gin.H{"detail": "请选择有效期单位"})
			return
		}
	}
	if req.ExpireUnit != nil && *req.ExpireUnit != "" && *req.ExpireUnit != "minutes" && *req.ExpireUnit != "hours" && *req.ExpireUnit != "days" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效的有效期单位"})
		return
	}

	// 校验速率限制
	if req.RateLimitCount != nil && *req.RateLimitCount > 0 {
		if req.RateLimitWindow == nil || *req.RateLimitWindow <= 0 {
			// 获取当前值
			current, err := database.GetTempAPIKeyByID(id)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"detail": "未找到临时密钥"})
				return
			}
			if current.RateLimitWindow <= 0 {
				c.JSON(http.StatusBadRequest, gin.H{"detail": "请设置速率限制的时间窗口"})
				return
			}
		}
	}
	if req.RateLimitUnit != nil && *req.RateLimitUnit != "" && *req.RateLimitUnit != "seconds" && *req.RateLimitUnit != "minutes" && *req.RateLimitUnit != "hours" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效的速率限制单位"})
		return
	}

	// 如果更新了模型，需要校验
	var allowedSet map[string]bool
	if len(req.AllowedModels) > 0 {
		allowedSet, _ = validateAllowedModels(req.AllowedModels)
	}

	if req.ModelLimits != nil && len(req.ModelLimits) > 0 {
		// 若未提供新的模型列表，则使用当前的
		if allowedSet == nil {
			current, err := database.GetTempAPIKeyByID(id)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"detail": "未找到临时密钥"})
				return
			}
			allowedSet = make(map[string]bool)
			for _, m := range current.AllowedModels {
				allowedSet[m] = true
			}
		}
		for k := range req.ModelLimits {
			if !allowedSet[k] {
				delete(req.ModelLimits, k)
			}
			if req.ModelLimits[k] < 0 {
				req.ModelLimits[k] = 0
			}
		}
	}

	key, err := database.UpdateTempAPIKey(id, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, key)
}

// DeleteTempAPIKey 删除临时密钥
func DeleteTempAPIKey(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无效的 ID"})
		return
	}

	if err := database.DeleteTempAPIKey(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "已删除"})
}
