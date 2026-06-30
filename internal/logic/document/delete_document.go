package document

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/google/uuid"
)

// DeleteDocumentLogic contains the deleteDocument use case.
type DeleteDocumentLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewDeleteDocumentLogic creates a DeleteDocumentLogic.
func NewDeleteDocumentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteDocumentLogic {
	return &DeleteDocumentLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.document.deletedocument"),
	}
}

// DeleteDocument 删除文档
func (l *DeleteDocumentLogic) DeleteDocument(userID uuid.UUID, input *request.UriDocumentIDRequest) error {
	documentID, err := parseDocumentID(input.DocumentID)
	if err != nil {
		return err
	}
	row, err := l.svcCtx.DocumentRepo.FindByID(l.ctx, userID, documentID)
	if err != nil {
		return err
	}
	if l.svcCtx.Storage != nil && row.FileKey != "" {
		if err := l.svcCtx.Storage.Delete(l.ctx, row.FileKey); err != nil {
			l.log.WarnContext(l.ctx, "删除文档存储文件失败（忽略）",
				slog.String("user_id", userID.String()),
				slog.String("document_id", documentID.String()),
				slog.String("file_key", row.FileKey),
				slog.String("error", err.Error()),
			)
		}
	}
	if err := l.svcCtx.DocumentRepo.Delete(l.ctx, userID, documentID); err != nil {
		return err
	}
	l.log.InfoContext(l.ctx, "删除文档",
		slog.String("user_id", userID.String()),
		slog.String("document_id", documentID.String()),
	)
	return nil
}
