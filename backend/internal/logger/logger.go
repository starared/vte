package logger

import (
	"fmt"
	"time"
)

// 输出一行带时间和级别的日志到标准输出（Docker 下即 docker logs）
func log(level, message string) {
	fmt.Printf("[%s] [%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), level, message)
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
