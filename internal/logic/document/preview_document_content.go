package document

import (
	"context"
	"strings"

	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/xerr"
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
	documentID, err := parseDocumentID(input.DocumentID)
	if err != nil {
		return nil, err
	}
	row, err := l.svcCtx.DocumentRepo.FindByID(l.ctx, userID, documentID)
	if err != nil {
		return nil, err
	}
	ext := strings.ToLower(row.FileExt)
	isMarkdown := ext == ".md" || ext == ".markdown"
	if !isPreviewTextExt(ext) {
		return nil, xerr.BadRequest("该文档类型暂不支持预览")
	}
	if l.svcCtx.Storage == nil {
		return nil, xerr.BadRequest("对象存储未初始化")
	}
	raw, err := l.svcCtx.Storage.Get(l.ctx, row.FileKey)
	if err != nil {
		l.log.WarnContext(l.ctx, "读取文档原文失败",
			slog.String("user_id", userID.String()),
			slog.String("document_id", documentID.String()),
			slog.String("file_key", row.FileKey),
			slog.String("error", err.Error()),
		)
		return nil, xerr.Wrap(err, "原始文件读取失败，可能已被清理")
	}
	content, truncated := truncatePreview(strings.TrimSpace(string(raw)))
	return &response.PreviewDocumentResponse{
		ID:         row.ID,
		FileName:   row.FileName,
		FileExt:    ext,
		IsMarkdown: isMarkdown,
		SourceUrl:  row.SourceUrl,
		Content:    content,
		Truncated:  truncated,
	}, nil
}
