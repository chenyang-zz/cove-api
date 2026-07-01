package valuex

import (
	"encoding/json"
	"testing"
)

func TestStringConvertsOnlyStringLikeValues(t *testing.T) {
	// 验证点：String 只接收字符串和 json.Number，其他类型返回空字符串，避免误把复杂结构转成展示文本。
	if got := String("hello"); got != "hello" {
		t.Fatalf("String(string) = %q, want hello", got)
	}
	if got := String(json.Number("42")); got != "42" {
		t.Fatalf("String(json.Number) = %q, want 42", got)
	}
	if got := String(42); got != "" {
		t.Fatalf("String(int) = %q, want empty", got)
	}
}

func TestStringListKeepsOnlyStringItems(t *testing.T) {
	// 验证点：StringList 支持 []string 和 []any，并过滤空字符串与非字符串元素。
	got := StringList([]any{"电脑", 42, "", true, "鼠标"})
	if len(got) != 2 || got[0] != "电脑" || got[1] != "鼠标" {
		t.Fatalf("StringList([]any) = %#v, want string items only", got)
	}

	got = StringList([]string{"电脑", "", "鼠标"})
	if len(got) != 2 || got[0] != "电脑" || got[1] != "鼠标" {
		t.Fatalf("StringList([]string) = %#v, want non-empty strings", got)
	}
}

func TestFloatConvertsNumericValues(t *testing.T) {
	// 验证点：Float 覆盖 ES 响应里常见的 float/int/json.Number 分数字段。
	cases := []struct {
		name  string
		value any
		want  float64
	}{
		{name: "float64", value: 1.25, want: 1.25},
		{name: "float32", value: float32(2.5), want: 2.5},
		{name: "int", value: 3, want: 3},
		{name: "int64", value: int64(4), want: 4},
		{name: "json number", value: json.Number("5.5"), want: 5.5},
		{name: "invalid", value: "bad", want: 0},
	}

	for _, tc := range cases {
		got := Float(tc.value)
		if got != tc.want {
			t.Fatalf("%s: Float(%#v) = %v, want %v", tc.name, tc.value, got, tc.want)
		}
	}
}

func TestTruncateRunesTrimsAndKeepsRuneBoundary(t *testing.T) {
	// 验证点：TruncateRunes 按 rune 截断并去掉首尾空白，不追加省略号，适合标签和 prompt 内容裁剪。
	if got := TruncateRunes("  你好世界  ", 2); got != "你好" {
		t.Fatalf("TruncateRunes = %q, want 你好", got)
	}
	if got := TruncateRunes("hello", 0); got != "" {
		t.Fatalf("TruncateRunes max 0 = %q, want empty", got)
	}
	if got := TruncateRunes("hi", 10); got != "hi" {
		t.Fatalf("TruncateRunes short = %q, want hi", got)
	}
}

func TestTruncateRunesWithSuffixAddsSuffixOnlyWhenTruncated(t *testing.T) {
	// 验证点：TruncateRunesWithSuffix 只在发生截断时追加后缀，适合展示标题裁剪。
	if got := TruncateRunesWithSuffix("你好世界", 2, "..."); got != "你好..." {
		t.Fatalf("TruncateRunesWithSuffix = %q, want 你好...", got)
	}
	if got := TruncateRunesWithSuffix("hi", 10, "..."); got != "hi" {
		t.Fatalf("TruncateRunesWithSuffix short = %q, want hi", got)
	}
	if got := TruncateRunesWithSuffix("hello", 0, "..."); got != "" {
		t.Fatalf("TruncateRunesWithSuffix max 0 = %q, want empty", got)
	}
}
