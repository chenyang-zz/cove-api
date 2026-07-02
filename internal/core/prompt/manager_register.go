// Package prompt 实现 Manager 的模板注册和查找能力。
//
// 本文件负责维护 Manager 的模板来源，不执行模板渲染。模板来源包括内存文本、
// 命名空间文件系统和旧 root 目录。
package prompt

import (
	"fmt"
	"path/filepath"
	"strings"
)

// RegisterFS 注册一个模板文件系统来源，namespace 会作为模板名称的第一段。
//
// namespace 不能为空，fsys 不能为 nil。注册后，Render("namespace/file.tmpl", data)
// 会从该文件系统读取 file.tmpl。同名内存文本模板的优先级高于文件系统模板。
func (m *Manager) RegisterFS(namespace string, fsys TemplateFS) error {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return fmt.Errorf("prompt namespace is empty")
	}
	if fsys == nil {
		return fmt.Errorf("prompt template fs is nil")
	}
	m.ensure()
	m.sources[namespace] = fsys
	return nil
}

// RegisterText 注册一段内存模板文本，name 是后续读取和渲染使用的完整名称。
//
// name 不能为空。内存模板按完整名称精确匹配，并优先于 RegisterFS 和旧 root
// 目录中的同名模板。
func (m *Manager) RegisterText(name string, text string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("prompt name is empty")
	}
	m.ensure()
	m.texts[name] = text
	return nil
}

// TemplateText 按名称读取模板原文，不解析模板语法。
//
// 读取顺序为内存文本模板、已注册 namespace 文件系统、旧 root 目录。root fallback
// 会在名称没有扩展名时自动追加 .tmpl，用于兼容旧 memory/agent 模板调用。
func (m *Manager) TemplateText(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("prompt name is empty")
	}
	m.ensure()

	// 内存模板来自数据库或配置中心，使用完整名称精确匹配，并覆盖其他来源。
	if text, ok := m.texts[name]; ok {
		return text, nil
	}

	namespace, templateName, err := splitRegisteredName(name)
	if err == nil {
		if fsys, ok := m.sources[namespace]; ok {
			return readTemplate(fsys, templateName)
		}
	}

	// root fallback 是旧 Manager 行为；只有没有 root 时才把 namespace 缺失视为错误。
	if m.root == "" {
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("prompt namespace %q is not registered", namespace)
	}
	return readTemplateFile(m.root, name)
}

// splitRegisteredName 把 Manager 注册名称拆分为 namespace 和模板相对路径。
func splitRegisteredName(name string) (string, string, error) {
	parts := strings.SplitN(name, "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("prompt name %q must be namespace/template", name)
	}
	return parts[0], parts[1], nil
}

// rootTemplateName 返回旧 root 目录下的模板相对路径。
func rootTemplateName(name string) string {
	if filepath.Ext(name) == "" {
		return fmt.Sprintf("%s.tmpl", name)
	}
	return name
}
