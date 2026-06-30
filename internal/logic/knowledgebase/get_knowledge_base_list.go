package knowledgebase

import (
	"context"

	"log/slog"

	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/google/uuid"
)

// GetKnowledgeBaseListLogic contains the getKnowledgeBaseList use case.
type GetKnowledgeBaseListLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewGetKnowledgeBaseListLogic creates a GetKnowledgeBaseListLogic.
func NewGetKnowledgeBaseListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetKnowledgeBaseListLogic {
	return &GetKnowledgeBaseListLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.knowledgebase.getknowledgebaselist"),
	}
}

// GetKnowledgeBaseList 查询知识库列表
func (l *GetKnowledgeBaseListLogic) GetKnowledgeBaseList(userID uuid.UUID) (*response.ListResponse[*response.KnowledgeBaseResponse], error) {
	if _, _, err := EnsureDefaultKnowledgeBase(l.ctx, l.svcCtx.KnowledgeBaseRepo, userID, l.log); err != nil {
		return nil, err
	}
	rows, err := l.svcCtx.KnowledgeBaseRepo.List(l.ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]*response.KnowledgeBaseResponse, 0, len(rows))
	counts, err := loadKnowledgeBaseContentCounts(l.ctx, l.svcCtx, userID, rows)
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		count := counts[row.ID]
		out = append(out, mapper.KnowledgeBaseToResponse(row, count.docCount, count.imageCount))
	}
	return &response.ListResponse[*response.KnowledgeBaseResponse]{List: out}, nil
}
