package valuex

import (
	"encoding/json"
	"testing"
)

// TestStringConvertsOnlyStringLikeValues 验证 String 只接收字符串和 json.Number。
func TestStringConvertsOnlyStringLikeValues(t *testing.T) {
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

// TestStringListKeepsOnlyStringItems 验证 StringList 支持 []string 和 []any，并过滤空字符串与非字符串元素。
func TestStringListKeepsOnlyStringItems(t *testing.T) {
	got := StringList([]any{"电脑", 42, "", true, "鼠标"})
	if len(got) != 2 || got[0] != "电脑" || got[1] != "鼠标" {
		t.Fatalf("StringList([]any) = %#v, want string items only", got)
	}

	got = StringList([]string{"电脑", "", "鼠标"})
	if len(got) != 2 || got[0] != "电脑" || got[1] != "鼠标" {
		t.Fatalf("StringList([]string) = %#v, want non-empty strings", got)
	}
}

// TestRequiredStringReadsTrimmedValue 验证 RequiredString 会读取并裁剪必填字符串。
func TestRequiredStringReadsTrimmedValue(t *testing.T) {
	got, err := RequiredString(map[string]any{"name": " skill "}, "name")
	if err != nil {
		t.Fatalf("RequiredString() error = %v, want nil", err)
	}
	if got != "skill" {
		t.Fatalf("RequiredString() = %q, want skill", got)
	}
}

// TestRequiredStringRejectsMissingOrInvalidValue 验证 RequiredString 会拒绝缺失、空白和错误类型。
func TestRequiredStringRejectsMissingOrInvalidValue(t *testing.T) {
	cases := []struct {
		name   string
		values map[string]any
	}{
		{name: "missing", values: map[string]any{}},
		{name: "blank", values: map[string]any{"name": " "}},
		{name: "invalid", values: map[string]any{"name": 1}},
	}

	for _, tc := range cases {
		if _, err := RequiredString(tc.values, "name"); err == nil {
			t.Fatalf("%s: RequiredString() error = nil, want error", tc.name)
		}
	}
}

// TestOptionalStringReadsTrimmedValue 验证 OptionalString 会读取字符串并在缺失时返回空字符串。
func TestOptionalStringReadsTrimmedValue(t *testing.T) {
	got, err := OptionalString(map[string]any{"description": " desc "}, "description")
	if err != nil {
		t.Fatalf("OptionalString() error = %v, want nil", err)
	}
	if got != "desc" {
		t.Fatalf("OptionalString() = %q, want desc", got)
	}

	got, err = OptionalString(map[string]any{}, "description")
	if err != nil {
		t.Fatalf("OptionalString(missing) error = %v, want nil", err)
	}
	if got != "" {
		t.Fatalf("OptionalString(missing) = %q, want empty", got)
	}
}

// TestOptionalStringRejectsInvalidValue 验证 OptionalString 会拒绝非字符串字段。
func TestOptionalStringRejectsInvalidValue(t *testing.T) {
	if _, err := OptionalString(map[string]any{"description": 1}, "description"); err == nil {
		t.Fatalf("OptionalString() error = nil, want error")
	}
}

// TestOptionalStringListReadsTrimmedValues 验证 OptionalStringList 会读取字符串列表并过滤空白项。
func TestOptionalStringListReadsTrimmedValues(t *testing.T) {
	got, err := OptionalStringList(map[string]any{"tags": []any{" builtin ", " ", "skill"}}, "tags")
	if err != nil {
		t.Fatalf("OptionalStringList() error = %v, want nil", err)
	}
	if len(got) != 2 || got[0] != "builtin" || got[1] != "skill" {
		t.Fatalf("OptionalStringList() = %#v, want trimmed non-empty items", got)
	}

	got, err = OptionalStringList(map[string]any{}, "tags")
	if err != nil {
		t.Fatalf("OptionalStringList(missing) error = %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("OptionalStringList(missing) = %#v, want nil", got)
	}
}

// TestOptionalStringListRejectsInvalidValue 验证 OptionalStringList 会拒绝非列表字段和非字符串元素。
func TestOptionalStringListRejectsInvalidValue(t *testing.T) {
	cases := []struct {
		name   string
		values map[string]any
	}{
		{name: "not list", values: map[string]any{"tags": "bad"}},
		{name: "item not string", values: map[string]any{"tags": []any{"ok", 1}}},
	}

	for _, tc := range cases {
		if _, err := OptionalStringList(tc.values, "tags"); err == nil {
			t.Fatalf("%s: OptionalStringList() error = nil, want error", tc.name)
		}
	}
}

// TestFloatConvertsNumericValues 验证 Float 覆盖常见的 float、int 和 json.Number 数值字段。
func TestFloatConvertsNumericValues(t *testing.T) {
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

// TestTruncateRunesTrimsAndKeepsRuneBoundary 验证 TruncateRunes 按 rune 截断并去掉首尾空白。
func TestTruncateRunesTrimsAndKeepsRuneBoundary(t *testing.T) {
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

// TestTruncateRunesWithSuffixAddsSuffixOnlyWhenTruncated 验证 TruncateRunesWithSuffix 只在发生截断时追加后缀。
func TestTruncateRunesWithSuffixAddsSuffixOnlyWhenTruncated(t *testing.T) {
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
