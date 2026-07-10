package toolconfig

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/google/uuid"
)

// ListToolConfigsLogic contains the listToolConfigs use case.
type ListToolConfigsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewListToolConfigsLogic creates a ListToolConfigsLogic.
func NewListToolConfigsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListToolConfigsLogic {
	return &ListToolConfigsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.toolconfig.listtoolconfigs"),
	}
}

// ListToolConfigs 查询工具配置列表
func (l *ListToolConfigsLogic) ListToolConfigs(userID uuid.UUID) (*response.ToolConfigListResponse, error) {
	builtinTools, err := builtinToolResponses(l.ctx, l.svcCtx)
	if err != nil {
		return nil, err
	}
	configRows, err := l.svcCtx.ToolConfigRepo.List(l.ctx, userID)
	if err != nil {
		return nil, err
	}
	serverRows, err := l.svcCtx.MCPServerRepo.List(l.ctx, userID)
	if err != nil {
		return nil, err
	}

	enabledByKey := toolEnabledMap(configRows)
	for _, item := range builtinTools {
		if enabled, ok := enabledByKey[item.ToolKey]; ok {
			item.Enabled = enabled
		}
	}
	return mapper.ToolConfigListToResponse(
		builtinTools,
		mcpToolGroups(l.ctx, l.svcCtx, serverRows, enabledByKey, l.log),
	), nil
}
