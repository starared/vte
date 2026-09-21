package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"vte/internal/config"
	"vte/internal/database"
	"vte/internal/router"
	"vte/internal/scheduler"
)

func main() {
	// 设置全局时区为北京时间
	zone := os.Getenv("TZ")
	if zone == "" {
		zone = "Asia/Shanghai"
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		log.Printf("Warning: Failed to load configured timezone, using UTC: %v", err)
	} else {
		time.Local = loc
	}

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

	// 启动定时任务
	scheduler.Start()

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

	// 使用 http.Server 以支持优雅关闭
	srv := &http.Server{
		Addr:    cfg.Addr(),
		Handler: r,
	}

	go func() {
		log.Printf("VTE started on %s:%d", cfg.Host, cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// 等待中断信号以优雅地关闭服务器（SIGINT / SIGTERM，例如 docker stop）
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// 给进行中的请求最多 30 秒完成
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}
	log.Println("Server exited")
}
