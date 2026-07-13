package tag

import (
	"context"
	"errors"
	"slices"
	"testing"

	ragchunker "github.com/boxify/api-go/internal/core/rag/chunker"
	"github.com/boxify/api-go/internal/domain/types"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

type fakeTagRepository struct {
	rows        []*models.Tag
	docCounts   map[uuid.UUID]int64
	imageCounts map[uuid.UUID]int64

	pageQuery repository.TagListQuery

	updateID      uuid.UUID
	updateColumns []string
	updateRow     *models.Tag

	deletedID uuid.UUID

	mergeSourceID uuid.UUID
	mergeTargetID uuid.UUID
	mergeRow      *models.Tag

	documentIDsByTag map[uuid.UUID][]uuid.UUID
	documentTagNames map[uuid.UUID][]string
}

func (r *fakeTagRepository) Create(ctx context.Context, userID uuid.UUID, row *models.Tag) (*models.Tag, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	row.UserID = userID
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *fakeTagRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.Tag, error) {
	return r.ListByScope(ctx, userID, string(types.TagScopeAll))
}

func (r *fakeTagRepository) ListByScope(ctx context.Context, userID uuid.UUID, scope string) ([]*models.Tag, error) {
	out := make([]*models.Tag, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeTagRepository) PageList(ctx context.Context, userID uuid.UUID, query repository.TagListQuery) ([]*models.Tag, int64, error) {
	r.pageQuery = query
	rows, err := r.ListByScope(ctx, userID, query.Scope)
	if err != nil {
		return nil, 0, err
	}
	limit, offset := query.LimitOffset(20)
	start := offset
	if start >= len(rows) {
		return []*models.Tag{}, int64(len(rows)), nil
	}
	end := start + limit
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end], int64(len(rows)), nil
}

func (r *fakeTagRepository) CountDocumentsByTags(ctx context.Context, userID uuid.UUID, tagIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	out := map[uuid.UUID]int64{}
	for _, id := range tagIDs {
		out[id] = r.docCounts[id]
	}
	return out, nil
}

func (r *fakeTagRepository) CountImagesByTags(ctx context.Context, userID uuid.UUID, tagIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	out := map[uuid.UUID]int64{}
	for _, id := range tagIDs {
		out[id] = r.imageCounts[id]
	}
	return out, nil
}

func (r *fakeTagRepository) FindByID(ctx context.Context, userID uuid.UUID, tagID uuid.UUID) (*models.Tag, error) {
	for _, row := range r.rows {
		if row.ID == tagID && row.UserID == userID {
			return row, nil
		}
	}
	return nil, xerr.NotFound("标签不存在")
}

func (r *fakeTagRepository) Update(ctx context.Context, userID uuid.UUID, row *models.Tag) (*models.Tag, error) {
	return r.UpdateFields(ctx, userID, row.ID, row, repository.NewTagUpdateFields().Name().Color())
}

func (r *fakeTagRepository) UpdateFields(ctx context.Context, userID uuid.UUID, tagID uuid.UUID, row *models.Tag, fields *repository.TagUpdateFields) (*models.Tag, error) {
	existing, err := r.FindByID(ctx, userID, tagID)
	if err != nil {
		return nil, err
	}
	r.updateID = tagID
	r.updateColumns = fields.Columns()
	r.updateRow = row
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

func (r *fakeTagRepository) SyncImageTags(ctx context.Context, userID uuid.UUID, imageID uuid.UUID, names []string) ([]models.Tag, error) {
	return nil, nil
}

func (r *fakeTagRepository) SyncDocumentTags(ctx context.Context, userID uuid.UUID, documentID uuid.UUID, names []string) ([]models.Tag, error) {
	return nil, errors.New("not used")
}

func (r *fakeTagRepository) ListDocumentIDsByTag(ctx context.Context, userID uuid.UUID, tagID uuid.UUID) ([]uuid.UUID, error) {
	out := append([]uuid.UUID(nil), r.documentIDsByTag[tagID]...)
	return out, nil
}

func (r *fakeTagRepository) ListDocumentTagNames(ctx context.Context, userID uuid.UUID, documentIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
	out := make(map[uuid.UUID][]string, len(documentIDs))
	for _, id := range documentIDs {
		out[id] = append([]string(nil), r.documentTagNames[id]...)
	}
	return out, nil
}

func (r *fakeTagRepository) Merge(ctx context.Context, userID uuid.UUID, sourceID uuid.UUID, targetID uuid.UUID) (*models.Tag, error) {
	r.mergeSourceID = sourceID
	r.mergeTargetID = targetID
	if r.mergeRow != nil {
		return r.mergeRow, nil
	}
	return r.FindByID(ctx, userID, targetID)
}

func (r *fakeTagRepository) Delete(ctx context.Context, userID uuid.UUID, tagID uuid.UUID) error {
	r.deletedID = tagID
	for i, row := range r.rows {
		if row.ID == tagID && row.UserID == userID {
			r.rows = append(r.rows[:i], r.rows[i+1:]...)
			return nil
		}
	}
	return xerr.NotFound("标签不存在")
}

type fakeTagRAGChunkRepository struct {
	updates []fakeTagChunkUpdate
	err     error
}

type fakeTagChunkUpdate struct {
	userID     uuid.UUID
	documentID uuid.UUID
	tags       []string
}

func (r *fakeTagRAGChunkRepository) EnsureIndex(ctx context.Context, embeddingDim int) error {
	return nil
}

func (r *fakeTagRAGChunkRepository) IndexDocumentChunks(ctx context.Context, document *models.Document, chunks []*ragchunker.Chunk, vectors [][]float64) error {
	return nil
}

func (r *fakeTagRAGChunkRepository) IndexImageChunk(ctx context.Context, image *models.Image, content string, vector []float64) error {
	return nil
}

func (r *fakeTagRAGChunkRepository) DeleteByDocument(ctx context.Context, userID uuid.UUID, documentID uuid.UUID) error {
	return nil
}

func (r *fakeTagRAGChunkRepository) UpdateKnowledgeBase(ctx context.Context, userID uuid.UUID, documentID uuid.UUID, kbID uuid.UUID) error {
	return nil
}

func (r *fakeTagRAGChunkRepository) UpdateTags(ctx context.Context, userID uuid.UUID, documentID uuid.UUID, tags []string) error {
	r.updates = append(r.updates, fakeTagChunkUpdate{
		userID:     userID,
		documentID: documentID,
		tags:       append([]string(nil), tags...),
	})
	return r.err
}

func (r *fakeTagRAGChunkRepository) DecodeSource(src map[string]any) (models.RAGChunkSource, error) {
	return models.RAGChunkSource{}, nil
}

func newTagTestSvc(repo *fakeTagRepository) *svc.ServiceContext {
	return &svc.ServiceContext{TagRepo: repo}
}

func newTagTestSvcWithChunks(repo *fakeTagRepository, chunkRepo *fakeTagRAGChunkRepository) *svc.ServiceContext {
	return &svc.ServiceContext{TagRepo: repo, RAGChunkRepo: chunkRepo}
}

func ptrString(v string) *string {
	return &v
}

func TestListTagsReturnsPagedCountsByScope(t *testing.T) {
	// 验证标签列表会按 scope 和分页参数查询，并把文档和图片计数映射到分页响应。
	userID := uuid.New()
	tagID := uuid.New()
	repo := &fakeTagRepository{
		rows:        []*models.Tag{{ID: tagID, UserID: userID, Name: "设计", Color: "#155EEF"}},
		docCounts:   map[uuid.UUID]int64{tagID: 3},
		imageCounts: map[uuid.UUID]int64{tagID: 2},
	}

	out, err := NewListTagsLogic(context.Background(), newTagTestSvc(repo)).ListTags(userID, &request.ListTagsRequest{
		PageRequest: request.PageRequest{Page: 2, PageSize: 10},
		Scope:       ptrString("image"),
	})
	if err != nil {
		t.Fatalf("ListTags error = %v", err)
	}
	if repo.pageQuery.Scope != string(types.TagScopeImage) || repo.pageQuery.Page != 2 || repo.pageQuery.PageSize != 10 {
		t.Fatalf("ListTags query = %+v, want scope=image page=2 page_size=10", repo.pageQuery)
	}
	if out.Total != 1 || out.Page != 2 || out.PageSize != 10 {
		t.Fatalf("ListTags page meta = total:%d page:%d page_size:%d, want 1/2/10", out.Total, out.Page, out.PageSize)
	}
	if len(out.List) != 0 {
		t.Fatalf("ListTags response list = %+v, want empty second page", out.List)
	}
}

func TestListTagsDefaultsPagination(t *testing.T) {
	// 验证标签列表在 logic 直接调用且分页参数为空时使用默认分页值。
	userID := uuid.New()
	tagID := uuid.New()
	repo := &fakeTagRepository{
		rows:        []*models.Tag{{ID: tagID, UserID: userID, Name: "设计", Color: "#155EEF"}},
		docCounts:   map[uuid.UUID]int64{tagID: 3},
		imageCounts: map[uuid.UUID]int64{tagID: 2},
	}

	out, err := NewListTagsLogic(context.Background(), newTagTestSvc(repo)).ListTags(userID, &request.ListTagsRequest{})
	if err != nil {
		t.Fatalf("ListTags error = %v", err)
	}
	if repo.pageQuery.Scope != string(types.TagScopeAll) || repo.pageQuery.Page != 1 || repo.pageQuery.PageSize != 20 {
		t.Fatalf("ListTags default query = %+v, want scope=all page=1 page_size=20", repo.pageQuery)
	}
	if out.Total != 1 || out.Page != 1 || out.PageSize != 20 {
		t.Fatalf("ListTags default meta = total:%d page:%d page_size:%d, want 1/1/20", out.Total, out.Page, out.PageSize)
	}
	if len(out.List) != 1 || out.List[0].DocCount != 3 || out.List[0].ImageCount != 2 {
		t.Fatalf("ListTags response = %+v, want first tag with counts", out.List)
	}
}

func TestUpdateTagTrimsAndUpdatesOnlyProvidedFields(t *testing.T) {
	// 验证更新标签只提交传入字段，并会 trim 名称与颜色后返回带计数的响应。
	userID := uuid.New()
	tagID := uuid.New()
	repo := &fakeTagRepository{
		rows:        []*models.Tag{{ID: tagID, UserID: userID, Name: "旧", Color: "#000000"}},
		docCounts:   map[uuid.UUID]int64{tagID: 4},
		imageCounts: map[uuid.UUID]int64{tagID: 1},
	}

	out, err := NewUpdateTagLogic(context.Background(), newTagTestSvc(repo)).UpdateTag(userID, &request.TagUpdateRequest{
		UriTagServerIDRequest: request.UriTagServerIDRequest{ID: tagID.String()},
		Name:                  ptrString(" 新标签 "),
	})
	if err != nil {
		t.Fatalf("UpdateTag error = %v", err)
	}
	if repo.updateID != tagID || !slices.Equal(repo.updateColumns, []string{"name"}) || repo.updateRow.Name != "新标签" {
		t.Fatalf("update patch id=%s columns=%v row=%+v, want name-only trimmed patch", repo.updateID, repo.updateColumns, repo.updateRow)
	}
	if out.Name != "新标签" || out.DocCount != 4 || out.ImageCount != 1 {
		t.Fatalf("UpdateTag response = %+v, want updated name and counts", out)
	}
}

func TestUpdateTagNameSyncsDocumentChunkTags(t *testing.T) {
	// 验证标签名称更新成功后，会按 PG 当前标签集合刷新受影响文档的 ES tags。
	userID := uuid.New()
	tagID := uuid.New()
	docID := uuid.New()
	repo := &fakeTagRepository{
		rows:             []*models.Tag{{ID: tagID, UserID: userID, Name: "旧", Color: "#000000"}},
		docCounts:        map[uuid.UUID]int64{tagID: 1},
		imageCounts:      map[uuid.UUID]int64{},
		documentIDsByTag: map[uuid.UUID][]uuid.UUID{tagID: []uuid.UUID{docID}},
		documentTagNames: map[uuid.UUID][]string{docID: []string{"新标签", "保留标签"}},
	}
	chunkRepo := &fakeTagRAGChunkRepository{}

	_, err := NewUpdateTagLogic(context.Background(), newTagTestSvcWithChunks(repo, chunkRepo)).UpdateTag(userID, &request.TagUpdateRequest{
		UriTagServerIDRequest: request.UriTagServerIDRequest{ID: tagID.String()},
		Name:                  ptrString(" 新标签 "),
	})
	if err != nil {
		t.Fatalf("UpdateTag error = %v", err)
	}
	if len(chunkRepo.updates) != 1 {
		t.Fatalf("UpdateTags call count = %d, want 1", len(chunkRepo.updates))
	}
	update := chunkRepo.updates[0]
	if update.userID != userID || update.documentID != docID || !slices.Equal(update.tags, []string{"新标签", "保留标签"}) {
		t.Fatalf("UpdateTags call = %+v, want user/doc/current tags", update)
	}
}

func TestUpdateTagColorDoesNotSyncDocumentChunkTags(t *testing.T) {
	// 验证只更新标签颜色不会刷新 ES tags，因为颜色不参与文档检索过滤。
	userID := uuid.New()
	tagID := uuid.New()
	repo := &fakeTagRepository{
		rows:        []*models.Tag{{ID: tagID, UserID: userID, Name: "标签", Color: "#000000"}},
		docCounts:   map[uuid.UUID]int64{tagID: 1},
		imageCounts: map[uuid.UUID]int64{},
	}
	chunkRepo := &fakeTagRAGChunkRepository{}

	_, err := NewUpdateTagLogic(context.Background(), newTagTestSvcWithChunks(repo, chunkRepo)).UpdateTag(userID, &request.TagUpdateRequest{
		UriTagServerIDRequest: request.UriTagServerIDRequest{ID: tagID.String()},
		Color:                 ptrString("#155EEF"),
	})
	if err != nil {
		t.Fatalf("UpdateTag color error = %v", err)
	}
	if len(chunkRepo.updates) != 0 {
		t.Fatalf("UpdateTags call count = %d, want 0", len(chunkRepo.updates))
	}
}

func TestUpdateTagIgnoresDocumentChunkTagSyncFailure(t *testing.T) {
	// 验证 ES tags 同步失败不会阻断 PG 标签更新响应，只作为派生索引失败记录。
	userID := uuid.New()
	tagID := uuid.New()
	docID := uuid.New()
	repo := &fakeTagRepository{
		rows:             []*models.Tag{{ID: tagID, UserID: userID, Name: "旧", Color: "#000000"}},
		docCounts:        map[uuid.UUID]int64{tagID: 1},
		imageCounts:      map[uuid.UUID]int64{},
		documentIDsByTag: map[uuid.UUID][]uuid.UUID{tagID: []uuid.UUID{docID}},
		documentTagNames: map[uuid.UUID][]string{docID: []string{"新标签"}},
	}
	chunkRepo := &fakeTagRAGChunkRepository{err: errors.New("es unavailable")}

	out, err := NewUpdateTagLogic(context.Background(), newTagTestSvcWithChunks(repo, chunkRepo)).UpdateTag(userID, &request.TagUpdateRequest{
		UriTagServerIDRequest: request.UriTagServerIDRequest{ID: tagID.String()},
		Name:                  ptrString("新标签"),
	})
	if err != nil {
		t.Fatalf("UpdateTag sync failure error = %v, want nil", err)
	}
	if out.Name != "新标签" {
		t.Fatalf("UpdateTag response name = %q, want 新标签", out.Name)
	}
	if len(chunkRepo.updates) != 1 {
		t.Fatalf("UpdateTags call count = %d, want 1", len(chunkRepo.updates))
	}
}

func TestUpdateTagRejectsEmptyPatch(t *testing.T) {
	// 验证没有任何更新字段时返回 bad request，避免空 patch 落到仓储层。
	userID := uuid.New()
	tagID := uuid.New()
	repo := &fakeTagRepository{}

	_, err := NewUpdateTagLogic(context.Background(), newTagTestSvc(repo)).UpdateTag(userID, &request.TagUpdateRequest{
		UriTagServerIDRequest: request.UriTagServerIDRequest{ID: tagID.String()},
	})
	if xerr.From(err).Kind != xerr.KindBadRequest {
		t.Fatalf("UpdateTag empty patch error = %v, want bad request", err)
	}
}

func TestDeleteTagDeletesParsedID(t *testing.T) {
	// 验证删除标签会解析 URI ID，并只删除当前用户标签。
	userID := uuid.New()
	tagID := uuid.New()
	repo := &fakeTagRepository{rows: []*models.Tag{{ID: tagID, UserID: userID, Name: "待删", Color: "#155EEF"}}}

	if err := NewDeleteTagLogic(context.Background(), newTagTestSvc(repo)).DeleteTag(userID, &request.UriTagServerIDRequest{ID: tagID.String()}); err != nil {
		t.Fatalf("DeleteTag error = %v", err)
	}
	if repo.deletedID != tagID {
		t.Fatalf("deleted id = %s, want %s", repo.deletedID, tagID)
	}
}

func TestDeleteTagSyncsRemainingDocumentChunkTags(t *testing.T) {
	// 验证删除标签后，会用删除后的剩余标签集合刷新受影响文档的 ES tags。
	userID := uuid.New()
	tagID := uuid.New()
	docID := uuid.New()
	repo := &fakeTagRepository{
		rows:             []*models.Tag{{ID: tagID, UserID: userID, Name: "待删", Color: "#155EEF"}},
		documentIDsByTag: map[uuid.UUID][]uuid.UUID{tagID: []uuid.UUID{docID}},
		documentTagNames: map[uuid.UUID][]string{docID: []string{"保留标签"}},
	}
	chunkRepo := &fakeTagRAGChunkRepository{}

	if err := NewDeleteTagLogic(context.Background(), newTagTestSvcWithChunks(repo, chunkRepo)).DeleteTag(userID, &request.UriTagServerIDRequest{ID: tagID.String()}); err != nil {
		t.Fatalf("DeleteTag error = %v", err)
	}
	if len(chunkRepo.updates) != 1 || chunkRepo.updates[0].documentID != docID || !slices.Equal(chunkRepo.updates[0].tags, []string{"保留标签"}) {
		t.Fatalf("UpdateTags calls = %+v, want remaining tags for deleted tag document", chunkRepo.updates)
	}
}

func TestMergeTagRejectsSameIDAndReturnsTargetCounts(t *testing.T) {
	// 验证合并标签拒绝相同 ID，成功后返回目标标签及最新文档和图片计数。
	userID := uuid.New()
	sourceID := uuid.New()
	targetID := uuid.New()
	repo := &fakeTagRepository{
		rows:        []*models.Tag{{ID: sourceID, UserID: userID, Name: "源", Color: "#111111"}, {ID: targetID, UserID: userID, Name: "目标", Color: "#222222"}},
		docCounts:   map[uuid.UUID]int64{targetID: 7},
		imageCounts: map[uuid.UUID]int64{targetID: 5},
	}
	logic := NewMergeTagLogic(context.Background(), newTagTestSvc(repo))

	_, err := logic.MergeTag(userID, &request.TagMergeRequest{SourceID: sourceID.String(), TargetID: sourceID.String()})
	if xerr.From(err).Kind != xerr.KindBadRequest {
		t.Fatalf("MergeTag same id error = %v, want bad request", err)
	}

	out, err := logic.MergeTag(userID, &request.TagMergeRequest{SourceID: sourceID.String(), TargetID: targetID.String()})
	if err != nil {
		t.Fatalf("MergeTag error = %v", err)
	}
	if repo.mergeSourceID != sourceID || repo.mergeTargetID != targetID {
		t.Fatalf("merge ids source=%s target=%s, want source=%s target=%s", repo.mergeSourceID, repo.mergeTargetID, sourceID, targetID)
	}
	if out.ID != targetID || out.DocCount != 7 || out.ImageCount != 5 {
		t.Fatalf("MergeTag response = %+v, want target with counts", out)
	}
}

func TestMergeTagSyncsTargetDocumentChunkTags(t *testing.T) {
	// 验证合并标签后，会刷新源标签影响文档的 ES tags 为合并后的当前标签集合。
	userID := uuid.New()
	sourceID := uuid.New()
	targetID := uuid.New()
	docID := uuid.New()
	repo := &fakeTagRepository{
		rows:             []*models.Tag{{ID: sourceID, UserID: userID, Name: "源", Color: "#111111"}, {ID: targetID, UserID: userID, Name: "目标", Color: "#222222"}},
		docCounts:        map[uuid.UUID]int64{targetID: 1},
		imageCounts:      map[uuid.UUID]int64{targetID: 0},
		documentIDsByTag: map[uuid.UUID][]uuid.UUID{sourceID: []uuid.UUID{docID}},
		documentTagNames: map[uuid.UUID][]string{docID: []string{"目标", "保留标签"}},
	}
	chunkRepo := &fakeTagRAGChunkRepository{}

	_, err := NewMergeTagLogic(context.Background(), newTagTestSvcWithChunks(repo, chunkRepo)).MergeTag(userID, &request.TagMergeRequest{
		SourceID: sourceID.String(),
		TargetID: targetID.String(),
	})
	if err != nil {
		t.Fatalf("MergeTag error = %v", err)
	}
	if len(chunkRepo.updates) != 1 || chunkRepo.updates[0].documentID != docID || !slices.Equal(chunkRepo.updates[0].tags, []string{"目标", "保留标签"}) {
		t.Fatalf("UpdateTags calls = %+v, want merged document tags", chunkRepo.updates)
	}
}
