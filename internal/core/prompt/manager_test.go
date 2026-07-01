// Package prompt_test 验证 prompt 管理器和通用模板渲染入口。
//
// 本文件覆盖旧 Manager 的磁盘模板兼容行为，以及新增包级 Render/RenderText/
// TemplateText 入口的基础行为。
package prompt_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/boxify/api-go/internal/core/prompt"
)

func TestManagerRenderReadsTemplateFromRootPath(t *testing.T) {
	// 验证旧 Manager 入口仍能从目录读取模板并完成渲染，保护 memory/agent 调用方。
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "memory"), 0o755); err != nil {
		t.Fatalf("mkdir prompt namespace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "memory", "example.tmpl"), []byte("hello {{ .Name }}"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	manager := prompt.NewManager(root)
	got, err := manager.Render("memory/example", map[string]string{"Name": "boxify"})
	if err != nil {
		t.Fatalf("Render error = %v", err)
	}
	if got != "hello boxify" {
		t.Fatalf("Render = %q, want hello boxify", got)
	}
}

func TestManagerRenderMissingTemplateIncludesPath(t *testing.T) {
	// 验证模板缺失时错误包含完整路径，方便调用方定位配置问题。
	root := t.TempDir()
	manager := prompt.NewManager(root)

	_, err := manager.Render("memory/missing", nil)
	if err == nil {
		t.Fatal("Render error = nil, want missing template error")
	}
	want := filepath.Join(root, "memory", "missing.tmpl")
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("Render error = %q, want path %q", err.Error(), want)
	}
}

func TestRenderTextExecutesGoTemplateWithSprigFuncs(t *testing.T) {
	// 验证通用文本渲染入口支持 Go template 和 sprig 函数。
	got, err := prompt.RenderText("hello {{ .Name | upper }}", map[string]string{"Name": "boxify"})
	if err != nil {
		t.Fatalf("RenderText error = %v", err)
	}
	if got != "hello BOXIFY" {
		t.Fatalf("RenderText = %q, want hello BOXIFY", got)
	}
}

func TestRenderReadsTemplateFromFS(t *testing.T) {
	// 验证通用 FS 渲染入口可脱离 Manager 使用，供 RAG 等 core 包复用。
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

func TestTemplateTextReadsTemplateFromFS(t *testing.T) {
	// 验证通用模板读取入口只读取原始文本，不执行模板占位符。
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
