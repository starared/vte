package logger

import (
	"fmt"
	"sync"
	"time"
)

const maxLogs = 100

var (
	logs  []string
	stats = Stats{LastReset: time.Now().Format(time.RFC3339)}
	mu    sync.RWMutex
)

type Stats struct {
	TotalRequests   int    `json:"total_requests"`
	SuccessRequests int    `json:"success_requests"`
	ErrorRequests   int    `json:"error_requests"`
	LastReset       string `json:"last_reset"`
}

func log(level, message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("[%s] [%s] %s", timestamp, level, message)

	mu.Lock()
	logs = append(logs, line)
	if len(logs) > maxLogs {
		logs = logs[1:]
	}
	mu.Unlock()

	fmt.Println(line)
}

func Info(message string) {
	log("INFO", message)
}

func Warn(message string) {
	log("WARN", message)
}

func Error(message string) {
	log("ERROR", message)
}

func RequestStart() {
	mu.Lock()
	stats.TotalRequests++
	mu.Unlock()
}

func RequestSuccess() {
	mu.Lock()
	stats.SuccessRequests++
	mu.Unlock()
}

func RequestError() {
	mu.Lock()
	stats.ErrorRequests++
	mu.Unlock()
}

func GetLogs() []string {
	mu.RLock()
	defer mu.RUnlock()
	result := make([]string, len(logs))
	copy(result, logs)
	return result
}

func GetStats() Stats {
	mu.RLock()
	defer mu.RUnlock()
	return stats
}

func ClearLogs() {
	mu.Lock()
	logs = nil
	mu.Unlock()
}

func ResetStats() {
	mu.Lock()
	stats = Stats{LastReset: time.Now().Format(time.RFC3339)}
	mu.Unlock()
}
