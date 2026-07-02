package tasks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	corellm "github.com/boxify/api-go/internal/core/llm"
	ragchunker "github.com/boxify/api-go/internal/core/rag/chunker"
	ragclassifier "github.com/boxify/api-go/internal/core/rag/classifier"
	ragparser "github.com/boxify/api-go/internal/core/rag/documentparse"
	"github.com/boxify/api-go/internal/domain"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type ParseDocumentTask struct {
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

func NewParseDocumentTask(svcCtx *svc.ServiceContext) *ParseDocumentTask {
	return &ParseDocumentTask{
		svcCtx: svcCtx,
		log:    xlog.Component("worker.tasks.parse_document"),
	}
}

func (h *ParseDocumentTask) Handle(ctx context.Context, task *domain.Task) error {
	payload, err := parseTaskPayload(task)
	if err != nil {
		return skipRetry(fmt.Errorf("解析文档任务 payload 失败: %w", err))
	}
	if h == nil || h.svcCtx == nil || h.svcCtx.DocumentRepo == nil || h.svcCtx.Storage == nil {
		return xerr.Internal("文档解析任务依赖未初始化", nil)
	}

	doc, err := h.svcCtx.DocumentRepo.FindByID(ctx, payload.UserID, payload.DocumentID)
	if err != nil {
		if xerr.From(err).Kind == xerr.KindNotFound {
			h.log.WarnContext(ctx, "文档不存在，跳过解析任务",
				slog.String("user_id", payload.UserID.String()),
				slog.String("document_id", payload.DocumentID.String()),
			)
			return skipRetry(err)
		}
		return err
	}

	h.log.InfoContext(ctx, "开始解析文档",
		slog.String("user_id", payload.UserID.String()),
		slog.String("document_id", payload.DocumentID.String()),
		slog.String("file_ext", doc.FileExt),
	)

	if err := h.updateParseState(ctx, doc, &models.Document{
		Status:   domain.DocumentStatusParsing,
		Progress: 0.1,
		ErrorMsg: nil,
	}, repository.NewDocumentUpdateFields().Status().Progress().ErrorMsg()); err != nil {
		return err
	}

	content, err := h.svcCtx.Storage.Get(ctx, doc.FileKey)
	if err != nil {
		_ = h.markParseFailed(ctx, doc, err)
		return err
	}
	h.log.InfoContext(ctx, "文档原始文件读取完成",
		slog.String("user_id", doc.UserID.String()),
		slog.String("document_id", doc.ID.String()),
		slog.String("file_ext", doc.FileExt),
		slog.Int("file_size", len(content)),
	)

	if h.svcCtx.RAGDocumentParser == nil || h.svcCtx.RAGChunker == nil {
		err := xerr.Internal("文档解析任务依赖未初始化", nil)
		_ = h.markParseFailed(ctx, doc, err)
		return nil
	}
	parsed, err := h.svcCtx.RAGDocumentParser.Parse(ctx, ragparser.Input{Data: content, FileExt: doc.FileExt})
	if err != nil {
		_ = h.markParseFailed(ctx, doc, err)
		return nil
	}
	h.log.InfoContext(ctx, "文档内容解析完成",
		slog.String("user_id", doc.UserID.String()),
		slog.String("document_id", doc.ID.String()),
		slog.Int("text_runes", len([]rune(parsed.Text))),
	)
	if err := h.updateParseState(ctx, doc, &models.Document{
		Progress: 0.3,
	}, repository.NewDocumentUpdateFields().Progress()); err != nil {
		return err
	}
	chunks := h.svcCtx.RAGChunker.Chunk(parsed.Text)
	if len(chunks) == 0 {
		err := errors.New("解析结果为空")
		_ = h.markParseFailed(ctx, doc, err)
		return nil
	}
	h.log.InfoContext(ctx, "文档分块完成",
		slog.String("user_id", doc.UserID.String()),
		slog.String("document_id", doc.ID.String()),
		slog.Int("parent_chunk_count", len(chunks)),
	)
	if err := h.updateParseState(ctx, doc, &models.Document{
		Progress: 0.8,
	}, repository.NewDocumentUpdateFields().Progress()); err != nil {
		return err
	}
	if h.svcCtx.RAGChunkRepo == nil || h.svcCtx.LLMManager == nil || h.svcCtx.TagRepo == nil {
		err := xerr.Internal("文档解析任务依赖未初始化", nil)
		_ = h.markParseFailed(ctx, doc, err)
		return nil
	}
	texts := documentChunkTexts(chunks)
	llmClient, err := h.embeddingClient(ctx, doc.UserID)
	if err != nil {
		_ = h.markParseFailed(ctx, doc, err)
		return nil
	}
	vectors, err := llmClient.Embed(ctx, texts, h.svcCtx.Config.Rag.EmbeddingDim, corellm.WithEmbeddingBatchSize(h.svcCtx.Config.Rag.EmbeddingBatchSize))
	if err != nil {
		_ = h.markParseFailed(ctx, doc, err)
		return nil
	}
	h.log.InfoContext(ctx, "文档向量化完成",
		slog.String("user_id", doc.UserID.String()),
		slog.String("document_id", doc.ID.String()),
		slog.Int("embedding_count", len(vectors)),
		slog.Int("embedding_dim", vectorDimension(vectors)),
	)
	if err := h.svcCtx.RAGChunkRepo.EnsureIndex(ctx, h.svcCtx.Config.Rag.EmbeddingDim); err != nil {
		_ = h.markParseFailed(ctx, doc, err)
		return nil
	}
	h.log.InfoContext(ctx, "文档 chunk 索引已确认",
		slog.String("user_id", doc.UserID.String()),
		slog.String("document_id", doc.ID.String()),
	)
	if err := h.svcCtx.RAGChunkRepo.DeleteByDocument(ctx, doc.UserID, doc.ID); err != nil {
		_ = h.markParseFailed(ctx, doc, err)
		return nil
	}
	if err := h.svcCtx.RAGChunkRepo.IndexDocumentChunks(ctx, doc, chunks, vectors); err != nil {
		_ = h.markParseFailed(ctx, doc, err)
		return nil
	}
	h.log.InfoContext(ctx, "文档 chunk 写入完成",
		slog.String("user_id", doc.UserID.String()),
		slog.String("document_id", doc.ID.String()),
		slog.Int("indexed_chunk_count", len(texts)),
	)
	tags := h.classifyDocumentTags(ctx, doc, parsed.Text)
	h.log.InfoContext(ctx, "文档自动标签提取完成",
		slog.String("user_id", doc.UserID.String()),
		slog.String("document_id", doc.ID.String()),
		slog.Int("tag_count", len(tags)),
	)
	syncedTags, err := h.svcCtx.TagRepo.SyncDocumentTags(ctx, doc.UserID, doc.ID, tags)
	if err != nil {
		_ = h.markParseFailed(ctx, doc, err)
		return nil
	}
	doc.Tags = syncedTags
	h.log.InfoContext(ctx, "文档标签已同步到数据库",
		slog.String("user_id", doc.UserID.String()),
		slog.String("document_id", doc.ID.String()),
		slog.Int("tag_count", len(tags)),
	)
	if err := h.svcCtx.RAGChunkRepo.UpdateTags(ctx, doc.UserID, doc.ID, tags); err != nil {
		_ = h.markParseFailed(ctx, doc, err)
		return nil
	}
	h.log.InfoContext(ctx, "文档标签已同步到 Elasticsearch",
		slog.String("user_id", doc.UserID.String()),
		slog.String("document_id", doc.ID.String()),
		slog.Int("tag_count", len(tags)),
	)

	if err := h.updateParseState(ctx, doc, &models.Document{
		Status:   domain.DocumentStatusDone,
		Progress: 1,
		ChunkNum: int64(len(chunks)),
		ErrorMsg: nil,
	}, repository.NewDocumentUpdateFields().Status().Progress().ChunkNum().ErrorMsg()); err != nil {
		return err
	}
	h.log.InfoContext(ctx, "文档解析完成",
		slog.String("user_id", payload.UserID.String()),
		slog.String("document_id", payload.DocumentID.String()),
		slog.Int("chunk_count", len(chunks)),
		slog.Int("indexed_chunk_count", len(texts)),
		slog.Int("tag_count", len(tags)),
	)
	return nil
}

func parseTaskPayload(task *domain.Task) (*domain.ParseDocumentPayload, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}
	payload, ok := task.Payload.(*domain.ParseDocumentPayload)
	if !ok || payload == nil {
		return nil, fmt.Errorf("payload type = %T", task.Payload)
	}
	if payload.UserID == uuid.Nil || payload.DocumentID == uuid.Nil {
		return nil, fmt.Errorf("payload ids are required")
	}
	return payload, nil
}

func (h *ParseDocumentTask) markParseFailed(ctx context.Context, doc *models.Document, cause error) error {
	message := cause.Error()
	if h != nil && h.log != nil {
		h.log.WarnContext(ctx, "文档解析失败",
			slog.String("document_id", doc.ID.String()),
			slog.String("error", message),
		)
	}
	return h.updateParseState(ctx, doc, &models.Document{
		Status:   domain.DocumentStatusFailed,
		Progress: doc.Progress,
		ErrorMsg: &message,
	}, repository.NewDocumentUpdateFields().Status().Progress().ErrorMsg())
}

func (h *ParseDocumentTask) updateParseState(ctx context.Context, doc *models.Document, patch *models.Document, fields *repository.DocumentUpdateFields) error {
	_, err := h.svcCtx.DocumentRepo.UpdateFields(ctx, doc.UserID, doc.ID, patch, fields)
	if err != nil {
		return err
	}
	for _, column := range fields.Columns() {
		switch column {
		case "status":
			doc.Status = patch.Status
		case "progress":
			doc.Progress = patch.Progress
		case "chunk_num":
			doc.ChunkNum = patch.ChunkNum
		case "error_msg":
			doc.ErrorMsg = patch.ErrorMsg
		}
	}
	return err
}

// embeddingClient 获取用户默认的向量模型客户端
func (h *ParseDocumentTask) embeddingClient(ctx context.Context, userID uuid.UUID) (corellm.Client, error) {
	if h == nil || h.svcCtx == nil || h.svcCtx.ModelConfigRepo == nil || h.svcCtx.SecretCipher == nil || h.svcCtx.LLMManager == nil {
		return nil, xerr.Internal("向量模型依赖未初始化", nil)
	}
	modelType := domain.EmbeddingModelType
	configs, err := h.svcCtx.ModelConfigRepo.List(ctx, userID, &modelType)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		return nil, xerr.BadRequest("未配置向量模型，请先在模型配置中添加")
	}
	defaultConfig := configs[0]
	for _, config := range configs {
		if config.IsDefault {
			defaultConfig = config
			break
		}
	}
	apiKey, err := h.svcCtx.SecretCipher.Decrypt(defaultConfig.APIKeyEncrypted)
	if err != nil {
		return nil, xerr.Internal("模型 API Key 解密失败", err)
	}
	return h.svcCtx.LLMManager.NewClient(corellm.ModelConfig{
		Provider:       defaultConfig.Provider,
		Model:          defaultConfig.ModelName,
		APIKey:         apiKey,
		BaseURL:        defaultConfig.BaseURL,
		EmbeddingModel: defaultConfig.ModelName,
	})
}

// chatClient 获取用户默认的文本模型客户端
func (h *ParseDocumentTask) chatClient(ctx context.Context, userID uuid.UUID) (corellm.Client, error) {
	if h == nil || h.svcCtx == nil || h.svcCtx.ModelConfigRepo == nil || h.svcCtx.SecretCipher == nil || h.svcCtx.LLMManager == nil {
		return nil, xerr.Internal("文本模型依赖未初始化", nil)
	}
	modelType := domain.ChatModelType
	configs, err := h.svcCtx.ModelConfigRepo.List(ctx, userID, &modelType)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		return nil, xerr.BadRequest("未配置文本模型，请先在模型配置中添加")
	}
	defaultConfig := configs[0]
	for _, config := range configs {
		if config.IsDefault {
			defaultConfig = config
			break
		}
	}
	apiKey, err := h.svcCtx.SecretCipher.Decrypt(defaultConfig.APIKeyEncrypted)
	if err != nil {
		return nil, xerr.Internal("模型 API Key 解密失败", err)
	}
	return h.svcCtx.LLMManager.NewClient(corellm.ModelConfig{
		Provider: defaultConfig.Provider,
		Model:    defaultConfig.ModelName,
		APIKey:   apiKey,
		BaseURL:  defaultConfig.BaseURL,
	})
}

// classifyDocumentTags 使用分类器为文档添加标签
func (h *ParseDocumentTask) classifyDocumentTags(ctx context.Context, doc *models.Document, content string) []string {
	existingTags := documentTagNames(doc.Tags)
	if h == nil || h.svcCtx == nil || h.svcCtx.RAGClassifier == nil {
		if h != nil && h.log != nil {
			h.log.WarnContext(ctx, "文档分类器未初始化，跳过自动标签提取",
				slog.String("document_id", doc.ID.String()),
			)
		}
		return existingTags
	}
	llmClient, err := h.chatClient(ctx, doc.UserID)
	if err != nil {
		h.log.WarnContext(ctx, "文档自动标签模型不可用，使用已有标签",
			slog.String("document_id", doc.ID.String()),
			slog.String("error", err.Error()),
		)
		return existingTags
	}
	result, err := h.svcCtx.RAGClassifier.Classify(ctx, ragclassifier.Input{
		Content:      content,
		ExistingTags: existingTags,
	}, ragclassifier.WithInputClient(llmClient))
	if err != nil {
		h.log.WarnContext(ctx, "文档自动标签提取失败，使用已有标签",
			slog.String("document_id", doc.ID.String()),
			slog.String("error", err.Error()),
		)
		return existingTags
	}
	if result == nil {
		return existingTags
	}
	return mergeTagNames(existingTags, result.Tags)
}

func skipRetry(err error) error {
	return errors.Join(err, asynq.SkipRetry)
}

func documentChunkTexts(chunks []*ragchunker.Chunk) []string {
	texts := make([]string, 0)
	for _, parent := range chunks {
		if parent == nil {
			continue
		}
		// 按 parent 后接 children 的顺序提取文本，确保 embedding 结果与后续索引写入顺序一致。
		if content := strings.TrimSpace(parent.Content); content != "" {
			texts = append(texts, content)
		}
		for _, child := range parent.Children {
			if content := strings.TrimSpace(child); content != "" {
				texts = append(texts, content)
			}
		}
	}
	return texts
}

func vectorDimension(vectors [][]float64) int {
	for _, vector := range vectors {
		if len(vector) != 0 {
			return len(vector)
		}
	}
	return 0
}

// documentTagNames 转换标签行数据为字符串切片
func documentTagNames(rows []models.Tag) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if name := strings.TrimSpace(row.Name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// mergeTagNames 合并标签名称切片，去重
func mergeTagNames(base []string, extra []string) []string {
	out := make([]string, 0, len(base)+len(extra))
	seen := map[string]struct{}{}
	for _, tags := range [][]string{base, extra} {
		for _, tag := range tags {
			name := strings.TrimSpace(tag)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	return out
}
