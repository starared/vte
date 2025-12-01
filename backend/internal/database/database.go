package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
	"golang.org/x/crypto/bcrypt"
)

var db *sql.DB

func Init(dbPath string) error {
	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return err
	}

	var err error
	db, err = sql.Open("sqlite", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return err
	}

	// SQLite 并发优化
	db.SetMaxOpenConns(1) // SQLite 只支持单写入
	db.SetMaxIdleConns(1)
	
	// 设置 PRAGMA 优化
	db.Exec("PRAGMA busy_timeout=5000")    // 等待5秒而不是立即失败
	db.Exec("PRAGMA synchronous=NORMAL")   // 提升性能
	db.Exec("PRAGMA cache_size=10000")     // 增加缓存
	db.Exec("PRAGMA temp_store=MEMORY")    // 临时表存内存

	// 创建表
	return createTables()
}

func Close() {
	if db != nil {
		db.Close()
	}
}

func DB() *sql.DB {
	return db
}

func createTables() error {
	schemas := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			hashed_password TEXT NOT NULL,
			api_key TEXT UNIQUE NOT NULL,
			is_admin INTEGER DEFAULT 0,
			is_active INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS providers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			base_url TEXT NOT NULL,
			api_key TEXT NOT NULL,
			model_prefix TEXT DEFAULT '',
			provider_type TEXT DEFAULT 'standard',
			vertex_project TEXT,
			vertex_location TEXT DEFAULT 'global',
			extra_headers TEXT,
			proxy_url TEXT,
			is_active INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS models (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider_id INTEGER NOT NULL,
			original_id TEXT NOT NULL,
			display_name TEXT,
			is_active INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (provider_id) REFERENCES providers(id)
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS provider_api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider_id INTEGER NOT NULL,
			api_key TEXT NOT NULL,
			name TEXT DEFAULT '',
			is_active INTEGER DEFAULT 1,
			usage_count INTEGER DEFAULT 0,
			last_used_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (provider_id) REFERENCES providers(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_provider_api_keys_provider ON provider_api_keys(provider_id)`,
		`CREATE TABLE IF NOT EXISTS token_usage (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			model_name TEXT NOT NULL,
			provider_name TEXT NOT NULL,
			prompt_tokens INTEGER DEFAULT 0,
			completion_tokens INTEGER DEFAULT 0,
			total_tokens INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_token_usage_created_at ON token_usage(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_token_usage_model ON token_usage(model_name)`,
	}

	for _, schema := range schemas {
		if _, err := db.Exec(schema); err != nil {
			return err
		}
	}

	// 迁移：添加缺失的列
	migrateAddMissingColumns()

	// 迁移：将 providers 表中的 api_key 迁移到 provider_api_keys 表
	migrateProviderAPIKeys()

	return nil
}

// migrateAddMissingColumns 添加缺失的列
func migrateAddMissingColumns() {
	// 检查并添加 usage_count 列
	db.Exec("ALTER TABLE provider_api_keys ADD COLUMN usage_count INTEGER DEFAULT 0")
	// 检查并添加 last_used_at 列
	db.Exec("ALTER TABLE provider_api_keys ADD COLUMN last_used_at DATETIME")
}

// migrateProviderAPIKeys 将 providers 表中的 api_key 迁移到 provider_api_keys 表
func migrateProviderAPIKeys() {
	// 查找所有有 api_key 但在 provider_api_keys 表中没有记录的提供商
	rows, err := db.Query(`
		SELECT p.id, p.api_key, p.created_at 
		FROM providers p 
		WHERE p.api_key != '' AND p.api_key IS NOT NULL
		AND NOT EXISTS (SELECT 1 FROM provider_api_keys pk WHERE pk.provider_id = p.id)
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var providerID int
		var apiKey string
		var createdAt time.Time
		if err := rows.Scan(&providerID, &apiKey, &createdAt); err != nil {
			continue
		}

		// 插入到 provider_api_keys 表
		db.Exec(`
			INSERT INTO provider_api_keys (provider_id, api_key, name, is_active, created_at)
			VALUES (?, ?, '密钥 1', 1, ?)
		`, providerID, apiKey, createdAt)
	}
}

// GetOrCreateSecretKey 获取或创建持久化的 SecretKey
func GetOrCreateSecretKey() string {
	var key string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'secret_key'").Scan(&key)
	if err == nil && key != "" {
		return key
	}

	// 生成新的 SecretKey
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		key = "vte-fallback-secret-" + hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	} else {
		key = hex.EncodeToString(b)
	}

	// 存储到数据库
	db.Exec("INSERT INTO settings (key, value) VALUES ('secret_key', ?) ON CONFLICT(key) DO UPDATE SET value = ?", key, key)
	return key
}

func EnsureAdmin(username, password string) error {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		// 管理员不存在，创建新管理员
		hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		apiKey := generateAPIKey()
		_, err = db.Exec(
			"INSERT INTO users (username, hashed_password, api_key, is_admin) VALUES (?, ?, ?, 1)",
			username, string(hashed), apiKey,
		)
		return err
	}

	// 管理员已存在，检查是否需要更新密码（仅当环境变量设置时）
	if envPwd := os.Getenv("ADMIN_PASSWORD"); envPwd != "" && envPwd != "admin123" {
		hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		_, err = db.Exec("UPDATE users SET hashed_password = ? WHERE username = ?", string(hashed), username)
		return err
	}
	return nil
}

func generateAPIKey() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// fallback
		return hex.EncodeToString([]byte("fallback-api-key-" + filepath.Base(os.Args[0])))[:64]
	}
	return hex.EncodeToString(b)
}
