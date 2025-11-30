package scheduler

import (
	"log"
	"time"
	"vte/internal/handlers"
	"vte/internal/logger"
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

// Start 启动定时任务
func Start() {
	go dailyCleanupTask()
}

// dailyCleanupTask 每天北京时间下午3点清理旧记录
func dailyCleanupTask() {
	for {
		// 使用北京时间计算
		now := GetBeijingTime()
		// 计算下一个北京时间下午3点
		next3PM := time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, beijingLoc)
		
		// 如果已经过了今天的3点，则设置为明天的3点
		if now.After(next3PM) {
			next3PM = next3PM.Add(24 * time.Hour)
		}
		
		// 等待到北京时间下午3点
		duration := next3PM.Sub(now)
		logger.Info("下次token统计重置时间(北京时间): " + next3PM.Format("2006-01-02 15:04:05 MST"))
		time.Sleep(duration)
		
		// 执行清理
		logger.Info("执行每日token记录清理任务(北京时间15:00)")
		if err := handlers.CleanOldTokenRecords(); err != nil {
			logger.Error("清理token记录失败: " + err.Error())
		} else {
			logger.Info("token记录清理完成")
		}
	}
}
