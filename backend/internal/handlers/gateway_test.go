package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"vte/internal/auth"
	"vte/internal/database"
	"vte/internal/models"
)

func setupGateway(t *testing.T, up http.HandlerFunc) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	if err := database.Init(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	requestTimes = nil
	customRequestTimes = map[string][]time.Time{}
	keyIndexMap = map[int]int{}
	atomic.StoreInt64(&currentConcurrency, 0)
	tempLimits.active = map[int]int{}
	tempLimits.times = map[int][]time.Time{}
	srv := httptest.NewServer(up)
	t.Cleanup(srv.Close)
	db := database.DB()
	statements := []struct {
		query string
		args  []interface{}
	}{
		{"INSERT INTO users(username,hashed_password,api_key,is_admin,is_active)VALUES('admin','unused','gateway-key',1,1)", nil},
		{"INSERT INTO providers(id,name,base_url,api_key,model_prefix)VALUES(1,'p',?,'','p')", []interface{}{srv.URL}},
		{"INSERT INTO models(provider_id,original_id,display_name,is_active)VALUES(1,'real','p/real',1)", nil},
		{"INSERT INTO provider_api_keys(id,provider_id,api_key,name)VALUES(1,1,'key-a','a'),(2,1,'key-b','b')", nil},
		{"INSERT INTO settings(key,value)VALUES('max_retries','0')", nil},
	}
	for _, st := range statements {
		if _, err := db.Exec(st.query, st.args...); err != nil {
			t.Fatal(err)
		}
	}
	r := gin.New()
	r.Use(gin.Recovery())
	r.POST("/v1/chat/completions", auth.APIKeyAuth(), OpenAIChatCompletions)
	r.GET("/v1/models", auth.APIKeyAuth(), OpenAIListModels)
	r.GET("/v1/chat/completions/ws", OpenAIChatCompletionsWS)
	r.POST("/test/:id", TestConnection)
	r.POST("/fetch/:id", FetchModels)
	r.GET("/providers", ListProviders)
	return r
}

const requestJSON = `{"model":"p/real","messages":[{"role":"user","content":"hello"}]}`
const completionJSON = `{"id":"c1","object":"chat.completion","model":"real","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`

func sendRequest(r http.Handler, path, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}
func setting(t *testing.T, key, value string) {
	t.Helper()
	_, err := database.DB().Exec("INSERT INTO settings(key,value)VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value)
	if err != nil {
		t.Fatal(err)
	}
}
func TestRotationCountsActualRequests(t *testing.T) {
	var got []string
	r := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("Authorization"))
		fmt.Fprint(w, completionJSON)
	})
	for i := 0; i < 4; i++ {
		rec := sendRequest(r, "/v1/chat/completions", "gateway-key", requestJSON)
		if rec.Code != 200 {
			t.Fatal(rec.Body.String())
		}
	}
	if strings.Join(got, ",") != "Bearer key-a,Bearer key-b,Bearer key-a,Bearer key-b" {
		t.Fatal(got)
	}
	for _, id := range []int{1, 2} {
		var count int
		database.DB().QueryRow("SELECT usage_count FROM provider_api_keys WHERE id=?", id).Scan(&count)
		if count != 2 {
			t.Fatalf("key %d usage=%d", id, count)
		}
	}
}
func TestExplicitPrefixNeverFallsBack(t *testing.T) {
	r := setupGateway(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected upstream request") })
	rec := sendRequest(r, "/v1/chat/completions", "gateway-key", strings.ReplaceAll(requestJSON, "p/real", "disabled/real"))
	if rec.Code != 404 {
		t.Fatalf("status %d", rec.Code)
	}
}
func TestStreamModesPreserveClientFormat(t *testing.T) {
	for _, mode := range []string{"auto", "force_stream", "force_non_stream"} {
		for _, clientStream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", mode, clientStream), func(t *testing.T) {
				r := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
					var payload map[string]interface{}
					json.NewDecoder(r.Body).Decode(&payload)
					stream, _ := payload["stream"].(bool)
					if mode == "force_stream" && !stream || mode == "force_non_stream" && stream {
						t.Error("wrong upstream stream")
					}
					if !stream {
						if _, ok := payload["stream_options"]; ok {
							t.Error("stream_options sent for non-stream request")
						}
						fmt.Fprint(w, completionJSON)
						return
					}
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"id\":\"c1\",\"model\":\"real\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hello\"},\"finish_reason\":\"stop\"}]}\n\n")
					fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\ndata: [DONE]\n\n")
				})
				setting(t, "stream_mode", mode)
				body := strings.TrimSuffix(requestJSON, "}") + fmt.Sprintf(",\"stream\":%v,\"stream_options\":{\"include_usage\":true}}", clientStream)
				rec := sendRequest(r, "/v1/chat/completions", "gateway-key", body)
				if rec.Code != 200 {
					t.Fatal(rec.Code, rec.Body.String())
				}
				if clientStream {
					if !strings.Contains(rec.Header().Get("Content-Type"), "text/event-stream") || !strings.Contains(rec.Body.String(), "[DONE]") {
						t.Fatal(rec.Body.String())
					}
				} else {
					var result map[string]interface{}
					if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
						t.Fatal(err, rec.Body.String())
					}
					if result["model"] != "p/real" {
						t.Fatal(result)
					}
				}
			})
		}
	}
}
func TestUpstreamErrorsPreserved(t *testing.T) {
	r := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "23")
		w.WriteHeader(429)
		fmt.Fprint(w, `{"error":{"message":"slow down","type":"rate_limit_error"}}`)
	})
	rec := sendRequest(r, "/v1/chat/completions", "gateway-key", requestJSON)
	if rec.Code != 429 || rec.Header().Get("Retry-After") != "23" || !strings.Contains(rec.Body.String(), "slow down") {
		t.Fatal(rec.Code, rec.Body.String())
	}
}
func TestDefaultConnectionKey(t *testing.T) {
	r := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key-a" {
			t.Error(r.Header.Get("Authorization"))
		}
		fmt.Fprint(w, completionJSON)
	})
	rec := sendRequest(r, "/test/1", "gateway-key", `{}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"success":true`) {
		t.Fatal(rec.Code, rec.Body.String())
	}
}
func TestMalformedModelListsPreserveModels(t *testing.T) {
	for _, body := range []string{`{}`, `{"data":null}`, `{"data":[]}`, `{"data":[{}]}`} {
		t.Run(body, func(t *testing.T) {
			r := setupGateway(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
			rec := sendRequest(r, "/fetch/1", "gateway-key", `{}`)
			if rec.Code == 200 {
				t.Fatal("invalid list accepted")
			}
			var n int
			database.DB().QueryRow("SELECT COUNT(*) FROM models").Scan(&n)
			if n != 1 {
				t.Fatal("models deleted")
			}
		})
	}
}
func makeTemp(t *testing.T, max, rate, concurrent int) *models.TempAPIKey {
	t.Helper()
	key, err := database.CreateTempAPIKey(models.TempAPIKeyCreateRequest{Name: "test", AllowedModels: []string{"p/real"}, MaxRequests: max, ModelLimits: map[string]int{"p/real": 1}, RateLimitCount: rate, RateLimitWindow: 1, RateLimitUnit: "minutes", ConcurrencyLimit: concurrent})
	if err != nil {
		t.Fatal(err)
	}
	return key
}
func TestModelQuotaRollbackReleasesDatabase(t *testing.T) {
	setupGateway(t, func(w http.ResponseWriter, r *http.Request) {})
	key := makeTemp(t, 0, 0, 0)
	if err := database.ConsumeTempAPIUsage(key.ID, "p/real"); err != nil {
		t.Fatal(err)
	}
	if err := database.ConsumeTempAPIUsage(key.ID, "p/real"); err != database.ErrTempAPIModelExceeded {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := database.DB().PingContext(ctx); err != nil {
		t.Fatal("quota rejection leaked transaction", err)
	}
}
func TestLocalRejectionDoesNotConsumeQuota(t *testing.T) {
	r := setupGateway(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, completionJSON) })
	setting(t, "custom_rate_limit_rules", `[{"id":1,"provider_id":1,"max_requests":1,"window":60,"enabled":true}]`)
	sendRequest(r, "/v1/chat/completions", "gateway-key", requestJSON)
	key := makeTemp(t, 5, 0, 0)
	rec := sendRequest(r, "/v1/chat/completions", key.Token, requestJSON)
	if rec.Code != 429 {
		t.Fatal(rec.Code)
	}
	stored, _ := database.GetTempAPIKeyByID(key.ID)
	if stored.UsedRequests != 0 {
		t.Fatal("rejected request consumed quota")
	}
}
func TestTempRateAndConcurrency(t *testing.T) {
	setupGateway(t, func(w http.ResponseWriter, r *http.Request) {})
	key := makeTemp(t, 0, 2, 1)
	release, err := acquireTempLimits(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireTempLimits(key); err == nil {
		t.Fatal("concurrency not enforced")
	}
	release()
	release, err = acquireTempLimits(key)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if _, err := acquireTempLimits(key); err == nil {
		t.Fatal("rate not enforced")
	}
}
func TestGlobalConcurrencyIsAtomic(t *testing.T) {
	setupGateway(t, func(w http.ResponseWriter, r *http.Request) {})
	setting(t, "concurrency_enabled", "true")
	setting(t, "concurrency_limit", "1")
	var accepted int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if acquireConcurrency() {
				atomic.AddInt32(&accepted, 1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if accepted != 1 {
		t.Fatal(accepted)
	}
	releaseConcurrency()
}
func TestEmptyModelsArray(t *testing.T) {
	r := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {})
	database.DB().Exec("UPDATE models SET is_active=0")
	req := httptest.NewRequest("GET", "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer gateway-key")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Fatal(rec.Body.String())
	}
}
func TestWebSocketUsesTempAuthAndRateLimits(t *testing.T) {
	r := setupGateway(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, completionJSON) })
	setting(t, "stream_mode", "force_non_stream")
	key := makeTemp(t, 0, 1, 1)
	srv := httptest.NewServer(r)
	defer srv.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/chat/completions/ws?api_key="+key.Token, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err := conn.WriteMessage(websocket.TextMessage, []byte(requestJSON)); err != nil {
		t.Fatal(err)
	}
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "[DONE]") {
			break
		}
	}
	conn.WriteMessage(websocket.TextMessage, []byte(requestJSON))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "error") {
		t.Fatal(string(data))
	}
}

func TestRequestExtensionsArePreserved(t *testing.T) {
	r := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["reasoning_effort"] != "high" || payload["tools"] == nil || payload["response_format"] == nil {
			t.Error(payload)
		}
		fmt.Fprint(w, completionJSON)
	})
	body := strings.TrimSuffix(requestJSON, "}") + `,"reasoning_effort":"high","tools":[{"type":"function","function":{"name":"test"}}],"response_format":{"type":"json_object"}}`
	rec := sendRequest(r, "/v1/chat/completions", "gateway-key", body)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
}
func TestInvalidParametersAndProxyReadback(t *testing.T) {
	r := setupGateway(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid request reached upstream") })
	body := strings.TrimSuffix(requestJSON, "}") + `,"stream":"true"}`
	if rec := sendRequest(r, "/v1/chat/completions", "gateway-key", body); rec.Code != 400 {
		t.Fatal(rec.Code)
	}
	if err := validateTempWindows(5, 0, "seconds", 0, ""); err == nil {
		t.Fatal("zero window accepted")
	}
	if err := validateTempWindows(5, 31536000, "hours", 0, ""); err == nil {
		t.Fatal("overflowing window accepted")
	}
	if err := validateProvider("standard", "https://example.com/v1/chat/completions", "", "", ""); err == nil {
		t.Fatal("full endpoint accepted")
	}
	if err := validateProvider("standard", "https://example.com/v1", "", `{"x":123}`, ""); err == nil {
		t.Fatal("invalid headers accepted")
	}
	database.DB().Exec("UPDATE providers SET proxy_url='http://127.0.0.1:7890'")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/providers", nil))
	if !strings.Contains(rec.Body.String(), `"proxy_url":"http://127.0.0.1:7890"`) {
		t.Fatal(rec.Body.String())
	}
}
