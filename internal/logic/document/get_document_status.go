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

// GetDocumentStatusLogic contains the getDocumentStatus use case.
type GetDocumentStatusLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewGetDocumentStatusLogic creates a GetDocumentStatusLogic.
func NewGetDocumentStatusLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetDocumentStatusLogic {
	return &GetDocumentStatusLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.document.getdocumentstatus"),
	}
}

// GetDocumentStatus 获取文档状态
func (l *GetDocumentStatusLogic) GetDocumentStatus(userID uuid.UUID, input *request.UriDocumentIDRequest) (*response.DocumentStatusResponse, error) {
	documentID, err := parseDocumentID(input.DocumentID)
	if err != nil {
		return nil, err
	}
	row, err := l.svcCtx.DocumentRepo.FindByID(l.ctx, userID, documentID)
	if err != nil {
		return nil, err
	}
	return &response.DocumentStatusResponse{
		Status:   row.Status,
		Progress: row.Progress,
		ErrorMsg: row.ErrorMsg,
	}, nil
}
