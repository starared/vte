package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Version 当前版本号。构建时可用 -ldflags "-X vte/internal/handlers.Version=x.y.z" 覆盖；
// 运行时如果能找到 VERSION 文件，以文件内容为准（Docker 镜像会携带该文件）。
var Version = "1.2.0"

// 最新版本检查结果缓存，避免每次打开「关于」页面都请求 GitHub（未认证的 GitHub API 每小时只有 60 次额度）
var latestCache struct {
	sync.Mutex
	version   string
	expiresAt time.Time
}

const (
	latestCacheTTL       = time.Hour
	latestCacheFailedTTL = 5 * time.Minute
)

func cachedLatestVersion() string {
	latestCache.Lock()
	defer latestCache.Unlock()
	if time.Now().Before(latestCache.expiresAt) {
		return latestCache.version
	}
	latest := getLatestVersion()
	ttl := latestCacheTTL
	if latest == "" {
		ttl = latestCacheFailedTTL
	}
	latestCache.version = latest
	latestCache.expiresAt = time.Now().Add(ttl)
	return latest
}

func CheckVersion(c *gin.Context) {
	// 尝试从多个可能的路径读取 VERSION 文件
	version := Version
	possiblePaths := []string{
		"VERSION",
		"../VERSION",
		"/app/VERSION",
		filepath.Join(filepath.Dir(os.Args[0]), "..", "VERSION"),
	}

	for _, path := range possiblePaths {
		if data, err := os.ReadFile(path); err == nil {
			version = strings.TrimSpace(string(data))
			break
		}
	}

	// 获取最新版本（从 Docker Hub 或 GitHub）
	latest := cachedLatestVersion()

	c.JSON(200, gin.H{
		"current": version,
		"latest":  latest,
	})
}

func getLatestVersion() string {
	client := &http.Client{Timeout: 5 * time.Second}

	// 从 GitHub Release 获取最新版本
	resp, err := client.Get("https://api.github.com/repos/starared/vte/releases/latest")
	if err != nil {
		log.Printf("[WARN] 检查更新失败: %v", err)
		return "" // 返回空字符串表示获取失败
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("[WARN] 检查更新失败: HTTP %d", resp.StatusCode)
		return ""
	}

	var result struct {
		TagName string `json:"tag_name"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[WARN] 解析版本信息失败: %v", err)
		return ""
	}

	// 移除 'v' 前缀
	version := strings.TrimPrefix(result.TagName, "v")
	return version
}
