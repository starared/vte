package handlers

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"vte/internal/database"
	"vte/internal/models"
)

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
	
	// 获取今天的开始时间
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	
	// 查询今天的总统计（转换为北京时间）
	var stats models.TokenStats
	err := db.QueryRow(`
		SELECT 
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(prompt_tokens), 0) as prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) as completion_tokens
		FROM token_usage
		WHERE datetime(created_at, '+8 hours') >= ?
	`, todayStart).Scan(&stats.TotalTokens, &stats.PromptTokens, &stats.CompletionTokens)
	
	if err != nil {
		c.JSON(500, gin.H{"detail": "查询统计失败"})
		return
	}
	
	// 查询每小时的统计（转换为北京时间）
	rows, err := db.Query(`
		SELECT 
			CAST(strftime('%H', datetime(created_at, '+8 hours')) AS INTEGER) as hour,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COUNT(*) as request_count
		FROM token_usage
		WHERE datetime(created_at, '+8 hours') >= ?
		GROUP BY hour
		ORDER BY hour
	`, todayStart)
	
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
	
	// 查询按模型分组的统计（转换为北京时间）
	modelRows, err := db.Query(`
		SELECT 
			model_name,
			provider_name,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(prompt_tokens), 0) as prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) as completion_tokens,
			COUNT(*) as request_count
		FROM token_usage
		WHERE datetime(created_at, '+8 hours') >= ?
		GROUP BY model_name, provider_name
		ORDER BY total_tokens DESC
	`, todayStart)
	
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

// CleanOldTokenRecords 清理旧的token记录（只保留今天的）
func CleanOldTokenRecords() error {
	db := database.DB()
	// 获取今天的开始时间（北京时间）
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	_, err := db.Exec("DELETE FROM token_usage WHERE datetime(created_at, '+8 hours') < ?", todayStart)
	return err
}

// ResetTodayTokenStats 重置今天的统计（用于每天下午3点刷新）
func ResetTodayTokenStats(c *gin.Context) {
	db := database.DB()
	
	// 获取今天的开始时间（北京时间）
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	
	_, err := db.Exec("DELETE FROM token_usage WHERE datetime(created_at, '+8 hours') >= ?", todayStart)
	if err != nil {
		c.JSON(500, gin.H{"detail": fmt.Sprintf("重置失败: %v", err)})
		return
	}
	
	c.JSON(200, gin.H{"message": "今日统计已重置"})
}
