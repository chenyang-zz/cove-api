// Package prompt_test 验证提示词管理器的注册、查找和渲染行为。
package prompt_test

import (
	"strings"
	"testing"
	"testing/fstest"
	"text/template"

	"github.com/boxify/api-go/internal/core/prompt"
	ragprompt "github.com/boxify/api-go/internal/core/rag/prompt"
)

func TestManagerRendersRegisteredFSWithStructData(t *testing.T) {
	// 验证注册 embed/fs 模板来源后，可以用结构体数据按命名空间渲染模板。
	manager := prompt.NewManager("")
	if err := manager.RegisterFS("rag", ragprompt.Templates); err != nil {
		t.Fatalf("RegisterFS error = %v", err)
	}

	out, err := manager.Render("rag/"+ragprompt.ContentClassifierTemplate, ragprompt.ContentClassifierData{
		Existing: "技术、学习",
		Content:  "这是一段内容",
	})
	if err != nil {
		t.Fatalf("Render error = %v", err)
	}
	if !strings.Contains(out, "技术、学习") || !strings.Contains(out, "这是一段内容") {
		t.Fatalf("Render = %q, want rendered struct data", out)
	}
}

func TestManagerRendersRegisteredTextWithStructData(t *testing.T) {
	// 验证数据库或远端下发的模板文本注册后，可以用业务结构体渲染。
	manager := prompt.NewManager("")
	type classifyData struct {
		Content string
		Tags    []string
	}
	if err := manager.RegisterText("db/classify", "内容：{{ .Content }} 标签：{{ range .Tags }}{{ . }} {{ end }}"); err != nil {
		t.Fatalf("RegisterText error = %v", err)
	}

	out, err := manager.Render("db/classify", classifyData{
		Content: "用户输入内容",
		Tags:    []string{"技术", "生活"},
	})
	if err != nil {
		t.Fatalf("Render error = %v", err)
	}
	if out != "内容：用户输入内容 标签：技术 生活 " {
		t.Fatalf("Render = %q, want struct rendered text", out)
	}
}

func TestManagerRendersRegisteredTextWithMapData(t *testing.T) {
	// 验证动态外部模板仍支持 map 数据，避免阻断数据库配置型提示词。
	manager := prompt.NewManager("")
	if err := manager.RegisterText("db/dynamic", "标题：{{ .Title }}"); err != nil {
		t.Fatalf("RegisterText error = %v", err)
	}

	out, err := manager.Render("db/dynamic", map[string]any{"Title": "动态标题"})
	if err != nil {
		t.Fatalf("Render error = %v", err)
	}
	if out != "标题：动态标题" {
		t.Fatalf("Render = %q, want map rendered text", out)
	}
}

func TestManagerTextTemplateOverridesFSTemplate(t *testing.T) {
	// 验证内存文本模板与 FS 模板同名时，优先使用内存文本模板。
	manager := prompt.NewManager("")
	fsys := fstest.MapFS{
		"classify.tmpl": {Data: []byte("fs={{ .Name }}")},
	}
	if err := manager.RegisterFS("custom", fsys); err != nil {
		t.Fatalf("RegisterFS error = %v", err)
	}
	if err := manager.RegisterText("custom/classify.tmpl", "text={{ .Name }}"); err != nil {
		t.Fatalf("RegisterText error = %v", err)
	}

	out, err := manager.Render("custom/classify.tmpl", map[string]string{"Name": "boxify"})
	if err != nil {
		t.Fatalf("Render error = %v", err)
	}
	if out != "text=boxify" {
		t.Fatalf("Render = %q, want text template override", out)
	}
}

func TestManagerValidationErrorsAreClear(t *testing.T) {
	// 验证注册和查找错误能明确指出 namespace、name 或模板来源问题。
	manager := prompt.NewManager("")
	cases := []struct {
		name string
		run  func() error
		want string
	}{
		{name: "empty namespace", run: func() error { return manager.RegisterFS("", fstest.MapFS{}) }, want: "namespace"},
		{name: "nil fs", run: func() error { return manager.RegisterFS("bad", nil) }, want: "template fs"},
		{name: "empty text name", run: func() error { return manager.RegisterText("", "text") }, want: "name"},
		{name: "empty template name", run: func() error { _, err := manager.Render("", nil); return err }, want: "name"},
		{name: "unknown namespace", run: func() error { _, err := manager.Render("missing/file.tmpl", nil); return err }, want: "namespace"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want contains %q", err, tc.want)
			}
		})
	}
}

func TestManagerMissingTemplateReturnsClearError(t *testing.T) {
	// 验证已注册 namespace 但模板文件不存在时，返回包含模板名的错误。
	manager := prompt.NewManager("")
	if err := manager.RegisterFS("custom", fstest.MapFS{}); err != nil {
		t.Fatalf("RegisterFS error = %v", err)
	}

	_, err := manager.Render("custom/missing.tmpl", nil)
	if err == nil || !strings.Contains(err.Error(), "missing.tmpl") {
		t.Fatalf("Render error = %v, want missing template name", err)
	}
}

func TestManagerRenderTextDoesNotRequireRegistration(t *testing.T) {
	// 验证 RenderText 是一次性文本渲染入口，不依赖任何管理器注册内容。
	manager := prompt.NewManager("")

	out, err := manager.RenderText("hello {{ .Name }}", map[string]string{"Name": "boxify"})
	if err != nil {
		t.Fatalf("RenderText error = %v", err)
	}
	if out != "hello boxify" {
		t.Fatalf("RenderText = %q, want hello boxify", out)
	}
}

func TestManagerAppliesRendererOptions(t *testing.T) {
	// 验证 Manager 复用 Renderer 的 option 机制，支持外部注入模板函数。
	manager := prompt.NewManager("", prompt.WithFuncs(template.FuncMap{
		"wrap": func(text string) string { return "[" + text + "]" },
	}))

	out, err := manager.RenderText(`{{ wrap .Name }}`, map[string]string{"Name": "boxify"})
	if err != nil {
		t.Fatalf("RenderText error = %v", err)
	}
	if out != "[boxify]" {
		t.Fatalf("RenderText = %q, want [boxify]", out)
	}
}

func TestManagerTemplateTextReadsRegisteredTemplate(t *testing.T) {
	// 验证 TemplateText 只读取已注册模板原文，不执行模板。
	manager := prompt.NewManager("")
	if err := manager.RegisterText("db/raw", "{{ .Name }}"); err != nil {
		t.Fatalf("RegisterText error = %v", err)
	}

	out, err := manager.TemplateText("db/raw")
	if err != nil {
		t.Fatalf("TemplateText error = %v", err)
	}
	if out != "{{ .Name }}" {
		t.Fatalf("TemplateText = %q, want raw template", out)
	}
}
