package builtin

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	coretool "github.com/boxify/api-go/internal/core/tool"
)

// 验证 current_time 描述使用统一 Schema 字段表达调用 schema，不再暴露输入/输出 schema 字段。
func TestCurrentTimeDescriptorUsesSchema(t *testing.T) {
	tool := NewCurrentTimeTool(WithClock(time.Now))

	descriptor, err := tool.Describe(context.Background())
	if err != nil {
		t.Fatalf("current_time Describe() error = %v, want nil", err)
	}
	want := coretool.Schema{
		Parameters: coretool.ParametersSchema{
			Type: "object",
			Properties: map[string]coretool.PropertySchema{
				"timezone": {
					"type":        "string",
					"description": "可选的 IANA 时区名称，默认使用 UTC。",
				},
			},
			AdditionalProperties: false,
		},
	}
	if !reflect.DeepEqual(descriptor.Schema, want) {
		t.Fatalf("current_time Descriptor.Schema = %#v, want %#v", descriptor.Schema, want)
	}
}

// 验证 current_time 默认使用 UTC，并返回稳定的文本和结构化元数据。
func TestCurrentTimeUsesUTCByDefault(t *testing.T) {
	ctx := context.Background()
	fixed := mustParseTime(t, "2026-07-05T10:11:12Z")
	tool := NewCurrentTimeTool(WithClock(func() time.Time {
		return fixed
	}))

	output, err := tool.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("current_time Invoke() error = %v, want nil", err)
	}
	if output.Text != "2026-07-05T10:11:12Z" {
		t.Fatalf("current_time Text = %q, want 2026-07-05T10:11:12Z", output.Text)
	}
	assertMetadata(t, output.Metadata, map[string]any{
		"time":     "2026-07-05T10:11:12Z",
		"unix":     int64(1783246272),
		"timezone": "UTC",
		"offset":   "+00:00",
	})
}

// 验证 current_time 支持 IANA 时区，并按指定时区转换输出。
func TestCurrentTimeConvertsIanaTimezone(t *testing.T) {
	ctx := context.Background()
	fixed := mustParseTime(t, "2026-07-05T10:11:12Z")
	tool := NewCurrentTimeTool(WithClock(func() time.Time {
		return fixed
	}))

	output, err := tool.Invoke(ctx, coretool.Input{"timezone": "Asia/Shanghai"})
	if err != nil {
		t.Fatalf("current_time Invoke(Asia/Shanghai) error = %v, want nil", err)
	}
	if output.Text != "2026-07-05T18:11:12+08:00" {
		t.Fatalf("current_time Text = %q, want 2026-07-05T18:11:12+08:00", output.Text)
	}
	assertMetadata(t, output.Metadata, map[string]any{
		"time":     "2026-07-05T18:11:12+08:00",
		"unix":     int64(1783246272),
		"timezone": "Asia/Shanghai",
		"offset":   "+08:00",
	})
}

// 验证 current_time 对无效时区返回错误，避免静默使用错误时间。
func TestCurrentTimeRejectsInvalidTimezone(t *testing.T) {
	ctx := context.Background()
	tool := NewCurrentTimeTool(WithClock(func() time.Time {
		return mustParseTime(t, "2026-07-05T10:11:12Z")
	}))

	_, err := tool.Invoke(ctx, coretool.Input{"timezone": "Mars/Base"})
	if err == nil {
		t.Fatal("current_time Invoke(invalid timezone) error = nil, want error")
	}
	if !errors.Is(err, ErrInvalidTimezone) {
		t.Fatalf("current_time Invoke(invalid timezone) error = %v, want ErrInvalidTimezone", err)
	}
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("time.Parse(%q) error = %v, want nil", value, err)
	}
	return parsed
}

func assertMetadata(t *testing.T, got map[string]any, want map[string]any) {
	t.Helper()
	for key, wantValue := range want {
		if got[key] != wantValue {
			t.Fatalf("metadata[%s] = %#v, want %#v", key, got[key], wantValue)
		}
	}
}
