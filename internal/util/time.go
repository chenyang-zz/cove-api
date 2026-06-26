package util

import (
	"fmt"
	"strings"
	"time"
)

// NullTimeString 表示提示词中使用的空时间占位字符串。
const NullTimeString = "NULL"

// ISO8601OrNULL 将时间格式化为 UTC RFC3339 字符串；零值时间返回 NullTimeString。
func ISO8601OrNULL(value time.Time) string {
	if value.IsZero() {
		return NullTimeString
	}
	return value.UTC().Format(time.RFC3339)
}

// ParseISOTime 将常见 ISO 时间字符串解析为 UTC time.Time。
func ParseISOTime(value string) (time.Time, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}, fmt.Errorf("iso time is empty")
	}
	for _, layout := range isoTimeLayouts {
		parsed, err := time.Parse(layout, text)
		if err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid iso time %q", value)
}

var isoTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05Z07:00",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006-01-02",
}
