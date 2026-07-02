package document

import (
	"context"
	"log/slog"
	"strings"

	"github.com/boxify/api-go/internal/domain"
	"github.com/boxify/api-go/internal/infrastructure/queue"
	knowledgebaselogic "github.com/boxify/api-go/internal/logic/knowledgebase"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

const (
	documentSourceFile  = "file"
	documentSourceURL   = "url"
	maxDocumentFileSize = 50 * 1024 * 1024
	previewMaxChars     = 80000
)

var supportedDocumentExts = map[string]struct{}{
	".pdf":      {},
	".docx":     {},
	".md":       {},
	".markdown": {},
	".txt":      {},
	".html":     {},
	".htm":      {},
}

var previewTextExts = map[string]struct{}{
	".md":       {},
	".markdown": {},
	".txt":      {},
	".html":     {},
	".htm":      {},
}

func parseDocumentID(raw string) (uuid.UUID, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return uuid.Nil, xerr.BadRequest("文档 ID 无效")
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, xerr.BadRequest("文档 ID 无效")
	}
	return id, nil
}

func parseOptionalKBID(raw *string) (*uuid.UUID, error) {
	if raw == nil {
		return nil, nil
	}
	value := strings.TrimSpace(*raw)
	if value == "" {
		return nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return nil, xerr.BadRequest("知识库 ID 无效")
	}
	return &id, nil
}

func parseRequiredKBID(raw string) (uuid.UUID, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return uuid.Nil, xerr.BadRequest("知识库 ID 无效")
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, xerr.BadRequest("知识库 ID 无效")
	}
	return id, nil
}

func supportedDocumentExt(ext string) (string, error) {
	ext = strings.ToLower(ext)
	if _, ok := supportedDocumentExts[ext]; !ok {
		return "", xerr.BadRequestf("不支持的文件类型: %s", ext)
	}
	return ext, nil
}

func resolveDocumentKnowledgeBaseID(ctx context.Context, repo repository.KnowledgeBaseRepository, log *slog.Logger, userID uuid.UUID, rawKBID *string) (uuid.UUID, error) {
	if repo == nil {
		return uuid.Nil, xerr.BadRequest("知识库仓储未初始化")
	}
	if parsed, err := parseOptionalKBID(rawKBID); err != nil {
		return uuid.Nil, err
	} else if parsed != nil {
		row, err := repo.FindByID(ctx, userID, *parsed)
		if err != nil {
			return uuid.Nil, err
		}
		return row.ID, nil
	}
	row, _, err := knowledgebaselogic.EnsureDefaultKnowledgeBase(ctx, repo, userID, log)
	if err != nil {
		return uuid.Nil, err
	}
	return row.ID, nil
}

func isPreviewTextExt(ext string) bool {
	_, ok := previewTextExts[strings.ToLower(ext)]
	return ok
}

func truncatePreview(text string) (string, bool) {
	runes := []rune(text)
	if len(runes) <= previewMaxChars {
		return text, false
	}
	return string(runes[:previewMaxChars]), true
}

// 队列提交文档解析任务
func enqueueParseDocumentTask(ctx context.Context, producer queue.Producer, userID uuid.UUID, documentID uuid.UUID) error {
	if producer == nil {
		return xerr.Internal("任务队列未初始化", nil)
	}
	task, err := domain.NewParseDocumentTask(userID, documentID)
	if err != nil {
		return xerr.Wrapf(err, "创建文档解析任务失败")
	}
	_, err = producer.Enqueue(ctx, task)
	if err != nil {
		return xerr.Wrapf(err, "提交文档解析任务失败")
	}
	return nil
}

// 标记文档解析任务分发失败
func markDocumentParseDispatchFailed(ctx context.Context, repo repository.DocumentRepository, userID uuid.UUID, documentID uuid.UUID, cause error) {
	if repo == nil || cause == nil {
		return
	}
	message := cause.Error()
	_, _ = repo.UpdateFields(ctx, userID, documentID, &models.Document{
		Status:   domain.DocumentStatusFailed,
		Progress: 0,
		ErrorMsg: &message,
	}, repository.NewDocumentUpdateFields().Status().Progress().ErrorMsg())
}

// 最努力地删除文档检索 chunk
func deleteDocumentChunksBestEffort(ctx context.Context, svcCtx *svc.ServiceContext, log *slog.Logger, userID uuid.UUID, documentID uuid.UUID) {
	if svcCtx == nil || svcCtx.RAGChunkRepo == nil {
		return
	}
	if err := svcCtx.RAGChunkRepo.DeleteByDocument(ctx, userID, documentID); err != nil && log != nil {
		log.WarnContext(ctx, "清理文档检索 chunk 失败（忽略）",
			slog.String("user_id", userID.String()),
			slog.String("document_id", documentID.String()),
			slog.String("error", err.Error()),
		)
	}
}

// 最努力地更新文档检索 chunk 的知识库归属
func updateDocumentChunksKnowledgeBaseBestEffort(ctx context.Context, svcCtx *svc.ServiceContext, log *slog.Logger, userID uuid.UUID, documentID uuid.UUID, kbID uuid.UUID) {
	if svcCtx == nil || svcCtx.RAGChunkRepo == nil {
		return
	}
	if err := svcCtx.RAGChunkRepo.UpdateKnowledgeBase(ctx, userID, documentID, kbID); err != nil && log != nil {
		log.WarnContext(ctx, "更新文档检索 chunk 知识库归属失败（忽略）",
			slog.String("user_id", userID.String()),
			slog.String("document_id", documentID.String()),
			slog.String("kb_id", kbID.String()),
			slog.String("error", err.Error()),
		)
	}
}
