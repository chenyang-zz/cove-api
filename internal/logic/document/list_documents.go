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

// ListDocumentsLogic contains the listDocuments use case.
type ListDocumentsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewListDocumentsLogic creates a ListDocumentsLogic.
func NewListDocumentsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListDocumentsLogic {
	return &ListDocumentsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.document.listdocuments"),
	}
}

// ListDocuments 获取文档列表
func (l *ListDocumentsLogic) ListDocuments(userID uuid.UUID, input *request.ListDocumentsRequest) (*response.ListResponse[*response.DocuementResponse], error) {
	_ = l
	return nil, nil
}
