package knowledgebase

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

// SetDefaultKnowledgeBaseLogic 包含切换默认知识库用例。
type SetDefaultKnowledgeBaseLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewSetDefaultKnowledgeBaseLogic 创建切换默认知识库用例。
func NewSetDefaultKnowledgeBaseLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SetDefaultKnowledgeBaseLogic {
	return &SetDefaultKnowledgeBaseLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.knowledgebase.setdefault"),
	}
}

// SetDefaultKnowledgeBase 将当前用户拥有的指定知识库设为唯一默认知识库。
func (l *SetDefaultKnowledgeBaseLogic) SetDefaultKnowledgeBase(userID uuid.UUID, input *request.UriKnowledgeBaseIDRequest) (*response.KnowledgeBaseResponse, error) {
	knowledgeBaseID, err := knowledgebaseIDFromInput(input)
	if err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.KnowledgeBaseRepo == nil {
		return nil, xerr.Internal("知识库仓储未初始化", nil)
	}
	row, err := l.svcCtx.KnowledgeBaseRepo.SetDefault(l.ctx, userID, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	counts, err := loadKnowledgeBaseContentCount(l.ctx, l.svcCtx, userID, row)
	if err != nil {
		return nil, err
	}

	l.log.InfoContext(l.ctx, "设置默认知识库",
		slog.String("user_id", userID.String()),
		slog.String("knowledge_base_id", knowledgeBaseID.String()),
	)
	return mapper.KnowledgeBaseToResponse(row, counts.docCount, counts.imageCount), nil
}
