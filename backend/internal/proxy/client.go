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

// ChatCompletion 非流式请求
func (cfg *ProviderConfig) ChatCompletion(payload map[string]interface{}) (map[string]interface{}, error) {
	client := getClient(cfg.ProxyURL)

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	chatURL := cfg.getChatURL()
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
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

// ChatCompletionStream 流式请求
func (cfg *ProviderConfig) ChatCompletionStream(payload map[string]interface{}) (*http.Response, error) {
	client := getClient(cfg.ProxyURL)

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	chatURL := cfg.getChatURL()
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

	return client.Do(req)
}
