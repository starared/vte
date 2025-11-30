package handlers

import (
	"fmt"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"vte/internal/database"
	"vte/internal/models"
)

// 北京时区 (UTC+8)
var beijingLoc *time.Location

func init() {
	var err error
	beijingLoc, err = time.LoadLocation("Asia/Shanghai")
	if err != nil {
		// 如果无法加载时区，手动创建 UTC+8
		beijingLoc = time.FixedZone("CST", 8*60*60)
		log.Printf("Warning: 无法加载 Asia/Shanghai 时区，使用固定 UTC+8")
	}
}

// GetBeijingTime 获取当前北京时间
func GetBeijingTime() time.Time {
	return time.Now().In(beijingLoc)
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

// GetTodayTokenStats 获取今天的token统计
func GetTodayTokenStats(c *gin.Context) {
	db := database.DB()
	
	// 使用北京时间获取今天的开始时间，然后转换为UTC用于数据库查询
	now := GetBeijingTime()
	todayStartBeijing := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, beijingLoc)
	todayStartUTC := todayStartBeijing.UTC().Format("2006-01-02 15:04:05")
	
	// 查询今天的总统计
	var stats models.TokenStats
	err := db.QueryRow(`
		SELECT 
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(prompt_tokens), 0) as prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) as completion_tokens
		FROM token_usage
		WHERE created_at >= ?
	`, todayStartUTC).Scan(&stats.TotalTokens, &stats.PromptTokens, &stats.CompletionTokens)
	
	if err != nil {
		c.JSON(500, gin.H{"detail": "查询统计失败"})
		return
	}
	
	// 查询每小时的统计（将UTC时间转换为北京时间显示）
	rows, err := db.Query(`
		SELECT 
			CAST(strftime('%H', datetime(created_at, '+8 hours')) AS INTEGER) as hour,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COUNT(*) as request_count
		FROM token_usage
		WHERE created_at >= ?
		GROUP BY hour
		ORDER BY hour
	`, todayStartUTC)
	
	if err != nil {
		c.JSON(500, gin.H{"detail": "查询小时统计失败"})
		return
	}
	defer rows.Close()
	
	type hourlyData struct {
		tokens   int
		requests int
	}
	hourlyMap := make(map[int]hourlyData)
	for rows.Next() {
		var hour, tokens, requests int
		rows.Scan(&hour, &tokens, &requests)
		hourlyMap[hour] = hourlyData{tokens: tokens, requests: requests}
	}
	
	// 填充24小时数据
	stats.HourlyStats = make([]models.HourlyTokenStats, 24)
	for i := 0; i < 24; i++ {
		data := hourlyMap[i]
		stats.HourlyStats[i] = models.HourlyTokenStats{
			Hour:         i,
			TotalTokens:  data.tokens,
			RequestCount: data.requests,
		}
	}
	
	// 查询按模型分组的统计
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
	`, todayStartUTC)
	
	if err != nil {
		c.JSON(500, gin.H{"detail": "查询模型统计失败"})
		return
	}
	defer modelRows.Close()
	
	stats.ModelStats = []models.ModelTokenStats{}
	for modelRows.Next() {
		var ms models.ModelTokenStats
		modelRows.Scan(&ms.ModelName, &ms.ProviderName, &ms.TotalTokens, 
			&ms.PromptTokens, &ms.CompletionTokens, &ms.RequestCount)
		stats.ModelStats = append(stats.ModelStats, ms)
	}
	
	c.JSON(200, stats)
}

// CleanOldTokenRecords 清理旧的token记录（只保留今天的，基于北京时间）
func CleanOldTokenRecords() error {
	db := database.DB()
	// 使用北京时间获取今天的开始时间
	now := GetBeijingTime()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, beijingLoc)
	// 转换为 UTC 时间进行数据库查询
	todayStartUTC := todayStart.UTC()
	_, err := db.Exec("DELETE FROM token_usage WHERE created_at < ?", todayStartUTC.Format("2006-01-02 15:04:05"))
	return err
}

// ResetTodayTokenStats 重置今天的统计（手动重置）
func ResetTodayTokenStats(c *gin.Context) {
	db := database.DB()
	
	// 使用北京时间获取今天的开始时间
	now := GetBeijingTime()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, beijingLoc)
	// 转换为 UTC 时间进行数据库查询
	todayStartUTC := todayStart.UTC()
	
	_, err := db.Exec("DELETE FROM token_usage WHERE created_at >= ?", todayStartUTC.Format("2006-01-02 15:04:05"))
	if err != nil {
		c.JSON(500, gin.H{"detail": fmt.Sprintf("重置失败: %v", err)})
		return
	}
	
	c.JSON(200, gin.H{"message": "今日统计已重置"})
}
