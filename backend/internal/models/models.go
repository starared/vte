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

type Model struct {
	ID           int    `json:"id"`
	ProviderID   int    `json:"provider_id"`
	ProviderName string `json:"provider_name,omitempty"`
	OriginalID   string `json:"original_id"`
	DisplayName  string `json:"display_name"`
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
