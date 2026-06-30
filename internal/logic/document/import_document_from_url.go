package document

import (
	"context"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"log/slog"
)

// ImportDocumentFromUrlLogic contains the importDocumentFromUrl use case.
type ImportDocumentFromUrlLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewImportDocumentFromUrlLogic creates a ImportDocumentFromUrlLogic.
func NewImportDocumentFromUrlLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ImportDocumentFromUrlLogic {
	return &ImportDocumentFromUrlLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.document.importdocumentfromurl"),
	}
}

// ImportDocumentFromUrl 从url导入文档
func (l *ImportDocumentFromUrlLogic) ImportDocumentFromUrl(input *request.URLImportRequest) (*response.DocumentResponse, error) {
	_ = l
	return nil, nil
}
