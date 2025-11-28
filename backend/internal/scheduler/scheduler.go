package scheduler

import (
	"time"
	"vte/internal/handlers"
	"vte/internal/logger"
)

// Start 启动定时任务
func Start() {
	go dailyResetTask()
	go cleanupTask()
}

// dailyResetTask 每天下午3点重置统计
func dailyResetTask() {
	for {
		now := time.Now()
		// 计算下一个下午3点的时间
		next3PM := time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, now.Location())
		
		// 如果已经过了今天的3点，则设置为明天的3点
		if now.After(next3PM) {
			next3PM = next3PM.Add(24 * time.Hour)
		}
		
		// 等待到下午3点
		duration := next3PM.Sub(now)
		logger.Info("下次token统计重置时间: " + next3PM.Format("2006-01-02 15:04:05"))
		time.Sleep(duration)
		
		// 执行重置（注意：这里不是真的删除数据，只是标记新的一天开始）
		// 实际上我们保留历史数据，前端只查询今天的数据
		logger.Info("执行每日token统计刷新")
	}
}

// cleanupTask 每天下午3点清理昨天的旧记录
func cleanupTask() {
	for {
		now := time.Now()
		// 计算下一个下午3点的时间（与刷新时间一致）
		next3PM := time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, now.Location())
		
		// 如果已经过了今天的3点，则设置为明天的3点
		if now.After(next3PM) {
			next3PM = next3PM.Add(24 * time.Hour)
		}
		
		// 等待到下午3点
		duration := next3PM.Sub(now)
		time.Sleep(duration)
		
		// 执行清理（在刷新之前清理昨天的数据）
		logger.Info("执行token记录清理任务")
		if err := handlers.CleanOldTokenRecords(); err != nil {
			logger.Error("清理token记录失败: " + err.Error())
		} else {
			logger.Info("token记录清理完成")
		}
	}
}
