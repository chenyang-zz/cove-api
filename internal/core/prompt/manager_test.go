// Package prompt_test 验证 prompt 管理器和通用模板渲染入口。
//
// 本文件覆盖包级 Render、RenderText 和 TemplateText 入口的基础行为。
package prompt_test

import (
	"testing"
	"testing/fstest"

	"github.com/boxify/api-go/internal/core/prompt"
)

// TestRenderTextExecutesGoTemplateWithSprigFuncs 验证通用文本渲染入口支持 Go template 和 sprig 函数。
func TestRenderTextExecutesGoTemplateWithSprigFuncs(t *testing.T) {
	got, err := prompt.RenderText("hello {{ .Name | upper }}", map[string]string{"Name": "boxify"})
	if err != nil {
		t.Fatalf("RenderText error = %v", err)
	}
	if got != "hello BOXIFY" {
		t.Fatalf("RenderText = %q, want hello BOXIFY", got)
	}
}

// TestRenderReadsTemplateFromFS 验证通用 FS 渲染入口可脱离 Manager 使用。
func TestRenderReadsTemplateFromFS(t *testing.T) {
	fsys := fstest.MapFS{
		"rag/example.tmpl": {Data: []byte("tag={{ .Tag }}")},
	}
	got, err := prompt.Render(fsys, "rag/example.tmpl", map[string]string{"Tag": "技术"})
	if err != nil {
		t.Fatalf("Render error = %v", err)
	}
	if got != "tag=技术" {
		t.Fatalf("Render = %q, want tag=技术", got)
	}
}

// TestTemplateTextReadsTemplateFromFS 验证通用模板读取入口只读取原始文本。
func TestTemplateTextReadsTemplateFromFS(t *testing.T) {
	fsys := fstest.MapFS{
		"raw.tmpl": {Data: []byte("{{ .Name }}")},
	}
	got, err := prompt.TemplateText(fsys, "raw.tmpl")
	if err != nil {
		t.Fatalf("TemplateText error = %v", err)
	}
	if got != "{{ .Name }}" {
		t.Fatalf("TemplateText = %q, want raw template", got)
	}
}
