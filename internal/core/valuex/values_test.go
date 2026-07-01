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
