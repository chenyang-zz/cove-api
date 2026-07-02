/**
 * @Time   : 2026/6/28 23:24
 * @Author : chenyangzhao542@gmail.com
 * @File   : agent.go
 **/

// Package prompt 封装 agent 相关提示词入口。
//
// 本文件只负责把 agent 业务侧方法名映射到 prompt/agent 目录下的模板文件，模板读取
// 和渲染仍由 Manager 统一处理。
package prompt

import "fmt"

// OptimizePromptData 表示优化智能体提示词模板需要的数据。
type OptimizePromptData struct {
	RawPrompt string
}

// AgentPrompts 提供 agent 模板的类型化入口。
type AgentPrompts struct {
	namespace string
	manager   *Manager
}

// NewAgentPrompts 创建 agent 提示词入口。
func NewAgentPrompts(manager *Manager) *AgentPrompts {
	return &AgentPrompts{
		namespace: "agent",
		manager:   manager,
	}
}

// OptimizePrompt 渲染提示词优化模板。
func (p *AgentPrompts) OptimizePrompt(data *OptimizePromptData) (string, error) {
	return p._render("optimize_prompt", data)
}

// _render 拼接 agent 模板名称，并交给 Manager 统一查找和渲染。
func (p *AgentPrompts) _render(promptName string, data any) (string, error) {
	return p.manager.Render(fmt.Sprintf("%s/%s", p.namespace, promptName), data)
}
