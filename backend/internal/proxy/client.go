package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
		Timeout:   300 * time.Second,
	}

	clientPool[proxyURL] = client
	return client
}

func InvalidateClient(proxyURL string) {
	poolMu.Lock()
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
func (cfg *ProviderConfig) ListModels() ([]map[string]interface{}, error) {
	modelsURL := cfg.getModelsURL()
	if modelsURL == "" {
		return nil, nil
	}

	client := getClient(cfg.ProxyURL)

	req, err := http.NewRequest("GET", modelsURL, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range cfg.getHeaders() {
		req.Header.Set(k, v)
	}

	if params := cfg.getQueryParams(); len(params) > 0 {
		req.URL.RawQuery = params.Encode()
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Data, nil
}

// ChatCompletion 非流式请求（带重试）
func (cfg *ProviderConfig) ChatCompletion(payload map[string]interface{}) (map[string]interface{}, error) {
	return cfg.ChatCompletionWithRetry(payload, 3) // 默认3次重试
}

// ChatCompletionWithRetry 带重试的非流式请求
func (cfg *ProviderConfig) ChatCompletionWithRetry(payload map[string]interface{}, maxRetries int) (map[string]interface{}, error) {
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
			time.Sleep(time.Duration(100*(1<<(attempt-1))) * time.Millisecond)
		}

		req, err := http.NewRequest("POST", chatURL, strings.NewReader(string(body)))
		if err != nil {
			return nil, err
		}

		for k, v := range cfg.getHeaders() {
			req.Header.Set(k, v)
		}

		if params := cfg.getQueryParams(); len(params) > 0 {
			req.URL.RawQuery = params.Encode()
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue // 网络错误，重试
		}

		if resp.StatusCode == 200 {
			defer resp.Body.Close()
			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				resp.Body.Close()
				return nil, err
			}
			return result, nil
		}

		// 5xx 错误重试，4xx 不重试
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		lastErr = fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))

		if resp.StatusCode < 500 {
			return nil, lastErr // 4xx 错误不重试
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %v", lastErr)
}

// ChatCompletionStream 流式请求（带重试）
func (cfg *ProviderConfig) ChatCompletionStream(payload map[string]interface{}) (*http.Response, error) {
	return cfg.ChatCompletionStreamWithRetry(payload, 3) // 默认3次重试
}

// ChatCompletionStreamWithRetry 带重试的流式请求
func (cfg *ProviderConfig) ChatCompletionStreamWithRetry(payload map[string]interface{}, maxRetries int) (*http.Response, error) {
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
			time.Sleep(time.Duration(100*(1<<(attempt-1))) * time.Millisecond)
		}

		req, err := http.NewRequest("POST", chatURL, strings.NewReader(string(body)))
		if err != nil {
			return nil, err
		}

		for k, v := range cfg.getHeaders() {
			req.Header.Set(k, v)
		}

		if params := cfg.getQueryParams(); len(params) > 0 {
			req.URL.RawQuery = params.Encode()
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue // 网络错误，重试
		}

		if resp.StatusCode == 200 {
			return resp, nil
		}

		// 5xx 错误重试，4xx 不重试
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		lastErr = fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))

		if resp.StatusCode < 500 {
			return nil, lastErr
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %v", lastErr)
}
