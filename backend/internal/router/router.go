package router

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"vte/internal/auth"
	"vte/internal/config"
	"vte/internal/handlers"
)

// defaultMaxBodyBytes 网关请求体默认上限（32MB，与 WebSocket 读取上限一致）
const defaultMaxBodyBytes = 32 << 20

// maxBodyBytes 从环境变量读取请求体上限（MB），默认 32MB
func maxBodyBytes() int64 {
	if v := os.Getenv("MAX_REQUEST_BODY_MB"); v != "" {
		if mb, err := strconv.Atoi(v); err == nil && mb > 0 {
			return int64(mb) << 20
		}
	}
	return defaultMaxBodyBytes
}

// BodyLimitMiddleware 限制请求体大小，避免超大请求体耗尽内存
func BodyLimitMiddleware(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

// CORS 中间件
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}

		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Vary", "Origin")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Header("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func Setup(cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	trusted := []string{"127.0.0.1", "::1"}
	if value := os.Getenv("TRUSTED_PROXIES"); value != "" {
		trusted = strings.Split(value, ",")
	}
	if err := r.SetTrustedProxies(trusted); err != nil {
		panic(err)
	}
	r.Use(gin.Recovery())
	r.Use(CORSMiddleware())

	// 设置 JWT 密钥
	auth.SetSecretKey(cfg.SecretKey)

	// 健康检查端点（无需认证）
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
		})
	})

	// API 路由
	api := r.Group("/api")
	{
		// 认证
		authGroup := api.Group("/auth")
		{
			authGroup.POST("/login", handlers.Login)
			authGroup.GET("/me", auth.JWTAuth(), handlers.GetMe)
			authGroup.POST("/change-password", auth.JWTAuth(), handlers.ChangePassword)
			authGroup.POST("/change-username", auth.JWTAuth(), handlers.ChangeUsername)
			authGroup.POST("/regenerate-api-key", auth.JWTAuth(), handlers.RegenerateAPIKey)
		}

		// 提供商管理
		providers := api.Group("/providers", auth.JWTAuth(), auth.AdminRequired())
		{
			providers.GET("", handlers.ListProviders)
			providers.POST("", handlers.CreateProvider)
			providers.PUT("/:id", handlers.UpdateProvider)
			providers.DELETE("/:id", handlers.DeleteProvider)
			providers.POST("/:id/fetch-models", handlers.FetchModels)
			providers.POST("/:id/add-model", handlers.AddModel)
			providers.GET("/:id/models", handlers.ListProviderModels)
			// API Keys 轮询管理
			providers.GET("/:id/api-keys", handlers.ListAPIKeys)
			providers.POST("/:id/api-keys", handlers.AddAPIKey)
			providers.PUT("/:id/api-keys/:keyId", handlers.UpdateAPIKey)
			providers.DELETE("/:id/api-keys/:keyId", handlers.DeleteAPIKey)
			// 测试连接
			providers.POST("/:id/test", handlers.TestConnection)
			providers.GET("/:id/test-options", handlers.GetTestOptions)
		}

		// 模型管理
		models := api.Group("/models", auth.JWTAuth(), auth.AdminRequired())
		{
			models.GET("", handlers.ListAllModels)
			models.PUT("/:id", handlers.UpdateModel)
			models.DELETE("/:id", handlers.DeleteModel)
			models.POST("/:id/reset-name", handlers.ResetModelDisplayName)
			models.POST("/batch-toggle", handlers.BatchToggleModels)
		}

		// 日志
		logs := api.Group("/logs", auth.JWTAuth(), auth.AdminRequired())
		{
			logs.GET("", handlers.GetLogs)
			logs.DELETE("", handlers.ClearLogs)
			logs.GET("/stats", handlers.GetStats)
			logs.DELETE("/stats", handlers.ResetStats)
		}

		// Token统计
		tokens := api.Group("/tokens", auth.JWTAuth(), auth.AdminRequired())
		{
			tokens.GET("/stats", handlers.GetTodayTokenStats)
			tokens.DELETE("/stats", handlers.ResetTodayTokenStats)
		}

		// 设置
		settings := api.Group("/settings", auth.JWTAuth(), auth.AdminRequired())
		{
			settings.GET("/stream-mode", handlers.GetStreamMode)
			settings.PUT("/stream-mode", handlers.SetStreamMode)
			settings.GET("/retry", handlers.GetRetrySettings)
			settings.PUT("/retry", handlers.SetRetrySettings)
			settings.GET("/theme", handlers.GetThemeSettings)
			settings.PUT("/theme", handlers.SetThemeSettings)
			settings.GET("/system-prompt", handlers.GetSystemPrompt)
			settings.PUT("/system-prompt", handlers.SetSystemPrompt)
			settings.GET("/custom-error", handlers.GetCustomErrorResponse)
			settings.PUT("/custom-error", handlers.SetCustomErrorResponse)
			settings.GET("/rate-limit", handlers.GetRateLimitSettings)
			settings.PUT("/rate-limit", handlers.SetRateLimitSettings)
			settings.GET("/concurrency", handlers.GetConcurrencySettings)
			settings.PUT("/concurrency", handlers.SetConcurrencySettings)
			settings.GET("/custom-rate-limit", handlers.GetCustomRateLimitRules)
			settings.PUT("/custom-rate-limit", handlers.SetCustomRateLimitRules)
		}

		// 临时 API 密钥
		tempKeys := api.Group("/temp-keys", auth.JWTAuth(), auth.AdminRequired())
		{
			tempKeys.GET("", handlers.ListTempAPIKeys)
			tempKeys.POST("", handlers.CreateTempAPIKey)
			tempKeys.PUT("/:id", handlers.UpdateTempAPIKey)
			tempKeys.DELETE("/:id", handlers.DeleteTempAPIKey)
		}

		// 版本
		api.GET("/version/check", handlers.CheckVersion)
	}

	// OpenAI 兼容接口
	v1 := r.Group("/v1", BodyLimitMiddleware(maxBodyBytes()), auth.APIKeyAuth())
	{
		v1.GET("/models", handlers.OpenAIListModels)
		v1.POST("/chat/completions", handlers.OpenAIChatCompletions)
	}

	// WebSocket 接口 (需要单独处理认证)
	r.GET("/v1/chat/completions/ws", handlers.OpenAIChatCompletionsWS)

	return r
}

func ServeFrontend(r *gin.Engine, dir string) {
	r.Static("/assets", filepath.Join(dir, "assets"))

	r.GET("/", func(c *gin.Context) {
		c.File(filepath.Join(dir, "index.html"))
	})

	r.NoRoute(func(c *gin.Context) {
		// API 路由返回 404
		if len(c.Request.URL.Path) > 4 && c.Request.URL.Path[:4] == "/api" {
			c.JSON(http.StatusNotFound, gin.H{"detail": "Not found"})
			return
		}
		if len(c.Request.URL.Path) > 3 && c.Request.URL.Path[:3] == "/v1" {
			c.JSON(http.StatusNotFound, gin.H{"detail": "Not found"})
			return
		}
		// SPA fallback
		c.File(filepath.Join(dir, "index.html"))
	})
}
