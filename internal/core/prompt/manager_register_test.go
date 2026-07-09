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

// TestManagerRendersRegisteredFSWithStructData 验证注册 FS 模板后可按命名空间使用结构体渲染。
func TestManagerRendersRegisteredFSWithStructData(t *testing.T) {
	manager := prompt.NewManager()
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

// TestManagerRendersRegisteredTextWithStructData 验证注册文本模板后可使用结构体数据渲染。
func TestManagerRendersRegisteredTextWithStructData(t *testing.T) {
	manager := prompt.NewManager()
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

// TestManagerRendersRegisteredTextWithMapData 验证动态外部模板支持 map 数据。
func TestManagerRendersRegisteredTextWithMapData(t *testing.T) {
	manager := prompt.NewManager()
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

// TestManagerTextTemplateOverridesFSTemplate 验证同名内存文本模板优先于 FS 模板。
func TestManagerTextTemplateOverridesFSTemplate(t *testing.T) {
	manager := prompt.NewManager()
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

// TestManagerValidationErrorsAreClear 验证注册和查找错误包含明确的问题来源。
func TestManagerValidationErrorsAreClear(t *testing.T) {
	manager := prompt.NewManager()
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

// TestManagerMissingTemplateReturnsClearError 验证已注册 namespace 缺少模板时返回模板名。
func TestManagerMissingTemplateReturnsClearError(t *testing.T) {
	manager := prompt.NewManager()
	if err := manager.RegisterFS("custom", fstest.MapFS{}); err != nil {
		t.Fatalf("RegisterFS error = %v", err)
	}

	_, err := manager.Render("custom/missing.tmpl", nil)
	if err == nil || !strings.Contains(err.Error(), "missing.tmpl") {
		t.Fatalf("Render error = %v, want missing template name", err)
	}
}

// TestManagerRenderTextDoesNotRequireRegistration 验证 RenderText 不依赖注册内容。
func TestManagerRenderTextDoesNotRequireRegistration(t *testing.T) {
	manager := prompt.NewManager()

	out, err := manager.RenderText("hello {{ .Name }}", map[string]string{"Name": "boxify"})
	if err != nil {
		t.Fatalf("RenderText error = %v", err)
	}
	if out != "hello boxify" {
		t.Fatalf("RenderText = %q, want hello boxify", out)
	}
}

// TestManagerAppliesRendererOptions 验证 Manager 构造选项可注入模板函数。
func TestManagerAppliesRendererOptions(t *testing.T) {
	manager := prompt.NewManager(prompt.WithFuncs(template.FuncMap{
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

// TestManagerTemplateTextReadsRegisteredTemplate 验证 TemplateText 只读取注册模板原文。
func TestManagerTemplateTextReadsRegisteredTemplate(t *testing.T) {
	manager := prompt.NewManager()
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

// TestZeroValueManagerSupportsRegistration 验证零值 Manager 会通过 ensure 初始化注册所需状态。
func TestZeroValueManagerSupportsRegistration(t *testing.T) {
	manager := &prompt.Manager{}
	if err := manager.RegisterText("custom/example", "hello {{ .Name }}"); err != nil {
		t.Fatalf("RegisterText error = %v, want nil", err)
	}

	out, err := manager.Render("custom/example", map[string]string{"Name": "Cove"})
	if err != nil {
		t.Fatalf("Render error = %v, want nil", err)
	}
	if out != "hello Cove" {
		t.Fatalf("Render output = %q, want hello Cove", out)
	}
}
