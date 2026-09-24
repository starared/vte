package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"vte/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"vte/internal/database"
	"vte/internal/logger"
	"vte/internal/models"
	"vte/internal/proxy"
	"vte/internal/tokenizer"
)

// 并发控制
var (
	currentConcurrency int64        // 当前并发数
	concurrencyMu      sync.RWMutex // 并发设置锁
)

// 自定义并发控制 - 基于提供商/模型
var (
	customConcurrencyMu      sync.Mutex
	customConcurrencyCurrent = make(map[string]int64) // key -> 当前并发数
)

// CustomConcurrencyRule 自定义并发限制规则
type CustomConcurrencyRule struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`          // 规则名称
	ProviderID   int    `json:"provider_id"`   // 提供商ID，0表示所有
	ProviderName string `json:"provider_name"` // 提供商名称（仅显示用）
	ModelName    string `json:"model_name"`    // 模型名称，空表示所有
	Limit        int    `json:"limit"`         // 最大并发数
	Enabled      bool   `json:"enabled"`       // 是否启用
}

// 速率限制 - 使用滑动窗口
var (
	rateLimitMu  sync.Mutex
	requestTimes []time.Time // 请求时间记录
)

// 自定义速率限制 - 基于提供商/模型
var (
	customRateLimitMu  sync.Mutex
	customRequestTimes = make(map[string][]time.Time) // key: "provider:xxx" 或 "model:xxx" 或 "provider:xxx:model:yyy"
)

// CustomRateLimitRule 自定义速率限制规则
type CustomRateLimitRule struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`          // 规则名称
	ProviderID   int    `json:"provider_id"`   // 提供商ID，0表示所有
	ProviderName string `json:"provider_name"` // 提供商名称（仅显示用）
	ModelName    string `json:"model_name"`    // 模型名称，空表示所有
	MaxRequests  int    `json:"max_requests"`  // 最大请求数
	Window       int    `json:"window"`        // 时间窗口（秒）
	Enabled      bool   `json:"enabled"`       // 是否启用
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
	pending := make(map[string][]time.Time)
	for i, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if rule.ProviderID > 0 && rule.ProviderID != providerID {
			continue
		}
		if rule.ModelName != "" && rule.ModelName != modelName {
			continue
		}
		key := fmt.Sprintf("rule:%d:%d:%d", rule.ID, i, rule.Window)
		cutoff := now.Add(-time.Duration(rule.Window) * time.Second)
		kept := make([]time.Time, 0, len(customRequestTimes[key]))
		for _, at := range customRequestTimes[key] {
			if at.After(cutoff) {
				kept = append(kept, at)
			}
		}
		if rule.MaxRequests <= 0 || rule.Window <= 0 || len(kept) >= rule.MaxRequests {
			return false, rule.Name
		}
		pending[key] = append(kept, now)
	}
	for key, times := range pending {
		customRequestTimes[key] = times
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

	for {
		current := atomic.LoadInt64(&currentConcurrency)
		if current >= int64(limit) {
			return false
		}
		if atomic.CompareAndSwapInt64(&currentConcurrency, current, current+1) {
			return true
		}
	}

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
	if err != nil || retries < 0 || retries > 10 {
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

	var allowed map[string]bool
	if tk, ok := c.Get("temp_api_key"); ok {
		if temp, ok2 := tk.(*models.TempAPIKey); ok2 {
			allowed = make(map[string]bool)
			for _, m := range temp.AllowedModels {
				allowed[m] = true
			}
		}
	}
	rows, err := db.Query(`
		SELECT m.display_name, m.original_id, p.name
		FROM models m
		JOIN providers p ON m.provider_id = p.id
		WHERE m.is_active = 1 AND p.is_active = 1
	`)
	if err != nil {
		apiError(c, 500, "request_error", "查询失败")
		return
	}
	defer rows.Close()

	data := make([]gin.H, 0)
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

		if allowed != nil {
			if !allowed[modelID] {
				continue
			}
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

	var tempKey *models.TempAPIKey
	var allowedModels map[string]bool
	if tk, ok := c.Get("temp_api_key"); ok {
		if t, ok2 := tk.(*models.TempAPIKey); ok2 {
			tempKey = t
			allowedModels = make(map[string]bool)
			for _, m := range tempKey.AllowedModels {
				allowedModels[m] = true
			}
		}
	}

	// 先解析请求获取模型名和stream参数，用于后续的自定义错误响应
	var payload map[string]interface{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			apiError(c, 413, "request_too_large", "请求体过大")
			return
		}
		apiError(c, 400, "request_error", "无效的 JSON")
		return
	}

	if err := validateChatPayload(payload); err != nil {
		apiError(c, 400, "invalid_request_error", err.Error())
		return
	}

	modelName, ok := payload["model"].(string)
	if !ok || modelName == "" {
		apiError(c, 400, "request_error", "缺少 model 参数")
		return
	}

	stream := false
	if s, ok := payload["stream"].(bool); ok {
		stream = s
	}

	// 临时密钥校验（过期、可用模型）
	if tempKey != nil {
		if tempKey.ExpiresAt != nil && time.Now().After(*tempKey.ExpiresAt) {
			apiError(c, 401, "request_error", "API Key 已过期")
			return
		}
		if len(allowedModels) > 0 && !allowedModels[modelName] {
			apiError(c, 403, "request_error", "当前密钥无权使用该模型")
			return
		}
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
		payload["stream"] = true
	} else if streamMode == "force_non_stream" {
		payload["stream"] = false
	}

	// 如果是流式请求，添加 stream_options 以获取 usage 信息
	upstreamStream, _ := payload["stream"].(bool)
	if !upstreamStream {
		delete(payload, "stream_options")
	}
	if upstreamStream && os.Getenv("INCLUDE_STREAM_USAGE") != "false" {
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
		errMsg := fmt.Sprintf("模型不可用: %s (%v)", modelName, err)
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
		apiError(c, 404, "request_error", errMsg)
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
		apiError(c, 503, "request_error", errMsg)
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

	if tempKey != nil {
		release, err := acquireTempLimits(tempKey)
		if err != nil {
			apiError(c, 429, "rate_limit_exceeded", err.Error())
			return
		}
		defer release()
	}
	// 替换模型名为原始 ID
	originalID := model.OriginalID
	if provider.ProviderType == "vertex_express" && len(originalID) > 0 {
		if len(originalID) < 7 || originalID[:7] != "google/" {
			originalID = "google/" + originalID
		}
	}
	payload["model"] = originalID

	apiKey, keyID, err := GetNextAPIKey(provider.ID)
	if err != nil {
		apiError(c, 503, "no_available_key", "提供商没有启用的密钥")
		return
	}
	provider.APIKey = apiKey
	c.Set("resolved_model", model)
	c.Set("resolved_provider", provider)
	// 临时密钥用量消耗
	if tempKey != nil {
		switch err := database.ConsumeTempAPIUsage(tempKey.ID, modelName); err {
		case nil:
			// ok
		case database.ErrTempAPILimitExceeded:
			apiError(c, 429, "request_error", "API Key 请求次数已用尽")
			return
		case database.ErrTempAPIModelExceeded:
			apiError(c, 429, "request_error", "该模型的可用次数已用尽")
			return
		case database.ErrTempAPIExpired:
			apiError(c, 401, "request_error", "API Key 已过期")
			return
		case database.ErrTempAPIDisabled:
			apiError(c, 401, "request_error", "API Key 已禁用")
			return
		default:
			apiError(c, 500, "request_error", "更新用量失败")
			return
		}
	}

	// 构建客户端配置
	cfg := &proxy.ProviderConfig{
		BeforeAttempt:  func() { recordKeyAttempt(keyID) },
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
	result, err := cfg.CompletionForClient(c.Request.Context(), payload, maxRetries)
	duration := time.Since(startTime).Seconds()

	if err != nil {
		errMsg := err.Error()

		// 检查是否有自定义错误响应
		db := database.DB()
		if matched, customResponse := checkCustomErrorResponse(db, errMsg); matched {
			logger.Info(fmt.Sprintf("%s | %s | %.2fs | 自定义响应(原错误: %s)", c.ClientIP(), modelName, duration, errMsg))
			logger.RequestError()
			c.JSON(200, buildFakeResponse(customResponse, modelName))
			return
		}

		logger.Error(fmt.Sprintf("%s | %s | %.2fs | %v", c.ClientIP(), modelName, duration, err))
		if errors.Is(err, context.Canceled) {
			logger.RequestCancelled()
		} else {
			logger.RequestError()
		}
		writeUpstreamError(c, err)
		return
	}

	result["model"] = modelName
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

		// 如果 token 都为 0，说明是被上游拦截的空响应，跳过日志记录
		if totalTokens == 0 && promptTokens == 0 && completionTokens == 0 {
			logger.RequestSuccess()
			c.JSON(200, result)
			return
		}

		// 获取provider名称
		model := c.MustGet("resolved_model").(*modelInfo)
		provider := c.MustGet("resolved_provider").(*providerInfo)
		providerName := "unknown"
		if provider != nil {
			providerName = provider.Name
		}
		displayName := modelName
		if model != nil && model.DisplayName != "" {
			displayName = model.DisplayName
		}

		RecordTokenUsage(displayName, providerName, promptTokens, completionTokens, totalTokens)
		logger.Info(fmt.Sprintf("%s | %s | %.2fs | Token: %d (in=%d, out=%d)", c.ClientIP(), modelName, duration, totalTokens, promptTokens, completionTokens))
		logger.RequestSuccess()
		c.JSON(200, result)
		return
	}

	logger.Info(fmt.Sprintf("%s | %s | %.2fs", c.ClientIP(), modelName, duration))
	logger.RequestSuccess()
	c.JSON(200, result)
}

func handleStreamResponse(c *gin.Context, cfg *proxy.ProviderConfig, payload map[string]interface{}, modelName string, startTime time.Time) {
	resp, err := cfg.StreamForClient(c.Request.Context(), payload, getMaxRetries())
	if err != nil {
		logger.RequestError()
		if matched, response := checkCustomErrorResponse(database.DB(), err.Error()); matched {
			c.Header("Content-Type", "text/event-stream")
			c.String(200, buildFakeStreamResponse(response, modelName))
			return
		}
		writeUpstreamError(c, err)
		return
	}
	defer resp.Body.Close()
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	var output strings.Builder
	var usage map[string]interface{}
	done := false
	send := func(data string) error {
		_, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		c.Writer.Flush()
		return err
	}
	err = proxy.ReadEvents(resp.Body, func(data string) error {
		if err := c.Request.Context().Err(); err != nil {
			return err
		}
		if data == "[DONE]" {
			if err := send(data); err != nil {
				return err
			}
			done = true
			return io.EOF
		}
		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("上游流式 JSON 格式错误")
		}
		if chunk == nil {
			return fmt.Errorf("上游流式响应必须为 JSON 对象")
		}
		if _, ok := chunk["error"]; ok {
			return fmt.Errorf("上游在流式响应中返回错误")
		}
		chunk["model"] = modelName
		if u, ok := chunk["usage"].(map[string]interface{}); ok {
			usage = u
		}
		if choices, ok := chunk["choices"].([]interface{}); ok {
			for _, item := range choices {
				if ch, ok := item.(map[string]interface{}); ok {
					if d, ok := ch["delta"].(map[string]interface{}); ok {
						if text, ok := d["content"].(string); ok {
							output.WriteString(text)
						}
					}
				}
			}
		}
		encoded, err := json.Marshal(chunk)
		if err != nil {
			return err
		}
		return send(string(encoded))
	})
	// Some upstreams keep the connection open after DONE; the callback terminates below.
	if (err != nil && err != io.EOF) || !done {
		if errors.Is(err, context.Canceled) || c.Request.Context().Err() != nil {
			logger.RequestCancelled()
		} else {
			logger.RequestInterrupted()
			if !c.Writer.Written() {
				apiError(c, 502, "upstream_stream_error", "上游流式响应中断")
			} else {
				send(`{"error":{"message":"上游流式响应中断","type":"upstream_stream_error"}}`)
			}
		}
		logger.Warn(fmt.Sprintf("%s | %s | 流未完成", c.ClientIP(), modelName))
		return
	}
	model := c.MustGet("resolved_model").(*modelInfo)
	provider := c.MustGet("resolved_provider").(*providerInfo)
	pt, ct, tt := 0, 0, 0
	estimated := usage == nil
	if usage != nil {
		pt = intNumber(usage["prompt_tokens"])
		ct = intNumber(usage["completion_tokens"])
		tt = intNumber(usage["total_tokens"])
	} else {
		messages, _ := payload["messages"].([]interface{})
		pt = tokenizer.CountMessagesTokens(messages, model.OriginalID)
		ct = tokenizer.CountTokens(output.String(), model.OriginalID)
		tt = pt + ct
	}
	if tt > 0 {
		RecordTokenUsage(modelName, provider.Name, pt, ct, tt)
	}
	logger.Info(fmt.Sprintf("%s | %s | %.2fs | Token: %d (estimated=%v)", c.ClientIP(), modelName, time.Since(startTime).Seconds(), tt, estimated))
	logger.RequestSuccess()
}
func intNumber(v interface{}) int { n, _ := v.(float64); return int(n) }

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
	for _, field := range []string{"display_name", "original_id"} {
		var count int
		where := "m." + field + " = ? AND m.is_active = 1 AND p.is_active = 1"
		if err := db.QueryRow("SELECT COUNT(*) FROM models m JOIN providers p ON m.provider_id = p.id WHERE "+where, modelName).Scan(&count); err != nil {
			return nil, nil, err
		}
		if count > 1 {
			return nil, nil, fmt.Errorf("ambiguous model: use a unique provider prefix or alias")
		}
		if count == 0 {
			continue
		}
		return scanModelProvider(db.QueryRow(`SELECT m.id, m.original_id, m.display_name,
 p.id,p.name,p.base_url,p.api_key,p.provider_type,COALESCE(p.vertex_project,''),
 COALESCE(p.vertex_location,'global'),COALESCE(p.extra_headers,''),COALESCE(p.proxy_url,''),p.is_active
 FROM models m JOIN providers p ON m.provider_id=p.id WHERE `+where, modelName))
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
	if c.GetHeader("Authorization") == "" && c.Query("api_key") != "" {
		c.Request.Header.Set("Authorization", "Bearer "+c.Query("api_key"))
	}
	auth.APIKeyAuth()(c)
	if c.IsAborted() {
		return
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(32 << 20)
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	messages := make(chan []byte, 1)
	go func() {
		defer cancel()
		defer close(messages)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			select {
			case messages <- message:
			case <-ctx.Done():
				return
			}
		}
	}()
	r := gin.New()
	r.Use(gin.Recovery())
	r.POST("/chat", auth.APIKeyAuth(), OpenAIChatCompletions)
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-messages:
			if !ok {
				return
			}
			var payload map[string]interface{}
			if json.Unmarshal(message, &payload) != nil || payload == nil {
				conn.WriteJSON(gin.H{"error": gin.H{"message": "无效的 JSON"}})
				continue
			}
			payload["stream"] = true
			body, _ := json.Marshal(payload)
			req, _ := http.NewRequestWithContext(ctx, "POST", "/chat", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", c.GetHeader("Authorization"))
			req.RemoteAddr = c.Request.RemoteAddr
			writer := &wsResponseWriter{conn: conn, header: make(http.Header), ctx: ctx, cancel: cancel}
			r.ServeHTTP(writer, req)
		}
	}
}
