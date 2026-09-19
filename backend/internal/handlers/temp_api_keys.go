package handlers

import (
	"fmt"
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
		return nil, err
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
		return nil, fmt.Errorf("以下模型不存在或未启用: %v", missing)
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

	if err := validateTempWindows(req.RateLimitCount, req.RateLimitWindow, req.RateLimitUnit, req.ExpireDuration, req.ExpireUnit); err != nil {
		c.JSON(400, gin.H{"detail": err.Error()})
		return
	}
	allowedSet, validationErr := validateAllowedModels(req.AllowedModels)
	if validationErr != nil {
		c.JSON(400, gin.H{"detail": validationErr.Error()})
		return
	}
	if req.MaxRequests < 0 || req.ConcurrencyLimit < 0 || req.RateLimitCount < 0 || req.RateLimitWindow < 0 || req.ExpireDuration < 0 || req.ExpireDuration > 36500 || req.RateLimitWindow > 31536000 {
		c.JSON(400, gin.H{"detail": "额度和时间参数超出有效范围"})
		return
	}

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

	for _, n := range []*int{req.MaxRequests, req.ConcurrencyLimit, req.RateLimitCount, req.RateLimitWindow, req.ExpireDuration} {
		if n != nil && *n < 0 {
			c.JSON(400, gin.H{"detail": "额度和时间参数不能为负数"})
			return
		}
	}
	if req.ExpireDuration != nil && *req.ExpireDuration > 36500 || req.RateLimitWindow != nil && *req.RateLimitWindow > 31536000 {
		c.JSON(400, gin.H{"detail": "时间参数超出有效范围"})
		return
	}
	current, err := database.GetTempAPIKeyByID(id)
	if err != nil {
		c.JSON(404, gin.H{"detail": "未找到临时密钥"})
		return
	}
	rateCount, rateWindow, rateUnit := current.RateLimitCount, current.RateLimitWindow, current.RateLimitUnit
	expiry, expiryUnit := current.ExpireDuration, current.ExpireUnit
	if req.RateLimitCount != nil {
		rateCount = *req.RateLimitCount
	}
	if req.RateLimitWindow != nil {
		rateWindow = *req.RateLimitWindow
	}
	if req.RateLimitUnit != nil {
		rateUnit = *req.RateLimitUnit
	}
	if req.ExpireDuration != nil {
		expiry = *req.ExpireDuration
	}
	if req.ExpireUnit != nil {
		expiryUnit = *req.ExpireUnit
	}
	if err := validateTempWindows(rateCount, rateWindow, rateUnit, expiry, expiryUnit); err != nil {
		c.JSON(400, gin.H{"detail": err.Error()})
		return
	}
	// 如果更新了模型，需要校验
	var allowedSet map[string]bool
	if len(req.AllowedModels) > 0 {
		var validationErr error
		allowedSet, validationErr = validateAllowedModels(req.AllowedModels)
		if validationErr != nil {
			c.JSON(400, gin.H{"detail": validationErr.Error()})
			return
		}
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

func validateTempWindows(count, window int, unit string, expiry int, expiryUnit string) error {
	scale := 1
	switch unit {
	case "", "seconds":
	case "minutes":
		scale = 60
	case "hours":
		scale = 3600
	default:
		return fmt.Errorf("无效的速率限制单位")
	}
	if count > 0 && window <= 0 {
		return fmt.Errorf("请设置速率限制的时间窗口")
	}
	if window < 0 || window > 31536000/scale {
		return fmt.Errorf("速率限制窗口不能超过一年")
	}
	if expiry > 0 && expiryUnit == "" {
		return fmt.Errorf("请选择有效期单位")
	}
	switch expiryUnit {
	case "", "minutes", "hours", "days":
	default:
		return fmt.Errorf("无效的有效期单位")
	}
	return nil
}
