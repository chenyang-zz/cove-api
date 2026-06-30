package document

import (
	"context"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
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
func (l *GetDocumentLogic) GetDocument(input *request.UriDocumentIDRequest) (*response.DocumentResponse, error) {
	_ = l
	return nil, nil
}
