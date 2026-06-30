package document

import (
	"context"

	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/xerr"
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
func (l *ListDocumentsLogic) ListDocuments(userID uuid.UUID, input *request.ListDocumentsRequest) (*response.PageListResponse[*response.DocumentResponse], error) {
	if input == nil {
		return nil, xerr.BadRequest("文档列表参数不能为空")
	}
	kbID, err := parseOptionalKBID(input.KBID)
	if err != nil {
		return nil, err
	}
	rows, total, err := l.svcCtx.DocumentRepo.PageList(l.ctx, userID, repository.DocumentListQuery{
		KBID: kbID,
		Tag:  input.Tag,
		PageQuery: repository.PageQuery{
			Page:     input.Page,
			PageSize: input.PageSize,
		},
	})
	if err != nil {
		return nil, err
	}
	out := make([]*response.DocumentResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapper.DocumentToResponse(row, nil))
	}
	return &response.PageListResponse[*response.DocumentResponse]{
		Total:    total,
		Page:     input.Page,
		PageSize: input.PageSize,
		List:     out,
	}, nil
}
