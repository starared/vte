package handlers

import (
	"encoding/json"
	"strconv"
	"strings"

	"vte/internal/database"
	"vte/internal/logger"
)

// gatewaySettingKeys 网关处理请求时需要读取的全部设置项
var gatewaySettingKeys = []string{
	"rate_limit_enabled", "rate_limit_max_requests", "rate_limit_window",
	"concurrency_enabled", "concurrency_limit",
	"custom_rate_limit_rules",
	"max_retries",
	"stream_mode",
	"system_prompt_enabled", "system_prompt",
	"custom_error_enabled", "custom_error_rules",
}

// gatewaySettings 单次请求使用的设置快照。
// 每个请求只查询一次数据库（原来要分别查询十次左右），并且不做跨请求缓存，
// 所以在管理界面修改设置后立即生效。
type gatewaySettings struct {
	rateLimitEnabled bool
	rateLimitMax     int
	rateLimitWindow  int

	concurrencyLimit int // 0 表示不限制

	customRateRules []CustomRateLimitRule

	maxRetries int
	streamMode string

	systemPrompt string // 为空表示未启用

	customErrorEnabled bool
	customErrorRules   []CustomErrorRule
}

func loadGatewaySettings() *gatewaySettings {
	values := make(map[string]string, len(gatewaySettingKeys))
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(gatewaySettingKeys)), ",")
	args := make([]interface{}, len(gatewaySettingKeys))
	for i, k := range gatewaySettingKeys {
		args[i] = k
	}
	rows, err := database.DB().Query("SELECT key, value FROM settings WHERE key IN ("+placeholders+")", args...)
	if err != nil {
		logger.Error("读取网关设置失败: " + err.Error())
	} else {
		for rows.Next() {
			var k, v string
			if rows.Scan(&k, &v) == nil {
				values[k] = v
			}
		}
		rows.Close()
	}
	return parseGatewaySettings(values)
}

func parseGatewaySettings(values map[string]string) *gatewaySettings {
	s := &gatewaySettings{maxRetries: 3, streamMode: values["stream_mode"]}

	if values["rate_limit_enabled"] == "true" {
		s.rateLimitEnabled = true
		s.rateLimitMax, _ = strconv.Atoi(values["rate_limit_max_requests"])
		s.rateLimitWindow, _ = strconv.Atoi(values["rate_limit_window"])
		if s.rateLimitMax <= 0 {
			s.rateLimitMax = 60
		}
		if s.rateLimitWindow <= 0 {
			s.rateLimitWindow = 60
		}
	}

	if values["concurrency_enabled"] == "true" {
		if limit, err := strconv.Atoi(values["concurrency_limit"]); err == nil && limit > 0 {
			s.concurrencyLimit = limit
		}
	}

	if raw := values["custom_rate_limit_rules"]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &s.customRateRules); err != nil {
			logger.Error("自定义速率限制规则格式错误，已忽略: " + err.Error())
			s.customRateRules = nil
		}
	}

	if raw, ok := values["max_retries"]; ok {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 && n <= 10 {
			s.maxRetries = n
		}
	}

	if values["system_prompt_enabled"] == "true" {
		s.systemPrompt = values["system_prompt"]
	}

	if values["custom_error_enabled"] == "true" {
		if raw := values["custom_error_rules"]; raw != "" {
			if err := json.Unmarshal([]byte(raw), &s.customErrorRules); err != nil {
				logger.Error("自定义错误响应规则格式错误，已忽略: " + err.Error())
			} else {
				s.customErrorEnabled = true
			}
		}
	}
	return s
}

// matchCustomError 错误信息命中自定义规则时返回替代内容
func (s *gatewaySettings) matchCustomError(errMsg string) (bool, string) {
	if !s.customErrorEnabled || len(s.customErrorRules) == 0 {
		return false, ""
	}
	errMsgLower := strings.ToLower(errMsg)
	for _, rule := range s.customErrorRules {
		if rule.Keyword != "" && strings.Contains(errMsgLower, strings.ToLower(rule.Keyword)) {
			return true, rule.Response
		}
	}
	return false, ""
}

// injectSystemPrompt 在 messages 最前面注入系统前置提示词
func (s *gatewaySettings) injectSystemPrompt(payload map[string]interface{}) {
	if s.systemPrompt == "" {
		return
	}
	messages, ok := payload["messages"].([]interface{})
	if !ok {
		return
	}
	systemMsg := map[string]interface{}{
		"role":    "system",
		"content": s.systemPrompt,
	}
	newMessages := make([]interface{}, 0, len(messages)+1)
	newMessages = append(newMessages, systemMsg)
	newMessages = append(newMessages, messages...)
	payload["messages"] = newMessages
}
