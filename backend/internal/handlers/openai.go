package handlers

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"vte/internal/database"
	"vte/internal/logger"
	"vte/internal/proxy"
)

// getMaxRetries 从数据库获取最大重试次数
func getMaxRetries() int {
	db := database.DB()
	var maxRetries string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'max_retries'").Scan(&maxRetries)
	if err != nil {
		return 3 // 默认3次
	}
	retries, err := strconv.Atoi(maxRetries)
	if err != nil {
		return 3
	}
	return retries
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func OpenAIListModels(c *gin.Context) {
	db := database.DB()
	rows, err := db.Query(`
		SELECT m.display_name, m.original_id, p.name
		FROM models m
		JOIN providers p ON m.provider_id = p.id
		WHERE m.is_active = 1 AND p.is_active = 1
	`)
	if err != nil {
		c.JSON(500, gin.H{"detail": "查询失败"})
		return
	}
	defer rows.Close()

	var data []gin.H
	currentTime := time.Now().Unix()
	for rows.Next() {
		var displayName, originalID, providerName *string
		rows.Scan(&displayName, &originalID, &providerName)

		modelID := ""
		if displayName != nil && *displayName != "" {
			modelID = *displayName
		} else if originalID != nil {
			modelID = *originalID
		}

		ownedBy := "unknown"
		if providerName != nil {
			ownedBy = *providerName
		}

		data = append(data, gin.H{
			"id":       modelID,
			"object":   "model",
			"created":  currentTime,
			"owned_by": ownedBy,
		})
	}

	logger.Info(fmt.Sprintf("%s | 获取模型列表 | %d个", c.ClientIP(), len(data)))
	c.JSON(200, gin.H{"object": "list", "data": data})
}

func OpenAIChatCompletions(c *gin.Context) {
	var payload map[string]interface{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(400, gin.H{"detail": "无效的 JSON"})
		return
	}

	modelName, ok := payload["model"].(string)
	if !ok || modelName == "" {
		c.JSON(400, gin.H{"detail": "缺少 model 参数"})
		return
	}

	stream := false
	if s, ok := payload["stream"].(bool); ok {
		stream = s
	}

	// 获取流式模式设置
	db := database.DB()
	var streamMode string
	db.QueryRow("SELECT value FROM settings WHERE key = 'stream_mode'").Scan(&streamMode)

	if streamMode == "force_stream" {
		stream = true
		payload["stream"] = true
	} else if streamMode == "force_non_stream" {
		stream = false
		payload["stream"] = false
	}

	// 如果是流式请求，添加 stream_options 以获取 usage 信息
	if stream {
		if _, exists := payload["stream_options"]; !exists {
			payload["stream_options"] = map[string]interface{}{
				"include_usage": true,
			}
		}
	}

	// 查找模型
	model, provider, err := findModel(modelName)
	if err != nil {
		c.JSON(404, gin.H{"detail": fmt.Sprintf("模型不存在: %s", modelName)})
		return
	}

	if !provider.IsActive {
		c.JSON(503, gin.H{"detail": fmt.Sprintf("提供商已禁用: %s", provider.Name)})
		return
	}

	// 替换模型名为原始 ID
	originalID := model.OriginalID
	if provider.ProviderType == "vertex_express" && len(originalID) > 0 {
		if len(originalID) < 7 || originalID[:7] != "google/" {
			originalID = "google/" + originalID
		}
	}
	payload["model"] = originalID

	// 构建客户端配置
	cfg := &proxy.ProviderConfig{
		BaseURL:        provider.BaseURL,
		APIKey:         provider.APIKey,
		ProviderType:   provider.ProviderType,
		VertexProject:  provider.VertexProject,
		VertexLocation: provider.VertexLocation,
		ProxyURL:       provider.ProxyURL,
	}

	if provider.ExtraHeaders != "" {
		json.Unmarshal([]byte(provider.ExtraHeaders), &cfg.ExtraHeaders)
	}

	startTime := time.Now()
	logger.RequestStart()

	if stream {
		handleStreamResponse(c, cfg, payload, modelName, startTime)
	} else {
		handleNonStreamResponse(c, cfg, payload, modelName, startTime)
	}
}

func handleNonStreamResponse(c *gin.Context, cfg *proxy.ProviderConfig, payload map[string]interface{}, modelName string, startTime time.Time) {
	maxRetries := getMaxRetries()
	result, err := cfg.ChatCompletionWithRetry(payload, maxRetries)
	duration := time.Since(startTime).Seconds()

	if err != nil {
		logger.Error(fmt.Sprintf("%s | %s | %.2fs | %v", c.ClientIP(), modelName, duration, err))
		logger.RequestError()
		c.JSON(500, gin.H{"detail": fmt.Sprintf("请求失败: %v", err)})
		return
	}

	// 记录token使用情况
	if usage, ok := result["usage"].(map[string]interface{}); ok {
		promptTokens := 0
		completionTokens := 0
		totalTokens := 0
		
		if pt, ok := usage["prompt_tokens"].(float64); ok {
			promptTokens = int(pt)
		}
		if ct, ok := usage["completion_tokens"].(float64); ok {
			completionTokens = int(ct)
		}
		if tt, ok := usage["total_tokens"].(float64); ok {
			totalTokens = int(tt)
		}
		
		// 获取provider名称
		model, provider, _ := findModel(modelName)
		providerName := "unknown"
		if provider != nil {
			providerName = provider.Name
		}
		displayName := modelName
		if model != nil && model.DisplayName != "" {
			displayName = model.DisplayName
		}
		
		RecordTokenUsage(displayName, providerName, promptTokens, completionTokens, totalTokens)
	}

	logger.Info(fmt.Sprintf("%s | %s | %.2fs", c.ClientIP(), modelName, duration))
	logger.RequestSuccess()
	c.JSON(200, result)
}

func handleStreamResponse(c *gin.Context, cfg *proxy.ProviderConfig, payload map[string]interface{}, modelName string, startTime time.Time) {
	maxRetries := getMaxRetries()
	resp, err := cfg.ChatCompletionStreamWithRetry(payload, maxRetries)
	if err != nil {
		duration := time.Since(startTime).Seconds()
		logger.Error(fmt.Sprintf("%s | %s | %.2fs | %v", c.ClientIP(), modelName, duration, err))
		logger.RequestError()
		c.JSON(500, gin.H{"detail": fmt.Sprintf("请求失败: %v", err)})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		duration := time.Since(startTime).Seconds()
		logger.Error(fmt.Sprintf("%s | %s | %.2fs | status %d: %s", c.ClientIP(), modelName, duration, resp.StatusCode, string(body)))
		logger.RequestError()
		c.JSON(resp.StatusCode, gin.H{"detail": string(body)})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// 用于累积token统计
	var totalPromptTokens, totalCompletionTokens, totalTotalTokens int
	// 用于估算token（当API不返回usage时的备选方案）
	var estimatedInputTokens, estimatedOutputTokens int
	
	// 从请求中估算输入token数（粗略估算：每4个字符约1个token）
	if messages, ok := payload["messages"].([]interface{}); ok {
		for _, msg := range messages {
			if m, ok := msg.(map[string]interface{}); ok {
				if content, ok := m["content"].(string); ok {
					estimatedInputTokens += len(content) / 4
				}
			}
		}
	}
	
	c.Stream(func(w io.Writer) bool {
		buf := make([]byte, 4096)
		n, err := resp.Body.Read(buf)
		if n > 0 {
			data := string(buf[:n])
			
			// 尝试解析SSE数据中的usage信息
			if strings.Contains(data, "\"usage\"") {
				lines := strings.Split(data, "\n")
				for _, line := range lines {
					if strings.HasPrefix(line, "data: ") && !strings.Contains(line, "[DONE]") {
						jsonData := strings.TrimPrefix(line, "data: ")
						var chunk map[string]interface{}
						if json.Unmarshal([]byte(jsonData), &chunk) == nil {
							if usage, ok := chunk["usage"].(map[string]interface{}); ok {
								if pt, ok := usage["prompt_tokens"].(float64); ok {
									totalPromptTokens = int(pt)
								}
								if ct, ok := usage["completion_tokens"].(float64); ok {
									totalCompletionTokens = int(ct)
								}
								if tt, ok := usage["total_tokens"].(float64); ok {
									totalTotalTokens = int(tt)
								}
							}
						}
					}
				}
			}
			
			// 估算输出token（从流式内容中提取）
			lines := strings.Split(data, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "data: ") && !strings.Contains(line, "[DONE]") {
					jsonData := strings.TrimPrefix(line, "data: ")
					var chunk map[string]interface{}
					if json.Unmarshal([]byte(jsonData), &chunk) == nil {
						if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
							if choice, ok := choices[0].(map[string]interface{}); ok {
								if delta, ok := choice["delta"].(map[string]interface{}); ok {
									if content, ok := delta["content"].(string); ok {
										estimatedOutputTokens += len(content) / 4
									}
								}
							}
						}
					}
				}
			}
			
			w.Write(buf[:n])
			return true
		}
		if err != nil {
			duration := time.Since(startTime).Seconds()
			if err == io.EOF {
				// 获取模型和提供商信息
				model, provider, _ := findModel(modelName)
				providerName := "unknown"
				if provider != nil {
					providerName = provider.Name
				}
				displayName := modelName
				if model != nil && model.DisplayName != "" {
					displayName = model.DisplayName
				}
				
				// 记录token使用情况
				if totalTotalTokens > 0 {
					// API返回了准确的usage信息
					RecordTokenUsage(displayName, providerName, totalPromptTokens, totalCompletionTokens, totalTotalTokens)
				} else if estimatedInputTokens > 0 || estimatedOutputTokens > 0 {
					// API没有返回usage，使用估算值（标记为估算）
					// 确保至少有1个token
					if estimatedInputTokens == 0 {
						estimatedInputTokens = 1
					}
					if estimatedOutputTokens == 0 {
						estimatedOutputTokens = 1
					}
					estimatedTotal := estimatedInputTokens + estimatedOutputTokens
					RecordTokenUsage(displayName, providerName, estimatedInputTokens, estimatedOutputTokens, estimatedTotal)
					logger.Info(fmt.Sprintf("%s | %s | Token估算: ~%d (API未返回usage)", c.ClientIP(), modelName, estimatedTotal))
				}
				
				logger.Info(fmt.Sprintf("%s | %s | %.2fs", c.ClientIP(), modelName, duration))
				logger.RequestSuccess()
			} else {
				logger.Error(fmt.Sprintf("%s | %s | %.2fs | %v", c.ClientIP(), modelName, duration, err))
				logger.RequestError()
			}
			return false
		}
		return true
	})
}

type modelWithProvider struct {
	Model    *modelInfo
	Provider *providerInfo
}

type modelInfo struct {
	ID          int
	OriginalID  string
	DisplayName string
}

type providerInfo struct {
	ID             int
	Name           string
	BaseURL        string
	APIKey         string
	ProviderType   string
	VertexProject  string
	VertexLocation string
	ExtraHeaders   string
	ProxyURL       string
	IsActive       bool
}

func findModel(modelName string) (*modelInfo, *providerInfo, error) {
	db := database.DB()

	// 先查 display_name
	row := db.QueryRow(`
		SELECT m.id, m.original_id, m.display_name,
		       p.id, p.name, p.base_url, p.api_key, p.provider_type, 
		       COALESCE(p.vertex_project, ''), COALESCE(p.vertex_location, 'global'),
		       COALESCE(p.extra_headers, ''), COALESCE(p.proxy_url, ''), p.is_active
		FROM models m
		JOIN providers p ON m.provider_id = p.id
		WHERE m.display_name = ? AND m.is_active = 1 AND p.is_active = 1
	`, modelName)

	model, provider, err := scanModelProvider(row)
	if err == nil {
		return model, provider, nil
	}

	// 再查 original_id
	row = db.QueryRow(`
		SELECT m.id, m.original_id, m.display_name,
		       p.id, p.name, p.base_url, p.api_key, p.provider_type, 
		       COALESCE(p.vertex_project, ''), COALESCE(p.vertex_location, 'global'),
		       COALESCE(p.extra_headers, ''), COALESCE(p.proxy_url, ''), p.is_active
		FROM models m
		JOIN providers p ON m.provider_id = p.id
		WHERE m.original_id = ? AND m.is_active = 1 AND p.is_active = 1
	`, modelName)

	model, provider, err = scanModelProvider(row)
	if err == nil {
		return model, provider, nil
	}

	// 尝试去掉前缀
	if idx := len(modelName); idx > 0 {
		for i, ch := range modelName {
			if ch == '/' {
				nameWithoutPrefix := modelName[i+1:]
				row = db.QueryRow(`
					SELECT m.id, m.original_id, m.display_name,
					       p.id, p.name, p.base_url, p.api_key, p.provider_type, 
					       COALESCE(p.vertex_project, ''), COALESCE(p.vertex_location, 'global'),
					       COALESCE(p.extra_headers, ''), COALESCE(p.proxy_url, ''), p.is_active
					FROM models m
					JOIN providers p ON m.provider_id = p.id
					WHERE m.original_id = ? AND m.is_active = 1 AND p.is_active = 1
				`, nameWithoutPrefix)

				model, provider, err = scanModelProvider(row)
				if err == nil {
					return model, provider, nil
				}
				break
			}
		}
	}

	return nil, nil, fmt.Errorf("model not found")
}

func scanModelProvider(row *sql.Row) (*modelInfo, *providerInfo, error) {
	var model modelInfo
	var provider providerInfo
	var displayName *string
	var isActive int

	err := row.Scan(
		&model.ID, &model.OriginalID, &displayName,
		&provider.ID, &provider.Name, &provider.BaseURL, &provider.APIKey,
		&provider.ProviderType, &provider.VertexProject, &provider.VertexLocation,
		&provider.ExtraHeaders, &provider.ProxyURL, &isActive,
	)
	if err != nil {
		return nil, nil, err
	}

	if displayName != nil {
		model.DisplayName = *displayName
	}
	provider.IsActive = isActive == 1

	return &model, &provider, nil
}


// OpenAIChatCompletionsWS 处理 WebSocket 连接的聊天完成请求
func OpenAIChatCompletionsWS(c *gin.Context) {
	// 从查询参数或 header 获取 API Key
	apiKey := c.Query("api_key")
	if apiKey == "" {
		apiKey = c.GetHeader("Authorization")
		if strings.HasPrefix(apiKey, "Bearer ") {
			apiKey = strings.TrimPrefix(apiKey, "Bearer ")
		}
	}

	if apiKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "缺少 API Key"})
		return
	}

	// 验证 API Key
	db := database.DB()
	var userID int
	err := db.QueryRow("SELECT id FROM users WHERE api_key = ? AND is_active = 1", apiKey).Scan(&userID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "无效的 API Key"})
		return
	}

	// 升级为 WebSocket 连接
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error(fmt.Sprintf("WebSocket 升级失败: %v", err))
		return
	}
	defer conn.Close()

	for {
		// 读取消息
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Error(fmt.Sprintf("WebSocket 读取错误: %v", err))
			}
			break
		}

		// 解析请求
		var payload map[string]interface{}
		if err := json.Unmarshal(message, &payload); err != nil {
			conn.WriteJSON(gin.H{"error": "无效的 JSON"})
			continue
		}

		modelName, ok := payload["model"].(string)
		if !ok || modelName == "" {
			conn.WriteJSON(gin.H{"error": "缺少 model 参数"})
			continue
		}

		// 强制使用流式
		payload["stream"] = true

		// 查找模型
		model, provider, err := findModel(modelName)
		if err != nil {
			conn.WriteJSON(gin.H{"error": fmt.Sprintf("模型不存在: %s", modelName)})
			continue
		}

		if !provider.IsActive {
			conn.WriteJSON(gin.H{"error": fmt.Sprintf("提供商已禁用: %s", provider.Name)})
			continue
		}

		// 替换模型名为原始 ID
		originalID := model.OriginalID
		if provider.ProviderType == "vertex_express" && len(originalID) > 0 {
			if len(originalID) < 7 || originalID[:7] != "google/" {
				originalID = "google/" + originalID
			}
		}
		payload["model"] = originalID

		// 构建客户端配置
		cfg := &proxy.ProviderConfig{
			BaseURL:        provider.BaseURL,
			APIKey:         provider.APIKey,
			ProviderType:   provider.ProviderType,
			VertexProject:  provider.VertexProject,
			VertexLocation: provider.VertexLocation,
			ProxyURL:       provider.ProxyURL,
		}

		if provider.ExtraHeaders != "" {
			json.Unmarshal([]byte(provider.ExtraHeaders), &cfg.ExtraHeaders)
		}

		startTime := time.Now()
		logger.RequestStart()

		// 发起流式请求
		resp, err := cfg.ChatCompletionStream(payload)
		if err != nil {
			duration := time.Since(startTime).Seconds()
			logger.Error(fmt.Sprintf("WebSocket | %s | %.2fs | %v", modelName, duration, err))
			logger.RequestError()
			conn.WriteJSON(gin.H{"error": fmt.Sprintf("请求失败: %v", err)})
			continue
		}

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			duration := time.Since(startTime).Seconds()
			logger.Error(fmt.Sprintf("WebSocket | %s | %.2fs | status %d", modelName, duration, resp.StatusCode))
			logger.RequestError()
			conn.WriteJSON(gin.H{"error": string(body)})
			continue
		}

		// 流式转发响应
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err != io.EOF {
					logger.Error(fmt.Sprintf("WebSocket 读取响应错误: %v", err))
				}
				break
			}

			lineStr := strings.TrimSpace(string(line))
			if lineStr == "" {
				continue
			}

			// 发送 SSE 数据到 WebSocket
			if err := conn.WriteMessage(websocket.TextMessage, []byte(lineStr)); err != nil {
				logger.Error(fmt.Sprintf("WebSocket 写入错误: %v", err))
				break
			}

			// 检查是否结束
			if lineStr == "data: [DONE]" {
				break
			}
		}
		resp.Body.Close()

		duration := time.Since(startTime).Seconds()
		logger.Info(fmt.Sprintf("WebSocket | %s | %.2fs", modelName, duration))
		logger.RequestSuccess()
	}
}
