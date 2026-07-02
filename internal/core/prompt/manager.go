// Package prompt 实现提示词管理器的构造入口。
//
// 本文件只负责创建 Manager、初始化默认模板函数，以及挂载旧 memory/agent
// 便捷入口；模板注册、读取和渲染逻辑由其他文件负责。
package prompt

// NewManager 创建提示词管理器，并兼容旧的 root 目录模板读取方式。
//
// root 可以为空；为空时 Manager 只使用 RegisterText 或 RegisterFS 注册的模板来源。
// opts 会覆盖默认模板函数集合。返回的 Manager 已初始化 MemoryPrompts 和 AgentPrompts。
func NewManager(root string, opts ...Option) *Manager {
	renderer := NewRenderer(nil, opts...)
	m := &Manager{
		root:    root,
		texts:   make(map[string]string),
		sources: make(map[string]TemplateFS),
		funcs:   renderer.funcs,
	}

	// Manager 先完成自身默认值初始化，再挂载旧的业务提示词包装，避免包装器拿到半初始化状态。
	m.MemoryPrompts = NewMemoryPrompts(m)
	m.AgentPrompts = NewAgentPrompts(m)

	return m
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
