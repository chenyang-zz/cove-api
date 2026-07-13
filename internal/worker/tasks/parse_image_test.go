package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/boxify/api-go/internal/config"
	corellm "github.com/boxify/api-go/internal/core/llm"
	ragclassifier "github.com/boxify/api-go/internal/core/rag/classifier"
	"github.com/boxify/api-go/internal/domain/types"
	infraes "github.com/boxify/api-go/internal/infrastructure/db/es"
	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	repositoryes "github.com/boxify/api-go/internal/repository/es"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type fakeImageRepository struct {
	rows   map[uuid.UUID]*models.Image
	events *[]string
}

func newFakeImageRepository(rows ...*models.Image) *fakeImageRepository {
	repo := &fakeImageRepository{rows: map[uuid.UUID]*models.Image{}}
	for _, row := range rows {
		repo.rows[row.ID] = row
	}
	return repo
}

func (r *fakeImageRepository) Create(ctx context.Context, userID uuid.UUID, image *models.Image) (*models.Image, error) {
	image.UserID = userID
	r.rows[image.ID] = image
	return image, nil
}

func (r *fakeImageRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.Image, error) {
	return nil, nil
}

func (r *fakeImageRepository) PageList(ctx context.Context, userID uuid.UUID, query repository.ImageListQuery) ([]*models.Image, int64, error) {
	return nil, 0, nil
}

func (r *fakeImageRepository) CountByKnowledgeBase(ctx context.Context, userID uuid.UUID, kbIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	return map[uuid.UUID]int64{}, nil
}

func (r *fakeImageRepository) FindByID(ctx context.Context, userID uuid.UUID, imageID uuid.UUID) (*models.Image, error) {
	row, ok := r.rows[imageID]
	if !ok || row.UserID != userID {
		return nil, xerr.NotFound("图片不存在")
	}
	return row, nil
}

func (r *fakeImageRepository) Update(ctx context.Context, userID uuid.UUID, image *models.Image) (*models.Image, error) {
	r.rows[image.ID] = image
	return image, nil
}

func (r *fakeImageRepository) UpdateFields(ctx context.Context, userID uuid.UUID, imageID uuid.UUID, image *models.Image, fields *repository.ImageUpdateFields) (*models.Image, error) {
	row, err := r.FindByID(ctx, userID, imageID)
	if err != nil {
		return nil, err
	}
	for _, column := range fields.Columns() {
		switch column {
		case "status":
			row.Status = image.Status
			if r.events != nil {
				*r.events = append(*r.events, "status:"+image.Status)
			}
		case "progress":
			row.Progress = image.Progress
			if r.events != nil {
				*r.events = append(*r.events, fmt.Sprintf("progress:%.1f", image.Progress))
			}
		case "error_msg":
			row.ErrorMsg = image.ErrorMsg
		case "description":
			row.Description = image.Description
		case "ocr_text":
			row.OCRText = image.OCRText
		case "objects":
			row.Objects = image.Objects
		case "scene":
			row.Scene = image.Scene
		}
	}
	return row, nil
}

func (r *fakeImageRepository) Delete(ctx context.Context, userID uuid.UUID, imageID uuid.UUID) error {
	delete(r.rows, imageID)
	return nil
}

type fakeVisionLLMClient struct {
	fakeLLMClient
	visionAnswer string
	visionErr    error
	gotPrompt    string
	gotImageB64  string
	gotMIME      string
	gotMaxTokens int64
}

func (c *fakeVisionLLMClient) Vision(ctx context.Context, prompt string, imageBase64 string, mime string, opts ...corellm.ModelCallOption) (*corellm.VisionResult, error) {
	c.gotPrompt = prompt
	c.gotImageB64 = imageBase64
	c.gotMIME = mime
	visionOpts := corellm.NewVisionOptions(opts...)
	if visionOpts.MaxTokens != nil {
		c.gotMaxTokens = *visionOpts.MaxTokens
	}
	if c.visionErr != nil {
		return nil, c.visionErr
	}
	return &corellm.VisionResult{
		Description: corellm.ParseVisionDescription(c.visionAnswer),
		Text:        c.visionAnswer,
		Provider:    "fake",
	}, nil
}

// 验证 parse:image 成功路径会描述图片、写 ES 并标记 done。
func TestHandleParseImageProcessesImage(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	imageID := uuid.New()
	row := &models.Image{
		ID: imageID, UserID: userID, FileName: "cat.png", FileExt: ".png",
		FileKey: "images/cat.png", Status: types.ImageStatusPending,
		Tags: []models.Tag{{Name: "手动"}},
	}
	store := newMemoryStore()
	store.data[row.FileKey] = []byte("fake-image-bytes")

	var events []string
	indexedDocs := map[string]map[string]any{}
	var updateTagsBody map[string]any
	esServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/boxify_chunks":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPut && r.URL.Path == "/boxify_chunks":
			_, _ = w.Write([]byte(`{"acknowledged":true}`))
		case r.Method == http.MethodPost && r.URL.Path == "/boxify_chunks/_delete_by_query":
			_, _ = w.Write([]byte(`{"deleted":0}`))
		case r.Method == http.MethodPost && r.URL.Path == "/boxify_chunks/_update_by_query":
			events = append(events, "es:update_tags")
			if err := json.NewDecoder(r.Body).Decode(&updateTagsBody); err != nil {
				t.Fatalf("decode update tags body: %v", err)
			}
			_, _ = w.Write([]byte(`{"updated":1}`))
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/boxify_chunks/_doc/"):
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode indexed chunk body: %v", err)
			}
			indexedDocs[strings.TrimPrefix(r.URL.Path, "/boxify_chunks/_doc/")] = body
			_, _ = w.Write([]byte(`{"result":"created"}`))
		default:
			t.Fatalf("unexpected ES request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer esServer.Close()
	esClient, err := infraes.NewClient(infraes.Config{URL: esServer.URL})
	if err != nil {
		t.Fatalf("NewClient error = %v", err)
	}
	cipher, err := security.NewSecretCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretCipher error = %v", err)
	}
	encryptedAPIKey, err := cipher.Encrypt("db-key")
	if err != nil {
		t.Fatalf("Encrypt API key error = %v", err)
	}

	visionClient := &fakeVisionLLMClient{
		fakeLLMClient: fakeLLMClient{invokeAnswer: `["风景","手动"]`},
		visionAnswer:  `{"description":"一只猫","ocr_text":"Cat","objects":["猫","沙发"],"scene":"室内"}`,
	}
	imageRepo := newFakeImageRepository(row)
	imageRepo.events = &events
	tagRepo := &fakeTagRepository{events: &events}
	handler := NewParseImageTask(&svc.ServiceContext{
		Config: config.Config{Rag: config.RagConfig{EmbeddingDim: 3, ChunkIndex: "boxify_chunks"}},
		ImageRepo: imageRepo,
		TagRepo:   tagRepo,
		ModelConfigRepo: &fakeModelConfigRepository{rows: []*models.ModelConfig{
			{UserID: userID, Type: string(types.Multimodal), Provider: "fake", ModelName: "db-vision", APIKeyEncrypted: encryptedAPIKey, BaseURL: "https://llm.example", IsDefault: true},
			{UserID: userID, Type: string(types.EmbeddingModelType), Provider: "fake", ModelName: "db-embed", APIKeyEncrypted: encryptedAPIKey, BaseURL: "https://llm.example", IsDefault: true},
			{UserID: userID, Type: string(types.ChatModelType), Provider: "fake", ModelName: "db-chat", APIKeyEncrypted: encryptedAPIKey, BaseURL: "https://llm.example", IsDefault: true},
		}},
		SecretCipher:  cipher,
		Storage:       store,
		Elasticsearch: esClient,
		RAGChunkRepo:  repositoryes.NewRAGChunkRepository(esClient, "boxify_chunks"),
		RAGClassifier: ragclassifier.NewClassifier(),
		LLMManager:    newFakeLLMManager(visionClient),
	})
	task, err := types.NewParseImageTask(userID, imageID)
	if err != nil {
		t.Fatalf("NewParseImageTask error = %v", err)
	}

	if err := handler.Handle(ctx, task); err != nil {
		t.Fatalf("HandleParseImage error = %v", err)
	}
	if row.Status != types.ImageStatusDone || row.ErrorMsg != nil || row.Progress != 1 {
		t.Fatalf("image after parse = %+v, want done progress=1 without error", row)
	}
	if row.Description == nil || *row.Description != "一只猫" || row.OCRText == nil || *row.OCRText != "Cat" || row.Scene == nil || *row.Scene != "室内" {
		t.Fatalf("image description fields = desc:%v ocr:%v scene:%v", row.Description, row.OCRText, row.Scene)
	}
	if len(row.Objects) != 2 || row.Objects[0] != "猫" || row.Objects[1] != "沙发" {
		t.Fatalf("objects = %#v, want string list", row.Objects)
	}
	if visionClient.gotImageB64 == "" || visionClient.gotMIME == "" {
		t.Fatalf("vision call = prompt:%q mime:%q b64 empty=%v", visionClient.gotPrompt, visionClient.gotMIME, visionClient.gotImageB64 == "")
	}
	joinedEvents := strings.Join(events, "|")
	if !strings.Contains(joinedEvents, "progress:0.1") || !strings.Contains(joinedEvents, "progress:0.3") ||
		!strings.Contains(joinedEvents, "progress:0.6") || !strings.Contains(joinedEvents, "progress:0.8") ||
		!strings.Contains(joinedEvents, "progress:1.0") {
		t.Fatalf("events = %v, want staged progress updates", events)
	}
	if len(indexedDocs) != 1 {
		t.Fatalf("indexed chunks = %d, want 1", len(indexedDocs))
	}
	for _, body := range indexedDocs {
		if body["document_id"] != imageID.String() || body["source_type"] != "image" || body["doc_name"] != "cat.png" {
			t.Fatalf("indexed body = %#v, want image source metadata", body)
		}
		if !strings.Contains(body["content"].(string), "一只猫") || !strings.Contains(body["content"].(string), "Cat") {
			t.Fatalf("indexed content = %#v, want searchable description text", body["content"])
		}
	}
	if tagRepo.syncedUserID != userID || tagRepo.syncedDocumentID != imageID {
		t.Fatalf("synced image tags user=%s image=%s, want current image", tagRepo.syncedUserID, tagRepo.syncedDocumentID)
	}
	wantPrefix := "status:processing"
	if !strings.HasPrefix(strings.Join(events, "|"), wantPrefix) || !strings.Contains(strings.Join(events, "|"), "status:done") {
		t.Fatalf("events = %v, want processing then done", events)
	}
}

// 验证图片不存在时跳过重试。
func TestHandleParseImageSkipsRetryWhenImageMissing(t *testing.T) {
	ctx := context.Background()
	handler := NewParseImageTask(&svc.ServiceContext{
		ImageRepo: newFakeImageRepository(),
		Storage:   newMemoryStore(),
	})
	task, err := types.NewParseImageTask(uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("NewParseImageTask error = %v", err)
	}
	err = handler.Handle(ctx, task)
	if err == nil || !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("Handle error = %v, want SkipRetry", err)
	}
}

// 验证多模态描述失败时标记 failed 且不重试。
func TestHandleParseImageMarksFailedWhenDescribeFails(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	imageID := uuid.New()
	row := &models.Image{ID: imageID, UserID: userID, FileName: "a.png", FileExt: ".png", FileKey: "images/a.png", Status: types.ImageStatusPending}
	store := newMemoryStore()
	store.data[row.FileKey] = []byte("img")
	cipher, err := security.NewSecretCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretCipher error = %v", err)
	}
	encryptedAPIKey, err := cipher.Encrypt("db-key")
	if err != nil {
		t.Fatalf("Encrypt error = %v", err)
	}
	visionClient := &fakeVisionLLMClient{visionErr: errors.New("vision down")}
	handler := NewParseImageTask(&svc.ServiceContext{
		ImageRepo: newFakeImageRepository(row),
		Storage:   store,
		ModelConfigRepo: &fakeModelConfigRepository{rows: []*models.ModelConfig{
			{UserID: userID, Type: string(types.Multimodal), Provider: "fake", ModelName: "db-vision", APIKeyEncrypted: encryptedAPIKey, IsDefault: true},
		}},
		SecretCipher: cipher,
		LLMManager:   newFakeLLMManager(visionClient),
	})
	task, _ := types.NewParseImageTask(userID, imageID)
	if err := handler.Handle(ctx, task); err != nil {
		t.Fatalf("Handle error = %v, want nil after mark failed", err)
	}
	if row.Status != types.ImageStatusFailed || row.ErrorMsg == nil || !strings.Contains(*row.ErrorMsg, "vision down") {
		t.Fatalf("image = %+v, want failed with vision error", row)
	}
}

// 验证描述结果为空时仍标记 done，且不写入 ES。
func TestHandleParseImageCompletesWithoutIndexWhenDescriptionEmpty(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	imageID := uuid.New()
	row := &models.Image{ID: imageID, UserID: userID, FileName: "a.png", FileExt: ".png", FileKey: "images/a.png", Status: types.ImageStatusPending}
	store := newMemoryStore()
	store.data[row.FileKey] = []byte("img")
	esServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected ES request %s %s", r.Method, r.URL.Path)
	}))
	defer esServer.Close()
	esClient, err := infraes.NewClient(infraes.Config{URL: esServer.URL})
	if err != nil {
		t.Fatalf("NewClient error = %v", err)
	}
	cipher, err := security.NewSecretCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretCipher error = %v", err)
	}
	encryptedAPIKey, err := cipher.Encrypt("db-key")
	if err != nil {
		t.Fatalf("Encrypt error = %v", err)
	}
	visionClient := &fakeVisionLLMClient{visionAnswer: `{"description":"","ocr_text":"","objects":[],"scene":""}`}
	handler := NewParseImageTask(&svc.ServiceContext{
		Config:    config.Config{Rag: config.RagConfig{EmbeddingDim: 3, ChunkIndex: "boxify_chunks"}},
		ImageRepo: newFakeImageRepository(row),
		Storage:   store,
		ModelConfigRepo: &fakeModelConfigRepository{rows: []*models.ModelConfig{
			{UserID: userID, Type: string(types.Multimodal), Provider: "fake", ModelName: "db-vision", APIKeyEncrypted: encryptedAPIKey, IsDefault: true},
			{UserID: userID, Type: string(types.ChatModelType), Provider: "fake", ModelName: "db-chat", APIKeyEncrypted: encryptedAPIKey, IsDefault: true},
		}},
		SecretCipher:  cipher,
		Elasticsearch: esClient,
		RAGChunkRepo:  repositoryes.NewRAGChunkRepository(esClient, "boxify_chunks"),
		RAGClassifier: ragclassifier.NewClassifier(),
		TagRepo:       &fakeTagRepository{},
		LLMManager:    newFakeLLMManager(visionClient),
	})
	task, _ := types.NewParseImageTask(userID, imageID)
	if err := handler.Handle(ctx, task); err != nil {
		t.Fatalf("Handle error = %v", err)
	}
	if row.Status != types.ImageStatusDone {
		t.Fatalf("status = %s, want done", row.Status)
	}
}

// 验证标签同步失败不阻断图片解析完成。
func TestHandleParseImageIgnoresTagSyncFailure(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	imageID := uuid.New()
	row := &models.Image{ID: imageID, UserID: userID, FileName: "a.png", FileExt: ".png", FileKey: "images/a.png", Status: types.ImageStatusPending}
	store := newMemoryStore()
	store.data[row.FileKey] = []byte("img")
	var indexCount int
	esServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/boxify_chunks":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/boxify_chunks/_delete_by_query":
			_, _ = w.Write([]byte(`{"deleted":0}`))
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/boxify_chunks/_doc/"):
			indexCount++
			_, _ = w.Write([]byte(`{"result":"created"}`))
		default:
			t.Fatalf("unexpected ES request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer esServer.Close()
	esClient, err := infraes.NewClient(infraes.Config{URL: esServer.URL})
	if err != nil {
		t.Fatalf("NewClient error = %v", err)
	}
	cipher, err := security.NewSecretCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretCipher error = %v", err)
	}
	encryptedAPIKey, err := cipher.Encrypt("db-key")
	if err != nil {
		t.Fatalf("Encrypt error = %v", err)
	}
	visionClient := &fakeVisionLLMClient{
		fakeLLMClient: fakeLLMClient{invokeAnswer: `["标签"]`},
		visionAnswer:  `{"description":"描述","ocr_text":"","objects":[],"scene":"场景"}`,
	}
	handler := NewParseImageTask(&svc.ServiceContext{
		Config:    config.Config{Rag: config.RagConfig{EmbeddingDim: 3, ChunkIndex: "boxify_chunks"}},
		ImageRepo: newFakeImageRepository(row),
		Storage:   store,
		ModelConfigRepo: &fakeModelConfigRepository{rows: []*models.ModelConfig{
			{UserID: userID, Type: string(types.Multimodal), Provider: "fake", ModelName: "db-vision", APIKeyEncrypted: encryptedAPIKey, IsDefault: true},
			{UserID: userID, Type: string(types.EmbeddingModelType), Provider: "fake", ModelName: "db-embed", APIKeyEncrypted: encryptedAPIKey, IsDefault: true},
			{UserID: userID, Type: string(types.ChatModelType), Provider: "fake", ModelName: "db-chat", APIKeyEncrypted: encryptedAPIKey, IsDefault: true},
		}},
		SecretCipher:  cipher,
		Elasticsearch: esClient,
		RAGChunkRepo:  repositoryes.NewRAGChunkRepository(esClient, "boxify_chunks"),
		RAGClassifier: ragclassifier.NewClassifier(),
		TagRepo:       &fakeTagRepository{syncErr: errors.New("tag db down")},
		LLMManager:    newFakeLLMManager(visionClient),
	})
	task, _ := types.NewParseImageTask(userID, imageID)
	if err := handler.Handle(ctx, task); err != nil {
		t.Fatalf("Handle error = %v", err)
	}
	if row.Status != types.ImageStatusDone {
		t.Fatalf("status = %s, want done despite tag sync failure", row.Status)
	}
	if indexCount != 1 {
		t.Fatalf("index count = %d, want 1", indexCount)
	}
}

// 验证缺少多模态模型配置时标记 failed。
func TestHandleParseImageMarksFailedWhenMultimodalConfigMissing(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	imageID := uuid.New()
	row := &models.Image{ID: imageID, UserID: userID, FileName: "a.png", FileExt: ".png", FileKey: "images/a.png", Status: types.ImageStatusPending}
	store := newMemoryStore()
	store.data[row.FileKey] = []byte("img")
	cipher, err := security.NewSecretCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretCipher error = %v", err)
	}
	handler := NewParseImageTask(&svc.ServiceContext{
		ImageRepo:       newFakeImageRepository(row),
		Storage:         store,
		ModelConfigRepo: &fakeModelConfigRepository{},
		SecretCipher:    cipher,
		LLMManager:      newFakeLLMManager(&fakeVisionLLMClient{}),
	})
	task, _ := types.NewParseImageTask(userID, imageID)
	if err := handler.Handle(ctx, task); err != nil {
		t.Fatalf("Handle error = %v, want nil after mark failed", err)
	}
	if row.Status != types.ImageStatusFailed || row.ErrorMsg == nil || !strings.Contains(*row.ErrorMsg, "未配置多模态模型") {
		t.Fatalf("image = %+v, want failed missing multimodal config", row)
	}
}
