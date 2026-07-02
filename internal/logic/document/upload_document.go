package document

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/domain"
	"github.com/boxify/api-go/internal/infrastructure/storage"
	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/util/uploadfile"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
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
	if input == nil || input.File == nil {
		return nil, xerr.BadRequest("上传文件不能为空")
	}
	fileInfo, err := uploadfile.Read(input.File, maxDocumentFileSize, "文件超过 50MB 限制", "读取上传文件失败")
	if err != nil {
		return nil, err
	}
	if fileInfo.FileName == "" {
		return nil, xerr.BadRequest("文件名不能为空")
	}
	ext, err := supportedDocumentExt(fileInfo.Ext)
	if err != nil {
		return nil, err
	}
	kbID, err := resolveDocumentKnowledgeBaseID(l.ctx, l.svcCtx.KnowledgeBaseRepo, l.log, userID, input.KBID)
	if err != nil {
		return nil, err
	}
	if l.svcCtx.Storage == nil {
		return nil, xerr.BadRequest("对象存储未初始化")
	}

	docID := uuid.New()
	fileKey := storage.BuildFileKey(userID, "documents", docID, ext)
	if err := l.svcCtx.Storage.Put(l.ctx, fileKey, fileInfo.Content); err != nil {
		return nil, err
	}

	row, err := l.svcCtx.DocumentRepo.Create(l.ctx, userID, &models.Document{
		ID:         docID,
		KBID:       &kbID,
		FileName:   fileInfo.FileName,
		FileExt:    ext,
		FileSize:   fileInfo.Size,
		FileKey:    fileKey,
		SourceType: documentSourceFile,
		Status:     domain.DocumentStatusPending,
	})
	if err != nil {
		return nil, err
	}
	l.log.InfoContext(l.ctx, "文档上传成功",
		slog.String("user_id", userID.String()),
		slog.String("document_id", row.ID.String()),
		slog.String("kb_id", kbID.String()),
		slog.String("file_ext", ext),
		slog.Int64("file_size", row.FileSize),
	)

	if err := enqueueParseDocumentTask(l.ctx, l.svcCtx.TaskProducer, userID, row.ID); err != nil {
		markDocumentParseDispatchFailed(l.ctx, l.svcCtx.DocumentRepo, userID, row.ID, err)
		return nil, err
	}
	l.log.InfoContext(l.ctx, "文档解析任务已入队",
		slog.String("document_id", row.ID.String()),
	)
	return mapper.DocumentToResponse(row, nil), nil
}
