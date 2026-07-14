// Package prompt 定义上下文摘要使用的内置模板资源和模板参数。
//
// 本包只声明模板，不负责注册或渲染；渲染继续使用 core/prompt 引擎。
package prompt

import "embed"

// Templates 暴露上下文摘要模板文件。
//
//go:embed *.tmpl
var Templates embed.FS

const (
	// SummaryTemplate 是滚动摘要提示词模板文件名。
	SummaryTemplate = "summary.tmpl"
)

// SummaryData 约束滚动摘要模板可访问的字段。
type SummaryData struct {
	PreviousSummary string
	Transcript      string
}
