package handlers

import (
	"encoding/json"
	"fmt"
	"net/url"
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
		       vertex_project, vertex_location, COALESCE(extra_headers, ''), COALESCE(proxy_url, ''), is_active, created_at
		FROM providers
	`)
	if err != nil {
		c.JSON(500, gin.H{"detail": "查询失败"})
		return
	}
	defer rows.Close()

	providers := make([]models.Provider, 0)
	for rows.Next() {
		var p models.Provider
		var isActive int
		var vertexProject, vertexLocation *string
		err := rows.Scan(&p.ID, &p.Name, &p.BaseURL, &p.ModelPrefix, &p.ProviderType,
			&vertexProject, &vertexLocation, &p.ExtraHeaders, &p.ProxyURL, &isActive, &p.CreatedAt)
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

	if err := validateProvider(req.ProviderType, req.BaseURL, req.VertexProject, req.ExtraHeaders, req.ProxyURL); err != nil {
		c.JSON(400, gin.H{"detail": err.Error()})
		return
	}
	db := database.DB()
	tx, err := db.Begin()
	if err != nil {
		c.JSON(500, gin.H{"detail": "创建失败"})
		return
	}
	defer tx.Rollback()
	result, err := tx.Exec(`
		INSERT INTO providers (name, base_url, api_key, model_prefix, provider_type, 
		                       vertex_project, vertex_location, extra_headers, proxy_url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, req.Name, req.BaseURL, "", req.ModelPrefix, req.ProviderType,
		req.VertexProject, req.VertexLocation, req.ExtraHeaders, req.ProxyURL)

	if err != nil {
		c.JSON(500, gin.H{"detail": "创建失败"})
		return
	}

	id, _ := result.LastInsertId()

	// 将 API Key 添加到 provider_api_keys 表
	if req.APIKey != "" {
		if _, err = tx.Exec(`
			INSERT INTO provider_api_keys (provider_id, api_key, name)
			VALUES (?, ?, ?)
		`, id, req.APIKey, "密钥 1"); err != nil {
			c.JSON(500, gin.H{"detail": "添加密钥失败"})
			return
		}
	}

	if err = tx.Commit(); err != nil {
		c.JSON(500, gin.H{"detail": "创建失败"})
		return
	}
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
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"detail": "无效的提供商ID"})
		return
	}

	var req models.ProviderUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}

	db := database.DB()

	// 获取当前提供商信息
	var oldPrefix, baseURL, proxyURL string
	var name string
	err = db.QueryRow("SELECT name, COALESCE(model_prefix, ''), base_url, COALESCE(proxy_url, '') FROM providers WHERE id = ?", id).
		Scan(&name, &oldPrefix, &baseURL, &proxyURL)
	if err != nil {
		c.JSON(404, gin.H{"detail": "提供商不存在"})
		return
	}

	var providerType, project, headers string
	if err := db.QueryRow("SELECT provider_type,COALESCE(vertex_project,''),COALESCE(extra_headers,'') FROM providers WHERE id=?", id).Scan(&providerType, &project, &headers); err != nil {
		c.JSON(500, gin.H{"detail": "查询失败"})
		return
	}
	newBase, newProxy := baseURL, proxyURL
	if req.ProviderType != nil {
		providerType = *req.ProviderType
	}
	if req.BaseURL != nil {
		newBase = *req.BaseURL
	}
	if req.VertexProject != nil {
		project = *req.VertexProject
	}
	if req.ExtraHeaders != nil {
		headers = *req.ExtraHeaders
	}
	if req.ProxyURL != nil {
		newProxy = *req.ProxyURL
	}
	if err := validateProvider(providerType, newBase, project, headers, newProxy); err != nil {
		c.JSON(400, gin.H{"detail": err.Error()})
		return
	}
	if req.APIKey != nil && *req.APIKey != "" {
		c.JSON(400, gin.H{"detail": "请通过密钥管理接口更新密钥，避免与轮询密钥冲突"})
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

	tx, err := db.Begin()
	if err != nil {
		c.JSON(500, gin.H{"detail": "更新失败"})
		return
	}
	defer tx.Rollback()

	if len(updates) > 0 {
		updates = append(updates, "updated_at = CURRENT_TIMESTAMP")
		query := "UPDATE providers SET " + strings.Join(updates, ", ") + " WHERE id = ?"
		args = append(args, id)
		if _, err := tx.Exec(query, args...); err != nil {
			c.JSON(500, gin.H{"detail": "更新失败"})
			return
		}
	}

	// 如果前缀改变，同步非自定义名称模型的 display_name，并更新引用这些名称的配置
	if req.ModelPrefix != nil && *req.ModelPrefix != oldPrefix {
		newPrefix := *req.ModelPrefix
		renames, err := applyPrefixChange(tx, id, newPrefix)
		if err != nil {
			if conflict, ok := err.(prefixConflictError); ok {
				c.JSON(409, gin.H{"detail": conflict.Error()})
				return
			}
			c.JSON(500, gin.H{"detail": "同步模型名称失败"})
			return
		}
		if err := renameModelReferences(tx, renames); err != nil {
			c.JSON(500, gin.H{"detail": "同步模型引用失败"})
			return
		}
		logger.Info(fmt.Sprintf("%s | 同步前缀 | %s | %s -> %s", c.ClientIP(), name, oldPrefix, newPrefix))
	}

	if err := tx.Commit(); err != nil {
		c.JSON(500, gin.H{"detail": "更新失败"})
		return
	}

	// 清理连接池
	proxy.InvalidateClient(proxyURL)

	logger.Info(fmt.Sprintf("%s | 更新提供商 | %s", c.ClientIP(), name))
	c.JSON(200, gin.H{"message": "更新成功"})
}

func DeleteProvider(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"detail": "无效的提供商ID"})
		return
	}
	db := database.DB()

	var name, baseURL, proxyURL string
	err = db.QueryRow("SELECT name, base_url, COALESCE(proxy_url, '') FROM providers WHERE id = ?", id).
		Scan(&name, &baseURL, &proxyURL)
	if err != nil {
		c.JSON(404, gin.H{"detail": "提供商不存在"})
		return
	}

	// 在同一个事务里删除提供商及其模型、密钥，避免留下孤立的密钥记录
	tx, err := db.Begin()
	if err != nil {
		c.JSON(500, gin.H{"detail": "删除失败"})
		return
	}
	defer tx.Rollback()
	for _, q := range []string{
		"DELETE FROM provider_api_keys WHERE provider_id = ?",
		"DELETE FROM models WHERE provider_id = ?",
		"DELETE FROM providers WHERE id = ?",
	} {
		if _, err := tx.Exec(q, id); err != nil {
			c.JSON(500, gin.H{"detail": "删除失败"})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		c.JSON(500, gin.H{"detail": "删除失败"})
		return
	}

	proxy.InvalidateClient(proxyURL)

	logger.Info(fmt.Sprintf("%s | 删除提供商 | %s", c.ClientIP(), name))
	c.JSON(200, gin.H{"message": "删除成功"})
}

func FetchModels(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"detail": "无效的提供商ID"})
		return
	}
	db := database.DB()

	var baseURL, apiKey, providerType, modelPrefix, proxyURL, name string
	var extraHeaders *string
	err = db.QueryRow(`
		SELECT name, base_url, api_key, provider_type, model_prefix, 
		       COALESCE(proxy_url, ''), extra_headers 
		FROM providers WHERE id = ?
	`, id).Scan(&name, &baseURL, &apiKey, &providerType, &modelPrefix, &proxyURL, &extraHeaders)
	if err != nil {
		c.JSON(404, gin.H{"detail": "提供商不存在"})
		return
	}

	// 如果 providers 表的 api_key 为空，尝试从轮询密钥获取
	if apiKey == "" {
		if rotatedKey, _, err := GetNextAPIKey(id); err == nil && rotatedKey != "" {
			apiKey = rotatedKey
		}
	}

	cfg := &proxy.ProviderConfig{
		BaseURL:      baseURL,
		APIKey:       apiKey,
		ProviderType: providerType,
		ProxyURL:     proxyURL,
	}

	if extraHeaders != nil && *extraHeaders != "" {
		if err := json.Unmarshal([]byte(*extraHeaders), &cfg.ExtraHeaders); err != nil {
			logger.Warn(fmt.Sprintf("提供商 %s 的额外请求头格式错误，已忽略", name))
		}
	}

	modelsData, err := cfg.ListModels(c.Request.Context())
	if err != nil {
		logger.Error(fmt.Sprintf("%s | 拉取模型失败 | %s | %v", c.ClientIP(), name, err))
		c.JSON(500, gin.H{"detail": fmt.Sprintf("拉取模型失败: %v", err)})
		return
	}

	if len(modelsData) == 0 {
		c.JSON(409, gin.H{"detail": "上游返回空模型列表；已保留现有模型，请手动确认后删除"})
		return
	}
	result, err := syncFetchedModels(id, modelPrefix, modelsData)
	if err != nil {
		logger.Error(fmt.Sprintf("%s | 同步模型失败 | %s | %v", c.ClientIP(), name, err))
		c.JSON(500, gin.H{"detail": "保存模型列表失败"})
		return
	}
	added, updated, deleted, disabled := result.added, result.updated, result.deleted, result.disabled
	restored, duplicates := result.restored, result.duplicates

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
	if disabled > 0 {
		messages = append(messages, fmt.Sprintf("停用 %d 个上游未列出的模型（已保留，可手动删除）", disabled))
	}
	if restored > 0 {
		messages = append(messages, fmt.Sprintf("重新启用 %d 个恢复上线的模型", restored))
	}
	if duplicates > 0 {
		messages = append(messages, fmt.Sprintf("%d 个新模型与已有模型重名，启用前请设置前缀或改名", duplicates))
	}
	if len(messages) == 0 {
		messages = append(messages, "没有变化")
	}

	logger.Info(fmt.Sprintf("%s | 拉取模型 | %s | %v", c.ClientIP(), name, messages))
	c.JSON(200, gin.H{"message": strings.Join(messages, "、"), "total_fetched": len(modelsData)})
}

func AddModel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"detail": "无效的提供商ID"})
		return
	}

	var req models.AddModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "无效的请求"})
		return
	}

	db := database.DB()

	var name, modelPrefix string
	err = db.QueryRow("SELECT name, model_prefix FROM providers WHERE id = ?", id).Scan(&name, &modelPrefix)
	if err != nil {
		c.JSON(404, gin.H{"detail": "提供商不存在"})
		return
	}

	req.ModelID = strings.TrimSpace(req.ModelID)
	if req.ModelID == "" {
		c.JSON(400, gin.H{"detail": "模型 ID 不能为空"})
		return
	}

	// 检查是否已存在
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM models WHERE provider_id = ? AND original_id = ?", id, req.ModelID).Scan(&count); err != nil {
		c.JSON(500, gin.H{"detail": "查询失败"})
		return
	}
	if count > 0 {
		c.JSON(400, gin.H{"detail": "模型已存在"})
		return
	}

	displayName := req.ModelID
	if modelPrefix != "" {
		displayName = modelPrefix + "/" + req.ModelID
	}
	if taken, err := displayNameTaken(db, displayName); err != nil {
		c.JSON(500, gin.H{"detail": "查询失败"})
		return
	} else if taken {
		c.JSON(409, gin.H{"detail": fmt.Sprintf("模型名称「%s」已被其他提供商的模型使用，请先给提供商设置前缀", displayName)})
		return
	}

	_, err = db.Exec(`
		INSERT INTO models (provider_id, original_id, display_name, custom_name, is_active, source) 
		VALUES (?, ?, ?, 0, 1, 'manual')
	`, id, req.ModelID, displayName)
	if err != nil {
		c.JSON(500, gin.H{"detail": "添加失败"})
		return
	}

	logger.Info(fmt.Sprintf("%s | 手动添加模型 | %s | %s", c.ClientIP(), name, req.ModelID))
	c.JSON(200, gin.H{"message": "添加成功"})
}

func ListProviderModels(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"detail": "无效的提供商ID"})
		return
	}
	db := database.DB()

	rows, err := db.Query(`
		SELECT m.id, m.provider_id, p.name, m.original_id, m.display_name, m.is_active,
		       COALESCE(m.custom_name, 0), COALESCE(m.source, ''), COALESCE(m.disabled_by_sync, 0)
		FROM models m
		JOIN providers p ON m.provider_id = p.id
		WHERE m.provider_id = ?
		ORDER BY m.original_id
	`, id)
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

func validateProvider(kind, base, project, headers, proxyAddress string) error {
	if kind != "standard" && kind != "vertex_express" {
		return fmt.Errorf("不支持的提供商类型")
	}
	if kind == "standard" {
		u, err := url.Parse(base)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("API 地址必须为不含查询参数的 HTTP(S) 基础地址")
		}
		if strings.HasSuffix(strings.TrimRight(u.Path, "/"), "/chat/completions") {
			return fmt.Errorf("请填写基础地址，不要包含 /chat/completions")
		}
	}
	if kind == "vertex_express" && strings.TrimSpace(project) == "" {
		return fmt.Errorf("请填写 Vertex 项目编号")
	}
	if headers != "" {
		var h map[string]string
		if json.Unmarshal([]byte(headers), &h) != nil || h == nil {
			return fmt.Errorf("extra_headers 必须为字符串键值组成的 JSON 对象")
		}
		for k, v := range h {
			if strings.ContainsAny(k+v, "\r\n") {
				return fmt.Errorf("请求头不能包含换行")
			}
		}
	}
	if proxyAddress != "" {
		u, err := url.Parse(proxyAddress)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") {
			return fmt.Errorf("代理地址格式错误")
		}
	}
	return nil
}

type modelSyncResult struct {
	added, updated, deleted, disabled, restored, duplicates int
}

// syncFetchedModels 用上游返回的模型列表更新本地模型（在一个事务内完成）。
//
// 上游列表里没有的本地模型：
//   - 手动添加的（source=manual）：保留不动，上游 /models 不列出的模型常常仍可调用
//   - 改过显示名称的、或来源未知的旧数据：停用但保留（标记 disabled_by_sync），避免丢失别名配置
//   - 其余从上游拉取的：删除
//
// 之前因此被停用的模型重新出现在上游列表时，会自动重新启用。
func syncFetchedModels(providerID int, modelPrefix string, modelsData []map[string]interface{}) (modelSyncResult, error) {
	var res modelSyncResult
	tx, err := database.DB().Begin()
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	type existing struct {
		id             int
		customName     bool
		source         string
		active         bool
		disabledBySync bool
		displayName    string
	}
	current := make(map[string]existing)
	rows, err := tx.Query(`SELECT id, original_id, COALESCE(custom_name, 0), COALESCE(source, ''), is_active,
		COALESCE(disabled_by_sync, 0), COALESCE(display_name, original_id) FROM models WHERE provider_id = ?`, providerID)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var e existing
		var originalID string
		var customName, active, bySync int
		if err := rows.Scan(&e.id, &originalID, &customName, &e.source, &active, &bySync, &e.displayName); err != nil {
			rows.Close()
			return res, err
		}
		e.customName = customName == 1
		e.active = active == 1
		e.disabledBySync = bySync == 1
		current[originalID] = e
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}

	renames := map[string]string{}
	fetched := make(map[string]bool)
	for _, m := range modelsData {
		modelID, ok := m["id"].(string)
		if !ok || modelID == "" || fetched[modelID] {
			continue
		}
		fetched[modelID] = true

		displayName := modelID
		if modelPrefix != "" {
			displayName = modelPrefix + "/" + modelID
		}

		if e, exists := current[modelID]; exists {
			source := e.source
			if source != "manual" {
				source = "fetched"
			}
			name := e.displayName
			if !e.customName && name != displayName {
				name = displayName
				renames[e.displayName] = displayName
			}
			if _, err := tx.Exec("UPDATE models SET display_name = ?, source = ? WHERE id = ?", name, source, e.id); err != nil {
				return res, err
			}
			res.updated++

			// 之前因上游下线被自动停用，现在又出现了：恢复启用（不能与其他已启用模型重名）
			if e.disabledBySync && !e.active {
				conflicts, err := activeNameConflicts(tx, []int{e.id})
				if err != nil {
					return res, err
				}
				if len(conflicts) == 0 {
					if _, err := tx.Exec("UPDATE models SET is_active = 1, disabled_by_sync = 0 WHERE id = ?", e.id); err != nil {
						return res, err
					}
					res.restored++
				}
			}
			continue
		}

		taken, err := displayNameTaken(tx, displayName)
		if err != nil {
			return res, err
		}
		if taken {
			res.duplicates++ // 仍然添加（默认不启用），启用时会被拦下并提示改名
		}
		if _, err := tx.Exec(`
			INSERT INTO models (provider_id, original_id, display_name, custom_name, is_active, source)
			VALUES (?, ?, ?, 0, 0, 'fetched')
		`, providerID, modelID, displayName); err != nil {
			return res, err
		}
		res.added++
	}

	for originalID, e := range current {
		if fetched[originalID] {
			continue
		}
		switch {
		case e.source == "manual":
			// 保留
		case e.customName || e.source == "":
			if e.active {
				if _, err := tx.Exec("UPDATE models SET is_active = 0, disabled_by_sync = 1 WHERE id = ?", e.id); err != nil {
					return res, err
				}
				res.disabled++
			}
		default:
			if _, err := tx.Exec("DELETE FROM models WHERE id = ?", e.id); err != nil {
				return res, err
			}
			res.deleted++
		}
	}

	if err := renameModelReferences(tx, renames); err != nil {
		return res, err
	}
	return res, tx.Commit()
}

type prefixConflictError []string

func (e prefixConflictError) Error() string {
	return fmt.Sprintf("修改前缀后以下模型名称会与其他模型重复：%s", strings.Join(e, "、"))
}

// applyPrefixChange 按新前缀重算提供商下非自定义名称模型的显示名称，返回 旧名称→新名称。
// 新名称与其他模型重复时返回 prefixConflictError，不做任何修改。
func applyPrefixChange(tx queryer, providerID int, newPrefix string) (map[string]string, error) {
	type item struct {
		id               int
		oldName, newName string
	}
	rows, err := tx.Query(`SELECT id, original_id, COALESCE(display_name, original_id) FROM models
		WHERE provider_id = ? AND (custom_name = 0 OR custom_name IS NULL)`, providerID)
	if err != nil {
		return nil, err
	}
	var items []item
	var ids []int
	for rows.Next() {
		var it item
		var originalID string
		if err := rows.Scan(&it.id, &originalID, &it.oldName); err != nil {
			rows.Close()
			return nil, err
		}
		it.newName = originalID
		if newPrefix != "" {
			it.newName = newPrefix + "/" + originalID
		}
		items = append(items, it)
		ids = append(ids, it.id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var conflicts prefixConflictError
	for _, it := range items {
		taken, err := displayNameTaken(tx, it.newName, ids...)
		if err != nil {
			return nil, err
		}
		if taken {
			conflicts = append(conflicts, it.newName)
		}
	}
	if len(conflicts) > 0 {
		return nil, conflicts
	}

	renames := map[string]string{}
	for _, it := range items {
		if it.newName == it.oldName {
			continue
		}
		if _, err := tx.Exec("UPDATE models SET display_name = ? WHERE id = ?", it.newName, it.id); err != nil {
			return nil, err
		}
		renames[it.oldName] = it.newName
	}
	return renames, nil
}
