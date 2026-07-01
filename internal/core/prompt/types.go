// Package prompt 定义提示词模板渲染的核心抽象。
//
// 本文件只放跨文件复用的类型：模板来源统一抽象为 TemplateFS，渲染状态统一
// 收敛到 Renderer，避免调用方依赖具体的 embed.FS、os.DirFS 或测试 fake FS。
//
// 核心类型示例：
//
// TemplateFS 可接收 embed.FS、os.DirFS 或测试用 fake FS：
//
//	renderer := prompt.NewRenderer(fsys)
//
// Renderer 可配合 Option 复用自定义模板函数：
//
//	renderer := prompt.NewRenderer(fsys, prompt.WithFuncs(funcs))
package prompt

import (
	"io/fs"
	"text/template"
)

// TemplateFS 表示承载提示词模板文件的文件系统。
type TemplateFS interface {
	fs.FS
}

// Renderer 绑定一组模板文件和模板函数，用于重复渲染同一来源的提示词。
type Renderer struct {
	fsys  TemplateFS
	funcs template.FuncMap
}
