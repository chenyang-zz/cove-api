package image

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/boxify/api-go/internal/config"
	corellm "github.com/boxify/api-go/internal/core/llm"
	ragchunker "github.com/boxify/api-go/internal/core/rag/chunker"
	ragsearch "github.com/boxify/api-go/internal/core/rag/search"
	"github.com/boxify/api-go/internal/domain/types"
	infraes "github.com/boxify/api-go/internal/infrastructure/db/es"
	"github.com/boxify/api-go/internal/infrastructure/queue"
	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	repositoryes "github.com/boxify/api-go/internal/repository/es"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

type fakeImageRepository struct {
	rows           map[uuid.UUID]*models.Image
	created        *models.Image
	deletedID      uuid.UUID
	partial        *models.Image
	fields         []string
	pageListQuery  repository.ImageListQuery
	pageListCalled bool
	listTotal      int64
}

func newFakeImageRepository(rows ...*models.Image) *fakeImageRepository {
	repo := &fakeImageRepository{rows: map[uuid.UUID]*models.Image{}}
	for _, row := range rows {
		repo.rows[row.ID] = row
	}
	return repo
}

func (r *fakeImageRepository) Create(ctx context.Context, userID uuid.UUID, row *models.Image) (*models.Image, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	row.UserID = userID
	r.created = row
	r.rows[row.ID] = row
	return row, nil
}

func (r *fakeImageRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.Image, error) {
	out, _, err := r.pageRows(userID)
	return out, err
}

func (r *fakeImageRepository) PageList(ctx context.Context, userID uuid.UUID, query repository.ImageListQuery) ([]*models.Image, int64, error) {
	r.pageListCalled = true
	r.pageListQuery = query
	return r.pageRows(userID)
}

func (r *fakeImageRepository) CountByKnowledgeBase(ctx context.Context, userID uuid.UUID, kbIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	return map[uuid.UUID]int64{}, nil
}

func (r *fakeImageRepository) pageRows(userID uuid.UUID) ([]*models.Image, int64, error) {
	out := make([]*models.Image, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].FileName < out[j].FileName
	})
	total := r.listTotal
	if total == 0 {
		total = int64(len(out))
	}
	return out, total, nil
}

func (r *fakeImageRepository) FindByID(ctx context.Context, userID uuid.UUID, imageID uuid.UUID) (*models.Image, error) {
	row, ok := r.rows[imageID]
	if !ok || row.UserID != userID {
		return nil, xerr.NotFound("图片不存在")
	}
	return row, nil
}

func (r *fakeImageRepository) Update(ctx context.Context, userID uuid.UUID, row *models.Image) (*models.Image, error) {
	r.rows[row.ID] = row
	return row, nil
}

func (r *fakeImageRepository) UpdateFields(ctx context.Context, userID uuid.UUID, imageID uuid.UUID, row *models.Image, fields *repository.ImageUpdateFields) (*models.Image, error) {
	r.partial = row
	r.fields = fields.Columns()
	existing, err := r.FindByID(ctx, userID, imageID)
	if err != nil {
		return nil, err
	}
	for _, column := range r.fields {
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

func (r *fakeImageRepository) Delete(ctx context.Context, userID uuid.UUID, imageID uuid.UUID) error {
	if _, err := r.FindByID(ctx, userID, imageID); err != nil {
		return err
	}
	r.deletedID = imageID
	delete(r.rows, imageID)
	return nil
}

type fakeImageKnowledgeBaseRepository struct {
	rows              map[uuid.UUID]*models.KnowledgeBase
	created           *models.KnowledgeBase
	findDefaultCalled bool
}

func newFakeImageKnowledgeBaseRepository(rows ...*models.KnowledgeBase) *fakeImageKnowledgeBaseRepository {
	repo := &fakeImageKnowledgeBaseRepository{rows: map[uuid.UUID]*models.KnowledgeBase{}}
	for _, row := range rows {
		repo.rows[row.ID] = row
	}
	return repo
}

func (r *fakeImageKnowledgeBaseRepository) Create(ctx context.Context, userID uuid.UUID, row *models.KnowledgeBase) (*models.KnowledgeBase, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	row.UserID = userID
	r.created = row
	r.rows[row.ID] = row
	return row, nil
}

func (r *fakeImageKnowledgeBaseRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.KnowledgeBase, error) {
	out := make([]*models.KnowledgeBase, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeImageKnowledgeBaseRepository) FindDefault(ctx context.Context, userID uuid.UUID) (*models.KnowledgeBase, error) {
	r.findDefaultCalled = true
	for _, row := range r.rows {
		if row.UserID == userID && row.IsDefault {
			return row, nil
		}
	}
	return nil, xerr.NotFound("默认知识库不存在")
}

func (r *fakeImageKnowledgeBaseRepository) FindByID(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID) (*models.KnowledgeBase, error) {
	row, ok := r.rows[knowledgeBaseID]
	if !ok || row.UserID != userID {
		return nil, xerr.NotFound("知识库不存在")
	}
	return row, nil
}

func (r *fakeImageKnowledgeBaseRepository) SetDefault(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID) (*models.KnowledgeBase, error) {
	row, err := r.FindByID(ctx, userID, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	row.IsDefault = true
	return row, nil
}

func (r *fakeImageKnowledgeBaseRepository) Update(ctx context.Context, userID uuid.UUID, row *models.KnowledgeBase) (*models.KnowledgeBase, error) {
	r.rows[row.ID] = row
	return row, nil
}

func (r *fakeImageKnowledgeBaseRepository) UpdateFields(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID, row *models.KnowledgeBase, fields *repository.KnowledgeBaseUpdateFields) (*models.KnowledgeBase, error) {
	return r.FindByID(ctx, userID, knowledgeBaseID)
}

func (r *fakeImageKnowledgeBaseRepository) Delete(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID) error {
	delete(r.rows, knowledgeBaseID)
	return nil
}

type memoryImageStore struct {
	data      map[string][]byte
	deleted   []string
	deleteErr error
}

func newMemoryImageStore() *memoryImageStore {
	return &memoryImageStore{data: map[string][]byte{}}
}

func (s *memoryImageStore) Ping(ctx context.Context) error { return nil }

func (s *memoryImageStore) Put(ctx context.Context, key string, data []byte) error {
	s.data[key] = append([]byte(nil), data...)
	return nil
}

func (s *memoryImageStore) Get(ctx context.Context, key string) ([]byte, error) {
	data, ok := s.data[key]
	if !ok {
		return nil, xerr.NotFound("文件不存在")
	}
	return append([]byte(nil), data...), nil
}

func (s *memoryImageStore) Delete(ctx context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.data, key)
	return nil
}

type fakeImageURLSigner struct{}

func (fakeImageURLSigner) URL(key string) string {
	return "https://cdn.example.com/" + key
}

type fakeImageTaskProducer struct {
	tasks    []*types.Task
	parseErr error
}

func (p *fakeImageTaskProducer) Enqueue(ctx context.Context, task *types.Task, opts ...queue.EnqueueOption) (*queue.TaskInfo, error) {
	if p.parseErr != nil {
		return nil, p.parseErr
	}
	p.tasks = append(p.tasks, task)
	return &queue.TaskInfo{ID: "task-id", Name: task.Name, Queue: task.Queue}, nil
}

func (p *fakeImageTaskProducer) Close() error { return nil }

type fakeImageRAGChunkRepository struct {
	deletedUserID     uuid.UUID
	deletedDocumentID uuid.UUID
	updatedUserID     uuid.UUID
	updatedDocumentID uuid.UUID
	updatedKBID       uuid.UUID
}

func (r *fakeImageRAGChunkRepository) EnsureIndex(ctx context.Context, embeddingDim int) error {
	return nil
}

func (r *fakeImageRAGChunkRepository) IndexDocumentChunks(ctx context.Context, document *models.Document, chunks []*ragchunker.Chunk, vectors [][]float64) error {
	return nil
}

func (r *fakeImageRAGChunkRepository) IndexImageChunk(ctx context.Context, image *models.Image, content string, vector []float64) error {
	return nil
}

func (r *fakeImageRAGChunkRepository) DeleteBySource(ctx context.Context, userID uuid.UUID, sourceID uuid.UUID) error {
	r.deletedUserID = userID
	r.deletedDocumentID = sourceID
	return nil
}

func (r *fakeImageRAGChunkRepository) UpdateKnowledgeBase(ctx context.Context, userID uuid.UUID, sourceID uuid.UUID, kbID uuid.UUID) error {
	r.updatedUserID = userID
	r.updatedDocumentID = sourceID
	r.updatedKBID = kbID
	return nil
}

func (r *fakeImageRAGChunkRepository) UpdateTags(ctx context.Context, userID uuid.UUID, sourceID uuid.UUID, tags []string) error {
	return nil
}

func (r *fakeImageRAGChunkRepository) DecodeSource(src map[string]any) (models.RAGChunkSource, error) {
	return models.RAGChunkSource{}, nil
}

type fakeSearchLLMFactory struct {
	client corellm.Client
}

func (f fakeSearchLLMFactory) NewClient(cfg corellm.ModelConfig) (corellm.Client, error) {
	return f.client, nil
}

type fakeSearchLLMClient struct{}

func (c fakeSearchLLMClient) Invoke(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (string, error) {
	return "", nil
}

func (c fakeSearchLLMClient) InvokeResult(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (*corellm.LLMResult, error) {
	text, err := c.Invoke(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	return &corellm.LLMResult{Text: text}, nil
}

func (c fakeSearchLLMClient) Stream(ctx context.Context, messages []*corellm.Message, opts ...corellm.ModelCallOption) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (c fakeSearchLLMClient) Embed(ctx context.Context, texts []string, dimensions int, opts ...corellm.EmbeddingOption) ([][]float64, error) {
	out := make([][]float64, 0, len(texts))
	for range texts {
		out = append(out, []float64{0.1, 0.2, 0.3})
	}
	return out, nil
}

func (c fakeSearchLLMClient) EmbedOne(ctx context.Context, text string, dimensions int) ([]float64, error) {
	return []float64{0.1, 0.2, 0.3}, nil
}

func newFakeSearchLLMManager() *corellm.Manager {
	manager := corellm.NewManager()
	manager.Register("fake", fakeSearchLLMFactory{client: fakeSearchLLMClient{}})
	return manager
}

type fakeSearchModelConfigRepository struct {
	rows []*models.ModelConfig
}

func (r *fakeSearchModelConfigRepository) Create(ctx context.Context, row *models.ModelConfig) (*models.ModelConfig, error) {
	return row, nil
}

func (r *fakeSearchModelConfigRepository) Update(ctx context.Context, row *models.ModelConfig) (*models.ModelConfig, error) {
	return row, nil
}

func (r *fakeSearchModelConfigRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (r *fakeSearchModelConfigRepository) List(ctx context.Context, userID uuid.UUID, modelType *types.ModelType) ([]*models.ModelConfig, error) {
	out := make([]*models.ModelConfig, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID == userID && (modelType == nil || row.Type == string(*modelType)) {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeSearchModelConfigRepository) FindByID(ctx context.Context, userID uuid.UUID, configID uuid.UUID) (*models.ModelConfig, error) {
	return nil, xerr.NotFound("模型配置不存在")
}

func testFileHeader(t *testing.T, name string, content []byte) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	reader := multipart.NewReader(&body, writer.Boundary())
	form, err := reader.ReadForm(int64(len(content)) + 1024)
	if err != nil {
		t.Fatalf("read form: %v", err)
	}
	return form.File["file"][0]
}

func ptrString(v string) *string { return &v }

func parseImagePayloadFromTask(t *testing.T, tasks []*types.Task, index int) *types.ParseImagePayload {
	t.Helper()
	if len(tasks) <= index {
		t.Fatalf("tasks = %+v, want task at index %d", tasks, index)
	}
	if tasks[index].Name != types.TaskParseImage {
		t.Fatalf("task name = %q, want %q", tasks[index].Name, types.TaskParseImage)
	}
	payload, ok := tasks[index].Payload.(*types.ParseImagePayload)
	if !ok {
		t.Fatalf("payload type = %T, want *types.ParseImagePayload", tasks[index].Payload)
	}
	return payload
}

// 验证上传图片会写入存储、创建默认知识库，并保存 pending 状态的图片元数据。
func TestUploadImageStoresFileAndCreatesDefaultKnowledgeBase(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	imageRepo := newFakeImageRepository()
	kbRepo := newFakeImageKnowledgeBaseRepository()
	store := newMemoryImageStore()
	producer := &fakeImageTaskProducer{}
	logic := NewUploadImageLogic(ctx, &svc.ServiceContext{
		ImageRepo:         imageRepo,
		KnowledgeBaseRepo: kbRepo,
		Storage:           store,
		TaskProducer:      producer,
		URLSigner:         fakeImageURLSigner{},
	})

	out, err := logic.UploadImage(userID, &request.UploadImageRequest{
		File: testFileHeader(t, " photo.PNG ", []byte("hello")),
	})
	if err != nil {
		t.Fatalf("UploadImage error = %v", err)
	}
	if kbRepo.created == nil || !kbRepo.created.IsDefault || kbRepo.created.Name != "默认知识库" {
		t.Fatalf("default kb = %+v, want created default knowledge base", kbRepo.created)
	}
	if !kbRepo.findDefaultCalled {
		t.Fatal("FindDefault was not called before creating default knowledge base")
	}
	if imageRepo.created == nil {
		t.Fatal("image was not created")
	}
	if imageRepo.created.UserID != userID || imageRepo.created.KBID == nil || *imageRepo.created.KBID != kbRepo.created.ID {
		t.Fatalf("created image owner/kb = %+v, want current user default kb", imageRepo.created)
	}
	if imageRepo.created.FileName != "photo.PNG" || imageRepo.created.FileExt != ".png" || imageRepo.created.FileSize != 5 || imageRepo.created.Status != types.ImageStatusPending {
		t.Fatalf("created image = %+v, want normalized file metadata", imageRepo.created)
	}
	if string(store.data[imageRepo.created.FileKey]) != "hello" {
		t.Fatalf("stored content = %q, want hello", string(store.data[imageRepo.created.FileKey]))
	}
	if out.ID != imageRepo.created.ID || out.FileName != "photo.PNG" || out.KBID == nil || *out.KBID != kbRepo.created.ID || out.Status != types.ImageStatusPending {
		t.Fatalf("response = %+v, want created image response", out)
	}
	if !strings.HasPrefix(out.Url, "https://cdn.example.com/") {
		t.Fatalf("response url = %q, want signed url", out.Url)
	}
	payload := parseImagePayloadFromTask(t, producer.tasks, 0)
	if payload.UserID != userID || payload.ImageID != imageRepo.created.ID {
		t.Fatalf("parse payload = %+v, want current user/image enqueued", payload)
	}
}

// 验证上传图片未指定知识库时会优先复用已有默认知识库，不重复创建默认库。
func TestUploadImageUsesExistingDefaultKnowledgeBase(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	defaultKB := &models.KnowledgeBase{ID: uuid.New(), UserID: userID, Name: "默认知识库", IsDefault: true}
	imageRepo := newFakeImageRepository()
	kbRepo := newFakeImageKnowledgeBaseRepository(defaultKB)
	logic := NewUploadImageLogic(ctx, &svc.ServiceContext{
		ImageRepo:         imageRepo,
		KnowledgeBaseRepo: kbRepo,
		Storage:           newMemoryImageStore(),
		TaskProducer:      &fakeImageTaskProducer{},
	})

	out, err := logic.UploadImage(userID, &request.UploadImageRequest{File: testFileHeader(t, "a.jpg", []byte("hello"))})
	if err != nil {
		t.Fatalf("UploadImage error = %v", err)
	}
	if kbRepo.created != nil {
		t.Fatalf("created default = %+v, want reuse existing default", kbRepo.created)
	}
	if out.KBID == nil || *out.KBID != defaultKB.ID {
		t.Fatalf("response kb id = %v, want existing default %s", out.KBID, defaultKB.ID)
	}
}

// 验证上传会拒绝不支持的扩展名和超过 20MB 的文件。
func TestUploadImageRejectsUnsupportedAndOversizedFiles(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	logic := NewUploadImageLogic(ctx, &svc.ServiceContext{
		ImageRepo:         newFakeImageRepository(),
		KnowledgeBaseRepo: newFakeImageKnowledgeBaseRepository(),
		Storage:           newMemoryImageStore(),
	})

	if _, err := logic.UploadImage(userID, &request.UploadImageRequest{File: testFileHeader(t, "bad.exe", []byte("x"))}); err == nil {
		t.Fatal("UploadImage unsupported ext error = nil, want error")
	}
	large := testFileHeader(t, "large.png", []byte("x"))
	large.Size = maxImageFileSize + 1
	if _, err := logic.UploadImage(userID, &request.UploadImageRequest{File: large}); err == nil {
		t.Fatal("UploadImage oversized error = nil, want error")
	}
}

// 验证上传指定知识库时会校验知识库归属，并把图片归入指定知识库。
func TestUploadImageValidatesExplicitKnowledgeBase(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	kbID := uuid.New()
	imageRepo := newFakeImageRepository()
	kbRepo := newFakeImageKnowledgeBaseRepository(&models.KnowledgeBase{ID: kbID, UserID: userID, Name: "项目库"})
	logic := NewUploadImageLogic(ctx, &svc.ServiceContext{
		ImageRepo:         imageRepo,
		KnowledgeBaseRepo: kbRepo,
		Storage:           newMemoryImageStore(),
		TaskProducer:      &fakeImageTaskProducer{},
	})

	out, err := logic.UploadImage(userID, &request.UploadImageRequest{
		File: testFileHeader(t, "a.webp", []byte("hello")),
		KBID: ptrString(kbID.String()),
	})
	if err != nil {
		t.Fatalf("UploadImage error = %v", err)
	}
	if out.KBID == nil || *out.KBID != kbID || imageRepo.created.KBID == nil || *imageRepo.created.KBID != kbID {
		t.Fatalf("kb id = response:%v row:%v, want %s", out.KBID, imageRepo.created.KBID, kbID)
	}
	if _, err := logic.UploadImage(userID, &request.UploadImageRequest{File: testFileHeader(t, "a.webp", []byte("hello")), KBID: ptrString(uuid.NewString())}); err == nil {
		t.Fatal("UploadImage missing kb error = nil, want error")
	}
}

// 验证图片创建后如果解析任务入队失败，会把图片标记为 failed，避免长期停留在 pending。
func TestUploadImageMarksFailedWhenQueueEnqueueFails(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	imageRepo := newFakeImageRepository()
	kbRepo := newFakeImageKnowledgeBaseRepository(&models.KnowledgeBase{ID: uuid.New(), UserID: userID, Name: "默认知识库", IsDefault: true})
	producer := &fakeImageTaskProducer{parseErr: xerr.Internal("queue down", nil)}
	logic := NewUploadImageLogic(ctx, &svc.ServiceContext{
		ImageRepo:         imageRepo,
		KnowledgeBaseRepo: kbRepo,
		Storage:           newMemoryImageStore(),
		TaskProducer:      producer,
	})

	if _, err := logic.UploadImage(userID, &request.UploadImageRequest{File: testFileHeader(t, "a.jpg", []byte("hello"))}); err == nil {
		t.Fatal("UploadImage enqueue error = nil, want error")
	}
	if imageRepo.created == nil || imageRepo.created.Status != types.ImageStatusFailed || imageRepo.created.ErrorMsg == nil || !strings.Contains(*imageRepo.created.ErrorMsg, "queue down") {
		t.Fatalf("created image after enqueue failure = %+v, want failed with error message", imageRepo.created)
	}
}

// 验证图片列表会把知识库、标签和分页参数下推给仓储，并返回仓储统计总数和标签响应。
func TestListImagesFiltersByKnowledgeBaseAndPaginates(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	kbID := uuid.New()
	now := time.Now()
	repo := newFakeImageRepository(
		&models.Image{ID: uuid.New(), UserID: userID, KBID: &kbID, FileName: "a.png", FileExt: ".png", Status: types.ImageStatusPending, CreatedAt: now, Tags: []models.Tag{{Name: "风景"}}},
		&models.Image{ID: uuid.New(), UserID: userID, KBID: &kbID, FileName: "b.png", FileExt: ".png", Status: types.ImageStatusPending, CreatedAt: now},
	)
	repo.listTotal = 7
	logic := NewListImagesLogic(ctx, &svc.ServiceContext{ImageRepo: repo})
	tag := "风景"

	out, err := logic.ListImages(userID, &request.ListImagesRequest{PageRequest: request.PageRequest{Page: 2, PageSize: 1}, KBID: ptrString(kbID.String()), Tag: &tag})
	if err != nil {
		t.Fatalf("ListImages error = %v", err)
	}
	if !repo.pageListCalled {
		t.Fatal("PageList was not called")
	}
	if repo.pageListQuery.KBID == nil || *repo.pageListQuery.KBID != kbID || repo.pageListQuery.Tag == nil || *repo.pageListQuery.Tag != tag ||
		repo.pageListQuery.Page != 2 || repo.pageListQuery.PageSize != 1 {
		t.Fatalf("page list query = %+v, want kb/tag/page/page_size passed to repository", repo.pageListQuery)
	}
	if out.Total != 7 || out.Page != 2 || out.PageSize != 1 || len(out.List) != 2 {
		t.Fatalf("list response = %+v, want repository rows and total", out)
	}
	if out.List[0].Tags == nil {
		t.Fatal("tags = nil, want empty slice")
	}
	if !reflect.DeepEqual(out.List[0].Tags, []string{"风景"}) {
		t.Fatalf("first tags = %#v, want row tag names", out.List[0].Tags)
	}
}

// 验证详情接口会使用当前用户读取图片，并在响应中返回标签名称与签名 URL。
func TestGetImageUsesUserScopedLookup(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	errMsg := "boom"
	desc := "a cat"
	scene := "indoor"
	row := &models.Image{
		ID: uuid.New(), UserID: userID, FileName: "a.png", FileExt: ".png", FileKey: "u/images/a.png",
		Description: &desc, Scene: &scene, Status: "failed", ErrorMsg: &errMsg,
		Tags: []models.Tag{{Name: "风景"}},
	}
	repo := newFakeImageRepository(row)
	svcCtx := &svc.ServiceContext{ImageRepo: repo, URLSigner: fakeImageURLSigner{}}

	detail, err := NewGetImageLogic(ctx, svcCtx).GetImage(userID, &request.UriImageIDRequest{ImageID: row.ID.String()})
	if err != nil {
		t.Fatalf("GetImage error = %v", err)
	}
	if detail.ID != row.ID || detail.ErrorMsg == nil || *detail.ErrorMsg != errMsg || detail.Description != desc || detail.Scene == nil || *detail.Scene != scene {
		t.Fatalf("detail = %+v, want row response", detail)
	}
	if !reflect.DeepEqual(detail.Tags, []string{"风景"}) {
		t.Fatalf("detail tags = %#v, want row tag names", detail.Tags)
	}
	if detail.Url != "https://cdn.example.com/u/images/a.png" {
		t.Fatalf("detail url = %q, want signed url", detail.Url)
	}
}

// 验证删除图片会先尝试删除存储文件，即使存储删除失败也会删除元数据和 ES chunk。
func TestDeleteImageDeletesStorageBestEffortAndMetadata(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	row := &models.Image{ID: uuid.New(), UserID: userID, FileName: "a.png", FileExt: ".png", FileKey: "images/a.png"}
	repo := newFakeImageRepository(row)
	store := newMemoryImageStore()
	store.deleteErr = xerr.Internal("delete failed", nil)
	chunkRepo := &fakeImageRAGChunkRepository{}

	if err := NewDeleteImageLogic(ctx, &svc.ServiceContext{ImageRepo: repo, Storage: store, RAGChunkRepo: chunkRepo}).DeleteImage(userID, &request.UriImageIDRequest{ImageID: row.ID.String()}); err != nil {
		t.Fatalf("DeleteImage error = %v", err)
	}
	if repo.deletedID != row.ID {
		t.Fatalf("deleted id = %s, want %s", repo.deletedID, row.ID)
	}
	if len(store.deleted) != 1 || store.deleted[0] != row.FileKey {
		t.Fatalf("storage deleted = %v, want file key", store.deleted)
	}
	if chunkRepo.deletedUserID != userID || chunkRepo.deletedDocumentID != row.ID {
		t.Fatalf("chunk delete = %s/%s, want %s/%s", chunkRepo.deletedUserID, chunkRepo.deletedDocumentID, userID, row.ID)
	}
}

// 验证移动图片会校验目标知识库，只更新数据库 kb_id，并同步更新 ES chunk 的 kb_id。
func TestMoveImageValidatesTargetKnowledgeBaseAndUpdatesOnlyKBID(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	oldKBID := uuid.New()
	newKBID := uuid.New()
	row := &models.Image{ID: uuid.New(), UserID: userID, KBID: &oldKBID, FileName: "a.png", FileExt: ".png"}
	imageRepo := newFakeImageRepository(row)
	kbRepo := newFakeImageKnowledgeBaseRepository(&models.KnowledgeBase{ID: newKBID, UserID: userID, Name: "目标库"})
	chunkRepo := &fakeImageRAGChunkRepository{}
	producer := &fakeImageTaskProducer{}

	out, err := NewMoveImageLogic(ctx, &svc.ServiceContext{
		ImageRepo:         imageRepo,
		KnowledgeBaseRepo: kbRepo,
		RAGChunkRepo:      chunkRepo,
		TaskProducer:      producer,
	}).MoveImage(userID, &request.MoveImageRequest{
		UriImageIDRequest: request.UriImageIDRequest{ImageID: row.ID.String()},
		KBID:              newKBID.String(),
	})
	if err != nil {
		t.Fatalf("MoveImage error = %v", err)
	}
	if out.KBID == nil || *out.KBID != newKBID || row.KBID == nil || *row.KBID != newKBID {
		t.Fatalf("kb id = response:%v row:%v, want %s", out.KBID, row.KBID, newKBID)
	}
	if strings.Join(imageRepo.fields, ",") != "kb_id" {
		t.Fatalf("fields = %v, want kb_id only", imageRepo.fields)
	}
	if chunkRepo.updatedUserID != userID || chunkRepo.updatedDocumentID != row.ID || chunkRepo.updatedKBID != newKBID {
		t.Fatalf("updated chunk ownership = %s/%s/%s, want %s/%s/%s", chunkRepo.updatedUserID, chunkRepo.updatedDocumentID, chunkRepo.updatedKBID, userID, row.ID, newKBID)
	}
	if len(producer.tasks) != 0 {
		t.Fatalf("queued tasks = %d, want 0 because move should not reparse", len(producer.tasks))
	}
}

// 验证图片检索会调用 RAG search，并按 user_id 与 source_type=image 过滤。
func TestSearchImagesUsesRAGSearchByUserAndSourceType(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	imageID := uuid.New()
	kbID := uuid.New()
	var searchBodies []map[string]any
	esServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		if r.Method != http.MethodPost || r.URL.Path != "/boxify_chunks/_search" {
			t.Fatalf("unexpected ES request %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode search body: %v", err)
		}
		searchBodies = append(searchBodies, body)
		_, _ = w.Write([]byte(`{"hits":{"hits":[{"_id":"11111111-1111-1111-1111-111111111111","_score":2,"_source":{"chunk_id":"11111111-1111-1111-1111-111111111111","source_id":"` + imageID.String() + `","user_id":"` + userID.String() + `","kb_id":"` + kbID.String() + `","name":"cat.png","source_type":"image","content":"a cat"}}]}}`))
	}))
	defer esServer.Close()
	esClient, err := infraes.NewClient(infraes.Config{URL: esServer.URL})
	if err != nil {
		t.Fatalf("NewClient error = %v", err)
	}
	ragChunkRepo := repositoryes.NewRAGChunkRepository(esClient, "boxify_chunks")
	cipher, err := security.NewSecretCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretCipher error = %v", err)
	}
	encryptedAPIKey, err := cipher.Encrypt("db-key")
	if err != nil {
		t.Fatalf("Encrypt API key error = %v", err)
	}
	svcCtx := &svc.ServiceContext{
		Config:          config.Config{Rag: config.RagConfig{EmbeddingDim: 3, ChunkIndex: "boxify_chunks"}},
		RAGSearcher:     ragsearch.NewSearcher[models.RAGChunkSource](esClient, ragsearch.WithIndex("boxify_chunks"), ragsearch.WithEmbeddingDim(3), ragsearch.WithSourceDecoder[models.RAGChunkSource](ragChunkRepo.DecodeSource)),
		ModelConfigRepo: &fakeSearchModelConfigRepository{rows: []*models.ModelConfig{{UserID: userID, Type: string(types.EmbeddingModelType), Provider: "fake", ModelName: "db-embed", APIKeyEncrypted: encryptedAPIKey, IsDefault: true}}},
		SecretCipher:    cipher,
		LLMManager:      newFakeSearchLLMManager(),
	}
	out, err := NewSearchImagesLogic(ctx, svcCtx).SearchImages(userID, &request.SearchImageRequest{Query: "cat", TopK: 5})
	if err != nil {
		t.Fatalf("SearchImages error = %v", err)
	}
	if len(out.List) != 1 || out.List[0].SourceID != imageID || out.List[0].KBID == nil || *out.List[0].KBID != kbID || out.List[0].ImageName != "cat.png" {
		t.Fatalf("SearchImages list = %#v, want mapped chunk response", out.List)
	}
	if len(searchBodies) < 2 {
		t.Fatalf("ES search calls = %d, want vector and bm25 calls", len(searchBodies))
	}
	encoded, err := json.Marshal(searchBodies)
	if err != nil {
		t.Fatalf("marshal search bodies: %v", err)
	}
	bodyText := string(encoded)
	if !strings.Contains(bodyText, `"user_id":"`+userID.String()+`"`) || !strings.Contains(bodyText, `"source_type":"image"`) {
		t.Fatalf("search filters = %s, want user_id and source_type image", bodyText)
	}
}

// 验证图片检索会拒绝空关键词。
func TestSearchImagesRejectsEmptyQuery(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	if _, err := NewSearchImagesLogic(ctx, &svc.ServiceContext{}).SearchImages(userID, &request.SearchImageRequest{Query: "  "}); err == nil {
		t.Fatal("SearchImages empty query error = nil, want error")
	}
}
