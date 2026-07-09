// Package promptsgen 提供由提示词注册表生成的类型化渲染客户端。
//
// Client 不负责注册或持有模板文件。调用方应先将模板注册到 Renderer，再通过
// NewClient 创建类型化入口。各 namespace 的 *_gen.go 文件由 codegen prompt 完全维护。
package promptsgen

import "fmt"

// Renderer 定义生成客户端渲染提示词所需的最小能力。
type Renderer interface {
	Render(name string, data any) (string, error)
}

// Client 通过生成方法把类型化参数交给通用 Renderer。
type Client struct {
	renderer Renderer
}

// NewClient 创建类型化提示词客户端。
//
// renderer 可以为 nil，但调用生成方法时会返回错误。
func NewClient(renderer Renderer) *Client {
	return &Client{renderer: renderer}
}

func (c *Client) render(name string, data any) (string, error) {
	if c == nil || c.renderer == nil {
		return "", fmt.Errorf("prompt renderer is nil")
	}
	return c.renderer.Render(name, data)
}
