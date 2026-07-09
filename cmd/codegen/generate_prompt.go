package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const defaultPromptOutputDir = "internal/prompts/promptsgen"

type promptDefinition struct {
	Name string
	File string
}

type promptTemplate struct {
	Name       string
	MethodName string
	ParamsName string
	Fields     []*promptField
}

// GeneratePrompts 扫描外部提示词注册表和模板 AST，并生成类型化调用代码。
func GeneratePrompts(opts PromptOptions) (Report, error) {
	root := opts.Root
	if root == "" {
		root = "."
	}
	outputDir := opts.Output
	if outputDir == "" {
		outputDir = defaultPromptOutputDir
	}
	if !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(root, outputDir)
	}
	report := Report{Root: root, Command: "prompt", Mode: generationMode(opts.DryRun, opts.Check)}
	if opts.DryRun && opts.Check {
		return report, fmt.Errorf("--dry-run and --check are mutually exclusive")
	}

	definitions, err := scanPromptDefinitions(filepath.Join(root, "internal", "prompts", "prompts.go"))
	if err != nil {
		return report, err
	}
	if opts.Verbose {
		report.AddDiagnostic("info", "prompt.scanned", fmt.Sprintf("scanned %d registered prompts", len(definitions)), "", "")
	}

	templates := make([]promptTemplate, 0, len(definitions))
	for _, definition := range definitions {
		templatePath := filepath.Join(root, "internal", "prompts", definition.File)
		text, err := os.ReadFile(templatePath)
		if err != nil {
			return report, fmt.Errorf("read prompt %s template %s: %w", definition.Name, definition.File, err)
		}
		fields, err := inferPromptFields(definition.Name, definition.File, string(text))
		if err != nil {
			return report, err
		}
		methodName, err := promptExportedName(definition.Name)
		if err != nil {
			return report, err
		}
		templates = append(templates, promptTemplate{
			Name:       definition.Name,
			MethodName: methodName,
			ParamsName: methodName + "Params",
			Fields:     fields,
		})
	}
	sort.Slice(templates, func(i, j int) bool {
		return templates[i].Name < templates[j].Name
	})

	templatesByNamespace := make(map[string][]promptTemplate)
	for _, item := range templates {
		namespace := strings.SplitN(item.Name, "/", 2)[0]
		templatesByNamespace[namespace] = append(templatesByNamespace[namespace], item)
	}
	namespaces := make([]string, 0, len(templatesByNamespace))
	for namespace := range templatesByNamespace {
		namespaces = append(namespaces, namespace)
	}
	sort.Strings(namespaces)

	// 每个 namespace 独立生成，避免无关模板变化导致整个提示词客户端文件重写。
	expectedFiles := make(map[string]struct{}, len(namespaces))
	for _, namespace := range namespaces {
		outputPath := filepath.Join(outputDir, snakeCase(namespace)+"_gen.go")
		expectedFiles[outputPath] = struct{}{}
		body := renderPromptCode(templatesByNamespace[namespace])
		if err := writeGeneratedFile(outputPath, generatedFile("promptsgen", nil, body, true), &report); err != nil {
			return report, err
		}
	}
	if err := removeStalePromptFiles(outputDir, expectedFiles, &report); err != nil {
		return report, err
	}
	return report, nil
}

func removeStalePromptFiles(outputDir string, expectedFiles map[string]struct{}, report *Report) error {
	entries, err := os.ReadDir(outputDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read generated prompt directory %s: %w", outputDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_gen.go") {
			continue
		}
		path := filepath.Join(outputDir, entry.Name())
		if _, ok := expectedFiles[path]; ok {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read stale generated prompt file %s: %w", path, err)
		}
		// 只清理带统一生成标记的文件，手写文件即使名称匹配也不会被删除。
		if !strings.HasPrefix(string(data), generatedHeader) {
			continue
		}
		if report.IsPreview() {
			report.Add(FileWouldDelete, path)
			continue
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove stale generated prompt file %s: %w", path, err)
		}
		report.Add(FileDeleted, path)
	}
	return nil
}

func promptExportedName(name string) (string, error) {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '/' || r == '_' || r == '-' || r == '.'
	})
	if len(parts) == 0 {
		return "", fmt.Errorf("prompt name %q cannot produce a Go identifier", name)
	}
	var out strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		out.WriteString(strings.ToUpper(part[:1]))
		out.WriteString(part[1:])
	}
	if out.Len() == 0 {
		return "", fmt.Errorf("prompt name %q cannot produce a Go identifier", name)
	}
	return out.String(), nil
}

func renderPromptCode(templates []promptTemplate) string {
	var out strings.Builder
	for _, item := range templates {
		writePromptStructs(&out, item.ParamsName, item.MethodName, item.Fields)
		fmt.Fprintf(&out, "// %s 使用 %s 渲染 %s 提示词。\n", item.MethodName, item.ParamsName, item.Name)
		fmt.Fprintf(&out, "func (c *Client) %s(params *%s) (string, error) {\n", item.MethodName, item.ParamsName)
		out.WriteString("\t// 生成方法只传递类型化参数，模板查找和解析统一交给 Renderer。\n")
		fmt.Fprintf(&out, "\treturn c.render(%q, params)\n", item.Name)
		out.WriteString("}\n\n")
	}
	return out.String()
}

func writePromptStructs(out *strings.Builder, paramsName string, prefix string, fields []*promptField) {
	fmt.Fprintf(out, "// %s 定义 %s 提示词允许使用的模板参数。\n", paramsName, prefix)
	fmt.Fprintf(out, "type %s struct {\n", paramsName)
	for _, field := range fields {
		fmt.Fprintf(out, "\t%s %s\n", field.Name, promptFieldGoType(prefix, field))
	}
	out.WriteString("}\n\n")
	for _, field := range fields {
		writeNestedPromptStructs(out, prefix, field)
	}
}

func writeNestedPromptStructs(out *strings.Builder, prefix string, field *promptField) {
	if field.Kind != promptFieldObject && field.Kind != promptFieldSliceObject {
		return
	}
	typeName := promptNestedTypeName(prefix, field.Name)
	fmt.Fprintf(out, "// %s 定义 %s 模板字段的嵌套参数。\n", typeName, field.Name)
	fmt.Fprintf(out, "type %s struct {\n", typeName)
	for _, child := range field.Children {
		fmt.Fprintf(out, "\t%s %s\n", child.Name, promptFieldGoType(typeName, child))
	}
	out.WriteString("}\n\n")
	for _, child := range field.Children {
		writeNestedPromptStructs(out, typeName, child)
	}
}

func promptFieldGoType(prefix string, field *promptField) string {
	switch field.Kind {
	case promptFieldObject:
		return "*" + promptNestedTypeName(prefix, field.Name)
	case promptFieldSliceScalar:
		return "[]string"
	case promptFieldSliceObject:
		return "[]*" + promptNestedTypeName(prefix, field.Name)
	default:
		return "string"
	}
}

func promptNestedTypeName(prefix string, fieldName string) string {
	return prefix + singularPromptName(fieldName)
}

func singularPromptName(value string) string {
	if strings.HasSuffix(value, "ies") && len(value) > 3 {
		return value[:len(value)-3] + "y"
	}
	if strings.HasSuffix(value, "s") && len(value) > 1 {
		return value[:len(value)-1]
	}
	return value
}
