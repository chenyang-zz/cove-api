// Package prompt 负责模板文件读取。
//
// 本文件把读取逻辑限制在“拿到模板文本”这一层，不解析模板语法；解析和执行
// 统一交给 template.go，旧 Manager 的磁盘文件读取也在这里兼容。
//
// 核心函数示例：
//
// (*Renderer).TemplateText 从 Renderer 绑定的模板来源读取原文：
//
//	text, err := renderer.TemplateText("content_classifier.tmpl")
package prompt

import (
	"fmt"
	"io/fs"
	"os"
)

// TemplateText 从 Renderer 绑定的文件系统读取原始模板文本。
func (r *Renderer) TemplateText(name string) (string, error) {
	if err := r.validateFS(); err != nil {
		return "", err
	}
	return readTemplate(r.fsys, name)
}

// readTemplate 只负责读取模板文件，不做模板语法解析。
func readTemplate(fsys TemplateFS, name string) (string, error) {
	if fsys == nil {
		return "", fmt.Errorf("prompt template fs is nil")
	}
	// 使用 fs.ReadFile 让 embed.FS、os.DirFS、测试 fake FS 走同一条读取路径。
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return "", fmt.Errorf("read prompt %s failed: %w", name, err)
	}
	return string(data), nil
}

// renderFile 兼容旧 Manager 的磁盘路径渲染方式。
func renderFile(path string, data any) (string, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read prompt %s failed: %w", path, err)
	}
	return renderTemplate(path, string(text), data, defaultFuncs())
}
