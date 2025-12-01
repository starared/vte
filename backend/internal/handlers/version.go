package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const currentVersion = "1.0.5"

func CheckVersion(c *gin.Context) {
	// 尝试从多个可能的路径读取 VERSION 文件
	version := currentVersion
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
	latest := getLatestVersion()

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
