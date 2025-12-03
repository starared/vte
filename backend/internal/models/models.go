package models

import "time"

type User struct {
	ID             int       `json:"id"`
	Username       string    `json:"username"`
	HashedPassword string    `json:"-"`
	APIKey         string    `json:"api_key"`
	IsAdmin        bool      `json:"is_admin"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
}

type Provider struct {
	ID             int       `json:"id"`
	Name           string    `json:"name"`
	BaseURL        string    `json:"base_url"`
	APIKey         string    `json:"-"`
	ModelPrefix    string    `json:"model_prefix"`
	ProviderType   string    `json:"provider_type"`
	VertexProject  string    `json:"vertex_project,omitempty"`
	VertexLocation string    `json:"vertex_location,omitempty"`
	ExtraHeaders   string    `json:"-"`
	ProxyURL       string    `json:"-"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
}

// ProviderAPIKey 提供商的多密钥支持
type ProviderAPIKey struct {
	ID         int        `json:"id"`
	ProviderID int        `json:"provider_id"`
	APIKey     string     `json:"api_key,omitempty"`
	Name       string     `json:"name"`
	IsActive   bool       `json:"is_active"`
	UsageCount int        `json:"usage_count"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// APIKeyCreate 创建密钥请求
type APIKeyCreate struct {
	APIKey string `json:"api_key" binding:"required"`
	Name   string `json:"name"`
}

// APIKeyUpdate 更新密钥请求
type APIKeyUpdate struct {
	Name     *string `json:"name"`
	IsActive *bool   `json:"is_active"`
}

// TestConnectionRequest 测试连接请求
type TestConnectionRequest struct {
	ModelID  *int `json:"model_id"`
	APIKeyID *int `json:"api_key_id"`
}

type Model struct {
	ID           int    `json:"id"`
	ProviderID   int    `json:"provider_id"`
	ProviderName string `json:"provider_name,omitempty"`
	OriginalID   string `json:"original_id"`
	DisplayName  string `json:"display_name"`
	CustomName   bool   `json:"custom_name"`
	IsActive     bool   `json:"is_active"`
}

type Setting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Request/Response schemas
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

type ProviderCreate struct {
	Name           string `json:"name" binding:"required"`
	BaseURL        string `json:"base_url"`
	APIKey         string `json:"api_key" binding:"required"`
	ModelPrefix    string `json:"model_prefix"`
	ProviderType   string `json:"provider_type"`
	VertexProject  string `json:"vertex_project"`
	VertexLocation string `json:"vertex_location"`
	ExtraHeaders   string `json:"extra_headers"`
	ProxyURL       string `json:"proxy_url"`
}

type ProviderUpdate struct {
	Name           *string `json:"name"`
	BaseURL        *string `json:"base_url"`
	APIKey         *string `json:"api_key"`
	ModelPrefix    *string `json:"model_prefix"`
	ProviderType   *string `json:"provider_type"`
	VertexProject  *string `json:"vertex_project"`
	VertexLocation *string `json:"vertex_location"`
	ExtraHeaders   *string `json:"extra_headers"`
	ProxyURL       *string `json:"proxy_url"`
	IsActive       *bool   `json:"is_active"`
}

type ModelUpdate struct {
	DisplayName *string `json:"display_name"`
	IsActive    *bool   `json:"is_active"`
}

type BatchToggleRequest struct {
	ModelIDs []int `json:"model_ids" binding:"required"`
	IsActive bool  `json:"is_active"`
}

type AddModelRequest struct {
	ModelID string `json:"model_id" binding:"required"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

type ChangeUsernameRequest struct {
	NewUsername string `json:"new_username" binding:"required"`
}

type StreamModeRequest struct {
	Mode string `json:"mode" binding:"required"`
}

type RetrySettingsRequest struct {
	MaxRetries int `json:"max_retries"`
}

type ThemeSettingsRequest struct {
	Theme string `json:"theme" binding:"required"`
}

type SystemPromptRequest struct {
	Prompt  string `json:"prompt"`
	Enabled bool   `json:"enabled"`
}

// CustomErrorRule 自定义错误响应规则
type CustomErrorRule struct {
	Keyword  string `json:"keyword"`  // 错误关键词匹配
	Response string `json:"response"` // 自定义响应内容
}

type CustomErrorResponseRequest struct {
	Enabled bool              `json:"enabled"`
	Rules   []CustomErrorRule `json:"rules"`
}

type RateLimitRequest struct {
	Enabled     bool `json:"enabled"`
	MaxRequests int  `json:"max_requests"`
	Window      int  `json:"window"`
}

type ConcurrencyRequest struct {
	Enabled bool `json:"enabled"`
	Limit   int  `json:"limit"`
}

type TokenUsage struct {
	ID               int       `json:"id"`
	ModelName        string    `json:"model_name"`
	ProviderName     string    `json:"provider_name"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	CreatedAt        time.Time `json:"created_at"`
}

type TokenStats struct {
	TotalTokens      int                `json:"total_tokens"`
	PromptTokens     int                `json:"prompt_tokens"`
	CompletionTokens int                `json:"completion_tokens"`
	HourlyStats      []HourlyTokenStats `json:"hourly_stats"`
	ModelStats       []ModelTokenStats  `json:"model_stats"`
}

type HourlyTokenStats struct {
	Hour         int `json:"hour"`
	TotalTokens  int `json:"total_tokens"`
	RequestCount int `json:"request_count"`
}

type ModelTokenStats struct {
	ModelName        string `json:"model_name"`
	ProviderName     string `json:"provider_name"`
	TotalTokens      int    `json:"total_tokens"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	RequestCount     int    `json:"request_count"`
}
