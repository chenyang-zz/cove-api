package promptsgen

import (
	"strings"
	"testing"
)

type fakeRenderer struct {
	name string
	data any
}

func (f *fakeRenderer) Render(name string, data any) (string, error) {
	f.name = name
	f.data = data
	return "rendered", nil
}

// TestClientRenderDelegatesToRenderer 验证生成客户端会把逻辑名称和类型化参数原样交给 Renderer。
func TestClientRenderDelegatesToRenderer(t *testing.T) {
	renderer := &fakeRenderer{}
	client := NewClient(renderer)
	params := &struct{ Content string }{Content: "hello"}

	got, err := client.render("agent/example", params)
	if err != nil {
		t.Fatalf("Client.render error = %v, want nil", err)
	}
	if got != "rendered" {
		t.Fatalf("Client.render result = %q, want rendered", got)
	}
	if renderer.name != "agent/example" || renderer.data != params {
		t.Fatalf("Renderer call = (%q, %#v), want (%q, same params)", renderer.name, renderer.data, "agent/example")
	}
}

// TestClientRenderRejectsNilRenderer 验证未配置 Renderer 时返回清晰错误而不是 panic。
func TestClientRenderRejectsNilRenderer(t *testing.T) {
	_, err := NewClient(nil).render("agent/example", nil)
	if err == nil || !strings.Contains(err.Error(), "renderer is nil") {
		t.Fatalf("Client.render error = %v, want nil renderer error", err)
	}
}
