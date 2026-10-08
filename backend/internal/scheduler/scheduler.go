package scheduler

import (
	"time"

	"vte/internal/handlers"
	"vte/internal/logger"
)

// Start 启动定时任务
func Start() {
	go dailyCleanupTask()
}

// dailyCleanupTask 在每个 Token 统计周期结束时清理旧记录
// （默认北京时间 15:00，可通过 TOKEN_STATS_TZ / TOKEN_STATS_RESET_HOUR 调整）
func dailyCleanupTask() {
	for {
		next := handlers.NextStatsReset()
		logger.Info("下次 token 统计重置时间: " + next.Format("2006-01-02 15:04:05 MST"))
		time.Sleep(time.Until(next))
		if time.Now().Before(next) {
			continue // 被提前唤醒（例如系统时间被调整），重新计算
		}

		logger.Info("执行每日 token 记录清理任务")
		if err := handlers.CleanOldTokenRecords(); err != nil {
			logger.Error("清理token记录失败: " + err.Error())
		} else {
			logger.Info("token记录清理完成")
		}

	}
}
