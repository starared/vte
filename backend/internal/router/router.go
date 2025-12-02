package router

import (
	"net/http"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"vte/internal/auth"
	"vte/internal/config"
	"vte/internal/handlers"
)

// CORS 中间件
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}

		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Credentials", "true")
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
		}

		// 版本
		api.GET("/version/check", handlers.CheckVersion)
	}

	// OpenAI 兼容接口
	v1 := r.Group("/v1", auth.APIKeyAuth())
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
