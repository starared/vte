package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"

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
	}

	for _, schema := range schemas {
		if _, err := db.Exec(schema); err != nil {
			return err
		}
	}
	return nil
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
