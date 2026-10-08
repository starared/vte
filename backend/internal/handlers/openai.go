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

// 并发控制：当前并发数（原子操作）
var currentConcurrency int64

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

// checkRateLimit 检查全局速率限制（滑动窗口）
func checkRateLimit(st *gatewaySettings) bool {
	if !st.rateLimitEnabled {
		return true
	}

	rateLimitMu.Lock()
	defer rateLimitMu.Unlock()

	now := time.Now()
	windowStart := now.Add(-time.Duration(st.rateLimitWindow) * time.Second)

	// 清理过期记录
	validTimes := make([]time.Time, 0, len(requestTimes))
	for _, t := range requestTimes {
		if t.After(windowStart) {
			validTimes = append(validTimes, t)
		}
	}
	requestTimes = validTimes

	if len(requestTimes) >= st.rateLimitMax {
		return false
	}
	requestTimes = append(requestTimes, now)
	return true
}

// checkCustomRateLimit 检查自定义速率限制
// 返回: (是否通过, 触发的规则名称)
func checkCustomRateLimit(rules []CustomRateLimitRule, providerID int, modelName string) (bool, string) {
	if len(rules) == 0 {
		return true, ""
	}

	customRateLimitMu.Lock()
	defer customRateLimitMu.Unlock()

	now := time.Now()
	pending := make(map[string][]time.Time)
	active := make(map[string]bool, len(rules))
	for _, rule := range rules {
		// 计数按规则 ID 和窗口长度区分；不含规则在列表中的位置，调整顺序不会清零计数
		key := fmt.Sprintf("rule:%d:%d", rule.ID, rule.Window)
		active[key] = true
		if !rule.Enabled {
			continue
		}
		if rule.ProviderID > 0 && rule.ProviderID != providerID {
			continue
		}
		if rule.ModelName != "" && rule.ModelName != modelName {
			continue
		}
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
	// 清理已删除或已修改窗口的规则留下的计数
	for key := range customRequestTimes {
		if !active[key] {
			delete(customRequestTimes, key)
		}
	}

	return true, ""
}

// acquireConcurrency 获取并发槽（读取当前设置）
func acquireConcurrency() bool {
	return acquireConcurrencyLimit(loadGatewaySettings().concurrencyLimit)
}

// acquireConcurrencyLimit 获取并发槽，limit 为 0 表示不限制
func acquireConcurrencyLimit(limit int) bool {
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

// CustomErrorRule 自定义错误响应规则
type CustomErrorRule struct {
	Keyword  string `json:"keyword"`
	Response string `json:"response"`
}

// respondCustomError 如果错误信息命中「自定义错误响应」规则，返回伪造的正常回复并返回 true
func respondCustomError(c *gin.Context, st *gatewaySettings, errMsg, reason, modelName string, stream bool) bool {
	matched, content := st.matchCustomError(errMsg)
	if !matched {
		return false
	}
	logger.Error(fmt.Sprintf("%s | %s | 自定义响应(原错误: %s)", c.ClientIP(), modelName, reason))
	writeFakeResponse(c, content, modelName, stream)
	return true
}

func writeFakeResponse(c *gin.Context, content, modelName string, stream bool) {
	if stream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.String(200, buildFakeStreamResponse(content, modelName))
		return
	}
	c.JSON(200, buildFakeResponse(content, modelName))
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

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源（使用 API Key 认证，不依赖 Cookie）
	},
	// 浏览器无法给 WebSocket 设置请求头，可通过子协议传递密钥：
	// new WebSocket(url, ["bearer", "<API Key>"])，服务端回应 "bearer"
	Subprotocols:    []string{"bearer"},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// wsAPIKey 从请求中取出 WebSocket 的 API Key。优先级：
// Authorization 请求头 > Sec-WebSocket-Protocol: bearer, <key> > ?api_key=（旧方式，密钥会出现在访问日志中，不推荐）
func wsAPIKey(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		return strings.TrimPrefix(h, "Bearer ")
	}
	protocols := websocket.Subprotocols(r)
	for i := 0; i+1 < len(protocols); i++ {
		if strings.EqualFold(protocols[i], "bearer") {
			return protocols[i+1]
		}
	}
	return r.URL.Query().Get("api_key")
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
		if err := rows.Scan(&displayName, &originalID, &providerName); err != nil {
			continue
		}

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

	// 本次请求用到的设置只读取一次
	st := loadGatewaySettings()

	// 检查速率限制
	if !checkRateLimit(st) {
		if respondCustomError(c, st, "请求过于频繁，请稍后重试 rate_limit_exceeded", "全局速率限制", modelName, stream) {
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
	if !acquireConcurrencyLimit(st.concurrencyLimit) {
		if respondCustomError(c, st, "服务器繁忙，请稍后重试 concurrency_limit_exceeded", "并发限制", modelName, stream) {
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

	// 流式模式设置
	if st.streamMode == "force_stream" {
		payload["stream"] = true
	} else if st.streamMode == "force_non_stream" {
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
	st.injectSystemPrompt(payload)

	// 查找模型
	model, provider, err := findModel(modelName)
	if err != nil {
		errMsg := fmt.Sprintf("模型不可用: %s (%v)", modelName, err)
		if respondCustomError(c, st, errMsg, "模型不存在", modelName, stream) {
			return
		}
		apiError(c, 404, "request_error", errMsg)
		return
	}

	if !provider.IsActive {
		errMsg := fmt.Sprintf("提供商已禁用: %s", provider.Name)
		if respondCustomError(c, st, errMsg, "提供商禁用", modelName, stream) {
			return
		}
		apiError(c, 503, "request_error", errMsg)
		return
	}

	// 统计和限流统一使用模型的显示名称（无论客户端用显示名还是原始 ID 请求）
	displayName := modelName
	if model.DisplayName != "" {
		displayName = model.DisplayName
	}

	// 检查自定义速率限制
	if passed, ruleName := checkCustomRateLimit(st.customRateRules, provider.ID, displayName); !passed {
		errMsg := fmt.Sprintf("触发自定义速率限制规则 [%s]，请稍后重试 custom_rate_limit_exceeded", ruleName)
		if respondCustomError(c, st, errMsg, "自定义速率限制 "+ruleName, modelName, stream) {
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
		if !strings.HasPrefix(originalID, "google/") {
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
		BaseURL:        provider.BaseURL,
		APIKey:         provider.APIKey,
		ProviderType:   provider.ProviderType,
		VertexProject:  provider.VertexProject,
		VertexLocation: provider.VertexLocation,
		ProxyURL:       provider.ProxyURL,
	}
	cfg.BeforeAttempt = func() { recordKeyAttempt(keyID) }
	// 上游返回 401/403/429 时换下一个还没试过的密钥
	tried := map[int]bool{keyID: true}
	cfg.RotateKey = func() bool {
		nextKey, nextID, err := GetNextAPIKeyExcluding(provider.ID, tried)
		if err != nil {
			return false
		}
		tried[nextID] = true
		logger.Warn(fmt.Sprintf("%s | %s | 密钥 #%d 被上游拒绝，切换到密钥 #%d", c.ClientIP(), modelName, keyID, nextID))
		keyID = nextID
		cfg.APIKey = nextKey
		return true
	}

	if provider.ExtraHeaders != "" {
		if err := json.Unmarshal([]byte(provider.ExtraHeaders), &cfg.ExtraHeaders); err != nil {
			logger.Warn(fmt.Sprintf("提供商 %s 的额外请求头格式错误，已忽略", provider.Name))
		}
	}

	startTime := time.Now()

	req := &gatewayRequest{
		settings:    st,
		cfg:         cfg,
		payload:     payload,
		modelName:   modelName,
		displayName: displayName,
		model:       model,
		provider:    provider,
		startTime:   startTime,
	}
	if stream {
		handleStreamResponse(c, req)
	} else {
		handleNonStreamResponse(c, req)
	}
}

// gatewayRequest 已解析好的一次网关请求
type gatewayRequest struct {
	settings    *gatewaySettings
	cfg         *proxy.ProviderConfig
	payload     map[string]interface{}
	modelName   string // 客户端请求时使用的模型名
	displayName string // 模型显示名称，用于统计
	model       *modelInfo
	provider    *providerInfo
	startTime   time.Time
}

func handleNonStreamResponse(c *gin.Context, r *gatewayRequest) {
	result, err := r.cfg.CompletionForClient(c.Request.Context(), r.payload, r.settings.maxRetries)
	duration := time.Since(r.startTime).Seconds()

	if err != nil {
		errMsg := err.Error()

		// 检查是否有自定义错误响应
		if matched, content := r.settings.matchCustomError(errMsg); matched {
			logger.Info(fmt.Sprintf("%s | %s | %.2fs | 自定义响应(原错误: %s)", c.ClientIP(), r.modelName, duration, errMsg))
			writeFakeResponse(c, content, r.modelName, false)
			return
		}

		logger.Error(fmt.Sprintf("%s | %s | %.2fs | %v", c.ClientIP(), r.modelName, duration, err))
		writeUpstreamError(c, err)
		return
	}

	result["model"] = r.modelName
	pt, ct, tt := 0, 0, 0
	if usage, ok := result["usage"].(map[string]interface{}); ok {
		pt = intNumber(usage["prompt_tokens"])
		ct = intNumber(usage["completion_tokens"])
		tt = intNumber(usage["total_tokens"])
	}
	// token 都为 0 通常是被上游拦截的空响应，不计入统计
	if tt > 0 || pt > 0 || ct > 0 {
		recordUsage(r, pt, ct, tt)
		logger.Info(fmt.Sprintf("%s | %s | %.2fs | Token: %d (in=%d, out=%d)", c.ClientIP(), r.modelName, duration, tt, pt, ct))
	} else {
		logger.Info(fmt.Sprintf("%s | %s | %.2fs", c.ClientIP(), r.modelName, duration))
	}
	c.JSON(200, result)
}

func recordUsage(r *gatewayRequest, pt, ct, tt int) {
	if err := RecordTokenUsage(r.displayName, r.provider.Name, pt, ct, tt); err != nil {
		logger.Error("记录 token 用量失败: " + err.Error())
	}
}

func handleStreamResponse(c *gin.Context, r *gatewayRequest) {
	modelName := r.modelName
	resp, err := r.cfg.StreamForClient(c.Request.Context(), r.payload, r.settings.maxRetries)
	if err != nil {
		if matched, content := r.settings.matchCustomError(err.Error()); matched {
			writeFakeResponse(c, content, modelName, true)
			return
		}
		logger.Error(fmt.Sprintf("%s | %s | %.2fs | %v", c.ClientIP(), modelName, time.Since(r.startTime).Seconds(), err))
		writeUpstreamError(c, err)
		return
	}
	defer resp.Body.Close()
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	var output strings.Builder
	var usage map[string]interface{}
	done := false     // 收到 [DONE]
	finished := false // 收到带 finish_reason 的 choice
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
		if e, ok := chunk["error"]; ok {
			return fmt.Errorf("上游在流式响应中返回错误: %s", proxy.ErrorMessage(e))
		}
		chunk["model"] = modelName
		if u, ok := chunk["usage"].(map[string]interface{}); ok {
			usage = u
		}
		if choices, ok := chunk["choices"].([]interface{}); ok {
			for _, item := range choices {
				if ch, ok := item.(map[string]interface{}); ok {
					if ch["finish_reason"] != nil {
						finished = true
					}
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
	// 有些上游在 [DONE] 之后不关闭连接，回调里返回 io.EOF 终止读取；
	// 也有些上游不发送 [DONE] 就正常结束：只要已经收到 finish_reason，同样视为完成，
	// 并补发 [DONE] 让客户端正常收尾。
	if err == nil && !done && finished {
		if sendErr := send("[DONE]"); sendErr == nil {
			done = true
		}
	}
	if (err != nil && err != io.EOF) || !done {
		if errors.Is(err, context.Canceled) || c.Request.Context().Err() != nil {
			logger.Warn(fmt.Sprintf("%s | %s | 客户端取消", c.ClientIP(), modelName))
			return
		}
		if !c.Writer.Written() {
			apiError(c, 502, "upstream_stream_error", "上游流式响应中断")
		} else {
			send(`{"error":{"message":"上游流式响应中断","type":"upstream_stream_error"}}`)
		}
		logger.Warn(fmt.Sprintf("%s | %s | 流未完成 | %v", c.ClientIP(), modelName, err))
		return
	}
	pt, ct, tt := 0, 0, 0
	estimated := usage == nil
	if usage != nil {
		pt = intNumber(usage["prompt_tokens"])
		ct = intNumber(usage["completion_tokens"])
		tt = intNumber(usage["total_tokens"])
	} else {
		messages, _ := r.payload["messages"].([]interface{})
		pt = tokenizer.CountMessagesTokens(messages, r.model.OriginalID)
		ct = tokenizer.CountTokens(output.String(), r.model.OriginalID)
		tt = pt + ct
	}
	if tt > 0 {
		recordUsage(r, pt, ct, tt)
	}
	logger.Info(fmt.Sprintf("%s | %s | %.2fs | Token: %d (estimated=%v)", c.ClientIP(), modelName, time.Since(r.startTime).Seconds(), tt, estimated))
}

func intNumber(v interface{}) int { n, _ := v.(float64); return int(n) }

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

// findModel 按显示名称查找启用的模型，找不到再按原始 ID 查找。
// 每个字段只查询一次（最多取两行，用于判断是否重名）。
func findModel(modelName string) (*modelInfo, *providerInfo, error) {
	db := database.DB()
	for _, field := range []string{"display_name", "original_id"} {
		rows, err := db.Query(`SELECT m.id, m.original_id, m.display_name,
 p.id,p.name,p.base_url,p.api_key,p.provider_type,COALESCE(p.vertex_project,''),
 COALESCE(p.vertex_location,'global'),COALESCE(p.extra_headers,''),COALESCE(p.proxy_url,''),p.is_active
 FROM models m JOIN providers p ON m.provider_id=p.id
 WHERE m.`+field+` = ? AND m.is_active = 1 AND p.is_active = 1 LIMIT 2`, modelName)
		if err != nil {
			return nil, nil, err
		}
		var found []modelWithProvider
		for rows.Next() {
			model, provider, err := scanModelProvider(rows)
			if err != nil {
				rows.Close()
				return nil, nil, err
			}
			found = append(found, modelWithProvider{model, provider})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, nil, err
		}
		switch len(found) {
		case 0:
			continue
		case 1:
			return found[0].Model, found[0].Provider, nil
		default:
			return nil, nil, fmt.Errorf("ambiguous model: use a unique provider prefix or alias")
		}
	}
	return nil, nil, fmt.Errorf("model not found")
}

type modelWithProvider struct {
	Model    *modelInfo
	Provider *providerInfo
}

func scanModelProvider(rows *sql.Rows) (*modelInfo, *providerInfo, error) {
	var model modelInfo
	var provider providerInfo
	var displayName *string
	var isActive int

	err := rows.Scan(
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
	if key := wsAPIKey(c.Request); key != "" {
		c.Request.Header.Set("Authorization", "Bearer "+key)
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
