package conversation

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/google/uuid"
)

type CreateConversationLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

func NewCreateConversationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateConversationLogic {
	return &CreateConversationLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.conversation.createconversation"),
	}
}

func (l *CreateConversationLogic) CreateConversation(userID uuid.UUID, input *request.CreateConversationRequest) (*response.ConversationResponse, error) {

	conversation := &models.Conversation{
		UserID: userID,
	}

	if input.Title == nil {
		conversation.Title = "新对话"
	} else {
		conversation.Title = *input.Title
	}

	conversation, err := l.svcCtx.ConversationRepo.Create(l.ctx, userID, conversation)
	if err != nil {
		return nil, err
	}

	return mapper.ConversationToResponse(conversation), nil
}
