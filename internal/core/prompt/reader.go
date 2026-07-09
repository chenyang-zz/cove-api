// Package prompt 负责模板文件读取。
//
// 本文件把读取逻辑限制在“拿到模板文本”这一层，不解析模板语法。所有模板来源
// 都通过 fs.FS 读取，解析和执行统一交给 template.go。
package prompt

import (
	"fmt"
	"io/fs"
)

// TemplateText 从 Renderer 绑定的文件系统读取原始模板文本，不解析模板语法。
func (r *Renderer) TemplateText(name string) (string, error) {
	if err := r.validateFS(); err != nil {
		return "", err
	}
	return readTemplate(r.fsys, name)
}

// readTemplate 只负责读取模板文件，不做模板语法解析。
func readTemplate(fsys TemplateFS, name string) (string, error) {
	if fsys == nil {
		return "", fmt.Errorf("prompt template fs is nil")
	}
	// 使用 fs.ReadFile 让 embed.FS、os.DirFS、测试 fake FS 走同一条读取路径。
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return "", fmt.Errorf("read prompt %s failed: %w", name, err)
	}
	return string(data), nil
}
