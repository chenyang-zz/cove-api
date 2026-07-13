package tasks

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	corellm "github.com/boxify/api-go/internal/core/llm"
	ragclassifier "github.com/boxify/api-go/internal/core/rag/classifier"
	ragimagedescribe "github.com/boxify/api-go/internal/core/rag/imagedescribe"
	"github.com/boxify/api-go/internal/domain/types"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

type ParseImageTask struct {
	svcCtx    *svc.ServiceContext
	log       *slog.Logger
	describer ragimagedescribe.DescriberAPI // 可选注入；nil 时按 multimodal 客户端构造默认 Describer
}

func NewParseImageTask(svcCtx *svc.ServiceContext) *ParseImageTask {
	return &ParseImageTask{
		svcCtx: svcCtx,
		log:    xlog.Component("worker.tasks.parse_image"),
	}
}

// WithDescriber 注入自定义描述器（测试或替代实现）。
func (h *ParseImageTask) WithDescriber(describer ragimagedescribe.DescriberAPI) *ParseImageTask {
	if h != nil {
		h.describer = describer
	}
	return h
}

func (h *ParseImageTask) Handle(ctx context.Context, task *types.Task) error {
	payload, err := parseImageTaskPayload(task)
	if err != nil {
		return skipRetry(fmt.Errorf("解析图片任务 payload 失败: %w", err))
	}
	if h == nil || h.svcCtx == nil || h.svcCtx.ImageRepo == nil || h.svcCtx.Storage == nil {
		return xerr.Internal("图片解析任务依赖未初始化", nil)
	}

	img, err := h.svcCtx.ImageRepo.FindByID(ctx, payload.UserID, payload.ImageID)
	if err != nil {
		if xerr.From(err).Kind == xerr.KindNotFound {
			h.log.WarnContext(ctx, "图片不存在，跳过解析任务",
				slog.String("user_id", payload.UserID.String()),
				slog.String("image_id", payload.ImageID.String()),
			)
			return skipRetry(err)
		}
		return err
	}

	h.log.InfoContext(ctx, "开始解析图片",
		slog.String("user_id", payload.UserID.String()),
		slog.String("image_id", payload.ImageID.String()),
		slog.String("file_ext", img.FileExt),
	)

	if err := h.updateParseState(ctx, img, &models.Image{
		Status:   types.ImageStatusProcessing,
		Progress: 0.1,
		ErrorMsg: nil,
	}, repository.NewImageUpdateFields().Status().Progress().ErrorMsg()); err != nil {
		return err
	}

	content, err := h.svcCtx.Storage.Get(ctx, img.FileKey)
	if err != nil {
		_ = h.markParseFailed(ctx, img, err)
		return err
	}
	if err := h.updateParseState(ctx, img, &models.Image{
		Progress: 0.3,
	}, repository.NewImageUpdateFields().Progress()); err != nil {
		return err
	}
	h.log.InfoContext(ctx, "图片原始文件读取完成",
		slog.String("user_id", img.UserID.String()),
		slog.String("image_id", img.ID.String()),
		slog.Int("file_size", len(content)),
	)

	desc, err := h.describeImage(ctx, img, content)
	if err != nil {
		_ = h.markParseFailed(ctx, img, err)
		return nil
	}
	descText := strings.TrimSpace(desc.Description)
	ocrText := strings.TrimSpace(desc.OCRText)
	sceneText := strings.TrimSpace(desc.Scene)
	var descriptionPtr *string
	var ocrPtr *string
	var scenePtr *string
	if descText != "" {
		descriptionPtr = &descText
	}
	if ocrText != "" {
		ocrPtr = &ocrText
	}
	if sceneText != "" {
		scenePtr = &sceneText
	}
	if err := h.updateParseState(ctx, img, &models.Image{
		Description: descriptionPtr,
		OCRText:     ocrPtr,
		Objects:     models.JSONStrings(desc.Objects),
		Scene:       scenePtr,
		Progress:    0.6,
	}, repository.NewImageUpdateFields().Description().OCRText().Objects().Scene().Progress()); err != nil {
		return err
	}
	h.log.InfoContext(ctx, "图片多模态描述完成",
		slog.String("image_id", img.ID.String()),
		slog.String("scene", sceneText),
		slog.Int("object_count", len(desc.Objects)),
	)

	searchable := imageSearchableText(descText, ocrText, sceneText)
	if searchable != "" {
		if err := h.indexImageDescription(ctx, img, searchable); err != nil {
			_ = h.markParseFailed(ctx, img, err)
			return nil
		}
	} else {
		h.log.WarnContext(ctx, "图片描述为空，跳过向量索引",
			slog.String("image_id", img.ID.String()),
		)
	}
	if err := h.updateParseState(ctx, img, &models.Image{
		Progress: 0.8,
	}, repository.NewImageUpdateFields().Progress()); err != nil {
		return err
	}

	// 自动标签失败不阻断完成（对齐 Comet best-effort）。
	tags := h.classifyImageTags(ctx, img, searchable)
	if h.svcCtx.TagRepo != nil {
		if syncedTags, err := h.svcCtx.TagRepo.SyncImageTags(ctx, img.UserID, img.ID, tags); err != nil {
			h.log.WarnContext(ctx, "同步图片标签失败（忽略）",
				slog.String("image_id", img.ID.String()),
				slog.String("error", err.Error()),
			)
		} else {
			img.Tags = syncedTags
			tags = imageTagNames(syncedTags)
			if searchable != "" && h.svcCtx.RAGChunkRepo != nil {
				if err := h.svcCtx.RAGChunkRepo.UpdateTags(ctx, img.UserID, img.ID, tags); err != nil {
					h.log.WarnContext(ctx, "更新图片 ES 标签失败（忽略）",
						slog.String("image_id", img.ID.String()),
						slog.String("error", err.Error()),
					)
				}
			}
		}
	}

	if err := h.updateParseState(ctx, img, &models.Image{
		Status:   types.ImageStatusDone,
		Progress: 1,
		ErrorMsg: nil,
	}, repository.NewImageUpdateFields().Status().Progress().ErrorMsg()); err != nil {
		return err
	}
	h.log.InfoContext(ctx, "图片解析完成",
		slog.String("user_id", payload.UserID.String()),
		slog.String("image_id", payload.ImageID.String()),
		slog.Int("tag_count", len(tags)),
	)
	return nil
}

func parseImageTaskPayload(task *types.Task) (*types.ParseImagePayload, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}
	payload, ok := task.Payload.(*types.ParseImagePayload)
	if !ok || payload == nil {
		return nil, fmt.Errorf("payload type = %T", task.Payload)
	}
	if payload.UserID == uuid.Nil || payload.ImageID == uuid.Nil {
		return nil, fmt.Errorf("payload ids are required")
	}
	return payload, nil
}

// describeImage 使用默认或注入的 Describer 生成图片描述。
func (h *ParseImageTask) describeImage(ctx context.Context, img *models.Image, content []byte) (*ragimagedescribe.Description, error) {
	describer := h.describer
	if describer == nil {
		client, err := svc.MultimodalClient(ctx, h.svcCtx, img.UserID)
		if err != nil {
			return nil, err
		}
		vision, ok := client.(corellm.VisionClient)
		if !ok || vision == nil {
			return nil, xerr.Internal("多模态模型不支持图片描述", nil)
		}
		// Describer 内部直接使用 corellm.VisionClient，无需外部适配器。
		describer = ragimagedescribe.NewDescriber(vision)
	}
	return describer.Describe(ctx, ragimagedescribe.Input{
		Data:    content,
		FileExt: img.FileExt,
	})
}

func (h *ParseImageTask) indexImageDescription(ctx context.Context, img *models.Image, searchable string) error {
	if h.svcCtx.RAGChunkRepo == nil || h.svcCtx.LLMManager == nil {
		return xerr.Internal("图片索引依赖未初始化", nil)
	}
	embedClient, err := svc.EmbeddingClient(ctx, h.svcCtx, img.UserID)
	if err != nil {
		return err
	}
	vector, err := embedClient.EmbedOne(ctx, searchable, h.svcCtx.Config.Rag.EmbeddingDim)
	if err != nil {
		return err
	}
	if err := h.svcCtx.RAGChunkRepo.EnsureIndex(ctx, h.svcCtx.Config.Rag.EmbeddingDim); err != nil {
		return err
	}
	if err := h.svcCtx.RAGChunkRepo.DeleteByDocument(ctx, img.UserID, img.ID); err != nil {
		return err
	}
	if err := h.svcCtx.RAGChunkRepo.IndexImageChunk(ctx, img, searchable, vector); err != nil {
		return err
	}
	h.log.InfoContext(ctx, "图片描述向量写入完成",
		slog.String("image_id", img.ID.String()),
		slog.Int("embedding_dim", len(vector)),
	)
	return nil
}

func (h *ParseImageTask) classifyImageTags(ctx context.Context, img *models.Image, content string) []string {
	existingTags := imageTagNames(img.Tags)
	if strings.TrimSpace(content) == "" {
		return existingTags
	}
	if h == nil || h.svcCtx == nil || h.svcCtx.RAGClassifier == nil {
		if h != nil && h.log != nil {
			h.log.WarnContext(ctx, "图片分类器未初始化，跳过自动标签提取",
				slog.String("image_id", img.ID.String()),
			)
		}
		return existingTags
	}
	llmClient, err := svc.ChatClient(ctx, h.svcCtx, img.UserID)
	if err != nil {
		h.log.WarnContext(ctx, "图片自动标签模型不可用，使用已有标签",
			slog.String("image_id", img.ID.String()),
			slog.String("error", err.Error()),
		)
		return existingTags
	}
	result, err := h.svcCtx.RAGClassifier.Classify(ctx, ragclassifier.Input{
		Content:      content,
		ExistingTags: existingTags,
	}, ragclassifier.WithInputClient(llmClient))
	if err != nil {
		h.log.WarnContext(ctx, "图片自动标签提取失败，使用已有标签",
			slog.String("image_id", img.ID.String()),
			slog.String("error", err.Error()),
		)
		return existingTags
	}
	if result == nil {
		return existingTags
	}
	return mergeTagNames(existingTags, result.Tags)
}

func (h *ParseImageTask) markParseFailed(ctx context.Context, img *models.Image, cause error) error {
	message := cause.Error()
	if h != nil && h.log != nil {
		h.log.WarnContext(ctx, "图片解析失败",
			slog.String("image_id", img.ID.String()),
			slog.String("error", message),
		)
	}
	return h.updateParseState(ctx, img, &models.Image{
		Status:   types.ImageStatusFailed,
		Progress: img.Progress,
		ErrorMsg: &message,
	}, repository.NewImageUpdateFields().Status().Progress().ErrorMsg())
}

func (h *ParseImageTask) updateParseState(ctx context.Context, img *models.Image, patch *models.Image, fields *repository.ImageUpdateFields) error {
	_, err := h.svcCtx.ImageRepo.UpdateFields(ctx, img.UserID, img.ID, patch, fields)
	if err != nil {
		return err
	}
	for _, column := range fields.Columns() {
		switch column {
		case "status":
			img.Status = patch.Status
		case "progress":
			img.Progress = patch.Progress
		case "error_msg":
			img.ErrorMsg = patch.ErrorMsg
		case "description":
			img.Description = patch.Description
		case "ocr_text":
			img.OCRText = patch.OCRText
		case "objects":
			img.Objects = patch.Objects
		case "scene":
			img.Scene = patch.Scene
		}
	}
	return nil
}

func imageSearchableText(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			out = append(out, value)
		}
	}
	return strings.Join(out, "\n")
}

func imageTagNames(rows []models.Tag) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if name := strings.TrimSpace(row.Name); name != "" {
			out = append(out, name)
		}
	}
	return out
}
