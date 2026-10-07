package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"vte/internal/auth"
	"vte/internal/database"
)

// ---------- 登录令牌 ----------

func setupAuth(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	setupGateway(t, func(w http.ResponseWriter, r *http.Request) {})
	auth.SetSecretKey("test-secret")
	hashed, _ := auth.HashPassword("old-password")
	if _, err := database.DB().Exec("UPDATE users SET hashed_password=? WHERE username='admin'", hashed); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.POST("/login", Login)
	r.GET("/me", auth.JWTAuth(), GetMe)
	r.POST("/change-username", auth.JWTAuth(), ChangeUsername)
	r.POST("/change-password", auth.JWTAuth(), ChangePassword)
	rec := sendJSON(r, "POST", "/login", "", `{"username":"admin","password":"old-password"}`)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	return r, body.AccessToken
}

func sendJSON(r http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestTokenSurvivesUsernameChange(t *testing.T) {
	r, token := setupAuth(t)
	if rec := sendJSON(r, "POST", "/change-username", token, `{"new_username":"renamed"}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	rec := sendJSON(r, "GET", "/me", token, "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"username":"renamed"`) {
		t.Fatal(rec.Code, rec.Body.String())
	}
}

func TestPasswordChangeRevokesOldTokens(t *testing.T) {
	r, _ := setupAuth(t)
	// 一个「一分钟前」签发的旧令牌（模拟其他设备上的登录）
	old := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "1", "uid": 1, "iat": time.Now().Add(-time.Minute).Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	})
	oldToken, _ := old.SignedString([]byte("test-secret"))
	if rec := sendJSON(r, "GET", "/me", oldToken, ""); rec.Code != 200 {
		t.Fatal("old token should work before password change", rec.Body.String())
	}

	rec := sendJSON(r, "POST", "/change-password", oldToken, `{"old_password":"old-password","new_password":"new-password"}`)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.AccessToken == "" {
		t.Fatal("no new token returned")
	}
	if rec := sendJSON(r, "GET", "/me", oldToken, ""); rec.Code != 401 {
		t.Fatal("old token still valid after password change")
	}
	if rec := sendJSON(r, "GET", "/me", body.AccessToken, ""); rec.Code != 200 {
		t.Fatal("new token rejected", rec.Body.String())
	}
}

func TestLegacyUsernameTokenStillAccepted(t *testing.T) {
	r, _ := setupAuth(t)
	legacy := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "admin", "exp": time.Now().Add(time.Hour).Unix()})
	token, _ := legacy.SignedString([]byte("test-secret"))
	if rec := sendJSON(r, "GET", "/me", token, ""); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
}

// ---------- 拉取模型 ----------

func TestFetchKeepsManualAndRenamedModels(t *testing.T) {
	r := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"new"},{"id":"kept"}]}`)
	})
	db := database.DB()
	for _, q := range []string{
		"INSERT INTO models(provider_id,original_id,display_name,is_active,source)VALUES(1,'manual-x','p/manual-x',1,'manual')",
		"INSERT INTO models(provider_id,original_id,display_name,custom_name,is_active,source)VALUES(1,'aliased','my-alias',1,1,'fetched')",
		"INSERT INTO models(provider_id,original_id,display_name,is_active,source)VALUES(1,'gone','p/gone',1,'fetched')",
		"INSERT INTO models(provider_id,original_id,display_name,is_active,source)VALUES(1,'kept','p/kept',1,'fetched')",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if rec := sendRequest(r, "/fetch/1", "gateway-key", `{}`); rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	state := map[string]string{}
	rows, _ := db.Query("SELECT original_id, is_active, COALESCE(source,'') FROM models")
	for rows.Next() {
		var id, source string
		var active int
		rows.Scan(&id, &active, &source)
		state[id] = fmt.Sprintf("%d/%s", active, source)
	}
	rows.Close()
	want := map[string]string{
		"real":     "0/",        // 旧数据（来源未知），上游没有 → 停用但保留
		"manual-x": "1/manual",  // 手动添加 → 原样保留
		"aliased":  "0/fetched", // 改过名 → 停用但保留
		"kept":     "1/fetched", // 上游仍然有 → 保留
		"new":      "0/fetched", // 新模型默认不启用
	}
	if _, ok := state["gone"]; ok {
		t.Fatal("model removed upstream should be deleted")
	}
	for id, w := range want {
		if state[id] != w {
			t.Errorf("%s: got %q want %q", id, state[id], w)
		}
	}
}

// ---------- 删除提供商 ----------

func TestDeleteProviderRemovesKeys(t *testing.T) {
	setupGateway(t, func(w http.ResponseWriter, r *http.Request) {})
	r := gin.New()
	r.DELETE("/providers/:id", DeleteProvider)
	if rec := sendJSON(r, "DELETE", "/providers/1", "", ""); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	for _, table := range []string{"provider_api_keys", "models", "providers"} {
		var n int
		database.DB().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n)
		if n != 0 {
			t.Errorf("%s still has %d rows", table, n)
		}
	}
}

// ---------- 密钥切换 ----------

func TestGatewayRotatesKeyOnRateLimit(t *testing.T) {
	var got []string
	r := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") == "Bearer key-a" {
			w.WriteHeader(429)
			fmt.Fprint(w, `{"error":{"message":"slow down"}}`)
			return
		}
		fmt.Fprint(w, completionJSON)
	})
	rec := sendRequest(r, "/v1/chat/completions", "gateway-key", requestJSON)
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if strings.Join(got, ",") != "Bearer key-a,Bearer key-b" {
		t.Fatal(got)
	}
	for id, want := range map[int]int{1: 1, 2: 1} {
		var n int
		database.DB().QueryRow("SELECT usage_count FROM provider_api_keys WHERE id=?", id).Scan(&n)
		if n != want {
			t.Errorf("key %d usage=%d want %d", id, n, want)
		}
	}
}

// ---------- 统计名称 ----------

func TestStreamAndNonStreamRecordSameModelName(t *testing.T) {
	r := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		json.NewDecoder(r.Body).Decode(&payload)
		if stream, _ := payload["stream"].(bool); stream {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}]}\n\n")
			fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\ndata: [DONE]\n\n")
			return
		}
		fmt.Fprint(w, completionJSON)
	})
	// 用原始 ID "real" 请求，统计里应记为显示名称 "p/real"
	byOriginal := strings.ReplaceAll(requestJSON, "p/real", "real")
	for _, body := range []string{byOriginal, strings.TrimSuffix(byOriginal, "}") + `,"stream":true}`} {
		if rec := sendRequest(r, "/v1/chat/completions", "gateway-key", body); rec.Code != 200 {
			t.Fatal(rec.Body.String())
		}
	}
	rows, _ := database.DB().Query("SELECT DISTINCT model_name FROM token_usage")
	var names []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		names = append(names, n)
	}
	rows.Close()
	if strings.Join(names, ",") != "p/real" {
		t.Fatal(names)
	}
}

// ---------- WebSocket 子协议认证 ----------

func TestWebSocketBearerSubprotocol(t *testing.T) {
	r := setupGateway(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, completionJSON) })
	setting(t, "stream_mode", "force_non_stream")
	srv := httptest.NewServer(r)
	defer srv.Close()
	dialer := websocket.Dialer{Subprotocols: []string{"bearer", "gateway-key"}}
	conn, resp, err := dialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/chat/completions/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if resp.Header.Get("Sec-WebSocket-Protocol") != "bearer" {
		t.Fatalf("subprotocol not negotiated: %q", resp.Header.Get("Sec-WebSocket-Protocol"))
	}
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err := conn.WriteMessage(websocket.TextMessage, []byte(requestJSON)); err != nil {
		t.Fatal(err)
	}
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "error") {
			t.Fatal(string(data))
		}
		if strings.Contains(string(data), "[DONE]") {
			return
		}
	}
}

// ---------- 统计周期 ----------

func TestStatsPeriodBoundaries(t *testing.T) {
	origLoc, origHour := statsLoc, statsResetHour
	defer func() { statsLoc, statsResetHour = origLoc, origHour }()

	statsLoc, _ = time.LoadLocation("America/Los_Angeles")
	statsResetHour = 0
	// 夏令时期间（PDT, UTC-7）：太平洋时间零点 = UTC 07:00
	summer := time.Date(2026, 7, 1, 6, 59, 0, 0, time.UTC)
	if got := periodStartAt(summer).UTC(); !got.Equal(time.Date(2026, 6, 30, 7, 0, 0, 0, time.UTC)) {
		t.Fatal("summer", got)
	}
	// 冬令时（PST, UTC-8）：太平洋时间零点 = UTC 08:00
	winter := time.Date(2026, 12, 1, 8, 0, 0, 0, time.UTC)
	if got := periodStartAt(winter).UTC(); !got.Equal(winter) {
		t.Fatal("winter", got)
	}
}
