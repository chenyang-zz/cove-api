package conversation

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/xerr"
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

// ListConversations 分页获取会话列表。
func (l *ListConversationsLogic) ListConversations(userID uuid.UUID, input *request.ListConversationsRequest) (*response.PageListResponse[*response.ConversationResponse], error) {
	if input == nil {
		return nil, xerr.BadRequest("会话列表参数不能为空")
	}

	rows, total, err := l.svcCtx.ConversationRepo.PageList(l.ctx, userID, repository.ConversationListQuery{
		PageQuery: repository.PageQuery{
			Page:     input.Page,
			PageSize: input.PageSize,
		},
	})
	if err != nil {
		return nil, err
	}

	list := mapper.ConversationsToListResponse(rows).List
	return &response.PageListResponse[*response.ConversationResponse]{
		Total:    total,
		Page:     input.Page,
		PageSize: input.PageSize,
		List:     list,
	}, nil
}
