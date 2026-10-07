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
