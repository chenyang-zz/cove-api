/**
 * @Time   : 2026/6/23 01:26
 * @Author : chenyangzhao542@gmail.com
 * @File   : manager.go
 **/

// Package prompt 保留旧版目录式提示词管理器。
//
// 本文件兼容 memory/agent 对磁盘模板目录的调用方式。新的通用模板能力在
// template.go、reader.go 和 renderer.go 中实现，Manager 只保留既有入口。
package prompt

import (
	"fmt"
	"path/filepath"
)

type Manager struct {
	root          string
	MemoryPrompts *MemoryPrompts
	AgentPrompts  *AgentPrompts
}

func NewManager(root string) *Manager {
	m := &Manager{
		root: root,
	}
	memoryPrompts := NewMemoryPrompts(m)
	agentPrompts := NewAgentPrompts(m)

	m.MemoryPrompts = memoryPrompts
	m.AgentPrompts = agentPrompts

	return m
}

func (m *Manager) Render(name string, data any) (string, error) {
	path := filepath.Join(m.root, fmt.Sprintf("%s.tmpl", name))
	return renderFile(path, data)
}
