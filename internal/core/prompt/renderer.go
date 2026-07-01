// Package prompt 实现可复用提示词渲染器。
//
// 本文件负责把模板文件系统和模板函数集合绑定到 Renderer 上，适用于同一调用方
// 多次读取或渲染同一批提示词模板的场景。
//
// 核心函数示例：
//
// NewRenderer 创建绑定模板来源的可复用渲染器：
//
//	renderer := prompt.NewRenderer(ragprompt.Templates)
//
// (*Renderer).Render 复用同一个渲染器读取并渲染模板：
//
//	out, err := renderer.Render(ragprompt.ContentClassifierTemplate, data)
//
// (*Renderer).RenderText 复用同一个渲染器的模板函数集合渲染字符串：
//
//	out, err := renderer.RenderText("标签：{{ .Tag }}", map[string]string{"Tag": "技术"})
package prompt

import (
	"fmt"
)

// NewRenderer 创建可复用的提示词渲染器，默认启用项目通用模板函数。
func NewRenderer(fsys TemplateFS, opts ...Option) *Renderer {
	renderer := &Renderer{
		fsys:  fsys,
		funcs: defaultFuncs(),
	}
	// 先写入完整默认值，再应用调用方传入的覆盖项。
	for _, opt := range opts {
		if opt != nil {
			opt(renderer)
		}
	}
	return renderer
}

// Render 使用 Renderer 绑定的文件系统读取并渲染指定模板。
func (r *Renderer) Render(name string, data any) (string, error) {
	if err := r.validateFS(); err != nil {
		return "", err
	}
	// 文件读取和模板执行分开处理，便于错误信息准确落到读取或渲染阶段。
	text, err := r.TemplateText(name)
	if err != nil {
		return "", err
	}
	return r.renderTemplate(name, text, data)
}

// RenderText 使用 Renderer 绑定的函数集合渲染内存中的模板文本。
func (r *Renderer) RenderText(text string, data any) (string, error) {
	if r == nil {
		return "", fmt.Errorf("prompt renderer is nil")
	}
	return r.renderTemplate("prompt", text, data)
}

// renderTemplate 把实例级函数集合注入统一的模板执行流程。
func (r *Renderer) renderTemplate(name string, text string, data any) (string, error) {
	return renderTemplate(name, text, data, r.funcs)
}

// validateFS 统一校验模板文件系统，避免每个读取入口重复 nil 判断。
func (r *Renderer) validateFS() error {
	if r == nil || r.fsys == nil {
		return fmt.Errorf("prompt template fs is nil")
	}
	return nil
}
