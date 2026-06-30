package document

import (
	"context"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/google/uuid"
	"log/slog"
)

// MoveDocumentLogic contains the moveDocument use case.
type MoveDocumentLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewMoveDocumentLogic creates a MoveDocumentLogic.
func NewMoveDocumentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MoveDocumentLogic {
	return &MoveDocumentLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.document.movedocument"),
	}
}

// MoveDocument 移动文档到指定知识库
func (l *MoveDocumentLogic) MoveDocument(userID uuid.UUID, input *request.MoveDocumentRequest) (*response.DocumentResponse, error) {
	_ = l
	return nil, nil
}
