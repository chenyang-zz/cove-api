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

// PreviewDocumentContentLogic contains the previewDocumentContent use case.
type PreviewDocumentContentLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewPreviewDocumentContentLogic creates a PreviewDocumentContentLogic.
func NewPreviewDocumentContentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PreviewDocumentContentLogic {
	return &PreviewDocumentContentLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.document.previewdocumentcontent"),
	}
}

// PreviewDocumentContent 预览文档原文内容
func (l *PreviewDocumentContentLogic) PreviewDocumentContent(userID uuid.UUID, input *request.UriDocumentIDRequest) (*response.PreviewDocumentResponse, error) {
	_ = l
	return nil, nil
}
