package es

import (
	"context"
	"fmt"
	"strings"

	ragchunker "github.com/boxify/api-go/internal/core/rag/chunker"
	"github.com/boxify/api-go/internal/core/valuex"
	infraes "github.com/boxify/api-go/internal/infrastructure/db/es"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

const DefaultChunkIndex = "boxify_chunks"

type RAGChunkRepository struct {
	client *infraes.Client
	index  string
}

func NewRAGChunkRepository(client *infraes.Client, index string) repository.RAGChunkRepository {
	if client == nil {
		panic("elasticsearch client is required")
	}
	index = strings.TrimSpace(index)
	if index == "" {
		index = DefaultChunkIndex
	}
	return &RAGChunkRepository{client: client, index: index}
}

// EnsureIndex 确保索引存在
func (r *RAGChunkRepository) EnsureIndex(ctx context.Context, embeddingDim int) error {
	if embeddingDim <= 0 {
		embeddingDim = 1024
	}
	exists, err := r.client.IndexExists(ctx, r.index)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = r.client.CreateIndex(ctx, r.index, chunkIndexMapping(embeddingDim))
	return err
}

// IndexDocumentChunks 索引文档 chunk
func (r *RAGChunkRepository) IndexDocumentChunks(ctx context.Context, doc *models.Document, chunks []*ragchunker.Chunk, vectors [][]float64) error {
	chunkDocs, err := r.buildChunkDocuments(doc, chunks, vectors)
	if err != nil {
		return err
	}
	for _, chunk := range chunkDocs {
		if _, err := r.client.Index(ctx, r.index, chunk.ChunkID, chunk); err != nil {
			return err
		}
	}
	return nil
}

// IndexImageChunk 索引图片描述 chunk。
//
// document_id 字段复用为图片 ID，便于 DeleteByDocument / UpdateTags 统一按来源清理。
func (r *RAGChunkRepository) IndexImageChunk(ctx context.Context, image *models.Image, content string, vector []float64) error {
	if image == nil {
		return xerr.Internal("图片为空", nil)
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return xerr.Internal("图片描述内容为空", nil)
	}
	if len(vector) == 0 {
		return xerr.Internal("图片向量为空", nil)
	}
	kbID := ""
	if image.KBID != nil {
		kbID = image.KBID.String()
	}
	chunkID := deterministicImageChunkID(image.ID).String()
	doc := models.RAGChunkDocument{
		ChunkID:    chunkID,
		DocumentID: image.ID.String(),
		UserID:     image.UserID.String(),
		KBID:       kbID,
		DocName:    image.FileName,
		SourceType: "image",
		Content:    content,
		Level:      "parent",
		Tags:       documentTagNames(image.Tags),
		Vector:     vector,
	}
	if _, err := r.client.Index(ctx, r.index, chunkID, doc); err != nil {
		return err
	}
	return nil
}

// DeleteByDocument 根据文档删除
func (r *RAGChunkRepository) DeleteByDocument(ctx context.Context, userID uuid.UUID, documentID uuid.UUID) error {
	_, err := r.client.DeleteByQuery(ctx, r.index, map[string]any{
		"query": map[string]any{
			"bool": map[string]any{
				"filter": []any{
					map[string]any{"term": map[string]any{"user_id": userID.String()}},
					map[string]any{"term": map[string]any{"document_id": documentID.String()}},
				},
			},
		},
	})
	return err
}

// UpdateKnowledgeBase 更新文档 chunk 的知识库归属
func (r *RAGChunkRepository) UpdateKnowledgeBase(ctx context.Context, userID uuid.UUID, documentID uuid.UUID, kbID uuid.UUID) error {
	_, err := r.client.UpdateByQuery(ctx, r.index, map[string]any{
		"script": map[string]any{
			"source": "ctx._source.kb_id = params.kb_id",
			"params": map[string]any{
				"kb_id": kbID.String(),
			},
		},
		"query": map[string]any{
			"bool": map[string]any{
				"filter": []any{
					map[string]any{"term": map[string]any{"user_id": userID.String()}},
					map[string]any{"term": map[string]any{"document_id": documentID.String()}},
				},
			},
		},
	})
	return err
}

// UpdateTags 更新文档 chunk 的标签。
func (r *RAGChunkRepository) UpdateTags(ctx context.Context, userID uuid.UUID, documentID uuid.UUID, tags []string) error {
	_, err := r.client.UpdateByQuery(ctx, r.index, map[string]any{
		"script": map[string]any{
			"source": "ctx._source.tags = params.tags",
			"params": map[string]any{
				"tags": tags,
			},
		},
		"query": map[string]any{
			"bool": map[string]any{
				"filter": []any{
					map[string]any{"term": map[string]any{"user_id": userID.String()}},
					map[string]any{"term": map[string]any{"document_id": documentID.String()}},
				},
			},
		},
	})
	return err
}

// DecodeSource 解码源数据
func (r *RAGChunkRepository) DecodeSource(src map[string]any) (models.RAGChunkSource, error) {
	chunkID, err := uuid.Parse(valuex.String(src["chunk_id"]))
	if err != nil {
		return models.RAGChunkSource{}, fmt.Errorf("invalid chunk_id: %w", err)
	}
	documentID, err := uuid.Parse(valuex.String(src["document_id"]))
	if err != nil {
		return models.RAGChunkSource{}, fmt.Errorf("invalid document_id: %w", err)
	}
	var kbID *uuid.UUID
	if rawKBID := valuex.String(src["kb_id"]); rawKBID != "" {
		parsed, err := uuid.Parse(rawKBID)
		if err != nil {
			return models.RAGChunkSource{}, fmt.Errorf("invalid kb_id: %w", err)
		}
		kbID = &parsed
	}
	return models.RAGChunkSource{
		ChunkID:    chunkID,
		DocumentID: documentID,
		KBID:       kbID,
		DocName:    valuex.String(src["doc_name"]),
		SourceType: valuex.String(src["source_type"]),
	}, nil
}

// buildChunkDocuments 构建文档 chunk 数据
func (r *RAGChunkRepository) buildChunkDocuments(doc *models.Document, chunks []*ragchunker.Chunk, vectors [][]float64) ([]models.RAGChunkDocument, error) {
	if doc == nil {
		return nil, xerr.Internal("文档为空", nil)
	}
	texts := chunkTexts(chunks)
	if len(texts) != len(vectors) {
		return nil, xerr.Internal("文档 chunk 向量数量不匹配", nil)
	}
	tags := documentTagNames(doc.Tags)
	kbID := ""
	if doc.KBID != nil {
		kbID = doc.KBID.String()
	}
	out := make([]models.RAGChunkDocument, 0, len(texts))
	vectorIndex := 0
	for parentIndex, parent := range chunks {
		if parent == nil {
			continue
		}
		parentContent := strings.TrimSpace(parent.Content)
		parentID := deterministicChunkID(doc.ID, parentIndex, -1).String()
		if parentContent != "" {
			out = append(out, models.RAGChunkDocument{
				ChunkID:    parentID,
				DocumentID: doc.ID.String(),
				UserID:     doc.UserID.String(),
				KBID:       kbID,
				DocName:    doc.FileName,
				SourceType: doc.SourceType,
				Content:    parentContent,
				Level:      "parent",
				Tags:       tags,
				Vector:     vectors[vectorIndex],
			})
			vectorIndex++
		}
		for childIndex, child := range parent.Children {
			childContent := strings.TrimSpace(child)
			if childContent == "" {
				continue
			}
			childID := deterministicChunkID(doc.ID, parentIndex, childIndex).String()
			out = append(out, models.RAGChunkDocument{
				ChunkID:    childID,
				ParentID:   parentID,
				DocumentID: doc.ID.String(),
				UserID:     doc.UserID.String(),
				KBID:       kbID,
				DocName:    doc.FileName,
				SourceType: doc.SourceType,
				Content:    childContent,
				Level:      "child",
				Tags:       tags,
				Vector:     vectors[vectorIndex],
			})
			vectorIndex++
		}
	}
	return out, nil
}

func chunkTexts(chunks []*ragchunker.Chunk) []string {
	texts := make([]string, 0)
	for _, parent := range chunks {
		if parent == nil {
			continue
		}
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

func chunkIndexMapping(embeddingDim int) map[string]any {
	return map[string]any{
		"mappings": map[string]any{
			"properties": map[string]any{
				"chunk_id":    map[string]any{"type": "keyword"},
				"parent_id":   map[string]any{"type": "keyword"},
				"document_id": map[string]any{"type": "keyword"},
				"user_id":     map[string]any{"type": "keyword"},
				"kb_id":       map[string]any{"type": "keyword"},
				"doc_name":    map[string]any{"type": "keyword"},
				"source_type": map[string]any{"type": "keyword"},
				"level":       map[string]any{"type": "keyword"},
				"tags":        map[string]any{"type": "keyword"},
				"content":     map[string]any{"type": "text"},
				"vector": map[string]any{
					"type":       "dense_vector",
					"dims":       embeddingDim,
					"index":      true,
					"similarity": "cosine",
				},
			},
		},
	}
}

func documentTagNames(rows []models.Tag) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if name := strings.TrimSpace(row.Name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// deterministicChunkID 确定性 chunk ID
func deterministicChunkID(documentID uuid.UUID, parentIndex int, childIndex int) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("boxify:document:%s:%d:%d", documentID.String(), parentIndex, childIndex)))
}

// deterministicImageChunkID 生成图片描述 chunk 的确定性 ID。
func deterministicImageChunkID(imageID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("boxify:image:%s:0", imageID.String())))
}
