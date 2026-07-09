package valuex

import (
	"encoding/json"
	"fmt"
	"strings"
)

// String 将字符串类值转换为字符串，非字符串类值返回空字符串。
func String(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

// StringList 将字符串列表类值转换为字符串切片，并过滤空字符串。
func StringList(value any) []string {
	switch v := value.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			text, ok := item.(string)
			if ok && text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return []string{}
	}
}

// RequiredString 从映射中读取必填字符串字段，并返回去除首尾空白后的值。
func RequiredString(values map[string]any, key string) (string, error) {
	value, err := OptionalString(values, key)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

// OptionalString 从映射中读取可选字符串字段，缺失或 nil 时返回空字符串。
func OptionalString(values map[string]any, key string) (string, error) {
	raw, ok := values[key]
	if !ok || raw == nil {
		return "", nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	return strings.TrimSpace(value), nil
}

// OptionalStringList 从映射中读取可选字符串列表字段，并过滤空白项。
func OptionalStringList(values map[string]any, key string) ([]string, error) {
	raw, ok := values[key]
	if !ok || raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a list", key)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		value, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s items must be strings", key)
		}
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out, nil
}

// Float 将常见数值类型转换为 float64，无法转换时返回 0。
func Float(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		out, _ := v.Float64()
		return out
	default:
		return 0
	}
}

// TruncateRunes 去除文本首尾空白后按 rune 数量截断，非正长度返回空字符串。
func TruncateRunes(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}

// TruncateRunesWithSuffix 在文本被截断时追加后缀，未截断时返回裁剪空白后的文本。
func TruncateRunesWithSuffix(text string, maxRunes int, suffix string) string {
	trimmed := strings.TrimSpace(text)
	truncated := TruncateRunes(trimmed, maxRunes)
	if maxRunes <= 0 {
		return ""
	}
	if len([]rune(trimmed)) <= maxRunes {
		return truncated
	}
	return truncated + suffix
}
