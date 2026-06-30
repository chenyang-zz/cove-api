package knowledgebase

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

// EnabledChatLogic contains the enabledChat use case.
type EnabledChatLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewEnabledChatLogic creates a EnabledChatLogic.
func NewEnabledChatLogic(ctx context.Context, svcCtx *svc.ServiceContext) *EnabledChatLogic {
	return &EnabledChatLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.knowledgebase.enabledchat"),
	}
}

// EnabledChat 启用或禁用知识库聊天
func (l *EnabledChatLogic) EnabledChat(userID uuid.UUID, input *request.EnabledChatRequest) (*response.KnowledgeBaseResponse, error) {
	if input == nil || input.ChatEnabled == nil {
		return nil, xerr.BadRequest("知识库聊天开关不能为空")
	}
	knowledgeBaseID, err := knowledgebaseIDFromInput(&input.UriKnowledgeBaseIDRequest)
	if err != nil {
		return nil, err
	}
	patch := &models.KnowledgeBase{ChatEnabled: *input.ChatEnabled}
	fields := repository.NewKnowledgeBaseUpdateFields().ChatEnabled()
	row, err := l.svcCtx.KnowledgeBaseRepo.UpdateFields(l.ctx, userID, knowledgeBaseID, patch, fields)
	if err != nil {
		return nil, err
	}

	counts, err := loadKnowledgeBaseContentCount(l.ctx, l.svcCtx, userID, row)
	if err != nil {
		return nil, err
	}

	l.log.InfoContext(l.ctx, "切换知识库聊天开关",
		slog.String("user_id", userID.String()),
		slog.String("knowledge_base_id", knowledgeBaseID.String()),
		slog.Bool("chat_enabled", *input.ChatEnabled),
	)
	return mapper.KnowledgeBaseToResponse(row, counts.docCount, counts.imageCount), nil
}
