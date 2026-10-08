package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"vte/internal/database"
)

// 上游流式响应正常结束但没有发送 [DONE]（已给出 finish_reason）时，应视为完整响应
func TestStreamWithoutDoneIsComplete(t *testing.T) {
	upstream := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"model\":\"real\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hello\"},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\n")
	}

	t.Run("stream client", func(t *testing.T) {
		r := setupGateway(t, upstream)
		rec := sendRequest(r, "/v1/chat/completions", "gateway-key", strings.TrimSuffix(requestJSON, "}")+`,"stream":true}`)
		body := rec.Body.String()
		if rec.Code != 200 || strings.Contains(body, "upstream_stream_error") || !strings.HasSuffix(body, "data: [DONE]\n\n") {
			t.Fatal(rec.Code, body)
		}
		var n int
		database.DB().QueryRow("SELECT COUNT(*) FROM token_usage").Scan(&n)
		if n != 1 {
			t.Fatalf("usage rows=%d, want 1", n)
		}
	})

	t.Run("non-stream client via force_stream", func(t *testing.T) {
		r := setupGateway(t, upstream)
		setting(t, "stream_mode", "force_stream")
		rec := sendRequest(r, "/v1/chat/completions", "gateway-key", requestJSON)
		if rec.Code != 200 {
			t.Fatal(rec.Code, rec.Body.String())
		}
		var result struct {
			Choices []struct {
				Message      map[string]string `json:"message"`
				FinishReason string            `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err, rec.Body.String())
		}
		if len(result.Choices) != 1 || result.Choices[0].Message["content"] != "hello" || result.Choices[0].FinishReason != "stop" {
			t.Fatal(rec.Body.String())
		}
	})

	t.Run("no finish_reason is still an interruption", func(t *testing.T) {
		r := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"id\":\"c1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hel\"},\"finish_reason\":null}]}\n\n")
		})
		rec := sendRequest(r, "/v1/chat/completions", "gateway-key", strings.TrimSuffix(requestJSON, "}")+`,"stream":true}`)
		if !strings.Contains(rec.Body.String(), "upstream_stream_error") {
			t.Fatal(rec.Body.String())
		}
	})
}

// 上游在流中返回 error 对象时，错误信息应传递给客户端而不是被固定文案吞掉
func TestUpstreamStreamErrorMessagePreserved(t *testing.T) {
	upstream := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"error\":{\"message\":\"quota exhausted for today\",\"type\":\"insufficient_quota\"}}\n\n")
	}
	r := setupGateway(t, upstream)
	setting(t, "stream_mode", "force_stream")
	rec := sendRequest(r, "/v1/chat/completions", "gateway-key", requestJSON)
	if rec.Code != 502 || !strings.Contains(rec.Body.String(), "quota exhausted for today") {
		t.Fatal(rec.Code, rec.Body.String())
	}
}

// 调整自定义限流规则的顺序不应清零计数
func TestCustomRateLimitSurvivesReorder(t *testing.T) {
	customRequestTimes = map[string][]time.Time{}
	a := CustomRateLimitRule{ID: 1, Name: "a", MaxRequests: 1, Window: 60, Enabled: true}
	b := CustomRateLimitRule{ID: 2, Name: "b", ModelName: "other", MaxRequests: 100, Window: 60, Enabled: true}
	if ok, _ := checkCustomRateLimit([]CustomRateLimitRule{a, b}, 1, "m"); !ok {
		t.Fatal("first request rejected")
	}
	if ok, name := checkCustomRateLimit([]CustomRateLimitRule{b, a}, 1, "m"); ok || name != "a" {
		t.Fatalf("limit lost after reorder: ok=%v rule=%q", ok, name)
	}
	// 规则被删除后，内存里的计数也应清理
	checkCustomRateLimit([]CustomRateLimitRule{b}, 1, "m")
	if _, ok := customRequestTimes["rule:1:60"]; ok {
		t.Fatal("stale counter kept for removed rule")
	}
}
