// Package prompt 实现提示词管理器的构造入口。
//
// 本文件只负责创建 Manager 和初始化默认模板函数；模板注册、读取和渲染逻辑由其他文件负责。
package prompt

// NewManager 创建只使用显式注册模板来源的提示词管理器。
//
// opts 会覆盖默认模板函数集合。磁盘模板应通过 RegisterFS 和 os.DirFS 显式注册。
func NewManager(opts ...Option) *Manager {
	renderer := NewRenderer(nil, opts...)
	return &Manager{
		texts:   make(map[string]string),
		sources: make(map[string]TemplateFS),
		funcs:   renderer.funcs,
	}
}

// ensure 补齐 Manager 的零值字段，兼容测试或调用方直接构造 Manager 的情况。
func (m *Manager) ensure() {
	if m.texts == nil {
		m.texts = make(map[string]string)
	}
	if m.sources == nil {
		m.sources = make(map[string]TemplateFS)
	}
	if m.funcs == nil {
		m.funcs = defaultFuncs()
	}
}
