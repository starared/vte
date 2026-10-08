package tokenizer

import (
	"net/http"
	"testing"
)

// 词表必须来自内置资源，不能在运行时联网下载
func TestCountTokensWorksOffline(t *testing.T) {
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected network request: %s", r.URL)
		return nil, nil
	})
	defer func() { http.DefaultTransport = orig }()

	enc, err := getEncoder("cl100k_base")
	if err != nil {
		t.Fatalf("encoder unavailable offline: %v", err)
	}
	if n := len(enc.Encode("hello world", nil, nil)); n != 2 {
		t.Fatalf("expected 2 tokens, got %d", n)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestEncodingSelection(t *testing.T) {
	cases := map[string]string{
		"gpt-4":                  "cl100k_base",
		"gpt-4-turbo":            "cl100k_base",
		"gpt-3.5-turbo":          "cl100k_base",
		"claude-sonnet-4":        "cl100k_base",
		"gpt-4o":                 "o200k_base",
		"openai/gpt-4o-mini":     "o200k_base",
		"GPT-4.1":                "o200k_base",
		"o3-mini":                "o200k_base",
		"gpt-5":                  "o200k_base",
		"deepseek/deepseek-chat": "cl100k_base",
	}
	for model, want := range cases {
		if got := getEncodingForModel(model); got != want {
			t.Errorf("%s: got %s want %s", model, got, want)
		}
	}
	if n := CountTokens("hello world", "gpt-4o"); n <= 0 {
		t.Fatal("o200k encoder failed", n)
	}
}
