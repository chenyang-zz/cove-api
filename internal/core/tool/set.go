package tool

import (
	"context"
	"errors"
)

// StaticSet 是由固定工具列表构成的 ToolSet。
type StaticSet struct {
	descriptor SetDescriptor
	tools      []Tool
}

// NewStaticSet 使用 descriptor 和 tools 创建固定工具集。
//
// descriptor 会被浅拷贝，tools 切片也会被复制，避免调用方后续修改影响工具集。
func NewStaticSet(descriptor SetDescriptor, tools ...Tool) *StaticSet {
	copied := make([]Tool, len(tools))
	copy(copied, tools)
	return &StaticSet{
		descriptor: cloneSetDescriptor(descriptor),
		tools:      copied,
	}
}

// Describe 返回工具集描述。
func (s *StaticSet) Describe(ctx context.Context) (SetDescriptor, error) {
	if s == nil {
		return SetDescriptor{}, errors.New("tool set is nil")
	}
	return cloneSetDescriptor(s.descriptor), nil
}

// Tools 返回工具集内的工具列表。
func (s *StaticSet) Tools(ctx context.Context) ([]Tool, error) {
	if s == nil {
		return nil, errors.New("tool set is nil")
	}
	copied := make([]Tool, len(s.tools))
	copy(copied, s.tools)
	return copied, nil
}
