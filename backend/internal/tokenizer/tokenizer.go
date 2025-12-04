package tokenizer

import (
	"strings"
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

var (
	encoderCache = make(map[string]*tiktoken.Tiktoken)
	cacheMu      sync.RWMutex
)

// 模型到编码器的映射
var modelEncodingMap = map[string]string{
	// Claude 模型使用 cl100k_base（与 GPT-4 相同）
	"claude":       "cl100k_base",
	"claude-3":     "cl100k_base",
	"claude-opus":  "cl100k_base",
	"claude-sonnet": "cl100k_base",
	"claude-haiku": "cl100k_base",
	
	// GPT-4 系列
	"gpt-4":        "cl100k_base",
	"gpt-4o":       "cl100k_base",
	"gpt-4-turbo":  "cl100k_base",
	
	// GPT-3.5 系列
	"gpt-3.5":      "cl100k_base",
	"gpt-35":       "cl100k_base",
	
	// 其他模型默认
	"default":      "cl100k_base",
}

// getEncodingForModel 根据模型名称获取编码器名称
func getEncodingForModel(modelName string) string {
	modelLower := strings.ToLower(modelName)
	
	// 检查精确匹配
	for prefix, encoding := range modelEncodingMap {
		if strings.Contains(modelLower, prefix) {
			return encoding
		}
	}
	
	return modelEncodingMap["default"]
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
	otherCount := len(text) - chineseCount
	tokens := int(float64(chineseCount)/1.5 + float64(otherCount)/4)
	if tokens < 1 && len(text) > 0 {
		tokens = 1
	}
	return tokens
}

// estimateMessagesTokens 估算消息的 token（备用方案）
func estimateMessagesTokens(messages []interface{}) int {
	totalChars := 0
	for _, msg := range messages {
		if m, ok := msg.(map[string]interface{}); ok {
			if content, ok := m["content"].(string); ok {
				totalChars += len(content)
			}
		}
	}
	return estimateTokens(string(make([]byte, totalChars)))
}
