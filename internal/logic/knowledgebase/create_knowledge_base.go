package knowledgebase

import (
	"context"
	"log/slog"
	"strings"

	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

// CreateKnowledgeBaseLogic contains the createKnowledgeBase use case.
type CreateKnowledgeBaseLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewCreateKnowledgeBaseLogic creates a CreateKnowledgeBaseLogic.
func NewCreateKnowledgeBaseLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateKnowledgeBaseLogic {
	return &CreateKnowledgeBaseLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.knowledgebase.createknowledgebase"),
	}
}

// CreateKnowledgeBase 创建知识库
func (l *CreateKnowledgeBaseLogic) CreateKnowledgeBase(userID uuid.UUID, input *request.CreateKnowledgeBaseRequest) (*response.KnowledgeBaseResponse, error) {
	if input == nil {
		return nil, xerr.BadRequest("知识库参数不能为空")
	}
	row := &models.KnowledgeBase{
		Name:        strings.TrimSpace(input.Name),
		Description: strings.TrimSpace(input.Description),
		Icon:        strings.TrimSpace(input.Icon),
		Color:       strings.TrimSpace(input.Color),
		IsDefault:   false,
		ChatEnabled: false,
	}
	row, err := l.svcCtx.KnowledgeBaseRepo.Create(l.ctx, userID, row)
	if err != nil {
		return nil, err
	}
	l.log.InfoContext(l.ctx, "创建知识库",
		slog.String("user_id", userID.String()),
		slog.String("knowledge_base_id", row.ID.String()),
		slog.String("name", row.Name),
	)
	return mapper.KnowledgeBaseToResponse(row, 0, 0), nil
}
