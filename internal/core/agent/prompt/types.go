// Package prompt 定义 Agent 体系默认提示词模板资源和模板参数结构。
//
// 本包只声明模板文件、模板名称和模板变量，不读取、不解析、不渲染模板。调用方需要
// 通过 internal/core/prompt 包完成模板管理。当前仅包含 ReAct 默认模板，后续其他
// Agent 类型的提示词定义也应集中放在本包。
package prompt

import "embed"

// Templates 暴露 Agent 默认提示词模板文件，具体读取和渲染由 core/prompt 负责。
//
//go:embed *.tmpl
var Templates embed.FS

const (
	// ReActSystemTemplate 是 ReAct 系统提示词模板文件名。
	ReActSystemTemplate = "react_system.tmpl"
)

// ReActToolData 约束 ReAct 系统模板可使用的工具字段。
type ReActToolData struct {
	// Name 是模型可调用的工具名称。
	Name string
	// Description 是工具用途描述。
	Description string
}

// ReActSystemData 约束 ReAct 系统默认模板可使用的变量。
type ReActSystemData struct {
	// Tools 是默认模板中展示给模型的工具清单。
	Tools []ReActToolData
	// SystemPrompt 是调用方传入的业务身份、人设或风格要求。
	SystemPrompt string
}
