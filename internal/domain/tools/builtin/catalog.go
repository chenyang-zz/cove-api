package builtin

import (
	"context"
	"fmt"

	coretool "github.com/boxify/api-go/internal/core/tool"
	"github.com/boxify/api-go/internal/svc"
)

const (
	// ToolSetSystem 是内置系统工具集名称。
	ToolSetSystem = "system"
	// ToolSetKnowledge 是内置知识库工具集名称。
	ToolSetKnowledge = "knowledge"
	// ToolCurrentTime 是获取当前时间的内置工具名称。
	ToolCurrentTime = "current_time"
	// ToolKnowledgeSearch 是知识库检索内置工具名称。
	ToolKnowledgeSearch = "knowledge_search"
)

// NewCatalog 创建并返回内置领域工具目录。
//
// 返回的 Catalog 始终包含 system 和 knowledge 工具集。svcCtx 为 nil 时返回错误；
// opts 会应用到所有需要长期配置的内置工具；注册工具集失败时返回错误。
func NewCatalog(svcCtx *svc.ServiceContext, opts ...Option) (*coretool.Catalog, error) {
	if svcCtx == nil {
		return nil, fmt.Errorf("service context is nil")
	}
	cfg := applyOptions(opts...)
	catalog := coretool.NewCatalog()
	systemSet := coretool.NewStaticSet(coretool.SetDescriptor{
		Name:        ToolSetSystem,
		Description: "提供运行上下文相关的基础系统工具。",
		Tags:        []string{"system"},
	}, newCurrentTimeTool(cfg))
	if err := catalog.RegisterSet(context.Background(), systemSet); err != nil {
		return nil, fmt.Errorf("register system tool set: %w", err)
	}
	knowledgeSet := coretool.NewStaticSet(coretool.SetDescriptor{
		Name:        ToolSetKnowledge,
		Description: "提供可信知识库范围内的检索工具。",
		Tags:        []string{"knowledge", "rag"},
	}, newKnowledgeSearchTool(svcCtx))
	if err := catalog.RegisterSet(context.Background(), knowledgeSet); err != nil {
		return nil, fmt.Errorf("register knowledge tool set: %w", err)
	}
	return catalog, nil
}
