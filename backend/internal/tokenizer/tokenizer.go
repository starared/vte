package tokenizer

import (
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/pkoukk/tiktoken-go"
	tiktoken_loader "github.com/pkoukk/tiktoken-go-loader"
)

func init() {
	// 使用内置词表，不在运行时从 openaipublic.blob.core.windows.net 下载
	// （国内网络下该下载可能很慢或失败，并会阻塞 token 估算）
	tiktoken.SetBpeLoader(tiktoken_loader.NewOfflineLoader())
}

var (
	encoderCache = make(map[string]*tiktoken.Tiktoken)
	cacheMu      sync.RWMutex
)

const defaultEncoding = "cl100k_base"

// o200k_base 是 gpt-4o 及之后 OpenAI 模型使用的词表；按顺序匹配，避免 "gpt-4" 抢先命中 "gpt-4o"。
// 非 OpenAI 模型（Claude、Gemini 等）没有公开词表，统一用 cl100k_base 近似估算。
var o200kPatterns = []string{
	"gpt-4o", "chatgpt-4o", "gpt-4.1", "gpt-4.5", "gpt-5",
	"o1", "o3", "o4",
}

// getEncodingForModel 根据模型名称获取编码器名称
func getEncodingForModel(modelName string) string {
	name := strings.ToLower(modelName)
	// 去掉提供商前缀（如 openai/gpt-4o）
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	for _, p := range o200kPatterns {
		if strings.HasPrefix(name, p) {
			return "o200k_base"
		}
	}
	return defaultEncoding
}

// getEncoder 获取或创建编码器（带缓存）
func getEncoder(encodingName string) (*tiktoken.Tiktoken, error) {
	cacheMu.RLock()
	if enc, ok := encoderCache[encodingName]; ok {
		cacheMu.RUnlock()
		return enc, nil
	}
	cacheMu.RUnlock()

	cacheMu.Lock()
	defer cacheMu.Unlock()

	// Double check
	if enc, ok := encoderCache[encodingName]; ok {
		return enc, nil
	}

	enc, err := tiktoken.GetEncoding(encodingName)
	if err != nil {
		return nil, err
	}

	encoderCache[encodingName] = enc
	return enc, nil
}

// CountTokens 计算文本的 token 数量
func CountTokens(text string, modelName string) int {
	encodingName := getEncodingForModel(modelName)
	enc, err := getEncoder(encodingName)
	if err != nil {
		// 如果获取编码器失败，回退到估算
		return estimateTokens(text)
	}

	tokens := enc.Encode(text, nil, nil)
	return len(tokens)
}

// CountMessagesTokens 计算消息数组的 token 数量（OpenAI chat 格式）
func CountMessagesTokens(messages []interface{}, modelName string) int {
	encodingName := getEncodingForModel(modelName)
	enc, err := getEncoder(encodingName)
	if err != nil {
		return estimateMessagesTokens(messages)
	}

	totalTokens := 0

	// 每条消息有固定的 token 开销
	// GPT-4/Claude: 每条消息约 3 token 开销（role + content 分隔符等）
	tokensPerMessage := 3

	for _, msg := range messages {
		if m, ok := msg.(map[string]interface{}); ok {
			totalTokens += tokensPerMessage

			// role
			if role, ok := m["role"].(string); ok {
				tokens := enc.Encode(role, nil, nil)
				totalTokens += len(tokens)
			}

			// content
			if content, ok := m["content"].(string); ok {
				tokens := enc.Encode(content, nil, nil)
				totalTokens += len(tokens)
			} else if contentArr, ok := m["content"].([]interface{}); ok {
				// 多模态内容（图片+文字）
				for _, item := range contentArr {
					if itemMap, ok := item.(map[string]interface{}); ok {
						if itemType, ok := itemMap["type"].(string); ok && itemType == "text" {
							if text, ok := itemMap["text"].(string); ok {
								tokens := enc.Encode(text, nil, nil)
								totalTokens += len(tokens)
							}
						}
						// 图片 token 估算（根据 OpenAI 文档，低分辨率约 85 token，高分辨率更多）
						if itemType, ok := itemMap["type"].(string); ok && itemType == "image_url" {
							totalTokens += 85 // 低分辨率图片的基础 token
						}
					}
				}
			}

			// name（如果有）
			if name, ok := m["name"].(string); ok {
				tokens := enc.Encode(name, nil, nil)
				totalTokens += len(tokens)
				totalTokens += 1 // name 字段有额外 1 token
			}
		}
	}

	// 每个请求有 3 token 的固定开销
	totalTokens += 3

	return totalTokens
}

// estimateTokens 估算 token（备用方案）
func estimateTokens(text string) int {
	// 中英混合文本的粗略估算
	// 英文约 4 字符/token，中文约 1.5 字符/token
	// 使用 2.5 字符/token 作为折中
	if len(text) == 0 {
		return 0
	}

	// 统计中文字符数量
	chineseCount := 0
	for _, r := range text {
		if r >= 0x4e00 && r <= 0x9fff {
			chineseCount++
		}
	}

	// 中文按 1.5 字符/token，其他按 4 字符/token
	otherCount := utf8.RuneCountInString(text) - chineseCount
	tokens := int(float64(chineseCount)/1.5 + float64(otherCount)/4)
	if tokens < 1 && len(text) > 0 {
		tokens = 1
	}
	return tokens
}

// estimateMessagesTokens 估算消息的 token（备用方案）
func estimateMessagesTokens(messages []interface{}) int {
	total := 3
	for _, msg := range messages {
		if m, ok := msg.(map[string]interface{}); ok {
			total += 3
			if content, ok := m["content"].(string); ok {
				total += estimateTokens(content)
			}
		}
	}
	return total
}
