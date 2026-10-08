package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	// RotateKey 在上游返回 401/403/429 时被调用：切换到另一个可用密钥（更新 APIKey）并返回 true；
	// 没有其他可用密钥时返回 false。为 nil 表示不切换。
	RotateKey func() bool
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

	// 注意：不设置 http.Client.Timeout。它会把读取响应体的总时间也算进去，
	// 导致超过时限的长流式输出被强行切断。UPSTREAM_TIMEOUT_SECONDS 现在表示
	// 「上游最长可以多久没有动静」：等待响应头的时间，以及读取响应体时两次数据之间的间隔
	// （见 idleTimeoutBody）。
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: upstreamTimeout(),
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
	}

	if proxyURL != "" {
		if proxyU, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxyU)
		}
	}

	client := &http.Client{Transport: transport}

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

// ChatCompletionWithRetry 带重试的非流式请求
func (cfg *ProviderConfig) ChatCompletionWithRetry(ctx context.Context, payload map[string]interface{}, maxRetries int) (map[string]interface{}, error) {
	resp, err := cfg.doWithRetry(ctx, payload, maxRetries)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("invalid upstream response: expected object")
	}
	return result, nil
}

// ChatCompletionStreamWithRetry 带重试的流式请求
func (cfg *ProviderConfig) ChatCompletionStreamWithRetry(ctx context.Context, payload map[string]interface{}, maxRetries int) (*http.Response, error) {
	return cfg.doWithRetry(ctx, payload, maxRetries)
}

// shouldRotateKey 这些状态码通常与具体密钥有关（失效、无权限、被限流），换一个密钥可能就能成功
func shouldRotateKey(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusTooManyRequests
}

// doWithRetry 发送聊天请求，返回 HTTP 200 的响应（调用方负责关闭 Body）。
//
// 重试策略：
//   - 请求还没发出去（连接/DNS/代理失败）：可以安全重试，计入 maxRetries
//   - 请求已发出但出错或超时：不重试，避免上游重复执行、重复计费
//   - 上游 5xx：重试，计入 maxRetries
//   - 上游 401/403/429：如果配置了 RotateKey 且还有别的密钥，换密钥立即再试，不计入 maxRetries
//   - 其他 4xx：直接返回
func (cfg *ProviderConfig) doWithRetry(ctx context.Context, payload map[string]interface{}, maxRetries int) (*http.Response, error) {
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

		// WroteRequest 在 Transport 的写协程里回调，用原子变量避免数据竞争
		var wrote atomic.Bool
		trace := &httptrace.ClientTrace{
			WroteRequest: func(httptrace.WroteRequestInfo) { wrote.Store(true) },
		}
		attemptCtx, cancelAttempt := context.WithCancel(ctx)
		req, err := http.NewRequestWithContext(httptrace.WithClientTrace(attemptCtx, trace), "POST", chatURL, bytes.NewReader(body))
		if err != nil {
			cancelAttempt()
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
			cancelAttempt()
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if isTimeout(err) {
				lastErr = fmt.Errorf("upstream timeout: %w", context.DeadlineExceeded)
			} else {
				lastErr = errors.New("upstream network request failed")
			}
			if wrote.Load() {
				// 请求已经发到上游，无法确定上游是否已处理，不能重试
				return nil, lastErr
			}
			continue
		}

		if resp.StatusCode == http.StatusOK {
			// 响应体读取期间如果上游长时间没有任何数据，就断开（防止请求永久挂起）
			resp.Body = newIdleTimeoutBody(resp.Body, upstreamTimeout(), cancelAttempt)
			return resp, nil
		}

		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		cancelAttempt()
		lastErr = &UpstreamError{Status: resp.StatusCode, Body: respBody, RetryAfter: resp.Header.Get("Retry-After")}

		if shouldRotateKey(resp.StatusCode) && cfg.RotateKey != nil && cfg.RotateKey() {
			attempt-- // 换密钥重试不占用重试次数；RotateKey 在所有密钥都试过后会返回 false
			continue
		}
		if resp.StatusCode < 500 {
			return nil, lastErr // 4xx 错误不重试
		}
	}

	if lastErr == nil {
		lastErr = errors.New("upstream request failed")
	}
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// ErrUpstreamIdle 上游在超时时间内没有发送任何数据
var ErrUpstreamIdle = fmt.Errorf("upstream idle timeout: %w", context.DeadlineExceeded)

// idleTimeoutBody 每读到数据就重置计时器；超过 timeout 没有新数据则取消请求，
// 之后的 Read 返回 ErrUpstreamIdle。
type idleTimeoutBody struct {
	io.ReadCloser
	timeout  time.Duration
	timer    *time.Timer
	timedOut atomic.Bool
	cancel   context.CancelFunc
}

func newIdleTimeoutBody(body io.ReadCloser, timeout time.Duration, cancel context.CancelFunc) *idleTimeoutBody {
	b := &idleTimeoutBody{ReadCloser: body, timeout: timeout, cancel: cancel}
	b.timer = time.AfterFunc(timeout, func() {
		b.timedOut.Store(true)
		cancel()
	})
	return b
}

func (b *idleTimeoutBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 && !b.timedOut.Load() {
		b.timer.Reset(b.timeout)
	}
	if err != nil && err != io.EOF && b.timedOut.Load() {
		return n, ErrUpstreamIdle
	}
	return n, err
}

func (b *idleTimeoutBody) Close() error {
	b.timer.Stop()
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
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
