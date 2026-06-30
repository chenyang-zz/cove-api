package document

import (
	"bytes"
	"context"
	"mime/multipart"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

type fakeDocumentRepository struct {
	rows           map[uuid.UUID]*models.Document
	created        *models.Document
	deletedID      uuid.UUID
	partial        *models.Document
	fields         []string
	updateID       uuid.UUID
	pageListQuery  repository.DocumentListQuery
	pageListCalled bool
	listTotal      int64
}

func newFakeDocumentRepository(rows ...*models.Document) *fakeDocumentRepository {
	repo := &fakeDocumentRepository{rows: map[uuid.UUID]*models.Document{}}
	for _, row := range rows {
		repo.rows[row.ID] = row
	}
	return repo
}

func (r *fakeDocumentRepository) Create(ctx context.Context, userID uuid.UUID, row *models.Document) (*models.Document, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	row.UserID = userID
	r.created = row
	r.rows[row.ID] = row
	return row, nil
}

func (r *fakeDocumentRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.Document, error) {
	out, _, err := r.pageRows(userID)
	return out, err
}

func (r *fakeDocumentRepository) PageList(ctx context.Context, userID uuid.UUID, query repository.DocumentListQuery) ([]*models.Document, int64, error) {
	r.pageListCalled = true
	r.pageListQuery = query
	return r.pageRows(userID)
}

func (r *fakeDocumentRepository) CountByKnowledgeBase(ctx context.Context, userID uuid.UUID, kbIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
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

func (r *fakeDocumentRepository) pageRows(userID uuid.UUID) ([]*models.Document, int64, error) {
	out := make([]*models.Document, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	total := r.listTotal
	if total == 0 {
		total = int64(len(out))
	}
	return out, total, nil
}

func (r *fakeDocumentRepository) FindByID(ctx context.Context, userID uuid.UUID, documentID uuid.UUID) (*models.Document, error) {
	row, ok := r.rows[documentID]
	if !ok || row.UserID != userID {
		return nil, xerr.NotFound("文档不存在")
	}
	return row, nil
}

func (r *fakeDocumentRepository) Update(ctx context.Context, userID uuid.UUID, row *models.Document) (*models.Document, error) {
	r.rows[row.ID] = row
	return row, nil
}

func (r *fakeDocumentRepository) UpdateFields(ctx context.Context, userID uuid.UUID, documentID uuid.UUID, row *models.Document, fields *repository.DocumentUpdateFields) (*models.Document, error) {
	r.updateID = documentID
	r.partial = row
	r.fields = fields.Columns()
	existing, err := r.FindByID(ctx, userID, documentID)
	if err != nil {
		return nil, err
	}
	for _, column := range r.fields {
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

func (r *fakeDocumentRepository) Delete(ctx context.Context, userID uuid.UUID, documentID uuid.UUID) error {
	if _, err := r.FindByID(ctx, userID, documentID); err != nil {
		return err
	}
	r.deletedID = documentID
	delete(r.rows, documentID)
	return nil
}

type fakeDocumentKnowledgeBaseRepository struct {
	rows              map[uuid.UUID]*models.KnowledgeBase
	created           *models.KnowledgeBase
	findDefaultCalled bool
}

func newFakeDocumentKnowledgeBaseRepository(rows ...*models.KnowledgeBase) *fakeDocumentKnowledgeBaseRepository {
	repo := &fakeDocumentKnowledgeBaseRepository{rows: map[uuid.UUID]*models.KnowledgeBase{}}
	for _, row := range rows {
		repo.rows[row.ID] = row
	}
	return repo
}

func (r *fakeDocumentKnowledgeBaseRepository) Create(ctx context.Context, userID uuid.UUID, row *models.KnowledgeBase) (*models.KnowledgeBase, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	row.UserID = userID
	r.created = row
	r.rows[row.ID] = row
	return row, nil
}

func (r *fakeDocumentKnowledgeBaseRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.KnowledgeBase, error) {
	out := make([]*models.KnowledgeBase, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeDocumentKnowledgeBaseRepository) FindDefault(ctx context.Context, userID uuid.UUID) (*models.KnowledgeBase, error) {
	r.findDefaultCalled = true
	for _, row := range r.rows {
		if row.UserID == userID && row.IsDefault {
			return row, nil
		}
	}
	return nil, xerr.NotFound("默认知识库不存在")
}

func (r *fakeDocumentKnowledgeBaseRepository) FindByID(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID) (*models.KnowledgeBase, error) {
	row, ok := r.rows[knowledgeBaseID]
	if !ok || row.UserID != userID {
		return nil, xerr.NotFound("知识库不存在")
	}
	return row, nil
}

func (r *fakeDocumentKnowledgeBaseRepository) Update(ctx context.Context, userID uuid.UUID, row *models.KnowledgeBase) (*models.KnowledgeBase, error) {
	r.rows[row.ID] = row
	return row, nil
}

func (r *fakeDocumentKnowledgeBaseRepository) UpdateFields(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID, row *models.KnowledgeBase, fields *repository.KnowledgeBaseUpdateFields) (*models.KnowledgeBase, error) {
	return r.FindByID(ctx, userID, knowledgeBaseID)
}

func (r *fakeDocumentKnowledgeBaseRepository) Delete(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID) error {
	delete(r.rows, knowledgeBaseID)
	return nil
}

type memoryDocumentStore struct {
	data      map[string][]byte
	deleted   []string
	deleteErr error
}

func newMemoryDocumentStore() *memoryDocumentStore {
	return &memoryDocumentStore{data: map[string][]byte{}}
}

func (s *memoryDocumentStore) Ping(ctx context.Context) error {
	return nil
}

func (s *memoryDocumentStore) Put(ctx context.Context, key string, data []byte) error {
	s.data[key] = append([]byte(nil), data...)
	return nil
}

func (s *memoryDocumentStore) Get(ctx context.Context, key string) ([]byte, error) {
	data, ok := s.data[key]
	if !ok {
		return nil, xerr.NotFound("文件不存在")
	}
	return append([]byte(nil), data...), nil
}

func (s *memoryDocumentStore) Delete(ctx context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.data, key)
	return nil
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

func TestUploadDocumentStoresFileAndCreatesDefaultKnowledgeBase(t *testing.T) {
	// 验证上传文档会写入存储、创建默认知识库，并保存 pending 状态的文档元数据。
	ctx := context.Background()
	userID := uuid.New()
	docRepo := newFakeDocumentRepository()
	kbRepo := newFakeDocumentKnowledgeBaseRepository()
	store := newMemoryDocumentStore()
	logic := NewUploadDocumentLogic(ctx, &svc.ServiceContext{DocumentRepo: docRepo, KnowledgeBaseRepo: kbRepo, Storage: store})

	out, err := logic.UploadDocument(userID, &request.UploadDocumentRequest{
		File: testFileHeader(t, " guide.md ", []byte("hello")),
	})
	if err != nil {
		t.Fatalf("UploadDocument error = %v", err)
	}
	if kbRepo.created == nil || !kbRepo.created.IsDefault || kbRepo.created.Name != "默认知识库" {
		t.Fatalf("default kb = %+v, want created default knowledge base", kbRepo.created)
	}
	if !kbRepo.findDefaultCalled {
		t.Fatal("FindDefault was not called before creating default knowledge base")
	}
	if docRepo.created == nil {
		t.Fatal("document was not created")
	}
	if docRepo.created.UserID != userID || docRepo.created.KBID == nil || *docRepo.created.KBID != kbRepo.created.ID {
		t.Fatalf("created document owner/kb = %+v, want current user default kb", docRepo.created)
	}
	if docRepo.created.FileName != "guide.md" || docRepo.created.FileExt != ".md" || docRepo.created.FileSize != 5 || docRepo.created.Status != documentStatusPending {
		t.Fatalf("created document = %+v, want normalized file metadata", docRepo.created)
	}
	if string(store.data[docRepo.created.FileKey]) != "hello" {
		t.Fatalf("stored content = %q, want hello", string(store.data[docRepo.created.FileKey]))
	}
	if out.ID != docRepo.created.ID || out.FileName != "guide.md" || out.KBID == nil || *out.KBID != kbRepo.created.ID || out.Status != documentStatusPending {
		t.Fatalf("response = %+v, want created document response", out)
	}
}

func TestUploadDocumentUsesExistingDefaultKnowledgeBase(t *testing.T) {
	// 验证上传文档未指定知识库时会优先复用已有默认知识库，不重复创建默认库。
	ctx := context.Background()
	userID := uuid.New()
	defaultKB := &models.KnowledgeBase{ID: uuid.New(), UserID: userID, Name: "默认知识库", IsDefault: true}
	docRepo := newFakeDocumentRepository()
	kbRepo := newFakeDocumentKnowledgeBaseRepository(defaultKB)
	logic := NewUploadDocumentLogic(ctx, &svc.ServiceContext{DocumentRepo: docRepo, KnowledgeBaseRepo: kbRepo, Storage: newMemoryDocumentStore()})

	out, err := logic.UploadDocument(userID, &request.UploadDocumentRequest{File: testFileHeader(t, "doc.txt", []byte("hello"))})
	if err != nil {
		t.Fatalf("UploadDocument error = %v", err)
	}
	if kbRepo.created != nil {
		t.Fatalf("created default = %+v, want reuse existing default", kbRepo.created)
	}
	if out.KBID == nil || *out.KBID != defaultKB.ID {
		t.Fatalf("response kb id = %v, want existing default %s", out.KBID, defaultKB.ID)
	}
}

func TestUploadDocumentRejectsUnsupportedAndOversizedFiles(t *testing.T) {
	// 验证上传会拒绝不支持的扩展名和超过 50MB 的文件。
	ctx := context.Background()
	userID := uuid.New()
	logic := NewUploadDocumentLogic(ctx, &svc.ServiceContext{DocumentRepo: newFakeDocumentRepository(), KnowledgeBaseRepo: newFakeDocumentKnowledgeBaseRepository(), Storage: newMemoryDocumentStore()})

	if _, err := logic.UploadDocument(userID, &request.UploadDocumentRequest{File: testFileHeader(t, "bad.exe", []byte("x"))}); err == nil {
		t.Fatal("UploadDocument unsupported ext error = nil, want error")
	}
	large := testFileHeader(t, "large.txt", []byte("x"))
	large.Size = maxDocumentFileSize + 1
	if _, err := logic.UploadDocument(userID, &request.UploadDocumentRequest{File: large}); err == nil {
		t.Fatal("UploadDocument oversized error = nil, want error")
	}
}

func TestUploadDocumentValidatesExplicitKnowledgeBase(t *testing.T) {
	// 验证上传指定知识库时会校验知识库归属，并把文档归入指定知识库。
	ctx := context.Background()
	userID := uuid.New()
	kbID := uuid.New()
	docRepo := newFakeDocumentRepository()
	kbRepo := newFakeDocumentKnowledgeBaseRepository(&models.KnowledgeBase{ID: kbID, UserID: userID, Name: "项目库"})
	logic := NewUploadDocumentLogic(ctx, &svc.ServiceContext{DocumentRepo: docRepo, KnowledgeBaseRepo: kbRepo, Storage: newMemoryDocumentStore()})

	out, err := logic.UploadDocument(userID, &request.UploadDocumentRequest{
		File: testFileHeader(t, "doc.txt", []byte("hello")),
		KBID: ptrString(kbID.String()),
	})
	if err != nil {
		t.Fatalf("UploadDocument error = %v", err)
	}
	if out.KBID == nil || *out.KBID != kbID || docRepo.created.KBID == nil || *docRepo.created.KBID != kbID {
		t.Fatalf("kb id = response:%v row:%v, want %s", out.KBID, docRepo.created.KBID, kbID)
	}
	if _, err := logic.UploadDocument(userID, &request.UploadDocumentRequest{File: testFileHeader(t, "doc.txt", []byte("hello")), KBID: ptrString(uuid.NewString())}); err == nil {
		t.Fatal("UploadDocument missing kb error = nil, want error")
	}
}

func TestListDocumentsFiltersByKnowledgeBaseAndPaginates(t *testing.T) {
	// 验证文档列表会把知识库、标签和分页参数下推给仓储，并返回仓储统计总数和标签响应。
	ctx := context.Background()
	userID := uuid.New()
	kbID := uuid.New()
	now := time.Now()
	repo := newFakeDocumentRepository(
		&models.Document{ID: uuid.New(), UserID: userID, KBID: &kbID, FileName: "a.txt", FileExt: ".txt", Status: documentStatusPending, CreatedAt: now, Tags: []models.Tag{{Name: "重要"}}},
		&models.Document{ID: uuid.New(), UserID: userID, KBID: &kbID, FileName: "b.txt", FileExt: ".txt", Status: documentStatusPending, CreatedAt: now},
	)
	repo.listTotal = 7
	logic := NewListDocumentsLogic(ctx, &svc.ServiceContext{DocumentRepo: repo})
	tag := "重要"

	out, err := logic.ListDocuments(userID, &request.ListDocumentsRequest{PageRequest: request.PageRequest{Page: 2, PageSize: 1}, KBID: ptrString(kbID.String()), Tag: &tag})
	if err != nil {
		t.Fatalf("ListDocuments error = %v", err)
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
	if !reflect.DeepEqual(out.List[0].Tags, []string{"重要"}) {
		t.Fatalf("first tags = %#v, want row tag names", out.List[0].Tags)
	}
}

func TestGetDocumentAndStatusUseUserScopedLookup(t *testing.T) {
	// 验证详情和状态接口都会使用当前用户读取文档，并在详情响应中返回标签名称。
	ctx := context.Background()
	userID := uuid.New()
	errMsg := "boom"
	row := &models.Document{ID: uuid.New(), UserID: userID, FileName: "a.txt", FileExt: ".txt", Status: "failed", Progress: 0.5, ErrorMsg: &errMsg, Tags: []models.Tag{{Name: "重要"}}}
	repo := newFakeDocumentRepository(row)
	svcCtx := &svc.ServiceContext{DocumentRepo: repo}

	detail, err := NewGetDocumentLogic(ctx, svcCtx).GetDocument(userID, &request.UriDocumentIDRequest{DocumentID: row.ID.String()})
	if err != nil {
		t.Fatalf("GetDocument error = %v", err)
	}
	if detail.ID != row.ID || detail.ErrorMsg == nil || *detail.ErrorMsg != errMsg {
		t.Fatalf("detail = %+v, want row response", detail)
	}
	if !reflect.DeepEqual(detail.Tags, []string{"重要"}) {
		t.Fatalf("detail tags = %#v, want row tag names", detail.Tags)
	}
	status, err := NewGetDocumentStatusLogic(ctx, svcCtx).GetDocumentStatus(userID, &request.UriDocumentIDRequest{DocumentID: row.ID.String()})
	if err != nil {
		t.Fatalf("GetDocumentStatus error = %v", err)
	}
	if status.Status != "failed" || status.Progress != 0.5 || status.ErrorMsg == nil || *status.ErrorMsg != errMsg {
		t.Fatalf("status = %+v, want row status", status)
	}
}

func TestReParseDocumentResetsStatusAndProgress(t *testing.T) {
	// 验证重新解析会只更新状态、进度和错误信息，并返回更新后的文档。
	ctx := context.Background()
	userID := uuid.New()
	errMsg := "boom"
	row := &models.Document{ID: uuid.New(), UserID: userID, FileName: "a.txt", FileExt: ".txt", Status: "failed", Progress: 1, ErrorMsg: &errMsg}
	repo := newFakeDocumentRepository(row)

	out, err := NewReParseDocumentLogic(ctx, &svc.ServiceContext{DocumentRepo: repo}).ReParseDocument(userID, &request.UriDocumentIDRequest{DocumentID: row.ID.String()})
	if err != nil {
		t.Fatalf("ReParseDocument error = %v", err)
	}
	if out.Status != documentStatusPending || out.Progress != 0 || out.ErrorMsg != nil {
		t.Fatalf("response = %+v, want reset parse state", out)
	}
	if strings.Join(repo.fields, ",") != "status,progress,error_msg" {
		t.Fatalf("fields = %v, want status/progress/error_msg", repo.fields)
	}
}

func TestDeleteDocumentDeletesStorageBestEffortAndMetadata(t *testing.T) {
	// 验证删除文档会先尝试删除存储文件，即使存储删除失败也会删除元数据。
	ctx := context.Background()
	userID := uuid.New()
	row := &models.Document{ID: uuid.New(), UserID: userID, FileName: "a.txt", FileExt: ".txt", FileKey: "docs/a.txt"}
	repo := newFakeDocumentRepository(row)
	store := newMemoryDocumentStore()
	store.deleteErr = xerr.Internal("delete failed", nil)

	if err := NewDeleteDocumentLogic(ctx, &svc.ServiceContext{DocumentRepo: repo, Storage: store}).DeleteDocument(userID, &request.UriDocumentIDRequest{DocumentID: row.ID.String()}); err != nil {
		t.Fatalf("DeleteDocument error = %v", err)
	}
	if repo.deletedID != row.ID {
		t.Fatalf("deleted id = %s, want %s", repo.deletedID, row.ID)
	}
	if len(store.deleted) != 1 || store.deleted[0] != row.FileKey {
		t.Fatalf("storage deleted = %v, want file key", store.deleted)
	}
}

func TestMoveDocumentValidatesTargetKnowledgeBaseAndUpdatesOnlyKBID(t *testing.T) {
	// 验证移动文档会校验目标知识库，并且只更新 kb_id 字段。
	ctx := context.Background()
	userID := uuid.New()
	oldKBID := uuid.New()
	newKBID := uuid.New()
	row := &models.Document{ID: uuid.New(), UserID: userID, KBID: &oldKBID, FileName: "a.txt", FileExt: ".txt"}
	docRepo := newFakeDocumentRepository(row)
	kbRepo := newFakeDocumentKnowledgeBaseRepository(&models.KnowledgeBase{ID: newKBID, UserID: userID, Name: "目标库"})

	out, err := NewMoveDocumentLogic(ctx, &svc.ServiceContext{DocumentRepo: docRepo, KnowledgeBaseRepo: kbRepo}).MoveDocument(userID, &request.MoveDocumentRequest{
		UriDocumentIDRequest: request.UriDocumentIDRequest{DocumentID: row.ID.String()},
		KBID:                 newKBID.String(),
	})
	if err != nil {
		t.Fatalf("MoveDocument error = %v", err)
	}
	if out.KBID == nil || *out.KBID != newKBID || row.KBID == nil || *row.KBID != newKBID {
		t.Fatalf("kb id = response:%v row:%v, want %s", out.KBID, row.KBID, newKBID)
	}
	if strings.Join(docRepo.fields, ",") != "kb_id" {
		t.Fatalf("fields = %v, want kb_id only", docRepo.fields)
	}
}

func TestPreviewDocumentContentReadsTextAndTruncates(t *testing.T) {
	// 验证文本类文档预览会从存储读取原文，并在超过上限时截断。
	ctx := context.Background()
	userID := uuid.New()
	content := strings.Repeat("a", previewMaxChars+5)
	row := &models.Document{ID: uuid.New(), UserID: userID, FileName: "a.md", FileExt: ".md", FileKey: "docs/a.md"}
	store := newMemoryDocumentStore()
	store.data[row.FileKey] = []byte(content)

	out, err := NewPreviewDocumentContentLogic(ctx, &svc.ServiceContext{DocumentRepo: newFakeDocumentRepository(row), Storage: store}).PreviewDocumentContent(userID, &request.UriDocumentIDRequest{DocumentID: row.ID.String()})
	if err != nil {
		t.Fatalf("PreviewDocumentContent error = %v", err)
	}
	if !out.IsMarkdown || !out.Truncated || len(out.Content) != previewMaxChars {
		t.Fatalf("preview = %+v, want markdown truncated to max chars", out)
	}
}

func TestPreviewDocumentContentRejectsBinaryParserMissing(t *testing.T) {
	// 验证 PDF/DOCX 预览在 Go 解析器未接入前会返回明确错误。
	ctx := context.Background()
	userID := uuid.New()
	row := &models.Document{ID: uuid.New(), UserID: userID, FileName: "a.pdf", FileExt: ".pdf", FileKey: "docs/a.pdf"}

	if _, err := NewPreviewDocumentContentLogic(ctx, &svc.ServiceContext{DocumentRepo: newFakeDocumentRepository(row), Storage: newMemoryDocumentStore()}).PreviewDocumentContent(userID, &request.UriDocumentIDRequest{DocumentID: row.ID.String()}); err == nil {
		t.Fatal("PreviewDocumentContent pdf error = nil, want error")
	}
}

func TestImportURLAndSearchAreExplicitNoOps(t *testing.T) {
	// 验证 URL 导入暂未实现，检索接口在未接入检索逻辑前返回空列表。
	ctx := context.Background()
	userID := uuid.New()

	if _, err := NewImportDocumentFromUrlLogic(ctx, &svc.ServiceContext{}).ImportDocumentFromUrl(userID, &request.URLImportRequest{Url: "https://example.com"}); err == nil {
		t.Fatal("ImportDocumentFromUrl error = nil, want not implemented error")
	}
	out, err := NewSearchDocumentsLogic(ctx, &svc.ServiceContext{}).SearchDocuments(userID, &request.SearchDocumentsRequest{Query: "hello", TopK: 5})
	if err != nil {
		t.Fatalf("SearchDocuments error = %v", err)
	}
	if len(out.List) != 0 {
		t.Fatalf("search list len = %d, want 0", len(out.List))
	}
}

func ptrString(v string) *string {
	return &v
}
