package conversation

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

// RenameConversationLogic contains the renameConversation use case.
type RenameConversationLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewRenameConversationLogic creates a RenameConversationLogic.
func NewRenameConversationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RenameConversationLogic {
	return &RenameConversationLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.conversation.renameconversation"),
	}
}

// RenameConversation 重命名会话
func (l *RenameConversationLogic) RenameConversation(userID uuid.UUID, input *request.RenameConversationRequest) (*response.ConversationResponse, error) {
	if input == nil {
		return nil, xerr.BadRequest("会话 ID 无效")
	}
	conversationID, err := conversationIDFromInput(&input.UriConversationIDRequest)
	if err != nil {
		return nil, err
	}
	conversation, err := l.svcCtx.ConversationRepo.UpdateFields(
		l.ctx,
		userID,
		conversationID,
		&models.Conversation{Title: input.Title},
		repository.NewConversationUpdateFields().Title(),
	)
	if err != nil {
		return nil, err
	}
	return mapper.ConversationToResponse(conversation), nil
}
