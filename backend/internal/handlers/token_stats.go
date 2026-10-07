package handlers

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"vte/internal/database"
	"vte/internal/models"
)

// Token 统计周期：每天在 statsLoc 时区的 statsResetHour 点开始新周期。
// 默认北京时间 15:00；可用环境变量调整，例如想对齐太平洋时间零点：
//
//	TOKEN_STATS_TZ=America/Los_Angeles TOKEN_STATS_RESET_HOUR=0
//
// 使用时区名称（而不是固定偏移）可以自动处理夏令时。
var (
	statsLoc       *time.Location
	statsResetHour = 15
)

func init() {
	tzName := os.Getenv("TOKEN_STATS_TZ")
	if tzName == "" {
		tzName = "Asia/Shanghai"
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		log.Printf("Warning: 无法加载时区 %s，Token 统计使用固定 UTC+8: %v", tzName, err)
		loc = time.FixedZone("UTC+8", 8*60*60)
	}
	statsLoc = loc

	if v := os.Getenv("TOKEN_STATS_RESET_HOUR"); v != "" {
		if h, err := strconv.Atoi(v); err == nil && h >= 0 && h <= 23 {
			statsResetHour = h
		} else {
			log.Printf("Warning: TOKEN_STATS_RESET_HOUR=%q 无效（应为 0-23），使用默认 15", v)
		}
	}
}

// periodStartAt 返回 t 所在统计周期的开始时间
func periodStartAt(t time.Time) time.Time {
	now := t.In(statsLoc)
	start := time.Date(now.Year(), now.Month(), now.Day(), statsResetHour, 0, 0, 0, statsLoc)
	if now.Before(start) {
		start = start.AddDate(0, 0, -1)
	}
	return start
}

// GetCurrentPeriodStart 获取当前统计周期的开始时间
func GetCurrentPeriodStart() time.Time {
	return periodStartAt(time.Now())
}

// NextStatsReset 获取下一次统计重置时间
func NextStatsReset() time.Time {
	start := GetCurrentPeriodStart()
	return time.Date(start.Year(), start.Month(), start.Day()+1, statsResetHour, 0, 0, 0, statsLoc)
}

// RecordTokenUsage 记录token使用情况
func RecordTokenUsage(modelName, providerName string, promptTokens, completionTokens, totalTokens int) error {
	db := database.DB()
	_, err := db.Exec(`
		INSERT INTO token_usage (model_name, provider_name, prompt_tokens, completion_tokens, total_tokens)
		VALUES (?, ?, ?, ?, ?)
	`, modelName, providerName, promptTokens, completionTokens, totalTokens)
	return err
}

// GetTodayTokenStats 获取当前统计周期的 token 统计
func GetTodayTokenStats(c *gin.Context) {
	db := database.DB()

	now := time.Now().In(statsLoc)
	periodStartUTC := GetCurrentPeriodStart().UTC().Format("2006-01-02 15:04:05")

	var stats models.TokenStats
	err := db.QueryRow(`
		SELECT 
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(prompt_tokens), 0) as prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) as completion_tokens
		FROM token_usage 
		WHERE created_at >= ?
	`, periodStartUTC).Scan(&stats.TotalTokens, &stats.PromptTokens, &stats.CompletionTokens)
	if err != nil {
		c.JSON(500, gin.H{"detail": "查询统计失败"})
		return
	}

	// 按模型分组的统计
	modelRows, err := db.Query(`
		SELECT 
			model_name,
			provider_name,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(prompt_tokens), 0) as prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) as completion_tokens,
			COUNT(*) as request_count
		FROM token_usage
		WHERE created_at >= ?
		GROUP BY model_name, provider_name
		ORDER BY total_tokens DESC
	`, periodStartUTC)
	if err != nil {
		c.JSON(500, gin.H{"detail": "查询模型统计失败"})
		return
	}
	defer modelRows.Close()

	stats.ModelStats = []models.ModelTokenStats{}
	for modelRows.Next() {
		var ms models.ModelTokenStats
		if err := modelRows.Scan(&ms.ModelName, &ms.ProviderName, &ms.TotalTokens,
			&ms.PromptTokens, &ms.CompletionTokens, &ms.RequestCount); err != nil {
			c.JSON(500, gin.H{"detail": "查询模型统计失败"})
			return
		}
		stats.ModelStats = append(stats.ModelStats, ms)
	}

	_, offset := now.Zone()
	c.JSON(200, gin.H{
		"total_tokens":      stats.TotalTokens,
		"prompt_tokens":     stats.PromptTokens,
		"completion_tokens": stats.CompletionTokens,
		"model_stats":       stats.ModelStats,
		"server_time":       now.Format("2006-01-02 15:04:05"),
		"next_reset_time":   NextStatsReset().Format("2006-01-02 15:04:05"),
		"timezone":          fmt.Sprintf("%s (UTC%s)", statsLoc.String(), formatOffset(offset)),
		"reset_hour":        statsResetHour,
	})
}

func formatOffset(seconds int) string {
	sign := "+"
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	h, m := seconds/3600, (seconds%3600)/60
	if m == 0 {
		return fmt.Sprintf("%s%d", sign, h)
	}
	return fmt.Sprintf("%s%d:%02d", sign, h, m)
}

// CleanOldTokenRecords 清理旧的token记录（删除当前周期之前的所有数据）
func CleanOldTokenRecords() error {
	db := database.DB()
	// 获取当前统计周期的开始时间
	periodStart := GetCurrentPeriodStart()
	// 转换为 UTC 时间进行数据库查询
	periodStartUTC := periodStart.UTC()
	_, err := db.Exec("DELETE FROM token_usage WHERE created_at < ?", periodStartUTC.Format("2006-01-02 15:04:05"))
	return err
}

// ResetTodayTokenStats 重置当前周期的统计（手动重置）
func ResetTodayTokenStats(c *gin.Context) {
	db := database.DB()

	// 获取当前统计周期的开始时间
	periodStart := GetCurrentPeriodStart()
	// 转换为 UTC 时间进行数据库查询
	periodStartUTC := periodStart.UTC()

	_, err := db.Exec("DELETE FROM token_usage WHERE created_at >= ?", periodStartUTC.Format("2006-01-02 15:04:05"))
	if err != nil {
		c.JSON(500, gin.H{"detail": fmt.Sprintf("重置失败: %v", err)})
		return
	}

	c.JSON(200, gin.H{"message": "当前周期统计已重置"})
}
