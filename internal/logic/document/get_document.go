package document

import (
	"context"

	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/google/uuid"
	"log/slog"
)

// GetDocumentLogic contains the getDocument use case.
type GetDocumentLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewGetDocumentLogic creates a GetDocumentLogic.
func NewGetDocumentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetDocumentLogic {
	return &GetDocumentLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.document.getdocument"),
	}
}

// GetDocument 获取文档详情
func (l *GetDocumentLogic) GetDocument(userID uuid.UUID, input *request.UriDocumentIDRequest) (*response.DocumentResponse, error) {
	documentID, err := parseDocumentID(input.DocumentID)
	if err != nil {
		return nil, err
	}
	row, err := l.svcCtx.DocumentRepo.FindByID(l.ctx, userID, documentID)
	if err != nil {
		return nil, err
	}
	return mapper.DocumentToResponse(row, nil), nil
}
