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
