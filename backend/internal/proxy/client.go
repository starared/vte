package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	clientPool = make(map[string]*http.Client)
	poolMu     sync.RWMutex
)

type ProviderConfig struct {
	BaseURL        string
	APIKey         string
	ProviderType   string
	VertexProject  string
	VertexLocation string
	ExtraHeaders   map[string]string
	ProxyURL       string
	BeforeAttempt  func()
}

func getClient(proxyURL string) *http.Client {
	poolMu.RLock()
	if client, ok := clientPool[proxyURL]; ok {
		poolMu.RUnlock()
		return client
	}
	poolMu.RUnlock()

	poolMu.Lock()
	defer poolMu.Unlock()

	// Double check
	if client, ok := clientPool[proxyURL]; ok {
		return client
	}

	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	}

	if proxyURL != "" {
		if proxyU, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxyU)
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   upstreamTimeout(),
	}

	clientPool[proxyURL] = client
	return client
}

func InvalidateClient(proxyURL string) {
	poolMu.Lock()
	if client := clientPool[proxyURL]; client != nil {
		client.CloseIdleConnections()
	}
	delete(clientPool, proxyURL)
	poolMu.Unlock()
}

func (cfg *ProviderConfig) getChatURL() string {
	if cfg.ProviderType == "vertex_express" {
		location := cfg.VertexLocation
		if location == "" {
			location = "global"
		}
		return fmt.Sprintf(
			"https://aiplatform.googleapis.com/v1beta1/projects/%s/locations/%s/endpoints/openapi/chat/completions",
			cfg.VertexProject, location,
		)
	}
	return strings.TrimSuffix(cfg.BaseURL, "/") + "/chat/completions"
}

func (cfg *ProviderConfig) getModelsURL() string {
	if cfg.ProviderType == "vertex_express" {
		return ""
	}
	return strings.TrimSuffix(cfg.BaseURL, "/") + "/models"
}

func (cfg *ProviderConfig) getHeaders() map[string]string {
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	if cfg.ProviderType != "vertex_express" {
		headers["Authorization"] = "Bearer " + cfg.APIKey
	}

	for k, v := range cfg.ExtraHeaders {
		headers[k] = v
	}

	return headers
}

func (cfg *ProviderConfig) getQueryParams() url.Values {
	params := url.Values{}
	if cfg.ProviderType == "vertex_express" {
		params.Set("key", cfg.APIKey)
	}
	return params
}

// ListModels 获取模型列表
func (cfg *ProviderConfig) ListModels(ctx context.Context) ([]map[string]interface{}, error) {
	modelsURL := cfg.getModelsURL()
	if modelsURL == "" {
		return nil, fmt.Errorf("this provider does not support model discovery")
	}

	client := getClient(cfg.ProxyURL)

	req, err := http.NewRequestWithContext(ctx, "GET", modelsURL, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range cfg.getHeaders() {
		req.Header.Set(k, v)
	}

	if params := cfg.getQueryParams(); len(params) > 0 {
		req.URL.RawQuery = params.Encode()
	}

	if cfg.BeforeAttempt != nil {
		cfg.BeforeAttempt()
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data *[]map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Data == nil {
		return nil, fmt.Errorf("invalid model list: data must be an array")
	}
	for _, model := range *result.Data {
		if id, ok := model["id"].(string); !ok || strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("invalid model list: missing model id")
		}
	}
	return *result.Data, nil
}

// ChatCompletion 非流式请求（带重试）
func (cfg *ProviderConfig) ChatCompletion(payload map[string]interface{}) (map[string]interface{}, error) {
	return cfg.ChatCompletionWithRetry(context.Background(), payload, 3) // 默认3次重试
}

// ChatCompletionWithRetry 带重试的非流式请求
func (cfg *ProviderConfig) ChatCompletionWithRetry(ctx context.Context, payload map[string]interface{}, maxRetries int) (map[string]interface{}, error) {
	client := getClient(cfg.ProxyURL)

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	chatURL := cfg.getChatURL()
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// 指数退避：100ms, 200ms, 400ms...
			if err := retryWait(ctx, attempt); err != nil {
				return nil, err
			}
		}

		req, err := http.NewRequestWithContext(ctx, "POST", chatURL, strings.NewReader(string(body)))
		if err != nil {
			return nil, err
		}

		for k, v := range cfg.getHeaders() {
			req.Header.Set(k, v)
		}

		if params := cfg.getQueryParams(); len(params) > 0 {
			req.URL.RawQuery = params.Encode()
		}

		if cfg.BeforeAttempt != nil {
			cfg.BeforeAttempt()
		}
		resp, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if errors.Is(err, context.DeadlineExceeded) {
				lastErr = context.DeadlineExceeded
			} else {
				lastErr = errors.New("upstream network request failed")
			}
			continue // 网络错误，重试
		}

		if resp.StatusCode == 200 {
			defer resp.Body.Close()
			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				resp.Body.Close()
				return nil, err
			}
			if result == nil {
				return nil, fmt.Errorf("invalid upstream response: expected object")
			}
			return result, nil
		}

		// 5xx 错误重试，4xx 不重试
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		lastErr = &UpstreamError{Status: resp.StatusCode, Body: respBody, RetryAfter: resp.Header.Get("Retry-After")}

		if resp.StatusCode < 500 {
			return nil, lastErr // 4xx 错误不重试
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// ChatCompletionStream 流式请求（带重试）
func (cfg *ProviderConfig) ChatCompletionStream(payload map[string]interface{}) (*http.Response, error) {
	return cfg.ChatCompletionStreamWithRetry(context.Background(), payload, 3) // 默认3次重试
}

// ChatCompletionStreamWithRetry 带重试的流式请求
func (cfg *ProviderConfig) ChatCompletionStreamWithRetry(ctx context.Context, payload map[string]interface{}, maxRetries int) (*http.Response, error) {
	client := getClient(cfg.ProxyURL)

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	chatURL := cfg.getChatURL()
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// 指数退避
			if err := retryWait(ctx, attempt); err != nil {
				return nil, err
			}
		}

		req, err := http.NewRequestWithContext(ctx, "POST", chatURL, strings.NewReader(string(body)))
		if err != nil {
			return nil, err
		}

		for k, v := range cfg.getHeaders() {
			req.Header.Set(k, v)
		}

		if params := cfg.getQueryParams(); len(params) > 0 {
			req.URL.RawQuery = params.Encode()
		}

		if cfg.BeforeAttempt != nil {
			cfg.BeforeAttempt()
		}
		resp, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if errors.Is(err, context.DeadlineExceeded) {
				lastErr = context.DeadlineExceeded
			} else {
				lastErr = errors.New("upstream network request failed")
			}
			continue // 网络错误，重试
		}

		if resp.StatusCode == 200 {
			return resp, nil
		}

		// 5xx 错误重试，4xx 不重试
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		lastErr = &UpstreamError{Status: resp.StatusCode, Body: respBody, RetryAfter: resp.Header.Get("Retry-After")}

		if resp.StatusCode < 500 {
			return nil, lastErr
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// UpstreamError preserves the HTTP contract without exposing request credentials.
type UpstreamError struct {
	Status     int
	Body       []byte
	RetryAfter string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("upstream status %d: %s", e.Status, e.Body)
}
func retryWait(ctx context.Context, attempt int) error {
	timer := time.NewTimer(time.Duration(100*(1<<uint(attempt-1))) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func upstreamTimeout() time.Duration {
	seconds, err := strconv.Atoi(os.Getenv("UPSTREAM_TIMEOUT_SECONDS"))
	if err != nil || seconds <= 0 || seconds > 86400 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}
