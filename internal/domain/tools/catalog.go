package tools

import (
	coretool "github.com/boxify/api-go/internal/core/tool"
	"github.com/boxify/api-go/internal/domain/tools/builtin"
	"github.com/boxify/api-go/internal/svc"
)

const (
	// ToolSetSystem 是领域层系统工具集名称。
	ToolSetSystem = builtin.ToolSetSystem
	// ToolSetKnowledge 是领域层知识库工具集名称。
	ToolSetKnowledge = builtin.ToolSetKnowledge
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
	return builtin.NewCatalog(svcCtx, opts...)
}
