// Package prompt 定义提示词模板管理和渲染的核心类型。
//
// 本文件只放跨文件复用的结构体和接口，具体读取、注册、渲染流程分别放在
// reader.go、manager_register.go 和 manager_render.go 中。
package prompt

import (
	"io/fs"
	"text/template"
)

// TemplateFS 表示承载提示词模板文件的文件系统。
type TemplateFS interface {
	fs.FS
}

// Renderer 绑定一组模板文件和模板函数，用于重复读取或渲染同一来源的提示词。
type Renderer struct {
	fsys  TemplateFS
	funcs template.FuncMap
}

// Manager 管理多个显式注册的提示词来源。
//
// Manager 的模板查找顺序固定为：内存文本模板、已注册命名空间文件系统。
// Manager 不做并发写保护；注册模板来源应在并发渲染前完成。
type Manager struct {
	texts   map[string]string
	sources map[string]TemplateFS
	funcs   template.FuncMap
}
