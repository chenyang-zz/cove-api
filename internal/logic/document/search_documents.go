package document

import (
	"context"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/google/uuid"
	"log/slog"
)

// SearchDocumentsLogic contains the searchDocuments use case.
type SearchDocumentsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewSearchDocumentsLogic creates a SearchDocumentsLogic.
func NewSearchDocumentsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SearchDocumentsLogic {
	return &SearchDocumentsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.document.searchdocuments"),
	}
}

// SearchDocuments 检索文档
func (l *SearchDocumentsLogic) SearchDocuments(userID uuid.UUID, input *response.ListResponse[*response.SearchDocumentResponse]) error {
	_ = l
	return nil
}
