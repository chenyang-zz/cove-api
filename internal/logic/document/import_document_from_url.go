package document

import (
	"context"
	"log/slog"
	"strings"
	"unicode"

	"github.com/boxify/api-go/internal/core/rag/webcrawl"
	"github.com/boxify/api-go/internal/domain"
	"github.com/boxify/api-go/internal/infrastructure/storage"
	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

// ImportDocumentFromUrlLogic contains the importDocumentFromUrl use case.
type ImportDocumentFromUrlLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewImportDocumentFromUrlLogic creates a ImportDocumentFromUrlLogic.
func NewImportDocumentFromUrlLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ImportDocumentFromUrlLogic {
	return &ImportDocumentFromUrlLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.document.importdocumentfromurl"),
	}
}

// ImportDocumentFromUrl 从url导入文档
func (l *ImportDocumentFromUrlLogic) ImportDocumentFromUrl(userID uuid.UUID, input *request.URLImportRequest) (*response.DocumentResponse, error) {
	if input == nil {
		return nil, xerr.BadRequest("URL 不能为空")
	}
	rawURL := strings.TrimSpace(input.Url)
	if rawURL == "" {
		return nil, xerr.BadRequest("URL 不能为空")
	}
	if l.svcCtx == nil || l.svcCtx.DocumentRepo == nil || l.svcCtx.Storage == nil || l.svcCtx.TaskProducer == nil {
		return nil, xerr.BadRequest("文档导入依赖未初始化")
	}
	if l.svcCtx.RAGWebCrawler == nil {
		return nil, xerr.BadRequest("网页抓取器未初始化")
	}
	kbID, err := resolveDocumentKnowledgeBaseID(l.ctx, l.svcCtx.KnowledgeBaseRepo, l.log, userID, input.KBID)
	if err != nil {
		return nil, err
	}
	page, err := l.svcCtx.RAGWebCrawler.Fetch(l.ctx, webcrawl.Input{URL: rawURL})
	if err != nil {
		return nil, xerr.BadRequestf("抓取网页内容失败: %v", err)
	}
	content := strings.TrimSpace(page.Content)
	if content == "" {
		return nil, xerr.BadRequest("网页正文为空")
	}
	contentBytes := []byte(content)
	if int64(len(contentBytes)) > maxDocumentFileSize {
		return nil, xerr.BadRequest("文件超过 50MB 限制")
	}
	l.log.InfoContext(l.ctx, "网页内容抓取完成",
		slog.String("user_id", userID.String()),
		slog.String("kb_id", kbID.String()),
		slog.Int64("content_size", int64(len(contentBytes))),
	)

	docID := uuid.New()
	fileKey := storage.BuildFileKey(userID, "documents", docID, ".txt")
	if err := l.svcCtx.Storage.Put(l.ctx, fileKey, contentBytes); err != nil {
		return nil, err
	}

	row, err := l.svcCtx.DocumentRepo.Create(l.ctx, userID, &models.Document{
		ID:         docID,
		KBID:       &kbID,
		FileName:   urlImportFileName(page.Title),
		FileExt:    ".txt",
		FileSize:   int64(len(contentBytes)),
		FileKey:    fileKey,
		SourceType: documentSourceURL,
		SourceUrl:  &rawURL,
		Status:     domain.DocumentStatusPending,
	})
	if err != nil {
		return nil, err
	}
	l.log.InfoContext(l.ctx, "URL 文档导入成功",
		slog.String("user_id", userID.String()),
		slog.String("document_id", row.ID.String()),
		slog.String("kb_id", kbID.String()),
		slog.Int64("file_size", row.FileSize),
	)

	if err := enqueueParseDocumentTask(l.ctx, l.svcCtx.TaskProducer, userID, row.ID); err != nil {
		markDocumentParseDispatchFailed(l.ctx, l.svcCtx.DocumentRepo, userID, row.ID, err)
		return nil, err
	}
	l.log.InfoContext(l.ctx, "URL 文档解析任务已入队",
		slog.String("document_id", row.ID.String()),
	)
	return mapper.DocumentToResponse(row, nil), nil
}

// urlImportFileName 生成导入文件名
func urlImportFileName(title string) string {
	base := strings.TrimSpace(title)
	if base == "" {
		base = "网页导入"
	}
	base = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || unicode.IsControl(r) {
			return '_'
		}
		return r
	}, base)
	base = strings.Trim(strings.TrimSpace(base), ". ")
	if base == "" {
		base = "网页导入"
	}
	runes := []rune(base)
	if len(runes) > 200 {
		base = string(runes[:200])
	}
	return base + ".txt"
}
