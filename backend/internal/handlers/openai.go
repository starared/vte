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
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"vte/internal/database"
	"vte/internal/logger"
	"vte/internal/proxy"
	"vte/internal/tokenizer"
)

// 并发控制
var (
	currentConcurrency int64          // 当前并发数
	concurrencyMu      sync.RWMutex   // 并发设置锁
)

// 自定义并发控制 - 基于提供商/模型
var (
	customConcurrencyMu      sync.Mutex
	customConcurrencyCurrent = make(map[string]int64) // key -> 当前并发数
)

// CustomConcurrencyRule 自定义并发限制规则
type CustomConcurrencyRule struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`           // 规则名称
	ProviderID   int    `json:"provider_id"`    // 提供商ID，0表示所有
	ProviderName string `json:"provider_name"`  // 提供商名称（仅显示用）
	ModelName    string `json:"model_name"`     // 模型名称，空表示所有
	Limit        int    `json:"limit"`          // 最大并发数
	Enabled      bool   `json:"enabled"`        // 是否启用
}

// 速率限制 - 使用滑动窗口
var (
	rateLimitMu     sync.Mutex
	requestTimes    []time.Time // 请求时间记录
)

// 自定义速率限制 - 基于提供商/模型
var (
	customRateLimitMu     sync.Mutex
	customRequestTimes    = make(map[string][]time.Time) // key: "provider:xxx" 或 "model:xxx" 或 "provider:xxx:model:yyy"
)

// CustomRateLimitRule 自定义速率限制规则
type CustomRateLimitRule struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`           // 规则名称
	ProviderID   int    `json:"provider_id"`    // 提供商ID，0表示所有
	ProviderName string `json:"provider_name"`  // 提供商名称（仅显示用）
	ModelName    string `json:"model_name"`     // 模型名称，空表示所有
	MaxRequests  int    `json:"max_requests"`   // 最大请求数
	Window       int    `json:"window"`         // 时间窗口（秒）
	Enabled      bool   `json:"enabled"`        // 是否启用
}

// getRateLimitSettings 获取速率限制设置
func getRateLimitSettings() (enabled bool, maxRequests int, windowSeconds int) {
	db := database.DB()
	var enabledStr, maxReqStr, windowStr string
	
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'rate_limit_enabled'").Scan(&enabledStr)
	if err != nil || enabledStr != "true" {
		return false, 0, 0
	}
	
	db.QueryRow("SELECT value FROM settings WHERE key = 'rate_limit_max_requests'").Scan(&maxReqStr)
	db.QueryRow("SELECT value FROM settings WHERE key = 'rate_limit_window'").Scan(&windowStr)
	
	maxRequests, _ = strconv.Atoi(maxReqStr)
	windowSeconds, _ = strconv.Atoi(windowStr)
	
	if maxRequests <= 0 {
		maxRequests = 60
	}
	if windowSeconds <= 0 {
		windowSeconds = 60
	}
	
	return true, maxRequests, windowSeconds
}

// checkRateLimit 检查速率限制
func checkRateLimit() bool {
	enabled, maxRequests, windowSeconds := getRateLimitSettings()
	if !enabled {
		return true
	}
	
	rateLimitMu.Lock()
	defer rateLimitMu.Unlock()
	
	now := time.Now()
	windowStart := now.Add(-time.Duration(windowSeconds) * time.Second)
	
	// 清理过期记录
	validTimes := make([]time.Time, 0, len(requestTimes))
	for _, t := range requestTimes {
		if t.After(windowStart) {
			validTimes = append(validTimes, t)
		}
	}
	requestTimes = validTimes
	
	// 检查是否超限
	if len(requestTimes) >= maxRequests {
		return false
	}
	
	// 记录本次请求
	requestTimes = append(requestTimes, now)
	return true
}

// getCustomRateLimitRules 获取自定义速率限制规则
func getCustomRateLimitRules() []CustomRateLimitRule {
	db := database.DB()
	var rulesJSON string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'custom_rate_limit_rules'").Scan(&rulesJSON)
	if err != nil || rulesJSON == "" {
		return nil
	}
	
	var rules []CustomRateLimitRule
	json.Unmarshal([]byte(rulesJSON), &rules)
	return rules
}

// checkCustomRateLimit 检查自定义速率限制
// 返回: (是否通过, 触发的规则名称)
func checkCustomRateLimit(providerID int, providerName string, modelName string) (bool, string) {
	rules := getCustomRateLimitRules()
	if len(rules) == 0 {
		return true, ""
	}
	
	customRateLimitMu.Lock()
	defer customRateLimitMu.Unlock()
	
	now := time.Now()
	
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		
		// 检查规则是否匹配
		matched := false
		var key string
		
		if rule.ProviderID > 0 && rule.ModelName != "" {
			// 特定提供商的特定模型
			if rule.ProviderID == providerID && rule.ModelName == modelName {
				matched = true
				key = fmt.Sprintf("provider:%d:model:%s", providerID, modelName)
			}
		} else if rule.ProviderID > 0 {
			// 特定提供商的所有模型
			if rule.ProviderID == providerID {
				matched = true
				key = fmt.Sprintf("provider:%d", providerID)
			}
		} else if rule.ModelName != "" {
			// 所有提供商的特定模型
			if rule.ModelName == modelName {
				matched = true
				key = fmt.Sprintf("model:%s", modelName)
			}
		}
		
		if !matched {
			continue
		}
		
		// 检查此规则的速率限制
		windowStart := now.Add(-time.Duration(rule.Window) * time.Second)
		
		// 清理过期记录
		times := customRequestTimes[key]
		validTimes := make([]time.Time, 0, len(times))
		for _, t := range times {
			if t.After(windowStart) {
				validTimes = append(validTimes, t)
			}
		}
		customRequestTimes[key] = validTimes
		
		// 检查是否超限
		if len(validTimes) >= rule.MaxRequests {
			return false, rule.Name
		}
		
		// 记录本次请求
		customRequestTimes[key] = append(validTimes, now)
	}
	
	return true, ""
}

// getConcurrencyLimit 获取并发限制
func getConcurrencyLimit() int {
	db := database.DB()
	var enabledStr, limitStr string
	
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'concurrency_enabled'").Scan(&enabledStr)
	if err != nil || enabledStr != "true" {
		return 0 // 0 表示不限制
	}
	
	db.QueryRow("SELECT value FROM settings WHERE key = 'concurrency_limit'").Scan(&limitStr)
	limit, _ := strconv.Atoi(limitStr)
	
	if limit <= 0 {
		return 0
	}
	return limit
}

// acquireConcurrency 获取并发槽
func acquireConcurrency() bool {
	limit := getConcurrencyLimit()
	if limit == 0 {
		atomic.AddInt64(&currentConcurrency, 1)
		return true
	}
	
	current := atomic.LoadInt64(&currentConcurrency)
	if current >= int64(limit) {
		return false
	}
	
	atomic.AddInt64(&currentConcurrency, 1)
	return true
}

// releaseConcurrency 释放并发槽
func releaseConcurrency() {
	atomic.AddInt64(&currentConcurrency, -1)
}

// GetCurrentConcurrency 获取当前并发数（用于API）
func GetCurrentConcurrency() int64 {
	return atomic.LoadInt64(&currentConcurrency)
}

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

// CustomErrorRule 自定义错误响应规则
type CustomErrorRule struct {
	Keyword  string `json:"keyword"`
	Response string `json:"response"`
}

// getCustomErrorRules 获取自定义错误响应规则
func getCustomErrorRules(db *sql.DB) (bool, []CustomErrorRule) {
	var enabled, rulesJSON string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'custom_error_enabled'").Scan(&enabled)
	if err != nil || enabled != "true" {
		return false, nil
	}
	
	err = db.QueryRow("SELECT value FROM settings WHERE key = 'custom_error_rules'").Scan(&rulesJSON)
	if err != nil {
		return false, nil
	}
	
	var rules []CustomErrorRule
	json.Unmarshal([]byte(rulesJSON), &rules)
	return true, rules
}

// checkCustomErrorResponse 检查错误是否匹配自定义响应规则
func checkCustomErrorResponse(db *sql.DB, errMsg string) (bool, string) {
	enabled, rules := getCustomErrorRules(db)
	if !enabled || len(rules) == 0 {
		return false, ""
	}
	
	errMsgLower := strings.ToLower(errMsg)
	for _, rule := range rules {
		if rule.Keyword != "" && strings.Contains(errMsgLower, strings.ToLower(rule.Keyword)) {
			return true, rule.Response
		}
	}
	return false, ""
}

// buildFakeResponse 构建伪造的正常响应
func buildFakeResponse(content string, model string) map[string]interface{} {
	return map[string]interface{}{
		"id":      "chatcmpl-fake-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     0,
			"completion_tokens": len(content) / 4,
			"total_tokens":      len(content) / 4,
		},
	}
}

// buildFakeStreamResponse 构建伪造的流式响应
func buildFakeStreamResponse(content string, model string) string {
	id := "chatcmpl-fake-" + fmt.Sprintf("%d", time.Now().UnixNano())
	created := time.Now().Unix()
	
	// 构建流式响应数据
	var sb strings.Builder
	
	// 第一个chunk - role
	chunk1 := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"delta": map[string]interface{}{
					"role": "assistant",
				},
				"finish_reason": nil,
			},
		},
	}
	data1, _ := json.Marshal(chunk1)
	sb.WriteString("data: ")
	sb.WriteString(string(data1))
	sb.WriteString("\n\n")
	
	// 第二个chunk - content
	chunk2 := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"delta": map[string]interface{}{
					"content": content,
				},
				"finish_reason": nil,
			},
		},
	}
	data2, _ := json.Marshal(chunk2)
	sb.WriteString("data: ")
	sb.WriteString(string(data2))
	sb.WriteString("\n\n")
	
	// 第三个chunk - finish
	chunk3 := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"delta":         map[string]interface{}{},
				"finish_reason": "stop",
			},
		},
	}
	data3, _ := json.Marshal(chunk3)
	sb.WriteString("data: ")
	sb.WriteString(string(data3))
	sb.WriteString("\n\n")
	
	// 结束标记
	sb.WriteString("data: [DONE]\n\n")
	
	return sb.String()
}

// injectSystemPrompt 注入系统前置提示词
func injectSystemPrompt(db *sql.DB, payload map[string]interface{}) {
	// 检查是否启用
	var enabled string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'system_prompt_enabled'").Scan(&enabled)
	if err != nil || enabled != "true" {
		return
	}

	// 获取提示词内容
	var prompt string
	err = db.QueryRow("SELECT value FROM settings WHERE key = 'system_prompt'").Scan(&prompt)
	if err != nil || prompt == "" {
		return
	}

	// 获取现有 messages
	messages, ok := payload["messages"].([]interface{})
	if !ok {
		return
	}

	// 创建系统提示词消息
	systemMsg := map[string]interface{}{
		"role":    "system",
		"content": prompt,
	}

	// 在最前面插入系统提示词
	newMessages := make([]interface{}, 0, len(messages)+1)
	newMessages = append(newMessages, systemMsg)
	newMessages = append(newMessages, messages...)
	payload["messages"] = newMessages
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

	c.JSON(200, gin.H{"object": "list", "data": data})
}

func OpenAIChatCompletions(c *gin.Context) {
	db := database.DB()
	
	// 先解析请求获取模型名和stream参数，用于后续的自定义错误响应
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
	
	// 检查速率限制
	if !checkRateLimit() {
		errMsg := "请求过于频繁，请稍后重试 rate_limit_exceeded"
		if matched, customResponse := checkCustomErrorResponse(db, errMsg); matched {
			logger.Error(fmt.Sprintf("%s | %s | 自定义响应(原错误: 全局速率限制)", c.ClientIP(), modelName))
			if stream {
				c.Header("Content-Type", "text/event-stream")
				c.Header("Cache-Control", "no-cache")
				c.Header("Connection", "keep-alive")
				c.String(200, buildFakeStreamResponse(customResponse, modelName))
			} else {
				c.JSON(200, buildFakeResponse(customResponse, modelName))
			}
			return
		}
		c.JSON(429, gin.H{
			"error": gin.H{
				"message": "请求过于频繁，请稍后重试",
				"type":    "rate_limit_error",
				"code":    "rate_limit_exceeded",
			},
		})
		return
	}
	
	// 检查并发限制
	if !acquireConcurrency() {
		errMsg := "服务器繁忙，请稍后重试 concurrency_limit_exceeded"
		if matched, customResponse := checkCustomErrorResponse(db, errMsg); matched {
			logger.Error(fmt.Sprintf("%s | %s | 自定义响应(原错误: 并发限制)", c.ClientIP(), modelName))
			if stream {
				c.Header("Content-Type", "text/event-stream")
				c.Header("Cache-Control", "no-cache")
				c.Header("Connection", "keep-alive")
				c.String(200, buildFakeStreamResponse(customResponse, modelName))
			} else {
				c.JSON(200, buildFakeResponse(customResponse, modelName))
			}
			return
		}
		c.JSON(503, gin.H{
			"error": gin.H{
				"message": "服务器繁忙，请稍后重试",
				"type":    "concurrency_limit_error",
				"code":    "concurrency_limit_exceeded",
			},
		})
		return
	}
	defer releaseConcurrency()

	// 获取流式模式设置
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

	// 注入系统前置提示词
	injectSystemPrompt(db, payload)

	// 查找模型
	model, provider, err := findModel(modelName)
	if err != nil {
		errMsg := fmt.Sprintf("模型不存在: %s", modelName)
		if matched, customResponse := checkCustomErrorResponse(db, errMsg); matched {
			logger.Error(fmt.Sprintf("%s | %s | 自定义响应(原错误: 模型不存在)", c.ClientIP(), modelName))
			if stream {
				c.Header("Content-Type", "text/event-stream")
				c.Header("Cache-Control", "no-cache")
				c.Header("Connection", "keep-alive")
				c.String(200, buildFakeStreamResponse(customResponse, modelName))
			} else {
				c.JSON(200, buildFakeResponse(customResponse, modelName))
			}
			return
		}
		c.JSON(404, gin.H{"detail": errMsg})
		return
	}

	if !provider.IsActive {
		errMsg := fmt.Sprintf("提供商已禁用: %s", provider.Name)
		if matched, customResponse := checkCustomErrorResponse(db, errMsg); matched {
			logger.Error(fmt.Sprintf("%s | %s | 自定义响应(原错误: 提供商禁用)", c.ClientIP(), modelName))
			if stream {
				c.Header("Content-Type", "text/event-stream")
				c.Header("Cache-Control", "no-cache")
				c.Header("Connection", "keep-alive")
				c.String(200, buildFakeStreamResponse(customResponse, modelName))
			} else {
				c.JSON(200, buildFakeResponse(customResponse, modelName))
			}
			return
		}
		c.JSON(503, gin.H{"detail": errMsg})
		return
	}

	// 检查自定义速率限制
	displayName := modelName
	if model.DisplayName != "" {
		displayName = model.DisplayName
	}
	if passed, ruleName := checkCustomRateLimit(provider.ID, provider.Name, displayName); !passed {
		errMsg := fmt.Sprintf("触发自定义速率限制规则 [%s]，请稍后重试 custom_rate_limit_exceeded", ruleName)
		if matched, customResponse := checkCustomErrorResponse(db, errMsg); matched {
			logger.Error(fmt.Sprintf("%s | %s | 自定义响应(原错误: 自定义速率限制 %s)", c.ClientIP(), modelName, ruleName))
			if stream {
				c.Header("Content-Type", "text/event-stream")
				c.Header("Cache-Control", "no-cache")
				c.Header("Connection", "keep-alive")
				c.String(200, buildFakeStreamResponse(customResponse, modelName))
			} else {
				c.JSON(200, buildFakeResponse(customResponse, modelName))
			}
			return
		}
		c.JSON(429, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("触发自定义速率限制规则 [%s]，请稍后重试", ruleName),
				"type":    "rate_limit_error",
				"code":    "custom_rate_limit_exceeded",
			},
		})
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
		errMsg := err.Error()
		
		// 检查是否有自定义错误响应
		db := database.DB()
		if matched, customResponse := checkCustomErrorResponse(db, errMsg); matched {
			logger.Info(fmt.Sprintf("%s | %s | %.2fs | 自定义响应(原错误: %s)", c.ClientIP(), modelName, duration, errMsg))
			logger.RequestSuccess()
			c.JSON(200, buildFakeResponse(customResponse, modelName))
			return
		}
		
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
		errMsg := err.Error()
		
		// 检查是否有自定义错误响应
		db := database.DB()
		if matched, customResponse := checkCustomErrorResponse(db, errMsg); matched {
			logger.Info(fmt.Sprintf("%s | %s | %.2fs | 自定义响应(原错误: %s)", c.ClientIP(), modelName, duration, errMsg))
			logger.RequestSuccess()
			
			// 返回伪造的流式响应
			c.Header("Content-Type", "text/event-stream")
			c.Header("Cache-Control", "no-cache")
			c.Header("Connection", "keep-alive")
			c.String(200, buildFakeStreamResponse(customResponse, modelName))
			return
		}
		
		logger.Error(fmt.Sprintf("%s | %s | %.2fs | %v", c.ClientIP(), modelName, duration, err))
		logger.RequestError()
		c.JSON(500, gin.H{"detail": fmt.Sprintf("请求失败: %v", err)})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		duration := time.Since(startTime).Seconds()
		
		// 检查是否有自定义错误响应
		db := database.DB()
		if matched, customResponse := checkCustomErrorResponse(db, bodyStr); matched {
			logger.Info(fmt.Sprintf("%s | %s | %.2fs | 自定义响应(原错误: status %d)", c.ClientIP(), modelName, duration, resp.StatusCode))
			logger.RequestSuccess()
			
			// 返回伪造的流式响应
			c.Header("Content-Type", "text/event-stream")
			c.Header("Cache-Control", "no-cache")
			c.Header("Connection", "keep-alive")
			c.String(200, buildFakeStreamResponse(customResponse, modelName))
			return
		}
		
		logger.Error(fmt.Sprintf("%s | %s | %.2fs | status %d: %s", c.ClientIP(), modelName, duration, resp.StatusCode, bodyStr))
		logger.RequestError()
		c.JSON(resp.StatusCode, gin.H{"detail": bodyStr})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// 用于累积 token 统计
	var totalPromptTokens, totalCompletionTokens, totalTotalTokens int
	// 用于收集输出内容（当 API 不返回 usage 时使用 tiktoken 计算）
	var outputContent strings.Builder
	
	// 使用 tiktoken 精确计算输入 token 数
	var inputTokens int
	if messages, ok := payload["messages"].([]interface{}); ok {
		inputTokens = tokenizer.CountMessagesTokens(messages, modelName)
	}
	
	// 用于跟踪请求是否已完成统计
	requestCompleted := false
	
	// 用于处理跨 buffer 的 SSE 数据
	var sseBuffer strings.Builder
	
	c.Stream(func(w io.Writer) bool {
		buf := make([]byte, 4096)
		n, err := resp.Body.Read(buf)
		if n > 0 {
			data := string(buf[:n])
			
			// 将数据追加到缓冲区
			sseBuffer.WriteString(data)
			bufferContent := sseBuffer.String()
			
			// 按完整的行处理
			lastNewline := strings.LastIndex(bufferContent, "\n")
			if lastNewline == -1 {
				// 没有完整的行，继续等待
				w.Write(buf[:n])
				return true
			}
			
			// 处理完整的行
			completeData := bufferContent[:lastNewline+1]
			// 保留不完整的部分
			sseBuffer.Reset()
			sseBuffer.WriteString(bufferContent[lastNewline+1:])
			
			// 尝试解析SSE数据中的usage信息
			if strings.Contains(completeData, "\"usage\"") {
				lines := strings.Split(completeData, "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
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
			
			// 收集输出内容（用于 tiktoken 计算 token）
			lines := strings.Split(completeData, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "data: ") && !strings.Contains(line, "[DONE]") {
					jsonData := strings.TrimPrefix(line, "data: ")
					var chunk map[string]interface{}
					if json.Unmarshal([]byte(jsonData), &chunk) == nil {
						if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
							if choice, ok := choices[0].(map[string]interface{}); ok {
								if delta, ok := choice["delta"].(map[string]interface{}); ok {
									if content, ok := delta["content"].(string); ok {
										outputContent.WriteString(content)
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
				
				// 记录token使用情况并打印日志（合并为一行）
				if totalTotalTokens > 0 {
					// API返回了准确的usage信息
					RecordTokenUsage(displayName, providerName, totalPromptTokens, totalCompletionTokens, totalTotalTokens)
					logger.Info(fmt.Sprintf("%s | %s | %.2fs | Token: %d (in=%d, out=%d)", c.ClientIP(), modelName, duration, totalTotalTokens, totalPromptTokens, totalCompletionTokens))
				} else {
					// API没有返回usage，使用 tiktoken 精确计算
					outputText := outputContent.String()
					outputTokens := tokenizer.CountTokens(outputText, modelName)
					
					if inputTokens > 0 || outputTokens > 0 {
						totalTokens := inputTokens + outputTokens
						RecordTokenUsage(displayName, providerName, inputTokens, outputTokens, totalTokens)
						logger.Info(fmt.Sprintf("%s | %s | %.2fs | Token: %d (in=%d, out=%d)", c.ClientIP(), modelName, duration, totalTokens, inputTokens, outputTokens))
					} else {
						logger.Info(fmt.Sprintf("%s | %s | %.2fs", c.ClientIP(), modelName, duration))
					}
				}
				
				logger.RequestSuccess()
				requestCompleted = true
			} else {
				logger.Error(fmt.Sprintf("%s | %s | %.2fs | %v", c.ClientIP(), modelName, duration, err))
				logger.RequestError()
				requestCompleted = true
			}
			return false
		}
		return true
	})
	
	// 如果流被中断但没有正常完成统计，记录为成功（数据可能已部分发送）
	if !requestCompleted {
		duration := time.Since(startTime).Seconds()
		logger.Info(fmt.Sprintf("%s | %s | %.2fs | 流被中断", c.ClientIP(), modelName, duration))
		logger.RequestSuccess()
	}
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
		// 获取轮询密钥
		if rotatedKey, _, err := GetNextAPIKey(provider.ID); err == nil && rotatedKey != "" {
			provider.APIKey = rotatedKey
		}
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
		// 获取轮询密钥
		if rotatedKey, _, err := GetNextAPIKey(provider.ID); err == nil && rotatedKey != "" {
			provider.APIKey = rotatedKey
		}
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
					// 获取轮询密钥
					if rotatedKey, _, err := GetNextAPIKey(provider.ID); err == nil && rotatedKey != "" {
						provider.APIKey = rotatedKey
					}
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
		
		// Token 统计变量
		var totalPromptTokens, totalCompletionTokens, totalTotalTokens int
		var estimatedOutputContent strings.Builder
		
		// 估算输入 token
		estimatedInputChars := 0
		if messages, ok := payload["messages"].([]interface{}); ok {
			for _, msg := range messages {
				if m, ok := msg.(map[string]interface{}); ok {
					if content, ok := m["content"].(string); ok {
						estimatedInputChars += len(content)
					}
				}
			}
		}
		
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

			// 解析 SSE 数据提取 token 信息
			if strings.HasPrefix(lineStr, "data: ") && !strings.Contains(lineStr, "[DONE]") {
				jsonData := strings.TrimPrefix(lineStr, "data: ")
				var chunk map[string]interface{}
				if json.Unmarshal([]byte(jsonData), &chunk) == nil {
					// 提取 usage 信息
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
					// 累积输出内容用于估算
					if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
						if choice, ok := choices[0].(map[string]interface{}); ok {
							if delta, ok := choice["delta"].(map[string]interface{}); ok {
								if content, ok := delta["content"].(string); ok {
									estimatedOutputContent.WriteString(content)
								}
							}
						}
					}
				}
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
		
		// 记录 token 使用情况
		displayName := modelName
		if model != nil && model.DisplayName != "" {
			displayName = model.DisplayName
		}
		providerName := "unknown"
		if provider != nil {
			providerName = provider.Name
		}
		
		if totalTotalTokens > 0 {
			RecordTokenUsage(displayName, providerName, totalPromptTokens, totalCompletionTokens, totalTotalTokens)
			logger.Info(fmt.Sprintf("WebSocket | %s | Token: %d", modelName, totalTotalTokens))
		} else {
			// 使用估算值
			estimatedInputTokens := estimatedInputChars / 3
			if estimatedInputTokens < 1 && estimatedInputChars > 0 {
				estimatedInputTokens = 1
			}
			outputContent := estimatedOutputContent.String()
			estimatedOutputTokens := len(outputContent) / 3
			if estimatedOutputTokens < 1 && len(outputContent) > 0 {
				estimatedOutputTokens = 1
			}
			if estimatedInputTokens > 0 || estimatedOutputTokens > 0 {
				estimatedTotal := estimatedInputTokens + estimatedOutputTokens
				RecordTokenUsage(displayName, providerName, estimatedInputTokens, estimatedOutputTokens, estimatedTotal)
				logger.Info(fmt.Sprintf("WebSocket | %s | Token估算: ~%d", modelName, estimatedTotal))
			}
		}
		
		logger.Info(fmt.Sprintf("WebSocket | %s | %.2fs", modelName, duration))
		logger.RequestSuccess()
	}
}
