package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCancellationStopsUpstreamAndRetries(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			started := make(chan struct{})
			stopped := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				close(started)
				<-r.Context().Done()
				close(stopped)
			}))
			defer server.Close()
			cfg := ProviderConfig{BaseURL: server.URL}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				if stream {
					_, err := cfg.ChatCompletionStreamWithRetry(ctx, map[string]interface{}{}, 3)
					result <- err
				} else {
					_, err := cfg.ChatCompletionWithRetry(ctx, map[string]interface{}{}, 3)
					result <- err
				}
			}()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("not started")
			}
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("request not cancelled")
			}
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("upstream not cancelled")
			}
		})
	}
}
func TestBackoffCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := retryWait(ctx, 10); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestToolCallsSurviveStreamAggregation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []string{
			`{"id":"c1","model":"real","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call1","type":"function","function":{"name":"weather","arguments":"{\"city\":"}}]}}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Paris\"}"}}]},"finish_reason":"tool_calls"}]}`,
			`{"choices":[],"usage":{"total_tokens":12}}`, "[DONE]"}
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data:%s\r\n\r\n", chunk)
		}
	}))
	defer server.Close()
	cfg := ProviderConfig{BaseURL: server.URL}
	result, err := cfg.CompletionForClient(context.Background(), map[string]interface{}{"stream": true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	choices := result["choices"].([]interface{})
	message := choices[0].(map[string]interface{})["message"].(map[string]interface{})
	tool := message["tool_calls"].([]interface{})[0].(map[string]interface{})
	fn := tool["function"].(map[string]interface{})
	if fn["arguments"] != `{"city":"Paris"}` || fn["name"] != "weather" || tool["id"] != "call1" {
		t.Fatal(tool)
	}
	if _, ok := tool["index"]; ok {
		t.Fatal("stream-only index leaked into complete tool call")
	}
}
func TestSSEEventsAndErrors(t *testing.T) {
	var events []string
	err := ReadEvents(strings.NewReader(": heartbeat\r\ndata: {\n"+"data: \"ok\":true}\n\ndata:[DONE]\n\n"), func(event string) error { events = append(events, event); return nil })
	if err != nil || len(events) != 2 || events[1] != "[DONE]" {
		t.Fatal(events, err)
	}
	var v map[string]interface{}
	if json.Unmarshal([]byte(events[0]), &v) != nil {
		t.Fatal(events[0])
	}
}
func TestTruncatedStreamIsNotSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "data: {\"choices\":[]}\n\n") }))
	defer server.Close()
	cfg := ProviderConfig{BaseURL: server.URL}
	if _, err := cfg.CompletionForClient(context.Background(), map[string]interface{}{"stream": true}, 0); err == nil {
		t.Fatal("truncated response accepted")
	}
}

func withHeaderTimeout(t *testing.T, seconds string) {
	t.Helper()
	t.Setenv("UPSTREAM_TIMEOUT_SECONDS", seconds)
	InvalidateClient("")
	t.Cleanup(func() { InvalidateClient("") })
}

// 请求已经发到上游后超时，不能重试（否则会重复调用、重复计费）
func TestTimeoutAfterRequestSentIsNotRetried(t *testing.T) {
	withHeaderTimeout(t, "1")
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		select {
		case <-time.After(3 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	cfg := ProviderConfig{BaseURL: server.URL}
	_, err := cfg.ChatCompletionWithRetry(context.Background(), map[string]interface{}{}, 3)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected timeout, got %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("expected 1 upstream call, got %d", n)
	}
}

// 超时只限制等待响应头的时间，响应头之后的长时间流式输出不能被切断
func TestLongStreamIsNotCutByTimeout(t *testing.T) {
	withHeaderTimeout(t, "1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		for i := 0; i < 4; i++ {
			fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"%d\"}}]}\n\n", i)
			w.(http.Flusher).Flush()
			time.Sleep(500 * time.Millisecond)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	cfg := ProviderConfig{BaseURL: server.URL}
	resp, err := cfg.ChatCompletionStreamWithRetry(context.Background(), map[string]interface{}{"stream": true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var events []string
	if err := ReadEvents(resp.Body, func(e string) error { events = append(events, e); return nil }); err != nil {
		t.Fatalf("stream cut after %d events: %v", len(events), err)
	}
	if len(events) != 5 || events[4] != "[DONE]" {
		t.Fatal(events)
	}
}

// 上游返回 429/401/403 时换密钥重试，且不占用普通重试次数
func TestRotateKeyOnRejectedKey(t *testing.T) {
	for _, status := range []int{401, 403, 429} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var seen []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = append(seen, r.Header.Get("Authorization"))
				if r.Header.Get("Authorization") != "Bearer good" {
					w.WriteHeader(status)
					fmt.Fprint(w, `{"error":{"message":"bad key"}}`)
					return
				}
				fmt.Fprint(w, `{"choices":[]}`)
			}))
			defer server.Close()
			keys := []string{"bad-2", "good"}
			cfg := &ProviderConfig{BaseURL: server.URL, APIKey: "bad-1"}
			cfg.RotateKey = func() bool {
				if len(keys) == 0 {
					return false
				}
				cfg.APIKey, keys = keys[0], keys[1:]
				return true
			}
			if _, err := cfg.ChatCompletionWithRetry(context.Background(), map[string]interface{}{}, 0); err != nil {
				t.Fatal(err)
			}
			if strings.Join(seen, ",") != "Bearer bad-1,Bearer bad-2,Bearer good" {
				t.Fatal(seen)
			}
		})
	}
}

// 所有密钥都被拒绝时，返回最后一个上游错误
func TestRotateKeyGivesUpWhenExhausted(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(429)
	}))
	defer server.Close()
	cfg := &ProviderConfig{BaseURL: server.URL, APIKey: "a"}
	rotated := false
	cfg.RotateKey = func() bool {
		if rotated {
			return false
		}
		rotated = true
		cfg.APIKey = "b"
		return true
	}
	_, err := cfg.ChatCompletionWithRetry(context.Background(), map[string]interface{}{}, 0)
	var upstream *UpstreamError
	if !errors.As(err, &upstream) || upstream.Status != 429 {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

// 连接失败（请求还没发出）可以安全重试
func TestConnectFailureIsRetried(t *testing.T) {
	var attempts int32
	cfg := ProviderConfig{BaseURL: "http://127.0.0.1:1", BeforeAttempt: func() { atomic.AddInt32(&attempts, 1) }}
	_, err := cfg.ChatCompletionWithRetry(context.Background(), map[string]interface{}{}, 2)
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

// 上游发了响应头之后长时间没有任何数据，应该断开而不是永久挂起
func TestStalledStreamTimesOut(t *testing.T) {
	withHeaderTimeout(t, "1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-time.After(5 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	cfg := ProviderConfig{BaseURL: server.URL}
	resp, err := cfg.ChatCompletionStreamWithRetry(context.Background(), map[string]interface{}{"stream": true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	start := time.Now()
	err = ReadEvents(resp.Body, func(string) error { return nil })
	if !errors.Is(err, ErrUpstreamIdle) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected idle timeout, got %v", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("took %v", d)
	}
}
