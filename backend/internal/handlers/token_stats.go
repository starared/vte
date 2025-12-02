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
	
	// 查询今天的总统计（24小时）
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
	
	// 获取当前北京时间的小时和分钟
	currentHour := now.Hour()
	currentMinute := now.Minute()
	// 计算当前时段（20分钟一个时段：0-19, 20-39, 40-59）
	currentSlot := currentHour*3 + currentMinute/20
	
	// 查询今天所有的统计数据（按 20 分钟分组）
	rows, err := db.Query(`
		SELECT 
			CAST(strftime('%H', datetime(created_at, '+8 hours')) AS INTEGER) as hour,
			CAST(strftime('%M', datetime(created_at, '+8 hours')) AS INTEGER) / 20 as minute_slot,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COUNT(*) as request_count
		FROM token_usage
		WHERE created_at >= ?
		GROUP BY hour, minute_slot
		ORDER BY hour, minute_slot
	`, todayStartUTC)
	
	if err != nil {
		c.JSON(500, gin.H{"detail": "查询时段统计失败"})
		return
	}
	defer rows.Close()
	
	// 用 slot 标识（hour*3 + minute_slot）作为 key
	type slotData struct {
		tokens   int
		requests int
	}
	slotMap := make(map[int]slotData)
	for rows.Next() {
		var hour, minuteSlot, tokens, requests int
		rows.Scan(&hour, &minuteSlot, &tokens, &requests)
		slotKey := hour*3 + minuteSlot
		slotMap[slotKey] = slotData{tokens: tokens, requests: requests}
	}
	
	// 填充前后各8个时段的数据（共 17 个时段，约 5.5 小时）
	stats.HourlyStats = make([]models.HourlyTokenStats, 0, 17)
	for s := currentSlot - 8; s <= currentSlot + 8; s++ {
		slot := s
		// 处理跨天的情况
		if slot < 0 {
			slot += 72 // 24*3
		} else if slot >= 72 {
			slot -= 72
		}
		
		data := slotMap[slot]
		// 计算小时和分钟
		hour := slot / 3
		minuteSlot := slot % 3
		minute := minuteSlot * 20
		
		stats.HourlyStats = append(stats.HourlyStats, models.HourlyTokenStats{
			Hour:         hour*100 + minute, // 用 HHMM 格式表示，如 1420 表示 14:20
			TotalTokens:  data.tokens,
			RequestCount: data.requests,
		})
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
	
	// 添加当前小时和重置时间信息
	// 计算下次重置时间（北京时间15:00）
	next3PM := time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, beijingLoc)
	if now.After(next3PM) {
		next3PM = next3PM.Add(24 * time.Hour)
	}
	
	c.JSON(200, gin.H{
		"total_tokens":      stats.TotalTokens,
		"prompt_tokens":     stats.PromptTokens,
		"completion_tokens": stats.CompletionTokens,
		"hourly_stats":      stats.HourlyStats,
		"model_stats":       stats.ModelStats,
		"server_time":       now.Format("2006-01-02 15:04:05"),
		"next_reset_time":   next3PM.Format("2006-01-02 15:04:05"),
		"timezone":          "Asia/Shanghai (UTC+8)",
	})
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
