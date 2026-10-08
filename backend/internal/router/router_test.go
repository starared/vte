package router

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"vte/internal/config"
)

// 来自受信任代理网段（如 Docker 网关）的请求应使用 X-Forwarded-For 中的真实 IP
func TestTrustedProxiesWithSpacesAndCIDR(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", " 127.0.0.1 , ::1, 172.16.0.0/12 ")
	r := Setup(&config.Config{SecretKey: "x"})
	var got string
	r.GET("/ip", func(c *gin.Context) { got = c.ClientIP() })

	req := httptest.NewRequest("GET", "/ip", nil)
	req.RemoteAddr = "172.18.0.1:5555"
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	r.ServeHTTP(httptest.NewRecorder(), req)
	if got != "203.0.113.7" {
		t.Fatalf("got %q", got)
	}

	req = httptest.NewRequest("GET", "/ip", nil)
	req.RemoteAddr = "198.51.100.9:5555"
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	r.ServeHTTP(httptest.NewRecorder(), req)
	if got != "198.51.100.9" {
		t.Fatalf("untrusted proxy accepted: %q", got)
	}
}

// index.html 不能被缓存（升级后会引用已不存在的旧资源），带哈希的静态资源可以长期缓存
func TestFrontendCacheHeaders(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o644)
	os.WriteFile(filepath.Join(dir, "assets", "app.abc123.js"), []byte("1"), 0o644)
	r := Setup(&config.Config{SecretKey: "x"})
	ServeFrontend(r, dir)

	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec
	}
	for _, path := range []string{"/", "/settings"} {
		rec := get(path)
		if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("%s: status %d cache-control %q", path, rec.Code, rec.Header().Get("Cache-Control"))
		}
	}
	rec := get("/assets/app.abc123.js")
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("asset: status %d cache-control %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if rec := get("/api/nope"); rec.Code != 404 {
		t.Fatal(rec.Code)
	}
}
