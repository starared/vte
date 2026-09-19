package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"vte/internal/database"
	"vte/internal/models"
	"vte/internal/proxy"
)

func apiError(c *gin.Context, status int, code, message string) {
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.JSON(status, gin.H{"error": gin.H{"message": message, "type": code, "code": code}, "detail": message})
}
func writeUpstreamError(c *gin.Context, err error) {
	if errors.Is(err, context.Canceled) {
		apiError(c, 499, "request_cancelled", "请求已取消")
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		apiError(c, 504, "upstream_timeout", "上游请求超时")
		return
	}
	var upstream *proxy.UpstreamError
	if errors.As(err, &upstream) {
		if upstream.RetryAfter != "" {
			c.Header("Retry-After", upstream.RetryAfter)
		}
		var body map[string]interface{}
		if json.Unmarshal(upstream.Body, &body) == nil {
			if e, ok := body["error"].(map[string]interface{}); ok {
				if _, ok := e["message"].(string); ok {
					c.JSON(upstream.Status, body)
					return
				}
			}
		}
		apiError(c, upstream.Status, "upstream_error", fmt.Sprintf("上游返回 HTTP %d", upstream.Status))
		return
	}
	apiError(c, 502, "upstream_error", err.Error())
}
func validateChatPayload(payload map[string]interface{}) error {
	if payload == nil {
		return fmt.Errorf("请求必须为 JSON 对象")
	}
	messages, ok := payload["messages"].([]interface{})
	if !ok || len(messages) == 0 {
		return fmt.Errorf("messages 必须是非空数组")
	}
	for _, item := range messages {
		m, ok := item.(map[string]interface{})
		if !ok {
			return fmt.Errorf("messages 元素必须为对象")
		}
		if role, ok := m["role"].(string); !ok || role == "" {
			return fmt.Errorf("message.role 必须为非空字符串")
		}
	}
	if v, exists := payload["stream"]; exists {
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("stream 必须为布尔值")
		}
	}
	if v, exists := payload["stream_options"]; exists && v != nil {
		if _, ok := v.(map[string]interface{}); !ok {
			return fmt.Errorf("stream_options 必须为对象")
		}
	}
	return nil
}
func recordKeyAttempt(id int) {
	_, err := database.DB().Exec("UPDATE provider_api_keys SET usage_count=usage_count+1,last_used_at=CURRENT_TIMESTAMP WHERE id=?", id)
	if err != nil { /* Accounting failure must not repeat a paid request. */
	}
}

var tempLimits = struct {
	sync.Mutex
	active map[int]int
	times  map[int][]time.Time
}{active: map[int]int{}, times: map[int][]time.Time{}}

func acquireTempLimits(key *models.TempAPIKey) (func(), error) {
	tempLimits.Lock()
	defer tempLimits.Unlock()
	if key.ConcurrencyLimit > 0 && tempLimits.active[key.ID] >= key.ConcurrencyLimit {
		return nil, fmt.Errorf("当前密钥并发数已达上限")
	}
	if key.RateLimitCount > 0 {
		unit := time.Second
		switch key.RateLimitUnit {
		case "minutes":
			unit = time.Minute
		case "hours":
			unit = time.Hour
		}
		now := time.Now()
		cutoff := now.Add(-time.Duration(key.RateLimitWindow) * unit)
		kept := tempLimits.times[key.ID][:0]
		for _, t := range tempLimits.times[key.ID] {
			if t.After(cutoff) {
				kept = append(kept, t)
			}
		}
		tempLimits.times[key.ID] = kept
		if len(kept) >= key.RateLimitCount {
			return nil, fmt.Errorf("当前密钥请求过于频繁")
		}
		tempLimits.times[key.ID] = append(kept, now)
	}
	tempLimits.active[key.ID]++
	var once sync.Once
	return func() {
		once.Do(func() {
			tempLimits.Lock()
			defer tempLimits.Unlock()
			tempLimits.active[key.ID]--
			if tempLimits.active[key.ID] == 0 {
				delete(tempLimits.active, key.ID)
			}
		})
	}, nil
}

type wsResponseWriter struct {
	conn   *websocket.Conn
	header http.Header
	ctx    context.Context
	cancel context.CancelFunc
	buffer string
}

func (w *wsResponseWriter) Header() http.Header    { return w.header }
func (w *wsResponseWriter) WriteHeader(status int) {}
func (w *wsResponseWriter) Flush()                 {}
func (w *wsResponseWriter) CloseNotify() <-chan bool {
	ch := make(chan bool, 1)
	go func() { <-w.ctx.Done(); ch <- true }()
	return ch
}
func (w *wsResponseWriter) Write(p []byte) (int, error) {
	send := func(data []byte) error {
		w.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
		err := w.conn.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			w.cancel()
		}
		return err
	}
	if !strings.HasPrefix(w.header.Get("Content-Type"), "text/event-stream") {
		if err := send(p); err != nil {
			return 0, err
		}
		return len(p), nil
	}
	w.buffer += string(p)
	for {
		i := strings.IndexByte(w.buffer, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimSpace(w.buffer[:i])
		w.buffer = w.buffer[i+1:]
		if line != "" {
			if err := send([]byte(line)); err != nil {
				return 0, err
			}
		}
	}
	return len(p), nil
}
