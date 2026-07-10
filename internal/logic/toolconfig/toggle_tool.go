package toolconfig

import (
	"context"
	"log/slog"
	"strings"

	domaintoolmcp "github.com/boxify/api-go/internal/domain/tools/mcp"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

// ToggleToolLogic contains the toggleTool use case.
type ToggleToolLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewToggleToolLogic creates a ToggleToolLogic.
func NewToggleToolLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ToggleToolLogic {
	return &ToggleToolLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.toolconfig.toggletool"),
	}
}

// ToggleTool 开启/关闭工具
func (l *ToggleToolLogic) ToggleTool(userID uuid.UUID, input *request.ToggleToolRequest) error {
	if input == nil || input.Enabled == nil {
		return xerr.BadRequest("工具开关不能为空")
	}
	toolKey := strings.TrimSpace(input.ToolKey)
	toolType, err := l.resolveToolType(userID, toolKey)
	if err != nil {
		return err
	}

	rows, err := l.svcCtx.ToolConfigRepo.List(l.ctx, userID)
	if err != nil {
		return err
	}
	existing := findToolConfig(rows, toolKey)
	if existing == nil {
		_, err = l.svcCtx.ToolConfigRepo.Create(l.ctx, userID, &models.ToolConfig{
			ID:       uuid.New(),
			ToolKey:  toolKey,
			ToolType: toolType,
			Enabled:  *input.Enabled,
			Config:   models.JSONMap{},
		})
	} else {
		_, err = l.svcCtx.ToolConfigRepo.UpdateFields(
			l.ctx,
			userID,
			existing.ID,
			&models.ToolConfig{Enabled: *input.Enabled},
			repository.NewToolConfigUpdateFields().Enabled(),
		)
	}
	if err != nil {
		return err
	}

	l.log.InfoContext(l.ctx, "切换工具开关",
		slog.String("user_id", userID.String()),
		slog.String("tool_key", toolKey),
		slog.Bool("enabled", *input.Enabled),
	)
	return nil
}

func (l *ToggleToolLogic) resolveToolType(userID uuid.UUID, toolKey string) (string, error) {
	if serverID, ok := domaintoolmcp.ParseToolKey(toolKey); ok {
		server, err := l.svcCtx.MCPServerRepo.FindByID(l.ctx, userID, serverID)
		if err != nil {
			return "", err
		}
		definitions, err := availableMCPDefinitions(l.ctx, l.svcCtx, server)
		if err != nil {
			return "", err
		}
		for _, definition := range definitions {
			if definition != nil && definition.Key == toolKey {
				return mcpToolType, nil
			}
		}
		return "", xerr.NotFound("工具不存在")
	}

	items, err := builtinToolResponses(l.ctx, l.svcCtx)
	if err != nil {
		return "", err
	}
	if !containsBuiltinTool(items, toolKey) {
		return "", xerr.NotFound("工具不存在")
	}
	return builtinToolType, nil
}

// containsBuiltinTool 检查工具列表中是否包含指定的内置工具
func containsBuiltinTool(items []*response.ToolConfigResponse, toolKey string) bool {
	for _, item := range items {
		if item != nil && item.ToolKey == toolKey {
			return true
		}
	}
	return false
}

// findToolConfig 在工具配置列表中查找指定工具的配置
func findToolConfig(rows []*models.ToolConfig, toolKey string) *models.ToolConfig {
	for _, row := range rows {
		if row != nil && row.ToolKey == toolKey {
			return row
		}
	}
	return nil
}
