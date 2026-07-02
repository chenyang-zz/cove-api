// Package prompt 定义提示词渲染器的长期配置项。
//
// 本文件只放构造期选项。请求级变量应通过 Render/RenderText 的 data 参数传入，
// 不应放进 Option，避免 Renderer 持有业务状态。
package prompt

import (
	"text/template"

	"github.com/Masterminds/sprig/v3"
)

// Option 用于调整 Renderer 和 Manager 共享的长期模板配置。
type Option func(*Renderer)

// WithFuncs 覆盖模板函数集合，调用方可注入自己的 Go template helper。
//
// funcs 为 nil 时不会覆盖默认函数集合。
func WithFuncs(funcs template.FuncMap) Option {
	return func(renderer *Renderer) {
		if funcs != nil {
			renderer.funcs = funcs
		}
	}
}

// defaultFuncs 保留项目既有 sprig 模板函数能力。
func defaultFuncs() template.FuncMap {
	return sprig.TxtFuncMap()
}
