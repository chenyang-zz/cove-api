package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGeneratePromptsInfersTemplateParameterTypes 验证生成器能从模板 AST 推断普通字段、嵌套结构、字符串切片和结构体切片。
func TestGeneratePromptsInfersTemplateParameterTypes(t *testing.T) {
	root := t.TempDir()
	writePromptFixture(t, root, `package prompts

type definition struct {
	name string
	file string
}

func registeredPrompts() []definition {
	return []definition{
		{name: "agent/example", file: "example.tmpl"},
	}
}
`, `{{ .Content }}
{{ .Entity.Name }}
{{ join ", " .Aliases }}
{{ range .Tags }}{{ . }}{{ end }}
{{ range .Members }}{{ .Name }}{{ .Description }}{{ end }}
`)

	report, err := GeneratePrompts(PromptOptions{Root: root})
	if err != nil {
		t.Fatalf("GeneratePrompts error = %v, want nil", err)
	}
	outputPath := filepath.Join(root, defaultPromptOutputDir, "agent_gen.go")
	outputName := filepath.ToSlash(filepath.Join(defaultPromptOutputDir, "agent_gen.go"))
	if !report.Has(FileAdded, outputName) {
		t.Fatalf("GeneratePrompts report = %+v, want added %s", report.Files, outputName)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", outputPath, err)
	}
	output := string(data)
	compactOutput := strings.Join(strings.Fields(output), " ")
	for _, want := range []string{
		"type AgentExampleParams struct",
		"Content string",
		"Entity *AgentExampleEntity",
		"Aliases []string",
		"Tags []string",
		"Members []*AgentExampleMember",
		"type AgentExampleEntity struct",
		"Name string",
		"type AgentExampleMember struct",
		"Description string",
		"func (c *Client) AgentExample(params *AgentExampleParams) (string, error)",
		`return c.render("agent/example", params)`,
	} {
		if !strings.Contains(compactOutput, want) {
			t.Fatalf("generated prompt code missing %q:\n%s", want, output)
		}
	}
}

// TestGeneratePromptsRejectsConflictingTypes 验证同一模板字段同时作为标量和对象使用时返回类型冲突错误。
func TestGeneratePromptsRejectsConflictingTypes(t *testing.T) {
	root := t.TempDir()
	writePromptFixture(t, root, `package prompts

type definition struct {
	name string
	file string
}

func registeredPrompts() []definition {
	return []definition{{name: "agent/conflict", file: "example.tmpl"}}
}
`, `{{ .Value }} {{ .Value.Name }}`)

	_, err := GeneratePrompts(PromptOptions{Root: root})
	if err == nil {
		t.Fatal("GeneratePrompts error = nil, want type conflict")
	}
	if !strings.Contains(err.Error(), "Value") || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("GeneratePrompts error = %v, want field conflict details", err)
	}
}

// TestGeneratePromptsCheckReportsStaleOutput 验证 check 模式发现生成文件缺失时只报告差异且不写文件。
func TestGeneratePromptsCheckReportsStaleOutput(t *testing.T) {
	root := t.TempDir()
	writePromptFixture(t, root, `package prompts

type definition struct {
	name string
	file string
}

func registeredPrompts() []definition {
	return []definition{{name: "agent/example", file: "example.tmpl"}}
}
`, `{{ .Content }}`)

	report, err := GeneratePrompts(PromptOptions{Root: root, Check: true})
	if err != nil {
		t.Fatalf("GeneratePrompts check error = %v, want nil", err)
	}
	outputName := filepath.ToSlash(filepath.Join(defaultPromptOutputDir, "agent_gen.go"))
	if !report.Has(FileWouldAdd, outputName) {
		t.Fatalf("GeneratePrompts check report = %+v, want would-add %s", report.Files, outputName)
	}
	if _, err := os.Stat(filepath.Join(root, outputName)); !os.IsNotExist(err) {
		t.Fatalf("generated output stat error = %v, want not exist", err)
	}
}

// TestGeneratePromptsRejectsMissingTemplate 验证注册表引用不存在的模板文件时返回包含逻辑名称和文件名的错误。
func TestGeneratePromptsRejectsMissingTemplate(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/prompts/prompts.go", `package prompts

type definition struct {
	name string
	file string
}

func registeredPrompts() []definition {
	return []definition{{name: "agent/missing", file: "missing.tmpl"}}
}
`)

	_, err := GeneratePrompts(PromptOptions{Root: root})
	if err == nil {
		t.Fatal("GeneratePrompts error = nil, want missing template error")
	}
	if !strings.Contains(err.Error(), "agent/missing") || !strings.Contains(err.Error(), "missing.tmpl") {
		t.Fatalf("GeneratePrompts error = %v, want logical name and file", err)
	}
}

// TestGeneratePromptsIsIdempotentAndProtectsManualFiles 验证重复生成保持不变，并拒绝覆盖没有生成标记的手写文件。
func TestGeneratePromptsIsIdempotentAndProtectsManualFiles(t *testing.T) {
	root := t.TempDir()
	writePromptFixture(t, root, `package prompts

type definition struct {
	name string
	file string
}

func registeredPrompts() []definition {
	return []definition{{name: "agent/example", file: "example.tmpl"}}
}
`, `{{ .Content }}`)

	if _, err := GeneratePrompts(PromptOptions{Root: root}); err != nil {
		t.Fatalf("GeneratePrompts first run error = %v, want nil", err)
	}
	report, err := GeneratePrompts(PromptOptions{Root: root})
	if err != nil {
		t.Fatalf("GeneratePrompts second run error = %v, want nil", err)
	}
	outputName := filepath.ToSlash(filepath.Join(defaultPromptOutputDir, "agent_gen.go"))
	if !report.Has(FileUnchanged, outputName) {
		t.Fatalf("GeneratePrompts second report = %+v, want unchanged %s", report.Files, outputName)
	}

	outputPath := filepath.Join(root, outputName)
	if err := os.WriteFile(outputPath, []byte("package promptsgen\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(manual output) error = %v", err)
	}
	_, err = GeneratePrompts(PromptOptions{Root: root})
	if err == nil || !strings.Contains(err.Error(), "refuse to overwrite non-codegen file") {
		t.Fatalf("GeneratePrompts manual output error = %v, want overwrite refusal", err)
	}
}

// TestGeneratePromptsSplitsOutputByNamespace 验证不同 namespace 会生成独立文件且不会继续生成单体 prompts_gen.go。
func TestGeneratePromptsSplitsOutputByNamespace(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/prompts/prompts.go", `package prompts

type definition struct {
	name string
	file string
}

func registeredPrompts() []definition {
	return []definition{
		{name: "agent/example", file: "agent.tmpl"},
		{name: "memory/summary", file: "memory.tmpl"},
	}
}
`)
	writeFile(t, root, "internal/prompts/agent.tmpl", `{{ .Content }}`)
	writeFile(t, root, "internal/prompts/memory.tmpl", `{{ .Members }}`)

	report, err := GeneratePrompts(PromptOptions{Root: root})
	if err != nil {
		t.Fatalf("GeneratePrompts error = %v, want nil", err)
	}
	for _, name := range []string{"agent_gen.go", "memory_gen.go"} {
		path := filepath.ToSlash(filepath.Join(defaultPromptOutputDir, name))
		if !report.Has(FileAdded, path) {
			t.Fatalf("GeneratePrompts report = %+v, want added %s", report.Files, path)
		}
	}
	if _, err := os.Stat(filepath.Join(root, defaultPromptOutputDir, "prompts_gen.go")); !os.IsNotExist(err) {
		t.Fatalf("prompts_gen.go stat error = %v, want not exist", err)
	}
}

// TestGeneratePromptsRemovesStaleNamespaceFile 验证写入模式删除多余生成文件，check 模式只报告待删除。
func TestGeneratePromptsRemovesStaleNamespaceFile(t *testing.T) {
	root := t.TempDir()
	writePromptFixture(t, root, `package prompts

type definition struct {
	name string
	file string
}

func registeredPrompts() []definition {
	return []definition{{name: "agent/example", file: "example.tmpl"}}
}
`, `{{ .Content }}`)
	stalePath := filepath.Join(root, defaultPromptOutputDir, "legacy_gen.go")
	writeFile(t, root, filepath.ToSlash(filepath.Join(defaultPromptOutputDir, "legacy_gen.go")), generatedHeader+"\n\npackage promptsgen\n")

	report, err := GeneratePrompts(PromptOptions{Root: root, Check: true})
	if err != nil {
		t.Fatalf("GeneratePrompts check error = %v, want nil", err)
	}
	staleName := filepath.ToSlash(filepath.Join(defaultPromptOutputDir, "legacy_gen.go"))
	if !report.Has(FileWouldDelete, staleName) {
		t.Fatalf("GeneratePrompts check report = %+v, want would-delete %s", report.Files, staleName)
	}
	if _, err := os.Stat(stalePath); err != nil {
		t.Fatalf("stale generated file stat error = %v, want file retained in check mode", err)
	}

	if _, err := GeneratePrompts(PromptOptions{Root: root}); err != nil {
		t.Fatalf("GeneratePrompts write error = %v, want nil", err)
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("stale generated file stat error = %v, want removed", err)
	}
}

// TestGeneratePromptsRejectsDuplicateRegistrations 验证重复逻辑名称或物理模板文件会在读取模板前被拒绝。
func TestGeneratePromptsRejectsDuplicateRegistrations(t *testing.T) {
	root := t.TempDir()
	writePromptFixture(t, root, `package prompts

type definition struct {
	name string
	file string
}

func registeredPrompts() []definition {
	return []definition{
		{name: "agent/example", file: "example.tmpl"},
		{name: "agent/example", file: "other.tmpl"},
	}
}
`, `{{ .Content }}`)

	_, err := GeneratePrompts(PromptOptions{Root: root})
	if err == nil || !strings.Contains(err.Error(), "registered more than once") {
		t.Fatalf("GeneratePrompts duplicate registration error = %v, want duplicate error", err)
	}
}

// TestGeneratePromptsRejectsInvalidLogicalName 验证逻辑名称必须由可稳定映射为 Go 标识符的路径段组成。
func TestGeneratePromptsRejectsInvalidLogicalName(t *testing.T) {
	root := t.TempDir()
	writePromptFixture(t, root, `package prompts

type definition struct {
	name string
	file string
}

func registeredPrompts() []definition {
	return []definition{{name: "agent/1example", file: "example.tmpl"}}
}
`, `{{ .Content }}`)

	_, err := GeneratePrompts(PromptOptions{Root: root})
	if err == nil || !strings.Contains(err.Error(), "must start with a letter") {
		t.Fatalf("GeneratePrompts invalid logical name error = %v, want validation error", err)
	}
}

func writePromptFixture(t *testing.T, root string, registry string, templateText string) {
	t.Helper()
	writeFile(t, root, "internal/prompts/prompts.go", registry)
	writeFile(t, root, "internal/prompts/example.tmpl", templateText)
}
