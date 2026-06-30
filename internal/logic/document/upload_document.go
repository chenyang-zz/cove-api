package document

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/infrastructure/storage"
	knowledgebaselogic "github.com/boxify/api-go/internal/logic/knowledgebase"
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
	kbID, err := l.resolveKnowledgeBaseID(userID, input.KBID)
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
		Status:     documentStatusPending,
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

	// TODO: 推入文档解析任务队列
	l.log.InfoContext(l.ctx, "文档解析任务暂未接入队列",
		slog.String("document_id", row.ID.String()),
	)
	return mapper.DocumentToResponse(row, nil), nil
}

func (l *UploadDocumentLogic) resolveKnowledgeBaseID(userID uuid.UUID, rawKBID *string) (uuid.UUID, error) {
	if l.svcCtx.KnowledgeBaseRepo == nil {
		return uuid.Nil, xerr.BadRequest("知识库仓储未初始化")
	}
	if parsed, err := parseOptionalKBID(rawKBID); err != nil {
		return uuid.Nil, err
	} else if parsed != nil {
		row, err := l.svcCtx.KnowledgeBaseRepo.FindByID(l.ctx, userID, *parsed)
		if err != nil {
			return uuid.Nil, err
		}
		return row.ID, nil
	}
	row, _, err := knowledgebaselogic.EnsureDefaultKnowledgeBase(l.ctx, l.svcCtx.KnowledgeBaseRepo, userID, l.log)
	if err != nil {
		return uuid.Nil, err
	}
	return row.ID, nil
}
