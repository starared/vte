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

const currentVersion = "1.0.0"

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

	// 尝试从 Docker Hub 获取
	resp, err := client.Get("https://hub.docker.com/v2/repositories/rtyedfty/vte/tags?page_size=1&ordering=last_updated")
	if err != nil {
		log.Printf("[WARN] 检查更新失败: %v", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("[WARN] 检查更新失败: HTTP %d", resp.StatusCode)
		return ""
	}

	var result struct {
		Results []struct {
			Name string `json:"name"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[WARN] 解析版本信息失败: %v", err)
		return ""
	}

	if len(result.Results) > 0 {
		tag := result.Results[0].Name
		if tag != "latest" {
			return tag
		}
	}

	return currentVersion
}
