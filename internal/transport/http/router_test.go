package http_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/boxify/api-go/internal/config"
	corellm "github.com/boxify/api-go/internal/core/llm"
	"github.com/boxify/api-go/internal/core/prompt"
	ragchunker "github.com/boxify/api-go/internal/core/rag/chunker"
	ragsearch "github.com/boxify/api-go/internal/core/rag/search"
	"github.com/boxify/api-go/internal/core/rag/webcrawl"
	"github.com/boxify/api-go/internal/domain/types"
	infraes "github.com/boxify/api-go/internal/infrastructure/db/es"
	"github.com/boxify/api-go/internal/infrastructure/queue"
	"github.com/boxify/api-go/internal/infrastructure/realtime"
	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/boxify/api-go/internal/models"
	appprompts "github.com/boxify/api-go/internal/prompts"
	"github.com/boxify/api-go/internal/prompts/promptsgen"
	"github.com/boxify/api-go/internal/repository"
	repositoryes "github.com/boxify/api-go/internal/repository/es"
	"github.com/boxify/api-go/internal/svc"
	httptransport "github.com/boxify/api-go/internal/transport/http"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

func newTestRouter(t *testing.T, enableDebugPanicRoute ...bool) http.Handler {
	t.Helper()
	return newTestRouterWithConfig(t, config.Config{
		App: config.AppConfig{Env: "test"},
		Docs: config.DocsConfig{
			Enabled:  false,
			Path:     "/docs",
			SpecPath: "/docs/openapi.json",
			Title:    "Test API",
			Version:  "test",
		},
	}, enableDebugPanicRoute...)
}

func newTestRouterWithConfig(t *testing.T, cfg config.Config, enableDebugPanicRoute ...bool) http.Handler {
	return newTestRouterWithConfigAndOverrides(t, cfg, nil, enableDebugPanicRoute...)
}

func newTestRouterWithConfigAndOverrides(t *testing.T, cfg config.Config, configure func(*svc.ServiceContext), enableDebugPanicRoute ...bool) http.Handler {
	t.Helper()
	cipher, err := security.NewSecretCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	if cfg.Rag.ChunkIndex == "" {
		cfg.Rag.ChunkIndex = "boxify_chunks"
	}
	if cfg.Rag.EmbeddingDim == 0 {
		cfg.Rag.EmbeddingDim = 3
	}
	if cfg.LLM.Provider == "" {
		cfg.LLM = config.LLMConfig{Provider: "fake", Model: "fake-model", APIKey: "fake-key", EmbeddingModel: "fake-embed"}
	}
	esServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		if r.Method == http.MethodGet && r.URL.Path == "/" {
			_, _ = w.Write([]byte(`{"version":{"number":"8.0.0"}}`))
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/boxify_chunks/_search" {
			_, _ = w.Write([]byte(`{"hits":{"hits":[]}}`))
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/boxify_chunks/_delete_by_query" {
			_, _ = w.Write([]byte(`{"deleted":1}`))
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/boxify_chunks/_update_by_query" {
			_, _ = w.Write([]byte(`{"updated":1}`))
			return
		}
		t.Fatalf("unexpected ES request %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(esServer.Close)
	esClient, err := infraes.NewClient(infraes.Config{URL: esServer.URL})
	if err != nil {
		t.Fatalf("new es client: %v", err)
	}
	ragChunkRepo := repositoryes.NewRAGChunkRepository(esClient, cfg.Rag.ChunkIndex)
	llmManager := newTestLLMManager()
	modelConfigRepo := &testModelConfigRepository{}
	promptManager := prompt.NewManager()
	if err := appprompts.Register(promptManager); err != nil {
		t.Fatalf("register prompts: %v", err)
	}
	svcCtx := &svc.ServiceContext{
		Config:            cfg,
		UserRepo:          newTestUserRepository(),
		RefreshTokenRepo:  newTestRefreshTokenRepository(),
		ModelConfigRepo:   modelConfigRepo,
		ConversationRepo:  newTestConversationRepository(),
		MessageRepo:       newTestMessageRepository(),
		KnowledgeBaseRepo: newTestKnowledgeBaseRepository(),
		SkillRepo:         newTestSkillRepository(),
		DocumentRepo:      newTestDocumentRepository(),
		ImageRepo:         newTestImageRepository(),
		TagRepo:           newTestTagRepository(),
		Storage:           newTestDocumentStore(),
		Elasticsearch:     esClient,
		RAGChunkRepo:      ragChunkRepo,
		RAGSearcher:       ragsearch.NewSearcher[models.RAGChunkSource](esClient, ragsearch.WithIndex(cfg.Rag.ChunkIndex), ragsearch.WithEmbeddingDim(cfg.Rag.EmbeddingDim), ragsearch.WithSourceDecoder[models.RAGChunkSource](ragChunkRepo.DecodeSource)),
		RAGWebCrawler:     webcrawl.NewCrawler(webcrawl.WithHTTPClient(testWebCrawlerHTTPClient{}), webcrawl.WithURLGuard(testWebCrawlerGuard{})),
		Realtime:          testRealtimeBroker{},
		TaskProducer:      testTaskProducer{},
		LLMManager:        llmManager,
		PromptManager:     promptManager,
		PromptClient:      promptsgen.NewClient(promptManager),
		SecretCipher:      cipher,
		TokenIssuer:       security.NewTokenIssuer("test-secret", time.Hour),
	}
	if configure != nil {
		configure(svcCtx)
	}
	deps := httptransport.Dependencies{
		Svc: svcCtx,
	}
	if len(enableDebugPanicRoute) > 0 {
		deps.EnableDebugPanicRoute = enableDebugPanicRoute[0]
	}
	return httptransport.NewRouter(deps)
}

type testLLMFactory struct{}

func (testLLMFactory) NewClient(cfg corellm.ModelConfig) (corellm.Client, error) {
	return testLLMClient{}, nil
}

type testLLMClient struct{}

func (testLLMClient) Invoke(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (string, error) {
	return "", nil
}

func (c testLLMClient) InvokeResult(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (*corellm.LLMResult, error) {
	text, err := c.Invoke(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	return &corellm.LLMResult{Text: text}, nil
}

func (testLLMClient) Stream(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (testLLMClient) Embed(ctx context.Context, texts []string, dimensions int, opts ...corellm.EmbeddingOption) ([][]float64, error) {
	out := make([][]float64, 0, len(texts))
	for range texts {
		out = append(out, []float64{0.1, 0.2, 0.3})
	}
	return out, nil
}

func (testLLMClient) EmbedOne(ctx context.Context, text string, dimensions int) ([]float64, error) {
	return []float64{0.1, 0.2, 0.3}, nil
}

func newTestLLMManager() *corellm.Manager {
	manager := corellm.NewManager()
	for _, provider := range []string{"fake", "openai", "qwen", "doubao", "deepseek", "zhipu", "qianfan"} {
		manager.Register(provider, testLLMFactory{})
	}
	return manager
}

type testRealtimeBroker struct{}

func (testRealtimeBroker) Publish(ctx context.Context, topic string, event types.Event) error {
	return nil
}

func (testRealtimeBroker) Subscribe(ctx context.Context, topic string) (realtime.Subscription, error) {
	events := make(chan types.Event, 2)
	events <- types.NewTokenEvent("345")
	events <- types.NewDoneEvent("ok")
	close(events)
	return testRealtimeSubscription{events: events}, nil
}

type testRealtimeSubscription struct {
	events <-chan types.Event
}

func (s testRealtimeSubscription) Events() <-chan types.Event {
	return s.events
}

func (s testRealtimeSubscription) Close(ctx context.Context) error {
	return nil
}

type testTaskProducer struct{}

func (testTaskProducer) Enqueue(ctx context.Context, task *types.Task, opts ...queue.EnqueueOption) (*queue.TaskInfo, error) {
	return &queue.TaskInfo{ID: "task-id", Name: task.Name, Queue: task.Queue}, nil
}

func (testTaskProducer) Close() error {
	return nil
}

type testWebCrawlerHTTPClient struct{}

func (testWebCrawlerHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`<html><head><title>Imported Page</title></head><body><main>Imported body</main></body></html>`)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

type testWebCrawlerGuard struct{}

func (testWebCrawlerGuard) Validate(ctx context.Context, rawURL string) error {
	return nil
}

type testModelConfigRepository struct {
	rows []*models.ModelConfig
}

func (r *testModelConfigRepository) Create(ctx context.Context, row *models.ModelConfig) (*models.ModelConfig, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *testModelConfigRepository) Update(ctx context.Context, row *models.ModelConfig) (*models.ModelConfig, error) {
	for i, existing := range r.rows {
		if existing.ID == row.ID {
			r.rows[i] = row
			return row, nil
		}
	}
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *testModelConfigRepository) Delete(ctx context.Context, id uuid.UUID) error {
	for i, row := range r.rows {
		if row.ID == id {
			r.rows = append(r.rows[:i], r.rows[i+1:]...)
			return nil
		}
	}
	return nil
}

func (r *testModelConfigRepository) List(ctx context.Context, userID uuid.UUID, modelType *types.ModelType) ([]*models.ModelConfig, error) {
	out := make([]*models.ModelConfig, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID == userID && (modelType == nil || row.Type == string(*modelType)) {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *testModelConfigRepository) FindByID(ctx context.Context, userID uuid.UUID, configID uuid.UUID) (*models.ModelConfig, error) {
	for _, row := range r.rows {
		if row.ID == configID && row.UserID == userID {
			return row, nil
		}
	}
	return nil, xerr.NotFound("模型配置不存在")
}

type testConversationRepository struct {
	rows []*models.Conversation
}

func newTestConversationRepository() *testConversationRepository {
	return &testConversationRepository{}
}

func (r *testConversationRepository) Create(ctx context.Context, userID uuid.UUID, row *models.Conversation) (*models.Conversation, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	row.UserID = userID
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *testConversationRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.Conversation, error) {
	out := make([]*models.Conversation, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *testConversationRepository) FindByID(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) (*models.Conversation, error) {
	for _, row := range r.rows {
		if row.ID == conversationID && row.UserID == userID {
			return row, nil
		}
	}
	return nil, xerr.NotFound("会话不存在")
}

func (r *testConversationRepository) Update(ctx context.Context, userID uuid.UUID, row *models.Conversation) (*models.Conversation, error) {
	for i, existing := range r.rows {
		if existing.ID == row.ID && existing.UserID == userID {
			row.UserID = userID
			r.rows[i] = row
			return row, nil
		}
	}
	return nil, xerr.NotFound("会话不存在")
}

func (r *testConversationRepository) UpdateFields(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID, row *models.Conversation, fields *repository.ConversationUpdateFields) (*models.Conversation, error) {
	existing, err := r.FindByID(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}
	for _, column := range fields.Columns() {
		if column == "title" {
			existing.Title = row.Title
		}
	}
	return existing, nil
}

func (r *testConversationRepository) Delete(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) error {
	for i, row := range r.rows {
		if row.ID == conversationID && row.UserID == userID {
			r.rows = append(r.rows[:i], r.rows[i+1:]...)
			return nil
		}
	}
	return xerr.NotFound("会话不存在")
}

type testMessageRepository struct {
	rows []*models.Message
}

func newTestMessageRepository() *testMessageRepository {
	return &testMessageRepository{}
}

func (r *testMessageRepository) Create(ctx context.Context, userID uuid.UUID, row *models.Message) (*models.Message, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *testMessageRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.Message, error) {
	return append([]*models.Message(nil), r.rows...), nil
}

func (r *testMessageRepository) ListByConversationID(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) ([]*models.Message, error) {
	out := make([]*models.Message, 0, len(r.rows))
	for _, row := range r.rows {
		if row.ConversationID == conversationID {
			out = append(out, row)
		}
	}
	slices.SortFunc(out, func(a, b *models.Message) int {
		return a.CreatedAt.Compare(b.CreatedAt)
	})
	return out, nil
}

func (r *testMessageRepository) FindByID(ctx context.Context, userID uuid.UUID, messageID uuid.UUID) (*models.Message, error) {
	for _, row := range r.rows {
		if row.ID == messageID {
			return row, nil
		}
	}
	return nil, xerr.NotFound("消息不存在")
}

func (r *testMessageRepository) Update(ctx context.Context, userID uuid.UUID, row *models.Message) (*models.Message, error) {
	for i, existing := range r.rows {
		if existing.ID == row.ID {
			r.rows[i] = row
			return row, nil
		}
	}
	return nil, xerr.NotFound("消息不存在")
}

func (r *testMessageRepository) UpdateFields(ctx context.Context, userID uuid.UUID, messageID uuid.UUID, row *models.Message, fields *repository.MessageUpdateFields) (*models.Message, error) {
	existing, err := r.FindByID(ctx, userID, messageID)
	if err != nil {
		return nil, err
	}
	for _, column := range fields.Columns() {
		switch column {
		case "conversation_id":
			existing.ConversationID = row.ConversationID
		case "role":
			existing.Role = row.Role
		case "sender_persona_id":
			existing.SenderPersonaID = row.SenderPersonaID
		case "sender_user_id":
			existing.SenderUserID = row.SenderUserID
		case "meta_data":
			existing.MetaData = row.MetaData
		}
	}
	return existing, nil
}

func (r *testMessageRepository) Delete(ctx context.Context, userID uuid.UUID, messageID uuid.UUID) error {
	for i, row := range r.rows {
		if row.ID == messageID {
			r.rows = append(r.rows[:i], r.rows[i+1:]...)
			return nil
		}
	}
	return xerr.NotFound("消息不存在")
}

func (r *testMessageRepository) Count(ctx context.Context, conversationID uuid.UUID) (int64, error) {
	var count int64
	for _, row := range r.rows {
		if row.ConversationID == conversationID {
			count++
		}
	}
	return count, nil
}

type testKnowledgeBaseRepository struct {
	rows []*models.KnowledgeBase
}

func newTestKnowledgeBaseRepository() *testKnowledgeBaseRepository {
	return &testKnowledgeBaseRepository{}
}

func (r *testKnowledgeBaseRepository) Create(ctx context.Context, userID uuid.UUID, row *models.KnowledgeBase) (*models.KnowledgeBase, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	row.UserID = userID
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *testKnowledgeBaseRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.KnowledgeBase, error) {
	out := make([]*models.KnowledgeBase, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *testKnowledgeBaseRepository) FindDefault(ctx context.Context, userID uuid.UUID) (*models.KnowledgeBase, error) {
	for _, row := range r.rows {
		if row.UserID == userID && row.IsDefault {
			return row, nil
		}
	}
	return nil, xerr.NotFound("默认知识库不存在")
}

func (r *testKnowledgeBaseRepository) FindByID(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID) (*models.KnowledgeBase, error) {
	for _, row := range r.rows {
		if row.ID == knowledgeBaseID && row.UserID == userID {
			return row, nil
		}
	}
	return nil, xerr.NotFound("知识库不存在")
}

func (r *testKnowledgeBaseRepository) Update(ctx context.Context, userID uuid.UUID, row *models.KnowledgeBase) (*models.KnowledgeBase, error) {
	for i, existing := range r.rows {
		if existing.ID == row.ID && existing.UserID == userID {
			row.UserID = userID
			r.rows[i] = row
			return row, nil
		}
	}
	return nil, xerr.NotFound("知识库不存在")
}

func (r *testKnowledgeBaseRepository) UpdateFields(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID, row *models.KnowledgeBase, fields *repository.KnowledgeBaseUpdateFields) (*models.KnowledgeBase, error) {
	existing, err := r.FindByID(ctx, userID, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	for _, column := range fields.Columns() {
		switch column {
		case "name":
			existing.Name = row.Name
		case "description":
			existing.Description = row.Description
		case "icon":
			existing.Icon = row.Icon
		case "color":
			existing.Color = row.Color
		case "chat_enabled":
			existing.ChatEnabled = row.ChatEnabled
		}
	}
	return existing, nil
}

func (r *testKnowledgeBaseRepository) Delete(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID) error {
	for i, row := range r.rows {
		if row.ID == knowledgeBaseID && row.UserID == userID {
			r.rows = append(r.rows[:i], r.rows[i+1:]...)
			return nil
		}
	}
	return xerr.NotFound("知识库不存在")
}

type testSkillRepository struct {
	rows []*models.Skill
}

func newTestSkillRepository() *testSkillRepository {
	return &testSkillRepository{}
}

func (r *testSkillRepository) Create(ctx context.Context, userID uuid.UUID, row *models.Skill) (*models.Skill, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	row.UserID = userID
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *testSkillRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.Skill, error) {
	out := make([]*models.Skill, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *testSkillRepository) FindByID(ctx context.Context, userID uuid.UUID, skillID uuid.UUID) (*models.Skill, error) {
	for _, row := range r.rows {
		if row.ID == skillID && row.UserID == userID {
			return row, nil
		}
	}
	return nil, xerr.NotFound("技能不存在")
}

func (r *testSkillRepository) Update(ctx context.Context, userID uuid.UUID, row *models.Skill) (*models.Skill, error) {
	for i, existing := range r.rows {
		if existing.ID == row.ID && existing.UserID == userID {
			row.UserID = userID
			r.rows[i] = row
			return row, nil
		}
	}
	return nil, xerr.NotFound("技能不存在")
}

func (r *testSkillRepository) UpdateFields(ctx context.Context, userID uuid.UUID, skillID uuid.UUID, row *models.Skill, fields *repository.SkillUpdateFields) (*models.Skill, error) {
	existing, err := r.FindByID(ctx, userID, skillID)
	if err != nil {
		return nil, err
	}
	for _, column := range fields.Columns() {
		switch column {
		case "name":
			existing.Name = row.Name
		case "description":
			existing.Description = row.Description
		case "icon":
			existing.Icon = row.Icon
		case "prompt":
			existing.Prompt = row.Prompt
		case "tool_keys":
			existing.ToolKeys = row.ToolKeys
		case "kb_id":
			existing.KBID = row.KBID
		case "config":
			existing.Config = row.Config
		case "enabled":
			existing.Enabled = row.Enabled
		}
	}
	return existing, nil
}

func (r *testSkillRepository) Delete(ctx context.Context, userID uuid.UUID, skillID uuid.UUID) error {
	for i, row := range r.rows {
		if row.ID == skillID && row.UserID == userID {
			r.rows = append(r.rows[:i], r.rows[i+1:]...)
			return nil
		}
	}
	return xerr.NotFound("技能不存在")
}

type testDocumentRepository struct {
	rows []*models.Document
}

func newTestDocumentRepository() *testDocumentRepository {
	return &testDocumentRepository{}
}

func (r *testDocumentRepository) Create(ctx context.Context, userID uuid.UUID, row *models.Document) (*models.Document, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	row.UserID = userID
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *testDocumentRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.Document, error) {
	out := make([]*models.Document, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *testDocumentRepository) PageList(ctx context.Context, userID uuid.UUID, query repository.DocumentListQuery) ([]*models.Document, int64, error) {
	out := make([]*models.Document, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID == userID && (query.KBID == nil || (row.KBID != nil && *row.KBID == *query.KBID)) {
			out = append(out, row)
		}
	}
	total := int64(len(out))
	limit, offset := query.LimitOffset(20)
	start := offset
	if start >= len(out) {
		return []*models.Document{}, total, nil
	}
	end := start + limit
	if end > len(out) {
		end = len(out)
	}
	return out[start:end], total, nil
}

func (r *testDocumentRepository) CountByKnowledgeBase(ctx context.Context, userID uuid.UUID, kbIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	allowed := map[uuid.UUID]struct{}{}
	for _, id := range kbIDs {
		allowed[id] = struct{}{}
	}
	out := map[uuid.UUID]int64{}
	for _, row := range r.rows {
		if row.UserID != userID || row.KBID == nil {
			continue
		}
		if _, ok := allowed[*row.KBID]; ok {
			out[*row.KBID]++
		}
	}
	return out, nil
}

func (r *testDocumentRepository) FindByID(ctx context.Context, userID uuid.UUID, documentID uuid.UUID) (*models.Document, error) {
	for _, row := range r.rows {
		if row.ID == documentID && row.UserID == userID {
			return row, nil
		}
	}
	return nil, xerr.NotFound("文档不存在")
}

func (r *testDocumentRepository) Update(ctx context.Context, userID uuid.UUID, row *models.Document) (*models.Document, error) {
	for i, existing := range r.rows {
		if existing.ID == row.ID && existing.UserID == userID {
			row.UserID = userID
			r.rows[i] = row
			return row, nil
		}
	}
	return nil, xerr.NotFound("文档不存在")
}

func (r *testDocumentRepository) UpdateFields(ctx context.Context, userID uuid.UUID, documentID uuid.UUID, row *models.Document, fields *repository.DocumentUpdateFields) (*models.Document, error) {
	existing, err := r.FindByID(ctx, userID, documentID)
	if err != nil {
		return nil, err
	}
	for _, column := range fields.Columns() {
		switch column {
		case "kb_id":
			existing.KBID = row.KBID
		case "status":
			existing.Status = row.Status
		case "progress":
			existing.Progress = row.Progress
		case "error_msg":
			existing.ErrorMsg = row.ErrorMsg
		}
	}
	return existing, nil
}

func (r *testDocumentRepository) Delete(ctx context.Context, userID uuid.UUID, documentID uuid.UUID) error {
	for i, row := range r.rows {
		if row.ID == documentID && row.UserID == userID {
			r.rows = append(r.rows[:i], r.rows[i+1:]...)
			return nil
		}
	}
	return xerr.NotFound("文档不存在")
}

type testImageRepository struct {
	rows []*models.Image
}

func newTestImageRepository() *testImageRepository {
	return &testImageRepository{}
}

func (r *testImageRepository) Create(ctx context.Context, userID uuid.UUID, row *models.Image) (*models.Image, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	row.UserID = userID
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *testImageRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.Image, error) {
	out := make([]*models.Image, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *testImageRepository) CountByKnowledgeBase(ctx context.Context, userID uuid.UUID, kbIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	allowed := map[uuid.UUID]struct{}{}
	for _, id := range kbIDs {
		allowed[id] = struct{}{}
	}
	out := map[uuid.UUID]int64{}
	for _, row := range r.rows {
		if row.UserID != userID || row.KBID == nil {
			continue
		}
		if _, ok := allowed[*row.KBID]; ok {
			out[*row.KBID]++
		}
	}
	return out, nil
}

func (r *testImageRepository) FindByID(ctx context.Context, userID uuid.UUID, imageID uuid.UUID) (*models.Image, error) {
	for _, row := range r.rows {
		if row.ID == imageID && row.UserID == userID {
			return row, nil
		}
	}
	return nil, xerr.NotFound("图片不存在")
}

func (r *testImageRepository) Update(ctx context.Context, userID uuid.UUID, row *models.Image) (*models.Image, error) {
	for i, existing := range r.rows {
		if existing.ID == row.ID && existing.UserID == userID {
			row.UserID = userID
			r.rows[i] = row
			return row, nil
		}
	}
	return nil, xerr.NotFound("图片不存在")
}

func (r *testImageRepository) UpdateFields(ctx context.Context, userID uuid.UUID, imageID uuid.UUID, row *models.Image, fields *repository.ImageUpdateFields) (*models.Image, error) {
	existing, err := r.FindByID(ctx, userID, imageID)
	if err != nil {
		return nil, err
	}
	for _, column := range fields.Columns() {
		switch column {
		case "kb_id":
			existing.KBID = row.KBID
		case "status":
			existing.Status = row.Status
		case "error_msg":
			existing.ErrorMsg = row.ErrorMsg
		}
	}
	return existing, nil
}

func (r *testImageRepository) Delete(ctx context.Context, userID uuid.UUID, imageID uuid.UUID) error {
	for i, row := range r.rows {
		if row.ID == imageID && row.UserID == userID {
			r.rows = append(r.rows[:i], r.rows[i+1:]...)
			return nil
		}
	}
	return xerr.NotFound("图片不存在")
}

type testTagRepository struct {
	rows        []*models.Tag
	docCounts   map[uuid.UUID]int64
	imageCounts map[uuid.UUID]int64

	documentIDsByTag map[uuid.UUID][]uuid.UUID
	documentTagNames map[uuid.UUID][]string
}

func newTestTagRepository() *testTagRepository {
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	sourceID := uuid.MustParse("10000000-0000-0000-0000-000000000001")
	targetID := uuid.MustParse("10000000-0000-0000-0000-000000000002")
	imageID := uuid.MustParse("10000000-0000-0000-0000-000000000003")
	return &testTagRepository{
		rows: []*models.Tag{
			{ID: sourceID, UserID: userID, Name: "源标签", Color: "#111111"},
			{ID: targetID, UserID: userID, Name: "目标标签", Color: "#222222"},
			{ID: imageID, UserID: userID, Name: "图片标签", Color: "#333333"},
		},
		docCounts:   map[uuid.UUID]int64{sourceID: 2, targetID: 1, imageID: 0},
		imageCounts: map[uuid.UUID]int64{sourceID: 1, targetID: 1, imageID: 3},
	}
}

func (r *testTagRepository) Create(ctx context.Context, userID uuid.UUID, row *models.Tag) (*models.Tag, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	row.UserID = userID
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *testTagRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.Tag, error) {
	return r.ListByScope(ctx, userID, "all")
}

func (r *testTagRepository) ListByScope(ctx context.Context, userID uuid.UUID, scope string) ([]*models.Tag, error) {
	out := make([]*models.Tag, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID != userID {
			continue
		}
		switch scope {
		case "document":
			if r.docCounts[row.ID] == 0 {
				continue
			}
		case "image":
			if r.imageCounts[row.ID] == 0 {
				continue
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func (r *testTagRepository) PageList(ctx context.Context, userID uuid.UUID, query repository.TagListQuery) ([]*models.Tag, int64, error) {
	rows, err := r.ListByScope(ctx, userID, query.Scope)
	if err != nil {
		return nil, 0, err
	}
	total := int64(len(rows))
	limit, offset := query.LimitOffset(20)
	if offset >= len(rows) {
		return []*models.Tag{}, total, nil
	}
	end := offset + limit
	if end > len(rows) {
		end = len(rows)
	}
	return rows[offset:end], total, nil
}

func (r *testTagRepository) CountDocumentsByTags(ctx context.Context, userID uuid.UUID, tagIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	out := map[uuid.UUID]int64{}
	for _, id := range tagIDs {
		out[id] = r.docCounts[id]
	}
	return out, nil
}

func (r *testTagRepository) CountImagesByTags(ctx context.Context, userID uuid.UUID, tagIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	out := map[uuid.UUID]int64{}
	for _, id := range tagIDs {
		out[id] = r.imageCounts[id]
	}
	return out, nil
}

func (r *testTagRepository) FindByID(ctx context.Context, userID uuid.UUID, tagID uuid.UUID) (*models.Tag, error) {
	for _, row := range r.rows {
		if row.ID == tagID && row.UserID == userID {
			return row, nil
		}
	}
	return nil, xerr.NotFound("标签不存在")
}

func (r *testTagRepository) Update(ctx context.Context, userID uuid.UUID, row *models.Tag) (*models.Tag, error) {
	return r.UpdateFields(ctx, userID, row.ID, row, repository.NewTagUpdateFields().Name().Color())
}

func (r *testTagRepository) UpdateFields(ctx context.Context, userID uuid.UUID, tagID uuid.UUID, row *models.Tag, fields *repository.TagUpdateFields) (*models.Tag, error) {
	existing, err := r.FindByID(ctx, userID, tagID)
	if err != nil {
		return nil, err
	}
	for _, column := range fields.Columns() {
		switch column {
		case "name":
			existing.Name = row.Name
		case "color":
			existing.Color = row.Color
		}
	}
	return existing, nil
}

func (r *testTagRepository) SyncDocumentTags(ctx context.Context, userID uuid.UUID, documentID uuid.UUID, names []string) ([]models.Tag, error) {
	return nil, nil
}

func (r *testTagRepository) ListDocumentIDsByTag(ctx context.Context, userID uuid.UUID, tagID uuid.UUID) ([]uuid.UUID, error) {
	return append([]uuid.UUID(nil), r.documentIDsByTag[tagID]...), nil
}

func (r *testTagRepository) ListDocumentTagNames(ctx context.Context, userID uuid.UUID, documentIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
	out := make(map[uuid.UUID][]string, len(documentIDs))
	for _, id := range documentIDs {
		out[id] = append([]string(nil), r.documentTagNames[id]...)
	}
	return out, nil
}

func (r *testTagRepository) Merge(ctx context.Context, userID uuid.UUID, sourceID uuid.UUID, targetID uuid.UUID) (*models.Tag, error) {
	target, err := r.FindByID(ctx, userID, targetID)
	if err != nil {
		return nil, err
	}
	if _, err := r.FindByID(ctx, userID, sourceID); err != nil {
		return nil, err
	}
	r.docCounts[targetID] += r.docCounts[sourceID]
	r.imageCounts[targetID] += r.imageCounts[sourceID]
	delete(r.docCounts, sourceID)
	delete(r.imageCounts, sourceID)
	_ = r.Delete(ctx, userID, sourceID)
	return target, nil
}

func (r *testTagRepository) Delete(ctx context.Context, userID uuid.UUID, tagID uuid.UUID) error {
	for i, row := range r.rows {
		if row.ID == tagID && row.UserID == userID {
			r.rows = append(r.rows[:i], r.rows[i+1:]...)
			return nil
		}
	}
	return xerr.NotFound("标签不存在")
}

type testRAGChunkRepository struct {
	updateTagsErr error
	tagUpdates    []testRAGTagUpdate
}

type testRAGTagUpdate struct {
	userID     uuid.UUID
	documentID uuid.UUID
	tags       []string
}

func (r *testRAGChunkRepository) EnsureIndex(ctx context.Context, embeddingDim int) error {
	return nil
}

func (r *testRAGChunkRepository) IndexDocumentChunks(ctx context.Context, document *models.Document, chunks []*ragchunker.Chunk, vectors [][]float64) error {
	return nil
}

func (r *testRAGChunkRepository) DeleteByDocument(ctx context.Context, userID uuid.UUID, documentID uuid.UUID) error {
	return nil
}

func (r *testRAGChunkRepository) UpdateKnowledgeBase(ctx context.Context, userID uuid.UUID, documentID uuid.UUID, kbID uuid.UUID) error {
	return nil
}

func (r *testRAGChunkRepository) UpdateTags(ctx context.Context, userID uuid.UUID, documentID uuid.UUID, tags []string) error {
	r.tagUpdates = append(r.tagUpdates, testRAGTagUpdate{
		userID:     userID,
		documentID: documentID,
		tags:       append([]string(nil), tags...),
	})
	return r.updateTagsErr
}

func (r *testRAGChunkRepository) DecodeSource(src map[string]any) (models.RAGChunkSource, error) {
	return models.RAGChunkSource{}, nil
}

type testDocumentStore struct {
	data map[string][]byte
}

func newTestDocumentStore() *testDocumentStore {
	return &testDocumentStore{data: map[string][]byte{}}
}

func (s *testDocumentStore) Ping(ctx context.Context) error {
	return nil
}

func (s *testDocumentStore) Put(ctx context.Context, key string, data []byte) error {
	s.data[key] = append([]byte(nil), data...)
	return nil
}

func (s *testDocumentStore) Get(ctx context.Context, key string) ([]byte, error) {
	data, ok := s.data[key]
	if !ok {
		return nil, xerr.NotFound("文件不存在")
	}
	return append([]byte(nil), data...), nil
}

func (s *testDocumentStore) Delete(ctx context.Context, key string) error {
	delete(s.data, key)
	return nil
}

type testUserRepository struct {
	byID    map[uuid.UUID]*models.User
	byLogin map[string]*models.User
}

func newTestUserRepository() *testUserRepository {
	return &testUserRepository{
		byID:    map[uuid.UUID]*models.User{},
		byLogin: map[string]*models.User{},
	}
}

func (r *testUserRepository) Create(ctx context.Context, user *models.User) (*models.User, error) {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	if _, ok := r.byLogin[user.Username]; ok {
		return nil, xerr.UserExists()
	}
	if user.Email != nil {
		if _, ok := r.byLogin[*user.Email]; ok {
			return nil, xerr.UserExists()
		}
	}
	r.byID[user.ID] = user
	r.byLogin[user.Username] = user
	if user.Email != nil {
		r.byLogin[*user.Email] = user
	}
	return user, nil
}

func (r *testUserRepository) Update(ctx context.Context, user *models.User) (*models.User, error) {
	if _, ok := r.byID[user.ID]; !ok {
		return nil, xerr.NotFound("用户不存在")
	}
	r.byID[user.ID] = user
	r.byLogin[user.Username] = user
	if user.Email != nil {
		r.byLogin[*user.Email] = user
	}
	return user, nil
}

func (r *testUserRepository) FindByLogin(ctx context.Context, login string) (*models.User, error) {
	user, ok := r.byLogin[login]
	if !ok {
		return nil, xerr.NotFound("用户不存在")
	}
	return user, nil
}

func (r *testUserRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	user, ok := r.byID[id]
	if !ok {
		return nil, xerr.NotFound("用户不存在")
	}
	return user, nil
}

type testRefreshTokenRepository struct {
	byHash map[string]*models.RefreshToken
}

func newTestRefreshTokenRepository() *testRefreshTokenRepository {
	return &testRefreshTokenRepository{byHash: map[string]*models.RefreshToken{}}
}

func (r *testRefreshTokenRepository) Create(ctx context.Context, token *models.RefreshToken) (*models.RefreshToken, error) {
	if token.ID == uuid.Nil {
		token.ID = uuid.New()
	}
	r.byHash[token.TokenHash] = token
	return token, nil
}

func (r *testRefreshTokenRepository) FindByHash(ctx context.Context, hash string) (*models.RefreshToken, error) {
	token, ok := r.byHash[hash]
	if !ok {
		return nil, xerr.InvalidToken()
	}
	return token, nil
}

func (r *testRefreshTokenRepository) Revoke(ctx context.Context, id uuid.UUID, revokedAt time.Time) error {
	for hash, token := range r.byHash {
		if token.ID == id {
			token.RevokedAt = &revokedAt
			r.byHash[hash] = token
			return nil
		}
	}
	return xerr.InvalidToken()
}

func TestRouterHealthUsesUnifiedResponse(t *testing.T) {
	router := newTestRouter(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var got struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			List []struct {
				ServerName string `json:"server_name"`
				IsHealthy  bool   `json:"is_healthy"`
				Error      string `json:"error"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if got.Code != 0 || got.Message != "ok" {
		t.Fatalf("body = %+v, want success envelope", got)
	}
	if len(got.Data.List) != 5 {
		t.Fatalf("health list len = %d, want 5", len(got.Data.List))
	}
}

func TestRouterRequiresExplicitDependencies(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewRouter did not panic for missing dependencies")
		}
	}()
	_ = httptransport.NewRouter(httptransport.Dependencies{})
}

func TestRouterMountsDocsWhenEnabled(t *testing.T) {
	// 验证配置开启时，router 会同时暴露 OpenAPI JSON 和 Swagger UI 页面。
	router := newTestRouterWithConfig(t, config.Config{
		App: config.AppConfig{Env: "development"},
		Docs: config.DocsConfig{
			Enabled:  true,
			Path:     "/docs",
			SpecPath: "/docs/openapi.json",
			Title:    "Test API",
			Version:  "test",
		},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("spec status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("spec content-type = %q, want application/json", got)
	}
	if !strings.Contains(w.Body.String(), `"openapi"`) {
		t.Fatalf("spec body = %s, want openapi json", w.Body.String())
	}

	ui := httptest.NewRecorder()
	uiReq := httptest.NewRequest(http.MethodGet, "/docs", nil)
	router.ServeHTTP(ui, uiReq)
	if ui.Code != http.StatusFound || ui.Header().Get("Location") != "/docs/index.html" {
		t.Fatalf("ui redirect status/location = %d/%q, want 302 /docs/index.html", ui.Code, ui.Header().Get("Location"))
	}

	index := httptest.NewRecorder()
	indexReq := httptest.NewRequest(http.MethodGet, "/docs/index.html", nil)
	router.ServeHTTP(index, indexReq)
	if index.Code != http.StatusOK {
		t.Fatalf("ui status = %d, want %d; body=%s", index.Code, http.StatusOK, index.Body.String())
	}
}

func TestRouterDoesNotMountDocsWhenDisabled(t *testing.T) {
	// 验证生产或显式关闭场景不会暴露接口文档路由。
	router := newTestRouterWithConfig(t, config.Config{
		App: config.AppConfig{Env: "production"},
		Docs: config.DocsConfig{
			Enabled:  false,
			Path:     "/docs",
			SpecPath: "/docs/openapi.json",
		},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestProtectedRouteRequiresBearerToken(t *testing.T) {
	router := newTestRouter(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(w.Body.String(), `"code":40100`) {
		t.Fatalf("body = %s, want auth error code", w.Body.String())
	}
}

func TestChatStreamSetsSSEHeadersAndEvents(t *testing.T) {
	router := newTestRouter(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat/stream", strings.NewReader(`{"message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", got)
	}

	scanner := bufio.NewScanner(strings.NewReader(w.Body.String()))
	events := map[string]bool{}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			events[strings.TrimPrefix(line, "event: ")] = true
		}
	}
	for _, name := range []string{"meta", "token", "done"} {
		if !events[name] {
			t.Fatalf("missing SSE event %q in body:\n%s", name, w.Body.String())
		}
	}
}

func TestKnowledgeBaseRoutesBindColorAndFalseChatEnabled(t *testing.T) {
	// 验证知识库 HTTP 路由会绑定 color 字段，允许 chat_enabled=false，并返回更新后的响应。
	router := newTestRouter(t)

	create := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/knowledge-base/create", strings.NewReader(`{"name":"资料库","description":"说明","icon":"book","color":"#22c55e"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(create, createReq)
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d body=%s", create.Code, create.Body.String())
	}
	var createBody struct {
		Data struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Color       string `json:"color"`
			ChatEnabled bool   `json:"chat_enabled"`
		} `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &createBody); err != nil {
		t.Fatalf("unmarshal create body: %v", err)
	}
	if createBody.Data.ID == "" || createBody.Data.Name != "资料库" || createBody.Data.Color != "#22c55e" || createBody.Data.ChatEnabled {
		t.Fatalf("create body = %+v, want saved color and disabled chat", createBody.Data)
	}

	toggle := httptest.NewRecorder()
	toggleReq := httptest.NewRequest(http.MethodPost, "/api/knowledge-base/"+createBody.Data.ID+"/chat-enabled", strings.NewReader(`{"chat_enabled":false}`))
	toggleReq.Header.Set("Content-Type", "application/json")
	toggleReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(toggle, toggleReq)
	if toggle.Code != http.StatusOK {
		t.Fatalf("toggle status = %d body=%s", toggle.Code, toggle.Body.String())
	}
	var toggleBody struct {
		Data struct {
			ID          string `json:"id"`
			ChatEnabled bool   `json:"chat_enabled"`
		} `json:"data"`
	}
	if err := json.Unmarshal(toggle.Body.Bytes(), &toggleBody); err != nil {
		t.Fatalf("unmarshal toggle body: %v", err)
	}
	if toggleBody.Data.ID != createBody.Data.ID || toggleBody.Data.ChatEnabled {
		t.Fatalf("toggle body = %+v, want updated knowledge base with chat disabled", toggleBody.Data)
	}
}

func TestKnowledgeBaseListCreatesDefaultForFreshUser(t *testing.T) {
	// 验证知识库列表接口会为没有默认知识库的新用户返回自动创建的默认库。
	router := newTestRouter(t)

	list := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/knowledge-base/", nil)
	listReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(list, listReq)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", list.Code, list.Body.String())
	}
	var listBody struct {
		Data struct {
			List []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Icon        string `json:"icon"`
				Color       string `json:"color"`
				IsDefault   bool   `json:"is_default"`
				ChatEnabled bool   `json:"chat_enabled"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("unmarshal list body: %v", err)
	}
	if len(listBody.Data.List) != 1 {
		t.Fatalf("list len = %d, want 1", len(listBody.Data.List))
	}
	got := listBody.Data.List[0]
	if got.Name != "默认知识库" || got.Description != "未分类资料默认归入此库" || got.Icon != "📚" ||
		got.Color != "#155EEF" || !got.IsDefault || !got.ChatEnabled {
		t.Fatalf("default knowledge base = %+v, want configured default", got)
	}
}

// TestSkillRoutesSupportCRUDAndOptimizePrompt 验证技能 HTTP 路由支持创建、列表、标准及兼容更新、删除和提示词优化响应。
func TestSkillRoutesSupportCRUDAndOptimizePrompt(t *testing.T) {
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	router := newTestRouterWithConfigAndOverrides(t, config.Config{}, func(svcCtx *svc.ServiceContext) {
		encrypted, err := svcCtx.SecretCipher.Encrypt("sk-secret")
		if err != nil {
			t.Fatalf("encrypt api key: %v", err)
		}
		svcCtx.ModelConfigRepo.(*testModelConfigRepository).rows = []*models.ModelConfig{
			{ID: uuid.New(), UserID: userID, Type: string(types.ChatModelType), Provider: "fake", ModelName: "fake-model", APIKeyEncrypted: encrypted, IsDefault: true},
		}
	})

	create := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/skill/create", strings.NewReader(`{"name":"  写作技能  ","description":"说明","prompt":"帮我写","tool_keys":[" time ",""],"config":{"quick_prompt":[" 快问 "],"few_shots":[{"input":"问","output":"答"}]}}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(create, createReq)
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d body=%s", create.Code, create.Body.String())
	}
	var createBody struct {
		Data struct {
			ID       string   `json:"id"`
			Name     string   `json:"name"`
			Icon     string   `json:"icon"`
			KBID     *string  `json:"kb_id"`
			ToolKeys []string `json:"tool_keys"`
			Config   struct {
				QuickPrompt []string `json:"quick_prompt"`
			} `json:"config"`
		} `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &createBody); err != nil {
		t.Fatalf("unmarshal create body: %v", err)
	}
	if createBody.Data.ID == "" || createBody.Data.Name != "写作技能" || createBody.Data.Icon != "🧩" || createBody.Data.KBID != nil {
		t.Fatalf("create skill body = %+v, want normalized skill", createBody.Data)
	}
	if !slices.Equal(createBody.Data.ToolKeys, []string{"time"}) || !slices.Equal(createBody.Data.Config.QuickPrompt, []string{"快问"}) {
		t.Fatalf("create normalized arrays = tool_keys:%v quick_prompt:%v", createBody.Data.ToolKeys, createBody.Data.Config.QuickPrompt)
	}

	list := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/skill/list", nil)
	listReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(list, listReq)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", list.Code, list.Body.String())
	}
	if !strings.Contains(list.Body.String(), createBody.Data.ID) {
		t.Fatalf("list body = %s, want created skill id", list.Body.String())
	}

	updateByPatch := httptest.NewRecorder()
	updateByPatchReq := httptest.NewRequest(http.MethodPatch, "/api/skill/"+createBody.Data.ID, strings.NewReader(`{"name":"PATCH 更新","enabled":false}`))
	updateByPatchReq.Header.Set("Content-Type", "application/json")
	updateByPatchReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(updateByPatch, updateByPatchReq)
	if updateByPatch.Code != http.StatusOK {
		t.Fatalf("patch update status = %d body=%s", updateByPatch.Code, updateByPatch.Body.String())
	}
	if !strings.Contains(updateByPatch.Body.String(), `"name":"PATCH 更新"`) || !strings.Contains(updateByPatch.Body.String(), `"enabled":false`) {
		t.Fatalf("patch update body = %s, want updated name and enabled=false", updateByPatch.Body.String())
	}

	updateByCompat := httptest.NewRecorder()
	updateByCompatReq := httptest.NewRequest(http.MethodPost, "/api/skill/"+createBody.Data.ID+"/update", strings.NewReader(`{"description":"兼容更新"}`))
	updateByCompatReq.Header.Set("Content-Type", "application/json")
	updateByCompatReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(updateByCompat, updateByCompatReq)
	if updateByCompat.Code != http.StatusOK {
		t.Fatalf("compat update status = %d body=%s", updateByCompat.Code, updateByCompat.Body.String())
	}
	if !strings.Contains(updateByCompat.Body.String(), `"description":"兼容更新"`) {
		t.Fatalf("compat update body = %s, want updated description", updateByCompat.Body.String())
	}

	optimize := httptest.NewRecorder()
	optimizeReq := httptest.NewRequest(http.MethodPost, "/api/skill/optimize-prompt", strings.NewReader(`{"prompt":"原始提示词"}`))
	optimizeReq.Header.Set("Content-Type", "application/json")
	optimizeReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(optimize, optimizeReq)
	if optimize.Code != http.StatusOK {
		t.Fatalf("optimize status = %d body=%s", optimize.Code, optimize.Body.String())
	}
	if !strings.Contains(optimize.Body.String(), `"optimized"`) {
		t.Fatalf("optimize body = %s, want optimized field", optimize.Body.String())
	}

	del := httptest.NewRecorder()
	delReq := httptest.NewRequest(http.MethodDelete, "/api/skill/"+createBody.Data.ID, nil)
	delReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(del, delReq)
	if del.Code != http.StatusOK {
		t.Fatalf("delete status = %d body=%s", del.Code, del.Body.String())
	}
}

// TestSkillRoutesRejectLegacyUpdateRouteAndOversizedFields 验证旧更新路由已移除且字段长度由 HTTP binding 拦截。
func TestSkillRoutesRejectLegacyUpdateRouteAndOversizedFields(t *testing.T) {
	router := newTestRouter(t)

	legacy := httptest.NewRecorder()
	legacyReq := httptest.NewRequest(http.MethodPatch, "/api/skill/", strings.NewReader(`{"id":"`+uuid.NewString()+`","name":"旧路由"}`))
	legacyReq.Header.Set("Content-Type", "application/json")
	legacyReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(legacy, legacyReq)
	if legacy.Code != http.StatusNotFound {
		t.Fatalf("legacy update status = %d body=%s, want 404", legacy.Code, legacy.Body.String())
	}

	tooLongName := httptest.NewRecorder()
	tooLongNameReq := httptest.NewRequest(http.MethodPost, "/api/skill/create", strings.NewReader(`{"name":"`+strings.Repeat("a", 65)+`"}`))
	tooLongNameReq.Header.Set("Content-Type", "application/json")
	tooLongNameReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(tooLongName, tooLongNameReq)
	if tooLongName.Code != http.StatusBadRequest {
		t.Fatalf("oversized name status = %d body=%s, want 400", tooLongName.Code, tooLongName.Body.String())
	}

	tooLongFewShot := httptest.NewRecorder()
	tooLongFewShotReq := httptest.NewRequest(http.MethodPost, "/api/skill/create", strings.NewReader(`{"name":"技能","config":{"few_shots":[{"input":"`+strings.Repeat("a", 2001)+`","output":"ok"}]}}`))
	tooLongFewShotReq.Header.Set("Content-Type", "application/json")
	tooLongFewShotReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(tooLongFewShot, tooLongFewShotReq)
	if tooLongFewShot.Code != http.StatusBadRequest {
		t.Fatalf("oversized few-shot status = %d body=%s, want 400", tooLongFewShot.Code, tooLongFewShot.Body.String())
	}
}

func TestTagRoutesSupportListUpdateMergeAndDelete(t *testing.T) {
	// 验证标签 HTTP 路由支持 scope 查询、image_count 响应、更新、合并与删除绑定。
	router := newTestRouter(t)
	sourceID := "10000000-0000-0000-0000-000000000001"
	targetID := "10000000-0000-0000-0000-000000000002"
	imageID := "10000000-0000-0000-0000-000000000003"

	list := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/tag?scope=image&page=1&page_size=2", nil)
	listReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(list, listReq)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", list.Code, list.Body.String())
	}
	var listBody struct {
		Data struct {
			Total    int64 `json:"total"`
			Page     int64 `json:"page"`
			PageSize int64 `json:"page_size"`
			List     []struct {
				ID         string `json:"id"`
				DocCount   int64  `json:"doc_count"`
				ImageCount int64  `json:"image_count"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("unmarshal list body: %v", err)
	}
	if listBody.Data.Total != 3 || listBody.Data.Page != 1 || listBody.Data.PageSize != 2 {
		t.Fatalf("list page meta = total:%d page:%d page_size:%d, want 3/1/2", listBody.Data.Total, listBody.Data.Page, listBody.Data.PageSize)
	}
	gotIDs := make([]string, 0, len(listBody.Data.List))
	for _, row := range listBody.Data.List {
		gotIDs = append(gotIDs, row.ID)
		if row.ID == imageID && (row.DocCount != 0 || row.ImageCount != 3) {
			t.Fatalf("image tag counts = %+v, want doc_count=0 image_count=3", row)
		}
	}
	slices.Sort(gotIDs)
	if !slices.Equal(gotIDs, []string{sourceID, targetID}) {
		t.Fatalf("image scope ids = %v, want first page image-related tags", gotIDs)
	}

	update := httptest.NewRecorder()
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/tag/"+imageID, strings.NewReader(`{"name":" 更新后 ","color":"#abcdef"}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(update, updateReq)
	if update.Code != http.StatusOK {
		t.Fatalf("update status = %d body=%s", update.Code, update.Body.String())
	}
	var updateBody struct {
		Data struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Color      string `json:"color"`
			ImageCount int64  `json:"image_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(update.Body.Bytes(), &updateBody); err != nil {
		t.Fatalf("unmarshal update body: %v", err)
	}
	if updateBody.Data.ID != imageID || updateBody.Data.Name != "更新后" || updateBody.Data.Color != "#abcdef" || updateBody.Data.ImageCount != 3 {
		t.Fatalf("update body = %+v, want trimmed updated tag with image count", updateBody.Data)
	}

	merge := httptest.NewRecorder()
	mergeReq := httptest.NewRequest(http.MethodPost, "/api/tag/merge", strings.NewReader(`{"source_id":"`+sourceID+`","target_id":"`+targetID+`"}`))
	mergeReq.Header.Set("Content-Type", "application/json")
	mergeReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(merge, mergeReq)
	if merge.Code != http.StatusOK {
		t.Fatalf("merge status = %d body=%s", merge.Code, merge.Body.String())
	}
	var mergeBody struct {
		Data struct {
			ID         string `json:"id"`
			DocCount   int64  `json:"doc_count"`
			ImageCount int64  `json:"image_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(merge.Body.Bytes(), &mergeBody); err != nil {
		t.Fatalf("unmarshal merge body: %v", err)
	}
	if mergeBody.Data.ID != targetID || mergeBody.Data.DocCount != 3 || mergeBody.Data.ImageCount != 2 {
		t.Fatalf("merge body = %+v, want target with merged counts", mergeBody.Data)
	}

	del := httptest.NewRecorder()
	delReq := httptest.NewRequest(http.MethodDelete, "/api/tag/"+imageID, nil)
	delReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(del, delReq)
	if del.Code != http.StatusOK {
		t.Fatalf("delete status = %d body=%s", del.Code, del.Body.String())
	}
}

func TestTagUpdateRouteIgnoresDocumentChunkTagSyncFailure(t *testing.T) {
	// 验证标签更新路由在 ES tags 同步失败时仍返回 PG 更新后的成功响应。
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	tagID := uuid.New()
	docID := uuid.New()
	tagRepo := &testTagRepository{
		rows:             []*models.Tag{{ID: tagID, UserID: userID, Name: "旧标签", Color: "#111111"}},
		docCounts:        map[uuid.UUID]int64{tagID: 1},
		imageCounts:      map[uuid.UUID]int64{},
		documentIDsByTag: map[uuid.UUID][]uuid.UUID{tagID: []uuid.UUID{docID}},
		documentTagNames: map[uuid.UUID][]string{docID: []string{"新标签"}},
	}
	chunkRepo := &testRAGChunkRepository{updateTagsErr: errors.New("es unavailable")}
	router := newTestRouterWithConfigAndOverrides(t, config.Config{App: config.AppConfig{Env: "test"}}, func(svcCtx *svc.ServiceContext) {
		svcCtx.TagRepo = tagRepo
		svcCtx.RAGChunkRepo = chunkRepo
	})

	update := httptest.NewRecorder()
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/tag/"+tagID.String(), strings.NewReader(`{"name":"新标签"}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(update, updateReq)
	if update.Code != http.StatusOK {
		t.Fatalf("update status = %d body=%s, want 200 despite ES sync failure", update.Code, update.Body.String())
	}
	var updateBody struct {
		Data struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(update.Body.Bytes(), &updateBody); err != nil {
		t.Fatalf("unmarshal update body: %v", err)
	}
	if updateBody.Data.ID != tagID.String() || updateBody.Data.Name != "新标签" {
		t.Fatalf("update body = %+v, want updated tag", updateBody.Data)
	}
	if len(chunkRepo.tagUpdates) != 1 || chunkRepo.tagUpdates[0].documentID != docID || !slices.Equal(chunkRepo.tagUpdates[0].tags, []string{"新标签"}) {
		t.Fatalf("tag updates = %+v, want one attempted ES tag sync", chunkRepo.tagUpdates)
	}
}

func TestDocumentRoutesSupportCoreAuthenticatedFlow(t *testing.T) {
	// 验证文档 HTTP 路由支持上传、列表、详情、状态、预览、重试、移动、删除和检索绑定。
	router := newTestRouter(t)

	kb := httptest.NewRecorder()
	kbReq := httptest.NewRequest(http.MethodGet, "/api/knowledge-base/", nil)
	kbReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(kb, kbReq)
	if kb.Code != http.StatusOK {
		t.Fatalf("kb status = %d body=%s", kb.Code, kb.Body.String())
	}
	var kbBody struct {
		Data struct {
			List []struct {
				ID string `json:"id"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(kb.Body.Bytes(), &kbBody); err != nil {
		t.Fatalf("unmarshal kb body: %v", err)
	}
	if len(kbBody.Data.List) != 1 || kbBody.Data.List[0].ID == "" {
		t.Fatalf("kb body = %+v, want default knowledge base id", kbBody.Data)
	}
	kbID := kbBody.Data.List[0].ID

	upload := httptest.NewRecorder()
	uploadReq := newMultipartDocumentRequest(t, "/api/document/upload", "doc.txt", "hello", kbID)
	uploadReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(upload, uploadReq)
	if upload.Code != http.StatusOK {
		t.Fatalf("upload status = %d body=%s", upload.Code, upload.Body.String())
	}
	var uploadBody struct {
		Data struct {
			ID       string   `json:"id"`
			KBID     string   `json:"kb_id"`
			FileName string   `json:"file_name"`
			FileExt  string   `json:"file_ext"`
			Status   string   `json:"status"`
			Tags     []string `json:"tags"`
		} `json:"data"`
	}
	if err := json.Unmarshal(upload.Body.Bytes(), &uploadBody); err != nil {
		t.Fatalf("unmarshal upload body: %v", err)
	}
	if uploadBody.Data.ID == "" || uploadBody.Data.KBID != kbID || uploadBody.Data.FileName != "doc.txt" || uploadBody.Data.FileExt != ".txt" || uploadBody.Data.Status != "pending" {
		t.Fatalf("upload body = %+v, want created document", uploadBody.Data)
	}
	docID := uploadBody.Data.ID

	importURL := httptest.NewRecorder()
	importURLReq := httptest.NewRequest(http.MethodPost, "/api/document/from-url", strings.NewReader(`{"url":"https://example.com/imported","kb_id":"`+kbID+`"}`))
	importURLReq.Header.Set("Content-Type", "application/json")
	importURLReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(importURL, importURLReq)
	if importURL.Code != http.StatusOK {
		t.Fatalf("import url status = %d body=%s", importURL.Code, importURL.Body.String())
	}
	var importURLBody struct {
		Data struct {
			ID         string `json:"id"`
			KBID       string `json:"kb_id"`
			FileName   string `json:"file_name"`
			FileExt    string `json:"file_ext"`
			SourceType string `json:"source_type"`
			SourceUrl  string `json:"source_url"`
			Status     string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(importURL.Body.Bytes(), &importURLBody); err != nil {
		t.Fatalf("unmarshal import url body: %v", err)
	}
	if importURLBody.Data.ID == "" || importURLBody.Data.KBID != kbID || importURLBody.Data.FileExt != ".txt" ||
		importURLBody.Data.SourceType != "url" || importURLBody.Data.SourceUrl != "https://example.com/imported" || importURLBody.Data.Status != "pending" {
		t.Fatalf("import url body = %+v, want created URL document", importURLBody.Data)
	}

	kbAfterUpload := httptest.NewRecorder()
	kbAfterUploadReq := httptest.NewRequest(http.MethodGet, "/api/knowledge-base/", nil)
	kbAfterUploadReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(kbAfterUpload, kbAfterUploadReq)
	if kbAfterUpload.Code != http.StatusOK {
		t.Fatalf("kb after upload status = %d body=%s", kbAfterUpload.Code, kbAfterUpload.Body.String())
	}
	var kbAfterUploadBody struct {
		Data struct {
			List []struct {
				ID         string `json:"id"`
				DocCount   int    `json:"doc_count"`
				ImageCount int    `json:"image_count"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(kbAfterUpload.Body.Bytes(), &kbAfterUploadBody); err != nil {
		t.Fatalf("unmarshal kb after upload body: %v", err)
	}
	if len(kbAfterUploadBody.Data.List) != 1 || kbAfterUploadBody.Data.List[0].ID != kbID ||
		kbAfterUploadBody.Data.List[0].DocCount != 2 || kbAfterUploadBody.Data.List[0].ImageCount != 0 {
		t.Fatalf("kb after upload body = %+v, want doc_count=2 image_count=0", kbAfterUploadBody.Data)
	}

	list := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/document/?page=1&page_size=10&kbid="+kbID, nil)
	listReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(list, listReq)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", list.Code, list.Body.String())
	}
	var listBody struct {
		Data struct {
			Total int `json:"total"`
			List  []struct {
				ID string `json:"id"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("unmarshal list body: %v", err)
	}
	if listBody.Data.Total != 2 || len(listBody.Data.List) != 2 || listBody.Data.List[0].ID != docID {
		t.Fatalf("list body = %+v, want uploaded and URL imported documents", listBody.Data)
	}

	for _, route := range []string{"/api/document/" + docID, "/api/document/" + docID + "/status", "/api/document/" + docID + "/preview"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, route, nil)
		req.Header.Set("Authorization", "Bearer dev-token")
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", route, w.Code, w.Body.String())
		}
	}

	retry := httptest.NewRecorder()
	retryReq := httptest.NewRequest(http.MethodPost, "/api/document/"+docID+"/retry", nil)
	retryReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(retry, retryReq)
	if retry.Code != http.StatusOK {
		t.Fatalf("retry status = %d body=%s", retry.Code, retry.Body.String())
	}

	move := httptest.NewRecorder()
	moveReq := httptest.NewRequest(http.MethodPost, "/api/document/"+docID+"/move", strings.NewReader(`{"kb_id":"`+kbID+`"}`))
	moveReq.Header.Set("Content-Type", "application/json")
	moveReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(move, moveReq)
	if move.Code != http.StatusOK {
		t.Fatalf("move status = %d body=%s", move.Code, move.Body.String())
	}

	model := httptest.NewRecorder()
	modelReq := httptest.NewRequest(http.MethodPost, "/api/model-configs/create", strings.NewReader(`{"type":"embedding","provider":"openai","name":"Embedding","model_name":"text-embedding-3-small","base_url":"https://api.openai.com/v1","api_key":"sk-test","is_default":true}`))
	modelReq.Header.Set("Content-Type", "application/json")
	modelReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(model, modelReq)
	if model.Code != http.StatusOK {
		t.Fatalf("create embedding model status = %d body=%s", model.Code, model.Body.String())
	}

	search := httptest.NewRecorder()
	searchReq := httptest.NewRequest(http.MethodPost, "/api/document/search", strings.NewReader(`{"query":"hello","top_k":5}`))
	searchReq.Header.Set("Content-Type", "application/json")
	searchReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(search, searchReq)
	if search.Code != http.StatusOK {
		t.Fatalf("search status = %d body=%s", search.Code, search.Body.String())
	}
	if !strings.Contains(search.Body.String(), `"list":[]`) {
		t.Fatalf("search body = %s, want empty list", search.Body.String())
	}

	del := httptest.NewRecorder()
	delReq := httptest.NewRequest(http.MethodDelete, "/api/document/"+docID, nil)
	delReq.Header.Set("Authorization", "Bearer dev-token")
	router.ServeHTTP(del, delReq)
	if del.Code != http.StatusOK {
		t.Fatalf("delete status = %d body=%s", del.Code, del.Body.String())
	}
}

func newMultipartDocumentRequest(t *testing.T, path string, fileName string, content string, kbID string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("kbid", kbID); err != nil {
		t.Fatalf("write kbid: %v", err)
	}
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}
