package mcpserver

import (
	"context"
	"time"

	"github.com/boxify/api-go/internal/core/mcp"
	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/google/uuid"
	"log/slog"
)

// SyncMCPServerLogic contains the syncMCPServer use case.
type SyncMCPServerLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewSyncMCPServerLogic creates a SyncMCPServerLogic.
func NewSyncMCPServerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SyncMCPServerLogic {
	return &SyncMCPServerLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.mcpserver.syncmcpserver"),
	}
}

// SyncMCPServer 同步mcp服务
func (l *SyncMCPServerLogic) SyncMCPServer(userID uuid.UUID, input *request.UriMCPServerIDRequest) (*response.MCPServerResponse, error) {
	mcpServerID, err := mcpServerIDFromInput(input)
	if err != nil {
		return nil, err
	}
	row, err := l.svcCtx.MCPServerRepo.FindByID(l.ctx, userID, mcpServerID)
	if err != nil {
		return nil, err
	}
	authConfig, err := decryptMCPAuthConfig(l.svcCtx.SecretCipher, row.AuthConfig)
	if err != nil {
		return nil, err
	}

	tools, err := l.svcCtx.MCPToolService.RefreshToolList(l.ctx, mapper.MCPServerToCoreServerConfig(row, authConfig))
	if err != nil {
		lastError := err.Error()
		patch := &models.MCPServer{
			Status:    "error",
			LastError: &lastError,
		}
		updated, updateErr := l.svcCtx.MCPServerRepo.UpdateFields(
			l.ctx,
			userID,
			mcpServerID,
			patch,
			repository.NewMCPServerUpdateFields().Status().LastError(),
		)
		if updateErr != nil {
			return nil, updateErr
		}
		_ = updated
		return nil, err
	}

	now := time.Now()
	emptyError := ""
	patch := &models.MCPServer{
		ToolsCache: mapper.MCPToolMetasToModelMetas(mcp.MetasFromTools(tools)),
		SyncedAt:   &now,
		Status:     "ready",
		LastError:  &emptyError,
	}
	row, err = l.svcCtx.MCPServerRepo.UpdateFields(
		l.ctx,
		userID,
		mcpServerID,
		patch,
		repository.NewMCPServerUpdateFields().ToolsCache().SyncedAt().Status().LastError(),
	)
	if err != nil {
		return nil, err
	}
	return mapper.MCPServerToResponse(row, maskMCPAuthConfig(row.AuthType, authConfig)), nil
}
