package conversation

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/google/uuid"
)

type ListConversationsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

func NewListConversationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListConversationsLogic {
	return &ListConversationsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.conversation.listconversations"),
	}
}

func (l *ListConversationsLogic) ListConversations(userID uuid.UUID) (*response.ListResponse[*response.ConversationResponse], error) {
	rows, err := l.svcCtx.ConversationRepo.List(l.ctx, userID)
	if err != nil {
		return nil, err
	}
	return mapper.ConversationsToListResponse(rows), nil
}
