package database

import (
	"context"
	"golang.org/x/crypto/bcrypt"
	"path/filepath"
	"testing"
	"time"
)

func TestLegacyKeyMigrationAndAdminPasswordPreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO providers(name,base_url,api_key)VALUES('old','https://example.com/v1','legacy-key')"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAdmin("admin", "first-password"); err != nil {
		t.Fatal(err)
	}
	Close()
	done := make(chan error, 1)
	go func() { done <- Init(path) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("migration deadlocked")
	}
	defer Close()
	var count int
	db.QueryRow("SELECT COUNT(*) FROM provider_api_keys WHERE api_key='legacy-key'").Scan(&count)
	if count != 1 {
		t.Fatal(count)
	}
	t.Setenv("ADMIN_PASSWORD", "different-password")
	if err := EnsureAdmin("admin", "different-password"); err != nil {
		t.Fatal(err)
	}
	var hash string
	db.QueryRow("SELECT hashed_password FROM users WHERE username='admin'").Scan(&hash)
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("first-password")) != nil {
		t.Fatal("upgrade changed password")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
}
