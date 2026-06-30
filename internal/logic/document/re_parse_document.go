package document

import (
	"context"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"log/slog"
)

// ReParseDocumentLogic contains the reParseDocument use case.
type ReParseDocumentLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewReParseDocumentLogic creates a ReParseDocumentLogic.
func NewReParseDocumentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReParseDocumentLogic {
	return &ReParseDocumentLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.document.reparsedocument"),
	}
}

// ReParseDocument 重新提交文档解析
func (l *ReParseDocumentLogic) ReParseDocument(input *request.UriDocumentIDRequest) (*response.DocumentResponse, error) {
	_ = l
	return nil, nil
}
