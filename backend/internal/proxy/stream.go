package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// ReadEvents handles SSE events independently of network packet boundaries.
func ReadEvents(r io.Reader, consume func(string) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 16<<20)
	var data []string
	emit := func() error {
		if len(data) == 0 {
			return nil
		}
		value := strings.Join(data, "\n")
		data = nil
		return consume(value)
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := emit(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return emit()
}

func (cfg *ProviderConfig) CompletionForClient(ctx context.Context, payload map[string]interface{}, retries int) (map[string]interface{}, error) {
	if stream, _ := payload["stream"].(bool); !stream {
		return cfg.ChatCompletionWithRetry(ctx, payload, retries)
	}
	resp, err := cfg.ChatCompletionStreamWithRetry(ctx, payload, retries)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	result := map[string]interface{}{"object": "chat.completion"}
	choices := map[int]map[string]interface{}{}
	done := false
	err = ReadEvents(resp.Body, func(data string) error {
		if data == "[DONE]" {
			done = true
			return io.EOF
		}
		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("invalid upstream SSE JSON: %w", err)
		}
		if chunk == nil {
			return fmt.Errorf("invalid upstream SSE: expected object")
		}
		if _, ok := chunk["error"]; ok {
			return fmt.Errorf("upstream reported an error during streaming")
		}
		for k, v := range chunk {
			if k != "choices" && k != "object" {
				if k != "usage" || v != nil {
					result[k] = v
				}
			}
		}
		items, _ := chunk["choices"].([]interface{})
		for _, item := range items {
			ch, ok := item.(map[string]interface{})
			if !ok {
				return fmt.Errorf("invalid upstream choice")
			}
			index, _ := ch["index"].(float64)
			dst := choices[int(index)]
			if dst == nil {
				dst = map[string]interface{}{"index": index, "message": map[string]interface{}{"role": "assistant"}, "finish_reason": nil}
				choices[int(index)] = dst
			}
			if finish := ch["finish_reason"]; finish != nil {
				dst["finish_reason"] = finish
			}
			if delta, ok := ch["delta"].(map[string]interface{}); ok {
				mergeDelta(dst["message"].(map[string]interface{}), delta)
			}
		}
		return nil
	})
	if err != nil && err != io.EOF {
		return nil, err
	}
	if !done {
		return nil, fmt.Errorf("upstream stream ended without [DONE]")
	}
	indexes := make([]int, 0, len(choices))
	for i := range choices {
		indexes = append(indexes, i)
	}
	sort.Ints(indexes)
	list := make([]interface{}, 0, len(indexes))
	for _, i := range indexes {
		list = append(list, choices[i])
	}
	result["choices"] = list
	return result, nil
}
func mergeDelta(dst, src map[string]interface{}) {
	for key, value := range src {
		if value == nil {
			continue
		}
		switch v := value.(type) {
		case string:
			if key == "role" || key == "id" || key == "type" {
				dst[key] = v
			} else {
				old, _ := dst[key].(string)
				dst[key] = old + v
			}
		case map[string]interface{}:
			old, ok := dst[key].(map[string]interface{})
			if !ok {
				old = map[string]interface{}{}
				dst[key] = old
			}
			mergeDelta(old, v)
		case []interface{}:
			if key != "tool_calls" {
				dst[key] = v
				continue
			}
			old, _ := dst[key].([]interface{})
			for _, item := range v {
				tool, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				idx, _ := tool["index"].(float64)
				if idx < 0 || idx > 1024 {
					continue
				}
				for len(old) <= int(idx) {
					old = append(old, map[string]interface{}{})
				}
				clean := map[string]interface{}{}
				for k, v := range tool {
					if k != "index" {
						clean[k] = v
					}
				}
				mergeDelta(old[int(idx)].(map[string]interface{}), clean)
			}
			dst[key] = old
		default:
			dst[key] = value
		}
	}
}
func (cfg *ProviderConfig) StreamForClient(ctx context.Context, payload map[string]interface{}, retries int) (*http.Response, error) {
	if stream, _ := payload["stream"].(bool); stream {
		return cfg.ChatCompletionStreamWithRetry(ctx, payload, retries)
	}
	result, err := cfg.ChatCompletionWithRetry(ctx, payload, retries)
	if err != nil {
		return nil, err
	}
	var out strings.Builder
	emit := func(chunk map[string]interface{}) {
		data, _ := json.Marshal(chunk)
		out.WriteString("data: " + string(data) + "\n\n")
	}
	base := func() map[string]interface{} {
		ch := map[string]interface{}{}
		for k, v := range result {
			if k != "choices" && k != "usage" {
				ch[k] = v
			}
		}
		ch["object"] = "chat.completion.chunk"
		return ch
	}
	choices, _ := result["choices"].([]interface{})
	deltas := make([]interface{}, 0, len(choices))
	finishes := make([]interface{}, 0, len(choices))
	for _, item := range choices {
		ch, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		message, _ := ch["message"].(map[string]interface{})
		if calls, ok := message["tool_calls"].([]interface{}); ok {
			for i, call := range calls {
				if t, ok := call.(map[string]interface{}); ok {
					t["index"] = i
				}
			}
		}
		deltas = append(deltas, map[string]interface{}{"index": ch["index"], "delta": message, "finish_reason": nil})
		finishes = append(finishes, map[string]interface{}{"index": ch["index"], "delta": map[string]interface{}{}, "finish_reason": ch["finish_reason"]})
	}
	chunk := base()
	chunk["choices"] = deltas
	emit(chunk)
	chunk = base()
	chunk["choices"] = finishes
	emit(chunk)
	if usage, ok := result["usage"]; ok {
		chunk = base()
		chunk["choices"] = []interface{}{}
		chunk["usage"] = usage
		emit(chunk)
	}
	out.WriteString("data: [DONE]\n\n")
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(out.String()))}, nil
}
