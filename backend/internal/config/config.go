package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Host          string
	Port          int
	DatabasePath  string
	SecretKey     string
	AdminUsername string
	AdminPassword string
}

func Load() *Config {
	cfg := &Config{
		Host:          getEnv("HOST", "0.0.0.0"),
		Port:          getEnvInt("PORT", 8050),
		DatabasePath:  getEnv("DATABASE_PATH", "./data/gateway.db"),
		SecretKey:     getEnv("SECRET_KEY", ""), // 如果为空，后续从数据库获取
		AdminUsername: getEnv("ADMIN_USERNAME", "admin"),
		AdminPassword: getEnv("ADMIN_PASSWORD", "admin123"),
	}
	return cfg
}

// SetSecretKey 设置 SecretKey（用于从数据库加载后更新）
func (c *Config) SetSecretKey(key string) {
	c.SecretKey = key
}

func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func generateSecretKey() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// fallback: 使用固定前缀 + 时间
		return "vte-secret-key-fallback-" + hex.EncodeToString([]byte(fmt.Sprintf("%d", os.Getpid())))
	}
	return hex.EncodeToString(b)
}
