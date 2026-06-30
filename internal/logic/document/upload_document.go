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

// UploadDocumentLogic contains the uploadDocument use case.
type UploadDocumentLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewUploadDocumentLogic creates a UploadDocumentLogic.
func NewUploadDocumentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UploadDocumentLogic {
	return &UploadDocumentLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.document.uploaddocument"),
	}
}

// UploadDocument 上传文档
func (l *UploadDocumentLogic) UploadDocument(userID uuid.UUID, input *request.UploadDocumentRequest) (*response.DocumentResponse, error) {
	_ = l
	return nil, nil
}
