package llm_test

import (
	"strings"
	"testing"

	. "github.com/boxify/api-go/internal/core/llm"
)

// 验证标准 JSON 看图输出会被解析为结构化字段，且 objects 只保留字符串。
func TestParseVisionDescriptionParsesStandardJSON(t *testing.T) {
	got := ParseVisionDescription(`{"description":"桌面","ocr_text":"Hello","objects":["电脑",42,"鼠标"],"scene":"办公室"}`)
	if got.Description != "桌面" || got.OCRText != "Hello" || got.Scene != "办公室" {
		t.Fatalf("ParseVisionDescription = %#v", got)
	}
	if len(got.Objects) != 2 || got.Objects[0] != "电脑" || got.Objects[1] != "鼠标" {
		t.Fatalf("objects = %#v, want string items only", got.Objects)
	}
}

// 验证 markdown 代码块包裹的 JSON 可被剥离后解析。
func TestParseVisionDescriptionStripsMarkdownFence(t *testing.T) {
	raw := "```json\n{\"description\":\"文档截图\",\"ocr_text\":\"标题\",\"objects\":[\"文字\"],\"scene\":\"文档\"}\n```"
	got := ParseVisionDescription(raw)
	if got.Description != "文档截图" || got.OCRText != "标题" || got.Scene != "文档" || len(got.Objects) != 1 || got.Objects[0] != "文字" {
		t.Fatalf("ParseVisionDescription = %#v", got)
	}
}

// 验证解析失败时用截断原文兜底 description，结构化字段清空。
func TestParseVisionDescriptionFallsBackToRawText(t *testing.T) {
	raw := strings.Repeat("描述", 1200)
	got := ParseVisionDescription(raw)
	if len([]rune(got.Description)) != 2000 {
		t.Fatalf("description rune len = %d, want 2000", len([]rune(got.Description)))
	}
	if got.OCRText != "" || got.Scene != "" || got.Objects == nil || len(got.Objects) != 0 {
		t.Fatalf("fallback = %#v, want empty structured fields", got)
	}
}
