// Package prompt 实现 Manager 的模板渲染入口。
//
// 本文件只负责把 Manager 查到的模板文本交给统一渲染流程，不处理模板来源注册。
package prompt

// Render 读取指定名称的模板并使用 data 渲染。
//
// data 可以是结构体、指向结构体的指针或 map。模板读取失败、模板语法错误和
// 执行错误都会作为 error 返回。
func (m *Manager) Render(name string, data any) (string, error) {
	text, err := m.TemplateText(name)
	if err != nil {
		return "", err
	}
	return m.renderTemplate(name, text, data)
}

// RenderText 直接渲染传入的模板文本，不依赖 Manager 注册内容。
//
// RenderText 会复用 Manager 的模板函数集合，适合外部已经解析好提示词来源、
// 只需要执行 Go template 的场景。
func (m *Manager) RenderText(text string, data any) (string, error) {
	m.ensure()
	return m.renderTemplate("prompt", text, data)
}

// renderTemplate 使用 Manager 当前函数集合执行统一模板渲染流程。
func (m *Manager) renderTemplate(name string, text string, data any) (string, error) {
	m.ensure()
	return renderTemplate(name, text, data, m.funcs)
}
