package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

var db *sql.DB

func Init(dbPath string) error {
	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return err
	}

	var err error
	// PRAGMA 写在连接串里：每次新建连接都会自动应用（连接断开重连后也不会丢失）
	dsn := dbPath +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(30000)" + // 等待30秒而不是立即失败
		"&_pragma=synchronous(NORMAL)" + // 提升性能
		"&_pragma=cache_size(10000)" + // 增加缓存
		"&_pragma=temp_store(MEMORY)" // 临时表存内存
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}

	// SQLite 并发优化
	db.SetMaxOpenConns(1) // SQLite 只支持单写入
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		return fmt.Errorf("open database: %w", err)
	}

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
			password_changed_at INTEGER DEFAULT 0,
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
			custom_name INTEGER DEFAULT 0,
			is_active INTEGER DEFAULT 1,
			source TEXT DEFAULT '',
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
		`CREATE TABLE IF NOT EXISTS temp_api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT,
			token TEXT UNIQUE NOT NULL,
			allowed_models TEXT NOT NULL,
			model_limits TEXT,
			model_usage TEXT,
			max_requests INTEGER DEFAULT 0,
			used_requests INTEGER DEFAULT 0,
			rate_limit_count INTEGER DEFAULT 0,
			rate_limit_window INTEGER DEFAULT 0,
			rate_limit_unit TEXT DEFAULT '',
			concurrency_limit INTEGER DEFAULT 0,
			expire_duration INTEGER DEFAULT 0,
			expire_unit TEXT DEFAULT '',
			expires_at DATETIME,
			is_active INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_temp_api_keys_token ON temp_api_keys(token)`,
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
	return migrateProviderAPIKeys()
}

// migrateAddMissingColumns 添加缺失的列
func migrateAddMissingColumns() {
	// 检查并添加 usage_count 列
	db.Exec("ALTER TABLE provider_api_keys ADD COLUMN usage_count INTEGER DEFAULT 0")
	// 检查并添加 last_used_at 列
	db.Exec("ALTER TABLE provider_api_keys ADD COLUMN last_used_at DATETIME")
	// 检查并添加 custom_name 列（用于标记用户自定义的模型显示名称）
	db.Exec("ALTER TABLE models ADD COLUMN custom_name INTEGER DEFAULT 0")
	// 临时 API Key 相关列
	db.Exec("ALTER TABLE temp_api_keys ADD COLUMN rate_limit_count INTEGER DEFAULT 0")
	db.Exec("ALTER TABLE temp_api_keys ADD COLUMN rate_limit_window INTEGER DEFAULT 0")
	db.Exec("ALTER TABLE temp_api_keys ADD COLUMN rate_limit_unit TEXT DEFAULT ''")
	db.Exec("ALTER TABLE temp_api_keys ADD COLUMN concurrency_limit INTEGER DEFAULT 0")
	db.Exec("ALTER TABLE temp_api_keys ADD COLUMN expire_duration INTEGER DEFAULT 0")
	db.Exec("ALTER TABLE temp_api_keys ADD COLUMN expire_unit TEXT DEFAULT ''")
	// 修改密码时间（用于让旧登录令牌失效）
	db.Exec("ALTER TABLE users ADD COLUMN password_changed_at INTEGER DEFAULT 0")
	// 模型来源：manual（手动添加）/ fetched（从上游拉取）/ ''（旧数据，来源未知）
	db.Exec("ALTER TABLE models ADD COLUMN source TEXT DEFAULT ''")
}

// migrateProviderAPIKeys 将 providers 表中的 api_key 迁移到 provider_api_keys 表
func migrateProviderAPIKeys() error {
	// A single statement avoids holding the only connection while inserting rows.
	_, err := db.Exec(`INSERT INTO provider_api_keys (provider_id, api_key, name, is_active, created_at)
		SELECT p.id, p.api_key, '密钥 1', 1, p.created_at FROM providers p
		WHERE p.api_key != '' AND NOT EXISTS
		(SELECT 1 FROM provider_api_keys k WHERE k.provider_id = p.id AND k.api_key = p.api_key)`)
	if err != nil {
		return fmt.Errorf("migrate provider keys: %w", err)
	}
	if _, err := db.Exec("UPDATE providers SET api_key = '' WHERE api_key != ''"); err != nil {
		return err
	}
	// 清理旧版本删除提供商时遗留的密钥和模型
	if _, err := db.Exec("DELETE FROM provider_api_keys WHERE provider_id NOT IN (SELECT id FROM providers)"); err != nil {
		return err
	}
	if _, err := db.Exec("DELETE FROM models WHERE provider_id NOT IN (SELECT id FROM providers)"); err != nil {
		return err
	}
	return nil
}

// GetOrCreateSecretKey 获取或创建持久化的 SecretKey
func GetOrCreateSecretKey() string {
	var key string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'secret_key'").Scan(&key)
	if err == nil && key != "" {
		return key
	}

	// 生成新的 SecretKey（crypto/rand 失败时宁可退出，也不能使用可预测的密钥）
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("生成 SecretKey 失败: %v", err)
	}
	key = hex.EncodeToString(b)

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
		if password == "" {
			b := make([]byte, 18)
			if _, err := rand.Read(b); err != nil {
				return err
			}
			password = hex.EncodeToString(b)
			log.Printf("Initial administrator %s password: %s (change after login)", username, password)
		}

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

	// Existing database credentials are preserved across upgrades.

	return nil
}

func generateAPIKey() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// 不能退回到可预测的固定值，否则所有实例的 API Key 都一样
		log.Fatalf("生成 API Key 失败: %v", err)
	}
	return hex.EncodeToString(b)
}
