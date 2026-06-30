package knowledgebase

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

const (
	defaultKnowledgeBaseName        = "默认知识库"
	defaultKnowledgeBaseDescription = "未分类资料默认归入此库"
	defaultKnowledgeBaseIcon        = "📚"
	defaultKnowledgeBaseColor       = "#155EEF"
)

func knowledgebaseIDFromInput(input *request.UriKnowledgeBaseIDRequest) (uuid.UUID, error) {
	if input == nil {
		return uuid.Nil, xerr.BadRequest("知识库 ID 无效")
	}
	id, err := uuid.Parse(input.KID)
	if err != nil {
		return uuid.Nil, xerr.BadRequest("知识库 ID 无效")
	}
	return id, nil
}

func EnsureDefaultKnowledgeBase(ctx context.Context, repo repository.KnowledgeBaseRepository, userID uuid.UUID, log *slog.Logger) (*models.KnowledgeBase, bool, error) {
	row, err := repo.FindDefault(ctx, userID)
	if err == nil {
		return row, false, nil
	}
	if xerr.From(err).Kind != xerr.KindNotFound {
		return nil, false, err
	}
	row, err = repo.Create(ctx, userID, &models.KnowledgeBase{
		Name:        defaultKnowledgeBaseName,
		Description: defaultKnowledgeBaseDescription,
		Icon:        defaultKnowledgeBaseIcon,
		Color:       defaultKnowledgeBaseColor,
		IsDefault:   true,
		ChatEnabled: true,
	})
	if err != nil {
		return nil, false, err
	}
	if log != nil {
		log.InfoContext(ctx, "创建默认知识库",
			slog.String("user_id", userID.String()),
			slog.String("knowledge_base_id", row.ID.String()),
		)
	}
	return row, true, nil
}

type knowledgeBaseContentCount struct {
	docCount   int64
	imageCount int64
}

// loadKnowledgeBaseContentCounts 批量加载知识库下的文档和图片数量。
// svcCtx 中的 DocumentRepo/ImageRepo 由 ServiceContext 初始化流程保证存在。
func loadKnowledgeBaseContentCounts(ctx context.Context, svcCtx *svc.ServiceContext, userID uuid.UUID, rows []*models.KnowledgeBase) (map[uuid.UUID]knowledgeBaseContentCount, error) {
	out := map[uuid.UUID]knowledgeBaseContentCount{}
	if len(rows) == 0 {
		return out, nil
	}

	seen := map[uuid.UUID]struct{}{}
	kbIDs := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		if _, ok := seen[row.ID]; ok {
			continue
		}
		seen[row.ID] = struct{}{}
		kbIDs = append(kbIDs, row.ID)
	}
	if len(kbIDs) == 0 {
		return out, nil
	}

	docCounts, err := svcCtx.DocumentRepo.CountByKnowledgeBase(ctx, userID, kbIDs)
	if err != nil {
		return nil, err
	}
	imageCounts, err := svcCtx.ImageRepo.CountByKnowledgeBase(ctx, userID, kbIDs)
	if err != nil {
		return nil, err
	}
	for _, id := range kbIDs {
		out[id] = knowledgeBaseContentCount{
			docCount:   docCounts[id],
			imageCount: imageCounts[id],
		}
	}
	return out, nil
}

// loadKnowledgeBaseContentCount 加载单个知识库的内容数量，供详情和更新类接口复用。
func loadKnowledgeBaseContentCount(ctx context.Context, svcCtx *svc.ServiceContext, userID uuid.UUID, row *models.KnowledgeBase) (knowledgeBaseContentCount, error) {
	counts, err := loadKnowledgeBaseContentCounts(ctx, svcCtx, userID, []*models.KnowledgeBase{row})
	if err != nil {
		return knowledgeBaseContentCount{}, err
	}
	if row == nil {
		return knowledgeBaseContentCount{}, nil
	}
	return counts[row.ID], nil
}
