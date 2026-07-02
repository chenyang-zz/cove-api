// Package prompt 提供一次性模板读取和渲染入口。
//
// 本文件只放包级便捷函数和统一的 Go template 执行流程。需要复用同一批模板文件
// 或自定义模板函数时，应优先使用 renderer.go 中的 Renderer。
package prompt

import (
	"bytes"
	"fmt"
	"text/template"
)

// TemplateText 从给定文件系统读取原始模板文本，不做解析或渲染。
//
// fsys 为 nil 或模板不存在时返回错误。
func TemplateText(fsys TemplateFS, name string) (string, error) {
	return readTemplate(fsys, name)
}

// Render 从给定文件系统读取模板并立即渲染，适合一次性使用场景。
//
// fsys 为 nil、模板读取失败、模板语法错误或执行失败时返回错误。
func Render(fsys TemplateFS, name string, data any) (string, error) {
	return NewRenderer(fsys).Render(name, data)
}

// RenderText 渲染已经在内存中的模板文本，适合外部系统直接提供提示词内容的场景。
//
// 模板语法错误或执行失败时返回错误。该函数使用默认模板函数集合。
func RenderText(text string, data any) (string, error) {
	return renderTemplate("prompt", text, data, defaultFuncs())
}

// renderTemplate 执行统一的 Go template 解析和渲染流程。
func renderTemplate(name string, text string, data any, funcs template.FuncMap) (string, error) {
	// 提示词模板统一使用 text/template，避免 HTML 自动转义影响模型输入。
	tpl, err := template.New(name).Funcs(funcs).Parse(text)
	if err != nil {
		return "", fmt.Errorf("parse prompt %s failed: %w", name, err)
	}

	// 解析成功后再执行模板，确保调用方能区分语法错误和数据渲染错误。
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render prompt %s failed: %w", name, err)
	}
	return buf.String(), nil
}
