package tools

import (
	"context"
	"fmt"

	coretool "github.com/boxify/api-go/internal/core/tool"
	"github.com/boxify/api-go/internal/domain/tools/builtin"
	"github.com/boxify/api-go/internal/svc"
)

const (
	// ToolSetSystem 是领域层系统工具集名称。
	ToolSetSystem = "system"
	// ToolSetKnowledge 是领域层知识库工具集名称。
	ToolSetKnowledge = "knowledge"
	// ToolCurrentTime 是获取当前时间的工具名称。
	ToolCurrentTime = builtin.ToolCurrentTime
	// ToolKnowledgeSearch 是知识库检索工具名称。
	ToolKnowledgeSearch = builtin.ToolKnowledgeSearch
)

// NewCatalog 创建并返回领域层本地工具目录。
//
// 返回的 Catalog 始终包含 system 和 knowledge 工具集。svcCtx 为 nil 时返回错误；
// opts 会应用到所有需要长期配置的领域工具；注册工具集失败时返回错误。
func NewCatalog(svcCtx *svc.ServiceContext, opts ...builtin.Option) (*coretool.Catalog, error) {
	if svcCtx == nil {
		return nil, fmt.Errorf("service context is nil")
	}
	catalog := coretool.NewCatalog()
	systemSet := coretool.NewStaticSet(coretool.SetDescriptor{
		Name:        ToolSetSystem,
		Description: "提供运行上下文相关的基础系统工具。",
		Tags:        []string{"system"},
	}, builtin.NewCurrentTimeTool(opts...))
	if err := catalog.RegisterSet(context.Background(), systemSet); err != nil {
		return nil, fmt.Errorf("register system tool set: %w", err)
	}
	knowledgeSet := coretool.NewStaticSet(coretool.SetDescriptor{
		Name:        ToolSetKnowledge,
		Description: "提供可信知识库范围内的检索工具。",
		Tags:        []string{"knowledge", "rag"},
	}, builtin.NewKnowledgeSearchTool(svcCtx))
	if err := catalog.RegisterSet(context.Background(), knowledgeSet); err != nil {
		return nil, fmt.Errorf("register knowledge tool set: %w", err)
	}
	return catalog, nil
}
