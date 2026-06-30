package knowledgebase

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/google/uuid"
)

// GetKnowledgeBaseLogic contains the getKnowledgeBase use case.
type GetKnowledgeBaseLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewGetKnowledgeBaseLogic creates a GetKnowledgeBaseLogic.
func NewGetKnowledgeBaseLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetKnowledgeBaseLogic {
	return &GetKnowledgeBaseLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.knowledgebase.getknowledgebase"),
	}
}

// GetKnowledgeBase 查询知识库
func (l *GetKnowledgeBaseLogic) GetKnowledgeBase(userID uuid.UUID, input *request.UriKnowledgeBaseIDRequest) (*response.KnowledgeBaseResponse, error) {
	knowledgeBaseID, err := knowledgebaseIDFromInput(input)
	if err != nil {
		return nil, err
	}
	row, err := l.svcCtx.KnowledgeBaseRepo.FindByID(l.ctx, userID, knowledgeBaseID)
	if err != nil {
		return nil, err
	}

	counts, err := loadKnowledgeBaseContentCount(l.ctx, l.svcCtx, userID, row)
	if err != nil {
		return nil, err
	}

	return mapper.KnowledgeBaseToResponse(row, counts.docCount, counts.imageCount), nil
}
