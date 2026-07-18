package postgres_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/boxify/api-go/internal/domain/types"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	repositorypostgres "github.com/boxify/api-go/internal/repository/postgres"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

func TestDocumentRepositoryListFiltersAndPaginatesWhenPostgresEnvIsConfigured(t *testing.T) {
	// 验证文档列表在数据库层完成 kb、tag、count、limit/offset 分页，并返回文档关联标签。
	db := newAuthTestDB(t)
	ctx := context.Background()
	userRepo := repositorypostgres.NewUserRepository(db)
	kbRepo := repositorypostgres.NewKnowledgeBaseRepository(db)
	docRepo := repositorypostgres.NewDocumentRepository(db)

	user, err := userRepo.Create(ctx, &models.User{Username: "doc-list-" + uuid.NewString(), PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("Create user error = %v", err)
	}
	kbA, err := kbRepo.Create(ctx, user.ID, &models.KnowledgeBase{Name: "A"})
	if err != nil {
		t.Fatalf("Create kbA error = %v", err)
	}
	kbB, err := kbRepo.Create(ctx, user.ID, &models.KnowledgeBase{Name: "B"})
	if err != nil {
		t.Fatalf("Create kbB error = %v", err)
	}
	tag := &models.Tag{ID: uuid.New(), UserID: user.ID, Name: "重要", Color: "#155EEF"}
	if err := db.WithContext(ctx).Create(tag).Error; err != nil {
		t.Fatalf("Create tag error = %v", err)
	}
	t.Cleanup(func() {
		db.WithContext(context.Background()).Exec("DELETE FROM document_tags WHERE tag_id = ?", tag.ID)
		db.WithContext(context.Background()).Exec("DELETE FROM documents WHERE user_id = ?", user.ID)
		db.WithContext(context.Background()).Exec("DELETE FROM knowledge_bases WHERE user_id = ?", user.ID)
		db.WithContext(context.Background()).Exec("DELETE FROM tags WHERE user_id = ?", user.ID)
		db.WithContext(context.Background()).Exec("DELETE FROM users WHERE id = ?", user.ID)
	})

	doc1, err := docRepo.Create(ctx, user.ID, &models.Document{KBID: &kbA.ID, FileName: "a.txt", FileExt: ".txt", FileSize: 1, FileKey: "a", SourceType: "file", Status: "pending"})
	if err != nil {
		t.Fatalf("Create doc1 error = %v", err)
	}
	doc2, err := docRepo.Create(ctx, user.ID, &models.Document{KBID: &kbA.ID, FileName: "b.txt", FileExt: ".txt", FileSize: 1, FileKey: "b", SourceType: "file", Status: "pending"})
	if err != nil {
		t.Fatalf("Create doc2 error = %v", err)
	}
	if _, err := docRepo.Create(ctx, user.ID, &models.Document{KBID: &kbB.ID, FileName: "c.txt", FileExt: ".txt", FileSize: 1, FileKey: "c", SourceType: "file", Status: "pending"}); err != nil {
		t.Fatalf("Create doc3 error = %v", err)
	}
	if err := db.WithContext(ctx).Exec("INSERT INTO document_tags (document_id, tag_id) VALUES (?, ?), (?, ?)", doc1.ID, tag.ID, doc2.ID, tag.ID).Error; err != nil {
		t.Fatalf("insert document_tags error = %v", err)
	}
	if err := db.WithContext(ctx).Model(&models.Document{}).Where("id = ?", doc1.ID).Update("updated_at", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatalf("update doc1 time error = %v", err)
	}
	if err := db.WithContext(ctx).Model(&models.Document{}).Where("id = ?", doc2.ID).Update("updated_at", time.Now()).Error; err != nil {
		t.Fatalf("update doc2 time error = %v", err)
	}

	allRows, err := docRepo.List(ctx, user.ID)
	if err != nil {
		t.Fatalf("List error = %v", err)
	}
	if len(allRows) != 3 {
		t.Fatalf("all rows len = %d, want 3", len(allRows))
	}

	rows, total, err := docRepo.PageList(ctx, user.ID, repository.DocumentListQuery{
		KBID: &kbA.ID,
		Tag:  &tag.Name,
		PageQuery: repository.PageQuery{
			Page:     2,
			PageSize: 1,
		},
	})
	if err != nil {
		t.Fatalf("PageList error = %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}
	if len(rows) != 1 || rows[0].ID != doc1.ID {
		t.Fatalf("rows = %+v, want second matching document after updated_at desc ordering", rows)
	}
	if len(rows[0].Tags) != 1 || rows[0].Tags[0].Name != tag.Name {
		t.Fatalf("page list tags = %+v, want loaded tag %q", rows[0].Tags, tag.Name)
	}

	found, err := docRepo.FindByID(ctx, user.ID, doc1.ID)
	if err != nil {
		t.Fatalf("FindByID error = %v", err)
	}
	if len(found.Tags) != 1 || found.Tags[0].Name != tag.Name {
		t.Fatalf("find tags = %+v, want loaded tag %q", found.Tags, tag.Name)
	}
}

func TestTagRepositorySyncDocumentTagsWhenPostgresEnvIsConfigured(t *testing.T) {
	// 验证标签仓储会按名称创建或复用当前用户标签，并替换文档标签关联。
	db := newAuthTestDB(t)
	ctx := context.Background()
	userRepo := repositorypostgres.NewUserRepository(db)
	docRepo := repositorypostgres.NewDocumentRepository(db)
	tagRepo := repositorypostgres.NewTagRepository(db)

	user, err := userRepo.Create(ctx, &models.User{Username: "tag-sync-" + uuid.NewString(), PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("Create user error = %v", err)
	}
	otherUser, err := userRepo.Create(ctx, &models.User{Username: "tag-sync-other-" + uuid.NewString(), PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("Create other user error = %v", err)
	}
	doc, err := docRepo.Create(ctx, user.ID, &models.Document{FileName: "a.txt", FileExt: ".txt", FileSize: 1, FileKey: "a", SourceType: "file", Status: "pending"})
	if err != nil {
		t.Fatalf("Create document error = %v", err)
	}
	existing := &models.Tag{ID: uuid.New(), UserID: user.ID, Name: "手动", Color: "#155EEF"}
	otherUserSameName := &models.Tag{ID: uuid.New(), UserID: otherUser.ID, Name: "自动", Color: "#155EEF"}
	if err := db.WithContext(ctx).Create(existing).Error; err != nil {
		t.Fatalf("Create existing tag error = %v", err)
	}
	if err := db.WithContext(ctx).Create(otherUserSameName).Error; err != nil {
		t.Fatalf("Create other user tag error = %v", err)
	}
	t.Cleanup(func() {
		db.WithContext(context.Background()).Exec("DELETE FROM document_tags WHERE document_id = ?", doc.ID)
		db.WithContext(context.Background()).Exec("DELETE FROM documents WHERE user_id IN ?", []uuid.UUID{user.ID, otherUser.ID})
		db.WithContext(context.Background()).Exec("DELETE FROM tags WHERE user_id IN ?", []uuid.UUID{user.ID, otherUser.ID})
		db.WithContext(context.Background()).Exec("DELETE FROM users WHERE id IN ?", []uuid.UUID{user.ID, otherUser.ID})
	})

	rows, err := tagRepo.SyncDocumentTags(ctx, user.ID, doc.ID, []string{" 手动 ", "自动", "手动", ""})
	if err != nil {
		t.Fatalf("SyncDocumentTags error = %v", err)
	}
	if len(rows) != 2 || rows[0].ID != existing.ID || rows[0].Name != "手动" || rows[1].Name != "自动" || rows[1].UserID != user.ID || rows[1].ID == otherUserSameName.ID {
		t.Fatalf("synced rows = %+v, want existing current-user tag and new current-user tag", rows)
	}

	found, err := docRepo.FindByID(ctx, user.ID, doc.ID)
	if err != nil {
		t.Fatalf("FindByID after sync error = %v", err)
	}
	gotNames := documentTagNamesForTest(found.Tags)
	if !slices.Equal(gotNames, []string{"手动", "自动"}) {
		t.Fatalf("document tags = %v, want synced names", gotNames)
	}

	rows, err = tagRepo.SyncDocumentTags(ctx, user.ID, doc.ID, []string{"新标签"})
	if err != nil {
		t.Fatalf("SyncDocumentTags replace error = %v", err)
	}
	if len(rows) != 1 || rows[0].Name != "新标签" {
		t.Fatalf("replaced rows = %+v, want only 新标签", rows)
	}
	found, err = docRepo.FindByID(ctx, user.ID, doc.ID)
	if err != nil {
		t.Fatalf("FindByID after replace error = %v", err)
	}
	gotNames = documentTagNamesForTest(found.Tags)
	if !slices.Equal(gotNames, []string{"新标签"}) {
		t.Fatalf("document tags after replace = %v, want only 新标签", gotNames)
	}

	rows, err = tagRepo.SyncDocumentTags(ctx, user.ID, doc.ID, nil)
	if err != nil {
		t.Fatalf("SyncDocumentTags empty error = %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("empty sync rows = %+v, want empty", rows)
	}
	found, err = docRepo.FindByID(ctx, user.ID, doc.ID)
	if err != nil {
		t.Fatalf("FindByID after empty sync error = %v", err)
	}
	if len(found.Tags) != 0 {
		t.Fatalf("document tags after empty sync = %+v, want empty association", found.Tags)
	}
}

func TestTagRepositoryListCountsAndMergeWhenPostgresEnvIsConfigured(t *testing.T) {
	// 验证标签仓储能按关联来源过滤、统计文档/图片数量，并在合并时迁移关联关系。
	db := newAuthTestDB(t)
	ctx := context.Background()
	userRepo := repositorypostgres.NewUserRepository(db)
	docRepo := repositorypostgres.NewDocumentRepository(db)
	imageRepo := repositorypostgres.NewImageRepository(db)
	tagRepo := repositorypostgres.NewTagRepository(db)

	user, err := userRepo.Create(ctx, &models.User{Username: "tag-list-merge-" + uuid.NewString(), PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("Create user error = %v", err)
	}
	otherUser, err := userRepo.Create(ctx, &models.User{Username: "tag-list-merge-other-" + uuid.NewString(), PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("Create other user error = %v", err)
	}
	source := &models.Tag{ID: uuid.New(), UserID: user.ID, Name: "源标签", Color: "#111111"}
	target := &models.Tag{ID: uuid.New(), UserID: user.ID, Name: "目标标签", Color: "#222222"}
	imageOnly := &models.Tag{ID: uuid.New(), UserID: user.ID, Name: "图片标签", Color: "#333333"}
	otherTag := &models.Tag{ID: uuid.New(), UserID: otherUser.ID, Name: "其他用户", Color: "#444444"}
	if err := db.WithContext(ctx).Create([]*models.Tag{source, target, imageOnly, otherTag}).Error; err != nil {
		t.Fatalf("Create tags error = %v", err)
	}
	doc1, err := docRepo.Create(ctx, user.ID, &models.Document{FileName: "doc1.txt", FileExt: ".txt", FileSize: 1, FileKey: uuid.NewString(), SourceType: "file", Status: "pending"})
	if err != nil {
		t.Fatalf("Create doc1 error = %v", err)
	}
	doc2, err := docRepo.Create(ctx, user.ID, &models.Document{FileName: "doc2.txt", FileExt: ".txt", FileSize: 1, FileKey: uuid.NewString(), SourceType: "file", Status: "pending"})
	if err != nil {
		t.Fatalf("Create doc2 error = %v", err)
	}
	otherDoc, err := docRepo.Create(ctx, otherUser.ID, &models.Document{FileName: "other.txt", FileExt: ".txt", FileSize: 1, FileKey: uuid.NewString(), SourceType: "file", Status: "pending"})
	if err != nil {
		t.Fatalf("Create other doc error = %v", err)
	}
	image1, err := imageRepo.Create(ctx, user.ID, &models.Image{FileName: "img1.png", FileExt: ".png", FileSize: 1, FileKey: uuid.NewString(), Status: "pending"})
	if err != nil {
		t.Fatalf("Create image1 error = %v", err)
	}
	image2, err := imageRepo.Create(ctx, user.ID, &models.Image{FileName: "img2.png", FileExt: ".png", FileSize: 1, FileKey: uuid.NewString(), Status: "pending"})
	if err != nil {
		t.Fatalf("Create image2 error = %v", err)
	}
	otherImage, err := imageRepo.Create(ctx, otherUser.ID, &models.Image{FileName: "other.png", FileExt: ".png", FileSize: 1, FileKey: uuid.NewString(), Status: "pending"})
	if err != nil {
		t.Fatalf("Create other image error = %v", err)
	}
	t.Cleanup(func() {
		db.WithContext(context.Background()).Exec("DELETE FROM image_tags WHERE tag_id IN ?", []uuid.UUID{source.ID, target.ID, imageOnly.ID, otherTag.ID})
		db.WithContext(context.Background()).Exec("DELETE FROM document_tags WHERE tag_id IN ?", []uuid.UUID{source.ID, target.ID, imageOnly.ID, otherTag.ID})
		db.WithContext(context.Background()).Exec("DELETE FROM images WHERE user_id IN ?", []uuid.UUID{user.ID, otherUser.ID})
		db.WithContext(context.Background()).Exec("DELETE FROM documents WHERE user_id IN ?", []uuid.UUID{user.ID, otherUser.ID})
		db.WithContext(context.Background()).Exec("DELETE FROM tags WHERE user_id IN ?", []uuid.UUID{user.ID, otherUser.ID})
		db.WithContext(context.Background()).Exec("DELETE FROM users WHERE id IN ?", []uuid.UUID{user.ID, otherUser.ID})
	})

	if err := db.WithContext(ctx).Exec(
		"INSERT INTO document_tags (document_id, tag_id) VALUES (?, ?), (?, ?), (?, ?), (?, ?)",
		doc1.ID, source.ID,
		doc2.ID, source.ID,
		doc2.ID, target.ID,
		otherDoc.ID, otherTag.ID,
	).Error; err != nil {
		t.Fatalf("insert document_tags error = %v", err)
	}
	if err := db.WithContext(ctx).Exec(
		"INSERT INTO image_tags (image_id, tag_id) VALUES (?, ?), (?, ?), (?, ?), (?, ?)",
		image1.ID, source.ID,
		image1.ID, target.ID,
		image2.ID, imageOnly.ID,
		otherImage.ID, otherTag.ID,
	).Error; err != nil {
		t.Fatalf("insert image_tags error = %v", err)
	}

	allRows, err := tagRepo.ListByScope(ctx, user.ID, string(types.TagScopeAll))
	if err != nil {
		t.Fatalf("ListByScope all error = %v", err)
	}
	if len(allRows) != 3 {
		t.Fatalf("all rows len = %d, want 3", len(allRows))
	}
	docRows, err := tagRepo.ListByScope(ctx, user.ID, string(types.TagScopeDocument))
	if err != nil {
		t.Fatalf("ListByScope document error = %v", err)
	}
	if got := tagNamesForTest(docRows); !slices.Equal(got, []string{"源标签", "目标标签"}) {
		t.Fatalf("document scope names = %v, want source and target", got)
	}
	imageRows, err := tagRepo.ListByScope(ctx, user.ID, string(types.TagScopeImage))
	if err != nil {
		t.Fatalf("ListByScope image error = %v", err)
	}
	if got := tagNamesForTest(imageRows); !slices.Equal(got, []string{"图片标签", "源标签", "目标标签"}) {
		t.Fatalf("image scope names = %v, want image-related tags", got)
	}
	pageRows, total, err := tagRepo.PageList(ctx, user.ID, repository.TagListQuery{
		Scope: string(types.TagScopeImage),
		PageQuery: repository.PageQuery{
			Page:     2,
			PageSize: 1,
		},
	})
	if err != nil {
		t.Fatalf("PageList image error = %v", err)
	}
	if total != 3 {
		t.Fatalf("PageList total = %d, want 3", total)
	}
	if len(pageRows) != 1 {
		t.Fatalf("PageList rows len = %d, want 1", len(pageRows))
	}

	docCounts, err := tagRepo.CountDocumentsByTags(ctx, user.ID, []uuid.UUID{source.ID, target.ID, imageOnly.ID, otherTag.ID})
	if err != nil {
		t.Fatalf("CountDocumentsByTags error = %v", err)
	}
	if docCounts[source.ID] != 2 || docCounts[target.ID] != 1 || docCounts[imageOnly.ID] != 0 || docCounts[otherTag.ID] != 0 {
		t.Fatalf("document counts = %+v, want source=2 target=1 imageOnly=0 other=0", docCounts)
	}
	imageCounts, err := tagRepo.CountImagesByTags(ctx, user.ID, []uuid.UUID{source.ID, target.ID, imageOnly.ID, otherTag.ID})
	if err != nil {
		t.Fatalf("CountImagesByTags error = %v", err)
	}
	if imageCounts[source.ID] != 1 || imageCounts[target.ID] != 1 || imageCounts[imageOnly.ID] != 1 || imageCounts[otherTag.ID] != 0 {
		t.Fatalf("image counts = %+v, want source=1 target=1 imageOnly=1 other=0", imageCounts)
	}

	sourceDocumentIDs, err := tagRepo.ListDocumentIDsByTag(ctx, user.ID, source.ID)
	if err != nil {
		t.Fatalf("ListDocumentIDsByTag error = %v", err)
	}
	slices.SortFunc(sourceDocumentIDs, func(a uuid.UUID, b uuid.UUID) int {
		return strings.Compare(a.String(), b.String())
	})
	wantDocumentIDs := []uuid.UUID{doc1.ID, doc2.ID}
	slices.SortFunc(wantDocumentIDs, func(a uuid.UUID, b uuid.UUID) int {
		return strings.Compare(a.String(), b.String())
	})
	if !slices.Equal(sourceDocumentIDs, wantDocumentIDs) {
		t.Fatalf("source document ids = %v, want %v", sourceDocumentIDs, wantDocumentIDs)
	}
	documentTagNames, err := tagRepo.ListDocumentTagNames(ctx, user.ID, []uuid.UUID{doc1.ID, doc2.ID, otherDoc.ID})
	if err != nil {
		t.Fatalf("ListDocumentTagNames error = %v", err)
	}
	if got := sortedStringsForTest(documentTagNames[doc1.ID]); !slices.Equal(got, []string{"源标签"}) {
		t.Fatalf("doc1 tag names = %v, want source", got)
	}
	if got := sortedStringsForTest(documentTagNames[doc2.ID]); !slices.Equal(got, []string{"源标签", "目标标签"}) {
		t.Fatalf("doc2 tag names = %v, want source and target", got)
	}
	if _, ok := documentTagNames[otherDoc.ID]; ok {
		t.Fatalf("other user document tag names = %+v, want excluded", documentTagNames[otherDoc.ID])
	}
	emptyDocumentTagNames, err := tagRepo.ListDocumentTagNames(ctx, user.ID, nil)
	if err != nil {
		t.Fatalf("ListDocumentTagNames empty error = %v", err)
	}
	if len(emptyDocumentTagNames) != 0 {
		t.Fatalf("empty document tag names = %+v, want empty map", emptyDocumentTagNames)
	}

	merged, err := tagRepo.Merge(ctx, user.ID, source.ID, target.ID)
	if err != nil {
		t.Fatalf("Merge error = %v", err)
	}
	if merged.ID != target.ID || merged.Name != target.Name || merged.Color != target.Color {
		t.Fatalf("merged row = %+v, want target metadata unchanged", merged)
	}
	if _, err := tagRepo.FindByID(ctx, user.ID, source.ID); xerr.From(err).Kind != xerr.KindNotFound {
		t.Fatalf("source after merge error = %v, want not found", err)
	}
	afterDocCounts, err := tagRepo.CountDocumentsByTags(ctx, user.ID, []uuid.UUID{target.ID})
	if err != nil {
		t.Fatalf("CountDocumentsByTags after merge error = %v", err)
	}
	afterImageCounts, err := tagRepo.CountImagesByTags(ctx, user.ID, []uuid.UUID{target.ID})
	if err != nil {
		t.Fatalf("CountImagesByTags after merge error = %v", err)
	}
	if afterDocCounts[target.ID] != 2 || afterImageCounts[target.ID] != 1 {
		t.Fatalf("target counts after merge docs=%+v images=%+v, want docs=2 images=1", afterDocCounts, afterImageCounts)
	}

	emptyDocCounts, err := tagRepo.CountDocumentsByTags(ctx, user.ID, nil)
	if err != nil {
		t.Fatalf("CountDocumentsByTags empty error = %v", err)
	}
	emptyImageCounts, err := tagRepo.CountImagesByTags(ctx, user.ID, []uuid.UUID{})
	if err != nil {
		t.Fatalf("CountImagesByTags empty error = %v", err)
	}
	if len(emptyDocCounts) != 0 || len(emptyImageCounts) != 0 {
		t.Fatalf("empty counts docs=%+v images=%+v, want empty maps", emptyDocCounts, emptyImageCounts)
	}
}

func TestRepositoryCountByKnowledgeBaseWhenPostgresEnvIsConfigured(t *testing.T) {
	// 验证文档和图片仓储会按当前用户、指定知识库批量统计数量，并忽略未归属和其他用户的数据。
	db := newAuthTestDB(t)
	ctx := context.Background()
	userRepo := repositorypostgres.NewUserRepository(db)
	kbRepo := repositorypostgres.NewKnowledgeBaseRepository(db)
	docRepo := repositorypostgres.NewDocumentRepository(db)
	imageRepo := repositorypostgres.NewImageRepository(db)

	user, err := userRepo.Create(ctx, &models.User{Username: "kb-count-" + uuid.NewString(), PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("Create user error = %v", err)
	}
	otherUser, err := userRepo.Create(ctx, &models.User{Username: "kb-count-other-" + uuid.NewString(), PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("Create other user error = %v", err)
	}
	kbA, err := kbRepo.Create(ctx, user.ID, &models.KnowledgeBase{Name: "A"})
	if err != nil {
		t.Fatalf("Create kbA error = %v", err)
	}
	kbB, err := kbRepo.Create(ctx, user.ID, &models.KnowledgeBase{Name: "B"})
	if err != nil {
		t.Fatalf("Create kbB error = %v", err)
	}
	otherKB, err := kbRepo.Create(ctx, otherUser.ID, &models.KnowledgeBase{Name: "Other"})
	if err != nil {
		t.Fatalf("Create other kb error = %v", err)
	}
	t.Cleanup(func() {
		db.WithContext(context.Background()).Exec("DELETE FROM images WHERE user_id IN ?", []uuid.UUID{user.ID, otherUser.ID})
		db.WithContext(context.Background()).Exec("DELETE FROM documents WHERE user_id IN ?", []uuid.UUID{user.ID, otherUser.ID})
		db.WithContext(context.Background()).Exec("DELETE FROM knowledge_bases WHERE user_id IN ?", []uuid.UUID{user.ID, otherUser.ID})
		db.WithContext(context.Background()).Exec("DELETE FROM users WHERE id IN ?", []uuid.UUID{user.ID, otherUser.ID})
	})

	for i, kbID := range []*uuid.UUID{&kbA.ID, &kbA.ID, &kbB.ID, nil} {
		if _, err := docRepo.Create(ctx, user.ID, &models.Document{KBID: kbID, FileName: "doc.txt", FileExt: ".txt", FileSize: int64(i + 1), FileKey: uuid.NewString(), SourceType: "file", Status: "pending"}); err != nil {
			t.Fatalf("Create document %d error = %v", i, err)
		}
	}
	if _, err := docRepo.Create(ctx, otherUser.ID, &models.Document{KBID: &kbA.ID, FileName: "other.txt", FileExt: ".txt", FileSize: 1, FileKey: uuid.NewString(), SourceType: "file", Status: "pending"}); err != nil {
		t.Fatalf("Create other user document error = %v", err)
	}
	if _, err := docRepo.Create(ctx, otherUser.ID, &models.Document{KBID: &otherKB.ID, FileName: "other-kb.txt", FileExt: ".txt", FileSize: 1, FileKey: uuid.NewString(), SourceType: "file", Status: "pending"}); err != nil {
		t.Fatalf("Create other kb document error = %v", err)
	}

	for i, kbID := range []*uuid.UUID{&kbA.ID, &kbB.ID, &kbB.ID, nil} {
		if _, err := imageRepo.Create(ctx, user.ID, &models.Image{KBID: kbID, FileName: "img.png", FileExt: ".png", FileSize: int64(i + 1), FileKey: uuid.NewString(), Status: "pending"}); err != nil {
			t.Fatalf("Create image %d error = %v", i, err)
		}
	}
	if _, err := imageRepo.Create(ctx, otherUser.ID, &models.Image{KBID: &kbB.ID, FileName: "other.png", FileExt: ".png", FileSize: 1, FileKey: uuid.NewString(), Status: "pending"}); err != nil {
		t.Fatalf("Create other user image error = %v", err)
	}

	docCounts, err := docRepo.CountByKnowledgeBase(ctx, user.ID, []uuid.UUID{kbA.ID, kbB.ID, otherKB.ID})
	if err != nil {
		t.Fatalf("Document CountByKnowledgeBase error = %v", err)
	}
	if docCounts[kbA.ID] != 2 || docCounts[kbB.ID] != 1 || docCounts[otherKB.ID] != 0 {
		t.Fatalf("doc counts = %+v, want A=2 B=1 other=0", docCounts)
	}
	imageCounts, err := imageRepo.CountByKnowledgeBase(ctx, user.ID, []uuid.UUID{kbA.ID, kbB.ID, otherKB.ID})
	if err != nil {
		t.Fatalf("Image CountByKnowledgeBase error = %v", err)
	}
	if imageCounts[kbA.ID] != 1 || imageCounts[kbB.ID] != 2 || imageCounts[otherKB.ID] != 0 {
		t.Fatalf("image counts = %+v, want A=1 B=2 other=0", imageCounts)
	}

	emptyDocCounts, err := docRepo.CountByKnowledgeBase(ctx, user.ID, nil)
	if err != nil {
		t.Fatalf("Document CountByKnowledgeBase empty error = %v", err)
	}
	emptyImageCounts, err := imageRepo.CountByKnowledgeBase(ctx, user.ID, []uuid.UUID{})
	if err != nil {
		t.Fatalf("Image CountByKnowledgeBase empty error = %v", err)
	}
	if len(emptyDocCounts) != 0 || len(emptyImageCounts) != 0 {
		t.Fatalf("empty counts = docs:%+v images:%+v, want empty maps", emptyDocCounts, emptyImageCounts)
	}
}

func documentTagNamesForTest(rows []models.Tag) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Name)
	}
	slices.Sort(out)
	return out
}

func tagNamesForTest(rows []*models.Tag) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Name)
	}
	slices.Sort(out)
	return out
}

func sortedStringsForTest(values []string) []string {
	out := append([]string(nil), values...)
	slices.Sort(out)
	return out
}

func TestKnowledgeBaseRepositoryFindDefaultWhenPostgresEnvIsConfigured(t *testing.T) {
	// 验证知识库仓储可以直接查询当前用户的默认知识库，并在不存在时返回 not found。
	db := newAuthTestDB(t)
	ctx := context.Background()
	userRepo := repositorypostgres.NewUserRepository(db)
	kbRepo := repositorypostgres.NewKnowledgeBaseRepository(db)

	user, err := userRepo.Create(ctx, &models.User{Username: "kb-default-" + uuid.NewString(), PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("Create user error = %v", err)
	}
	otherUser, err := userRepo.Create(ctx, &models.User{Username: "kb-default-other-" + uuid.NewString(), PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("Create other user error = %v", err)
	}
	t.Cleanup(func() {
		db.WithContext(context.Background()).Exec("DELETE FROM knowledge_bases WHERE user_id IN ?", []uuid.UUID{user.ID, otherUser.ID})
		db.WithContext(context.Background()).Exec("DELETE FROM users WHERE id IN ?", []uuid.UUID{user.ID, otherUser.ID})
	})

	if _, err := kbRepo.FindDefault(ctx, user.ID); xerr.From(err).Kind != xerr.KindNotFound {
		t.Fatalf("FindDefault missing error = %v, want not found", err)
	}
	defaultKB, err := kbRepo.Create(ctx, user.ID, &models.KnowledgeBase{Name: "默认知识库", IsDefault: true, ChatEnabled: true})
	if err != nil {
		t.Fatalf("Create default kb error = %v", err)
	}
	if _, err := kbRepo.Create(ctx, otherUser.ID, &models.KnowledgeBase{Name: "其他默认", IsDefault: true, ChatEnabled: true}); err != nil {
		t.Fatalf("Create other default kb error = %v", err)
	}

	found, err := kbRepo.FindDefault(ctx, user.ID)
	if err != nil {
		t.Fatalf("FindDefault error = %v", err)
	}
	if found.ID != defaultKB.ID || found.UserID != user.ID {
		t.Fatalf("FindDefault = %+v, want current user's default", found)
	}
}

// TestKnowledgeBaseRepositorySetDefaultWhenPostgresEnvIsConfigured 验证默认知识库切换具有唯一性和用户隔离。
func TestKnowledgeBaseRepositorySetDefaultWhenPostgresEnvIsConfigured(t *testing.T) {
	db := newAuthTestDB(t)
	ctx := context.Background()
	userRepo := repositorypostgres.NewUserRepository(db)
	kbRepo := repositorypostgres.NewKnowledgeBaseRepository(db)

	user, err := userRepo.Create(ctx, &models.User{Username: "kb-set-default-" + uuid.NewString(), PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("Create user error = %v", err)
	}
	otherUser, err := userRepo.Create(ctx, &models.User{Username: "kb-set-default-other-" + uuid.NewString(), PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("Create other user error = %v", err)
	}
	t.Cleanup(func() {
		db.WithContext(context.Background()).Exec("DELETE FROM knowledge_bases WHERE user_id IN ?", []uuid.UUID{user.ID, otherUser.ID})
		db.WithContext(context.Background()).Exec("DELETE FROM users WHERE id IN ?", []uuid.UUID{user.ID, otherUser.ID})
	})

	oldDefault, err := kbRepo.Create(ctx, user.ID, &models.KnowledgeBase{Name: "旧默认", IsDefault: true})
	if err != nil {
		t.Fatalf("Create old default error = %v", err)
	}
	target, err := kbRepo.Create(ctx, user.ID, &models.KnowledgeBase{Name: "新默认"})
	if err != nil {
		t.Fatalf("Create target error = %v", err)
	}
	otherDefault, err := kbRepo.Create(ctx, otherUser.ID, &models.KnowledgeBase{Name: "其他用户默认", IsDefault: true})
	if err != nil {
		t.Fatalf("Create other default error = %v", err)
	}

	selected, err := kbRepo.SetDefault(ctx, user.ID, target.ID)
	if err != nil {
		t.Fatalf("SetDefault error = %v", err)
	}
	if selected.ID != target.ID || !selected.IsDefault {
		t.Fatalf("selected = %+v, want target as default", selected)
	}

	oldRow, err := kbRepo.FindByID(ctx, user.ID, oldDefault.ID)
	if err != nil {
		t.Fatalf("Find old default error = %v", err)
	}
	otherRow, err := kbRepo.FindByID(ctx, otherUser.ID, otherDefault.ID)
	if err != nil {
		t.Fatalf("Find other default error = %v", err)
	}
	if oldRow.IsDefault || !otherRow.IsDefault {
		t.Fatalf("default flags old=%v other=%v, want false/true", oldRow.IsDefault, otherRow.IsDefault)
	}
}
