package document

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
	"github.com/google/uuid"
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
	documentID, err := parseDocumentID(input.DocumentID)
	if err != nil {
		return nil, err
	}
	kbID, err := parseRequiredKBID(input.KBID)
	if err != nil {
		return nil, err
	}
	if _, err := l.svcCtx.KnowledgeBaseRepo.FindByID(l.ctx, userID, kbID); err != nil {
		return nil, err
	}
	row, err := l.svcCtx.DocumentRepo.UpdateFields(l.ctx, userID, documentID, &models.Document{
		KBID: &kbID,
	}, repository.NewDocumentUpdateFields().KBID())
	if err != nil {
		return nil, err
	}
	l.log.InfoContext(l.ctx, "移动文档到知识库",
		slog.String("user_id", userID.String()),
		slog.String("document_id", documentID.String()),
		slog.String("kb_id", kbID.String()),
	)
	return mapper.DocumentToResponse(row, nil), nil
}
