package knowledgebase

import (
	"context"
	"log/slog"
	"strings"

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

// UpdateKnowledgeBaseLogic contains the updateKnowledgeBase use case.
type UpdateKnowledgeBaseLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewUpdateKnowledgeBaseLogic creates a UpdateKnowledgeBaseLogic.
func NewUpdateKnowledgeBaseLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateKnowledgeBaseLogic {
	return &UpdateKnowledgeBaseLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.knowledgebase.updateknowledgebase"),
	}
}

// UpdateKnowledgeBase 更新知识库
func (l *UpdateKnowledgeBaseLogic) UpdateKnowledgeBase(userID uuid.UUID, input *request.UpdateKnowledgeBaseRequest) (*response.KnowledgeBaseResponse, error) {
	if input == nil {
		return nil, xerr.BadRequest("知识库参数不能为空")
	}
	knowledgeBaseID, err := knowledgebaseIDFromInput(&input.UriKnowledgeBaseIDRequest)
	if err != nil {
		return nil, err
	}

	patch := &models.KnowledgeBase{}
	fields := repository.NewKnowledgeBaseUpdateFields()
	if input.Name != nil {
		patch.Name = strings.TrimSpace(*input.Name)
		fields.Name()
	}
	if input.Description != nil {
		patch.Description = strings.TrimSpace(*input.Description)
		fields.Description()
	}
	if input.Icon != nil {
		patch.Icon = strings.TrimSpace(*input.Icon)
		fields.Icon()
	}
	if input.Color != nil {
		patch.Color = strings.TrimSpace(*input.Color)
		fields.Color()
	}

	row, err := l.svcCtx.KnowledgeBaseRepo.UpdateFields(l.ctx, userID, knowledgeBaseID, patch, fields)
	if err != nil {
		return nil, err
	}

	counts, err := loadKnowledgeBaseContentCount(l.ctx, l.svcCtx, userID, row)
	if err != nil {
		return nil, err
	}

	l.log.InfoContext(l.ctx, "更新知识库",
		slog.String("user_id", userID.String()),
		slog.String("knowledge_base_id", knowledgeBaseID.String()),
		slog.Any("fields", fields.Columns()),
	)
	return mapper.KnowledgeBaseToResponse(row, counts.docCount, counts.imageCount), nil
}
