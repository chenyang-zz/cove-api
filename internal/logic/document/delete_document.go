package document

import (
	"context"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/google/uuid"
	"log/slog"
)

// DeleteDocumentLogic contains the deleteDocument use case.
type DeleteDocumentLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewDeleteDocumentLogic creates a DeleteDocumentLogic.
func NewDeleteDocumentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteDocumentLogic {
	return &DeleteDocumentLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.document.deletedocument"),
	}
}

// DeleteDocument 删除文档
func (l *DeleteDocumentLogic) DeleteDocument(userID uuid.UUID, input *request.UriDocumentIDRequest) error {
	_ = l
	return nil
}
