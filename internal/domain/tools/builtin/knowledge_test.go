package builtin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	corellm "github.com/boxify/api-go/internal/core/llm"
	ragsearch "github.com/boxify/api-go/internal/core/rag/search"
	coretool "github.com/boxify/api-go/internal/core/tool"
	"github.com/boxify/api-go/internal/domain/types"
	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/util"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

// 验证 knowledge_search 描述不会暴露 user_id 或 kb_ids 参数，范围只能来自 context。
func TestKnowledgeSearchDescriptorHidesTrustedContextFields(t *testing.T) {
	tool := NewKnowledgeSearchTool(&svc.ServiceContext{})

	descriptor, err := tool.Describe(context.Background())
	if err != nil {
		t.Fatalf("knowledge_search Describe() error = %v, want nil", err)
	}
	properties := descriptor.Schema.Parameters.Properties
	if _, ok := properties["user_id"]; ok {
		t.Fatalf("knowledge_search schema properties = %#v, want no user_id", properties)
	}
	if _, ok := properties["kb_ids"]; ok {
		t.Fatalf("knowledge_search schema properties = %#v, want no kb_ids", properties)
	}
	if descriptor.Schema.Parameters.AdditionalProperties != false {
		t.Fatalf("knowledge_search additionalProperties = %#v, want false", descriptor.Schema.Parameters.AdditionalProperties)
	}
}

// 验证知识库工具会把精确命中的已有标签转换为 RAG tags 过滤条件。
func TestKnowledgeSearchToolAppliesExactMatchedTagFilter(t *testing.T) {
	userID := uuid.New()
	kbID := uuid.New()
	documentID := uuid.New()
	chunkID := uuid.New()
	esClient := &fakeKnowledgeToolES{
		chunkID:    chunkID,
		documentID: documentID,
		kbID:       kbID,
		userID:     userID,
	}
	svcCtx := newKnowledgeToolTestServiceContext(t, userID, esClient, kbID)
	svcCtx.TagRepo = &fakeKnowledgeToolTagRepo{rows: []*models.Tag{{ID: uuid.New(), UserID: userID, Name: "重要"}}}
	ctx := util.WithUserID(context.Background(), userID)
	ctx = util.WithKnowledgeBaseIDs(ctx, []uuid.UUID{kbID})

	output, err := NewKnowledgeSearchTool(svcCtx).Invoke(ctx, coretool.Input{
		"query": "  hello  ",
		"top_k": float64(3),
		"tags":  []any{"  重要  ", ""},
	})
	if err != nil {
		t.Fatalf("knowledge_search Invoke() error = %v, want nil", err)
	}
	if output.Metadata["query"] != "hello" || output.Metadata["count"] != 1 || output.Metadata["top_k"] != 3 {
		t.Fatalf("knowledge_search metadata summary = %#v, want query/count/top_k", output.Metadata)
	}
	if gotKBIDs, ok := output.Metadata["kb_ids"].([]string); !ok || len(gotKBIDs) != 1 || gotKBIDs[0] != kbID.String() {
		t.Fatalf("knowledge_search metadata[kb_ids] = %#v, want %s", output.Metadata["kb_ids"], kbID)
	}
	results, ok := output.Metadata["results"].([]map[string]any)
	if !ok || len(results) != 1 {
		t.Fatalf("knowledge_search metadata[results] = %#v, want one result", output.Metadata["results"])
	}
	if results[0]["chunk_id"] != chunkID.String() || results[0]["source_id"] != documentID.String() || results[0]["kb_id"] != kbID.String() {
		t.Fatalf("knowledge_search result metadata = %#v, want mapped RAG result ids", results[0])
	}
	queryText := fmt.Sprintf("%#v", esClient.queries)
	if !strings.Contains(queryText, `"user_id":"`+userID.String()) {
		t.Fatalf("knowledge_search queries = %s, want user_id filter", queryText)
	}
	if !strings.Contains(queryText, kbID.String()) || !strings.Contains(queryText, "kb_id") {
		t.Fatalf("knowledge_search queries = %s, want kb_id terms filter", queryText)
	}
	if !strings.Contains(queryText, "重要") || !strings.Contains(queryText, "tags") {
		t.Fatalf("knowledge_search queries = %s, want tags filter", queryText)
	}
	assertStringSliceMetadata(t, output.Metadata, "requested_tags", []string{"重要"})
	assertStringSliceMetadata(t, output.Metadata, "matched_tags", []string{"重要"})
	assertStringSliceMetadata(t, output.Metadata, "unmatched_tags", nil)
	if output.Metadata["tag_filter_applied"] != true {
		t.Fatalf("knowledge_search metadata[tag_filter_applied] = %#v, want true", output.Metadata["tag_filter_applied"])
	}
}

// 验证知识库工具会通过向量相似度把近似标签解析到已有标签。
func TestKnowledgeSearchToolResolvesApproximateTagsWithEmbedding(t *testing.T) {
	userID := uuid.New()
	kbID := uuid.New()
	esClient := &fakeKnowledgeToolES{chunkID: uuid.New(), documentID: uuid.New(), kbID: kbID, userID: userID}
	svcCtx := newKnowledgeToolTestServiceContextWithVectors(t, userID, esClient, map[string][]float64{
		"报销": {1, 0, 0},
		"财务": {0.98, 0.02, 0},
		"产品": {0, 1, 0},
	}, kbID)
	svcCtx.TagRepo = &fakeKnowledgeToolTagRepo{rows: []*models.Tag{
		{ID: uuid.New(), UserID: userID, Name: "财务"},
		{ID: uuid.New(), UserID: userID, Name: "产品"},
	}}
	ctx := util.WithKnowledgeBaseIDs(util.WithUserID(context.Background(), userID), []uuid.UUID{kbID})

	output, err := NewKnowledgeSearchTool(svcCtx).Invoke(ctx, coretool.Input{"query": "hello", "tags": []any{"报销"}})
	if err != nil {
		t.Fatalf("knowledge_search Invoke(approximate tag) error = %v, want nil", err)
	}
	queryText := fmt.Sprintf("%#v", esClient.queries)
	if !strings.Contains(queryText, "财务") || !strings.Contains(queryText, "tags") {
		t.Fatalf("knowledge_search queries = %s, want resolved tag filter 财务", queryText)
	}
	if strings.Contains(queryText, "报销") {
		t.Fatalf("knowledge_search queries = %s, want existing tag filter instead of requested tag", queryText)
	}
	assertStringSliceMetadata(t, output.Metadata, "requested_tags", []string{"报销"})
	assertStringSliceMetadata(t, output.Metadata, "matched_tags", []string{"财务"})
	assertStringSliceMetadata(t, output.Metadata, "unmatched_tags", nil)
	if output.Metadata["tag_filter_applied"] != true {
		t.Fatalf("knowledge_search metadata[tag_filter_applied] = %#v, want true", output.Metadata["tag_filter_applied"])
	}
}

// 验证未知标签不会作为硬过滤条件，避免降低知识库召回率。
func TestKnowledgeSearchToolSkipsTagFilterWhenTagsDoNotMatch(t *testing.T) {
	userID := uuid.New()
	kbID := uuid.New()
	esClient := &fakeKnowledgeToolES{chunkID: uuid.New(), documentID: uuid.New(), kbID: kbID, userID: userID}
	svcCtx := newKnowledgeToolTestServiceContextWithVectors(t, userID, esClient, map[string][]float64{
		"陌生": {1, 0, 0},
		"财务": {0, 1, 0},
	}, kbID)
	svcCtx.TagRepo = &fakeKnowledgeToolTagRepo{rows: []*models.Tag{{ID: uuid.New(), UserID: userID, Name: "财务"}}}
	ctx := util.WithKnowledgeBaseIDs(util.WithUserID(context.Background(), userID), []uuid.UUID{kbID})

	output, err := NewKnowledgeSearchTool(svcCtx).Invoke(ctx, coretool.Input{"query": "hello", "tags": []any{"陌生"}})
	if err != nil {
		t.Fatalf("knowledge_search Invoke(unmatched tag) error = %v, want nil", err)
	}
	queryText := fmt.Sprintf("%#v", esClient.queries)
	if strings.Contains(queryText, "tags") {
		t.Fatalf("knowledge_search queries = %s, want no tags filter", queryText)
	}
	assertStringSliceMetadata(t, output.Metadata, "requested_tags", []string{"陌生"})
	assertStringSliceMetadata(t, output.Metadata, "matched_tags", nil)
	assertStringSliceMetadata(t, output.Metadata, "unmatched_tags", []string{"陌生"})
	if output.Metadata["tag_filter_applied"] != false {
		t.Fatalf("knowledge_search metadata[tag_filter_applied] = %#v, want false", output.Metadata["tag_filter_applied"])
	}
}

// 验证标签仓储失败时工具降级为不按标签过滤，并在 metadata 中暴露原因。
func TestKnowledgeSearchToolSkipsTagFilterWhenTagRepoFails(t *testing.T) {
	userID := uuid.New()
	kbID := uuid.New()
	esClient := &fakeKnowledgeToolES{chunkID: uuid.New(), documentID: uuid.New(), kbID: kbID, userID: userID}
	svcCtx := newKnowledgeToolTestServiceContext(t, userID, esClient, kbID)
	svcCtx.TagRepo = &fakeKnowledgeToolTagRepo{err: errors.New("tag repo unavailable")}
	ctx := util.WithKnowledgeBaseIDs(util.WithUserID(context.Background(), userID), []uuid.UUID{kbID})

	output, err := NewKnowledgeSearchTool(svcCtx).Invoke(ctx, coretool.Input{"query": "hello", "tags": []any{"重要"}})
	if err != nil {
		t.Fatalf("knowledge_search Invoke(tag repo error) error = %v, want nil", err)
	}
	queryText := fmt.Sprintf("%#v", esClient.queries)
	if strings.Contains(queryText, "tags") {
		t.Fatalf("knowledge_search queries = %s, want no tags filter after fail-open", queryText)
	}
	assertStringSliceMetadata(t, output.Metadata, "requested_tags", []string{"重要"})
	assertStringSliceMetadata(t, output.Metadata, "matched_tags", nil)
	assertStringSliceMetadata(t, output.Metadata, "unmatched_tags", []string{"重要"})
	if output.Metadata["tag_filter_applied"] != false {
		t.Fatalf("knowledge_search metadata[tag_filter_applied] = %#v, want false", output.Metadata["tag_filter_applied"])
	}
	if !strings.Contains(fmt.Sprint(output.Metadata["tag_resolution_error"]), "tag repo unavailable") {
		t.Fatalf("knowledge_search metadata[tag_resolution_error] = %#v, want repo error", output.Metadata["tag_resolution_error"])
	}
}

// 验证知识库工具对空 query、非法 top_k 和非法 tags 返回错误。
func TestKnowledgeSearchToolRejectsInvalidInput(t *testing.T) {
	tool := NewKnowledgeSearchTool(&svc.ServiceContext{})

	cases := []struct {
		name  string
		input coretool.Input
	}{
		{name: "empty query", input: coretool.Input{"query": "  "}},
		{name: "missing query", input: coretool.Input{}},
		{name: "low top_k", input: coretool.Input{"query": "hello", "top_k": float64(0)}},
		{name: "high top_k", input: coretool.Input{"query": "hello", "top_k": float64(21)}},
		{name: "bad tags", input: coretool.Input{"query": "hello", "tags": []any{"ok", 1}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tool.Invoke(context.Background(), tc.input)
			if err == nil {
				t.Fatalf("knowledge_search Invoke(%s) error = nil, want error", tc.name)
			}
		})
	}
}

// 验证知识库工具缺少用户或知识库范围时直接返回错误。
func TestKnowledgeSearchToolRequiresTrustedContext(t *testing.T) {
	userID := uuid.New()
	kbID := uuid.New()
	svcCtx := newKnowledgeToolTestServiceContext(t, userID, &fakeKnowledgeToolES{kbID: kbID, userID: userID}, kbID)

	_, err := NewKnowledgeSearchTool(svcCtx).Invoke(context.Background(), coretool.Input{"query": "hello"})
	if xerr.From(err).Kind != xerr.KindUnauthorized {
		t.Fatalf("knowledge_search missing user error = %v, want unauthorized", err)
	}
	ctx := util.WithUserID(context.Background(), userID)
	_, err = NewKnowledgeSearchTool(svcCtx).Invoke(ctx, coretool.Input{"query": "hello"})
	if xerr.From(err).Kind != xerr.KindBadRequest {
		t.Fatalf("knowledge_search missing kb ids error = %v, want bad_request", err)
	}
}

// 验证知识库工具会先校验知识库归属，避免越权检索。
func TestKnowledgeSearchToolValidatesKnowledgeBaseOwnership(t *testing.T) {
	userID := uuid.New()
	ownedKBID := uuid.New()
	missingKBID := uuid.New()
	svcCtx := newKnowledgeToolTestServiceContext(t, userID, &fakeKnowledgeToolES{kbID: ownedKBID, userID: userID}, ownedKBID)
	ctx := util.WithUserID(context.Background(), userID)
	ctx = util.WithKnowledgeBaseIDs(ctx, []uuid.UUID{ownedKBID, missingKBID})

	_, err := NewKnowledgeSearchTool(svcCtx).Invoke(ctx, coretool.Input{"query": "hello"})
	if xerr.From(err).Kind != xerr.KindNotFound {
		t.Fatalf("knowledge_search ownership error = %v, want not_found", err)
	}
}

func newKnowledgeToolTestServiceContext(t *testing.T, userID uuid.UUID, esClient *fakeKnowledgeToolES, kbIDs ...uuid.UUID) *svc.ServiceContext {
	return newKnowledgeToolTestServiceContextWithVectors(t, userID, esClient, nil, kbIDs...)
}

func newKnowledgeToolTestServiceContextWithVectors(t *testing.T, userID uuid.UUID, esClient *fakeKnowledgeToolES, vectors map[string][]float64, kbIDs ...uuid.UUID) *svc.ServiceContext {
	t.Helper()
	cipher, err := security.NewSecretCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretCipher error = %v, want nil", err)
	}
	encryptedAPIKey, err := cipher.Encrypt("test-key")
	if err != nil {
		t.Fatalf("SecretCipher.Encrypt error = %v, want nil", err)
	}
	llmManager := corellm.NewManager()
	llmManager.Register("fake", fakeKnowledgeToolLLMFactory{vectors: vectors})
	return &svc.ServiceContext{
		KnowledgeBaseRepo: &fakeKnowledgeToolKBRepo{rows: knowledgeToolRows(userID, kbIDs...)},
		ModelConfigRepo: &fakeKnowledgeToolModelConfigRepo{rows: []*models.ModelConfig{{
			UserID:          userID,
			Type:            string(types.EmbeddingModelType),
			Provider:        "fake",
			ModelName:       "fake-embedding",
			APIKeyEncrypted: encryptedAPIKey,
			IsDefault:       true,
		}}},
		SecretCipher: cipher,
		LLMManager:   llmManager,
		RAGSearcher: ragsearch.NewSearcher[models.RAGChunkSource](
			esClient,
			ragsearch.WithIndex("boxify_chunks"),
			ragsearch.WithEmbeddingDim(3),
			ragsearch.WithSourceDecoder[models.RAGChunkSource](decodeKnowledgeToolSource),
		),
	}
}

func assertStringSliceMetadata(t *testing.T, metadata map[string]any, key string, want []string) {
	t.Helper()
	got, ok := metadata[key].([]string)
	if !ok {
		t.Fatalf("metadata[%s] = %#v, want []string", key, metadata[key])
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("metadata[%s] = %#v, want %#v", key, got, want)
	}
}

func knowledgeToolRows(userID uuid.UUID, kbIDs ...uuid.UUID) map[uuid.UUID]*models.KnowledgeBase {
	rows := make(map[uuid.UUID]*models.KnowledgeBase, len(kbIDs))
	for _, kbID := range kbIDs {
		rows[kbID] = &models.KnowledgeBase{ID: kbID, UserID: userID, Name: "kb"}
	}
	return rows
}

type fakeKnowledgeToolKBRepo struct {
	rows map[uuid.UUID]*models.KnowledgeBase
}

func (r *fakeKnowledgeToolKBRepo) Create(ctx context.Context, userID uuid.UUID, row *models.KnowledgeBase) (*models.KnowledgeBase, error) {
	return row, nil
}

func (r *fakeKnowledgeToolKBRepo) List(ctx context.Context, userID uuid.UUID) ([]*models.KnowledgeBase, error) {
	return nil, nil
}

func (r *fakeKnowledgeToolKBRepo) FindDefault(ctx context.Context, userID uuid.UUID) (*models.KnowledgeBase, error) {
	return nil, xerr.NotFound("默认知识库不存在")
}

func (r *fakeKnowledgeToolKBRepo) FindByID(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID) (*models.KnowledgeBase, error) {
	row, ok := r.rows[knowledgeBaseID]
	if !ok || row.UserID != userID {
		return nil, xerr.NotFound("知识库不存在")
	}
	return row, nil
}

func (r *fakeKnowledgeToolKBRepo) Update(ctx context.Context, userID uuid.UUID, row *models.KnowledgeBase) (*models.KnowledgeBase, error) {
	return row, nil
}

func (r *fakeKnowledgeToolKBRepo) UpdateFields(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID, row *models.KnowledgeBase, fields *repository.KnowledgeBaseUpdateFields) (*models.KnowledgeBase, error) {
	return row, nil
}

func (r *fakeKnowledgeToolKBRepo) Delete(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID) error {
	return nil
}

type fakeKnowledgeToolModelConfigRepo struct {
	rows []*models.ModelConfig
}

func (r *fakeKnowledgeToolModelConfigRepo) Create(ctx context.Context, modelConfig *models.ModelConfig) (*models.ModelConfig, error) {
	return modelConfig, nil
}

func (r *fakeKnowledgeToolModelConfigRepo) Update(ctx context.Context, modelConfig *models.ModelConfig) (*models.ModelConfig, error) {
	return modelConfig, nil
}

func (r *fakeKnowledgeToolModelConfigRepo) Delete(ctx context.Context, ID uuid.UUID) error {
	return nil
}

func (r *fakeKnowledgeToolModelConfigRepo) List(ctx context.Context, userID uuid.UUID, modelType *types.ModelType) ([]*models.ModelConfig, error) {
	out := make([]*models.ModelConfig, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID == userID && (modelType == nil || row.Type == string(*modelType)) {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeKnowledgeToolModelConfigRepo) FindByID(ctx context.Context, userID uuid.UUID, configID uuid.UUID) (*models.ModelConfig, error) {
	return nil, errors.New("not implemented")
}

type fakeKnowledgeToolTagRepo struct {
	rows []*models.Tag
	err  error
}

func (r *fakeKnowledgeToolTagRepo) Create(ctx context.Context, userID uuid.UUID, row *models.Tag) (*models.Tag, error) {
	return row, nil
}

func (r *fakeKnowledgeToolTagRepo) List(ctx context.Context, userID uuid.UUID) ([]*models.Tag, error) {
	return r.ListByScope(ctx, userID, string(types.TagScopeAll))
}

func (r *fakeKnowledgeToolTagRepo) ListByScope(ctx context.Context, userID uuid.UUID, scope string) ([]*models.Tag, error) {
	if r.err != nil {
		return nil, r.err
	}
	out := make([]*models.Tag, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeKnowledgeToolTagRepo) PageList(ctx context.Context, userID uuid.UUID, query repository.TagListQuery) ([]*models.Tag, int64, error) {
	return nil, 0, nil
}

func (r *fakeKnowledgeToolTagRepo) CountDocumentsByTags(ctx context.Context, userID uuid.UUID, tagIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	return nil, nil
}

func (r *fakeKnowledgeToolTagRepo) CountImagesByTags(ctx context.Context, userID uuid.UUID, tagIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	return nil, nil
}

func (r *fakeKnowledgeToolTagRepo) FindByID(ctx context.Context, userID uuid.UUID, tagID uuid.UUID) (*models.Tag, error) {
	return nil, nil
}

func (r *fakeKnowledgeToolTagRepo) Update(ctx context.Context, userID uuid.UUID, row *models.Tag) (*models.Tag, error) {
	return row, nil
}

func (r *fakeKnowledgeToolTagRepo) UpdateFields(ctx context.Context, userID uuid.UUID, tagID uuid.UUID, row *models.Tag, fields *repository.TagUpdateFields) (*models.Tag, error) {
	return row, nil
}

func (r *fakeKnowledgeToolTagRepo) SyncDocumentTags(ctx context.Context, userID uuid.UUID, documentID uuid.UUID, names []string) ([]models.Tag, error) {
	return nil, nil
}

func (r *fakeKnowledgeToolTagRepo) ListDocumentIDsByTag(ctx context.Context, userID uuid.UUID, tagID uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}

func (r *fakeKnowledgeToolTagRepo) ListDocumentTagNames(ctx context.Context, userID uuid.UUID, documentIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
	return nil, nil
}

func (r *fakeKnowledgeToolTagRepo) Merge(ctx context.Context, userID uuid.UUID, sourceID uuid.UUID, targetID uuid.UUID) (*models.Tag, error) {
	return nil, nil
}

func (r *fakeKnowledgeToolTagRepo) Delete(ctx context.Context, userID uuid.UUID, tagID uuid.UUID) error {
	return nil
}

type fakeKnowledgeToolLLMFactory struct {
	vectors map[string][]float64
}

func (f fakeKnowledgeToolLLMFactory) NewClient(cfg corellm.ModelConfig) (corellm.Client, error) {
	return fakeKnowledgeToolLLM{vectors: f.vectors}, nil
}

type fakeKnowledgeToolLLM struct {
	vectors map[string][]float64
}

func (fakeKnowledgeToolLLM) Invoke(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (string, error) {
	return "", nil
}

func (fakeKnowledgeToolLLM) Stream(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (<-chan string, error) {
	return nil, nil
}

func (c fakeKnowledgeToolLLM) Embed(ctx context.Context, texts []string, dimensions int, opts ...corellm.EmbeddingOption) ([][]float64, error) {
	out := make([][]float64, 0, len(texts))
	for _, text := range texts {
		vector, _ := c.EmbedOne(ctx, text, dimensions)
		out = append(out, vector)
	}
	return out, nil
}

func (c fakeKnowledgeToolLLM) EmbedOne(ctx context.Context, text string, dimensions int) ([]float64, error) {
	if vector, ok := c.vectors[text]; ok {
		return vector, nil
	}
	return []float64{0.1, 0.2, 0.3}, nil
}

type fakeKnowledgeToolES struct {
	queries    []any
	chunkID    uuid.UUID
	documentID uuid.UUID
	kbID       uuid.UUID
	userID     uuid.UUID
}

func (e *fakeKnowledgeToolES) Search(ctx context.Context, index string, query any) (map[string]any, error) {
	e.queries = append(e.queries, query)
	return map[string]any{
		"hits": map[string]any{
			"hits": []any{
				map[string]any{
					"_id":    e.chunkID.String(),
					"_score": float64(2),
					"_source": map[string]any{
						"chunk_id":    e.chunkID.String(),
						"document_id": e.documentID.String(),
						"user_id":     e.userID.String(),
						"kb_id":       e.kbID.String(),
						"doc_name":    "guide.md",
						"source_type": "file",
						"content":     "hello chunk",
					},
				},
			},
		},
	}, nil
}

func decodeKnowledgeToolSource(src map[string]any) (models.RAGChunkSource, error) {
	chunkID, err := uuid.Parse(fmt.Sprint(src["chunk_id"]))
	if err != nil {
		return models.RAGChunkSource{}, err
	}
	documentID, err := uuid.Parse(fmt.Sprint(src["document_id"]))
	if err != nil {
		return models.RAGChunkSource{}, err
	}
	kbID, err := uuid.Parse(fmt.Sprint(src["kb_id"]))
	if err != nil {
		return models.RAGChunkSource{}, err
	}
	return models.RAGChunkSource{
		ChunkID:    chunkID,
		DocumentID: documentID,
		KBID:       &kbID,
		DocName:    fmt.Sprint(src["doc_name"]),
		SourceType: fmt.Sprint(src["source_type"]),
	}, nil
}
