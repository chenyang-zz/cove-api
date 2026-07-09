// Package prompt 定义 Agent 核心实现内置的提示词模板资源和模板参数。
//
// 本包只声明模板文件、模板名称和模板变量；读取与渲染继续复用 core/prompt 引擎。
// 内置模板随 core 一同发布，不需要注册到 internal/prompts。
package prompt

import "embed"

// Templates 暴露 Agent 内置提示词模板文件。
//
//go:embed *.tmpl
var Templates embed.FS

const (
	// ReActSystemTemplate 是 ReAct 系统提示词模板文件名。
	ReActSystemTemplate = "react_system.tmpl"
)

// ReActToolData 约束 ReAct 系统模板可使用的工具字段。
type ReActToolData struct {
	Name        string
	Description string
}

// ReActSystemData 约束 ReAct 系统模板可使用的变量。
type ReActSystemData struct {
	Tools        []ReActToolData
	SystemPrompt string
}
