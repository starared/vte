package router

import (
	"net/http/httptest"
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
