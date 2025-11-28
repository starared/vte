package main

import (
	"log"
	"os"
	"path/filepath"

	"vte/internal/config"
	"vte/internal/database"
	"vte/internal/router"
)

func main() {
	// 初始化配置
	cfg := config.Load()

	// 初始化数据库
	if err := database.Init(cfg.DatabasePath); err != nil {
		log.Fatalf("Failed to init database: %v", err)
	}
	defer database.Close()

	// 确保管理员账户存在
	if err := database.EnsureAdmin(cfg.AdminUsername, cfg.AdminPassword); err != nil {
		log.Fatalf("Failed to ensure admin: %v", err)
	}

	// 如果没有通过环境变量设置 SecretKey，则从数据库获取或生成
	if cfg.SecretKey == "" {
		cfg.SetSecretKey(database.GetOrCreateSecretKey())
		log.Printf("Using persisted SecretKey from database")
	}

	// 设置路由
	r := router.Setup(cfg)

	// 静态文件服务 - 尝试多个可能的路径
	possiblePaths := []string{
		filepath.Join(filepath.Dir(os.Args[0]), "..", "frontend", "dist"),
		filepath.Join(".", "..", "frontend", "dist"),
		filepath.Join(".", "frontend", "dist"),
		"/app/frontend/dist",
	}

	for _, frontendDir := range possiblePaths {
		if _, err := os.Stat(frontendDir); err == nil {
			log.Printf("Serving frontend from: %s", frontendDir)
			router.ServeFrontend(r, frontendDir)
			break
		}
	}

	log.Printf("VTE started on %s:%d", cfg.Host, cfg.Port)
	if err := r.Run(cfg.Addr()); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
