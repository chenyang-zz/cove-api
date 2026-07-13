package es_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ragchunker "github.com/boxify/api-go/internal/core/rag/chunker"
	infraes "github.com/boxify/api-go/internal/infrastructure/db/es"
	"github.com/boxify/api-go/internal/models"
	repositoryes "github.com/boxify/api-go/internal/repository/es"
	"github.com/google/uuid"
)

func TestRAGChunkRepositoryEnsureIndexCreatesMissingIndex(t *testing.T) {
	// 验证 RAG chunk repository 会在索引不存在时创建 dense_vector mapping。
	var createdBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/boxify_chunks":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPut && r.URL.Path == "/boxify_chunks":
			if err := json.NewDecoder(r.Body).Decode(&createdBody); err != nil {
				t.Fatalf("decode create index body: %v", err)
			}
			_, _ = w.Write([]byte(`{"acknowledged":true}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := infraes.NewClient(infraes.Config{URL: server.URL})
	if err != nil {
		t.Fatalf("NewClient error = %v", err)
	}

	repo := repositoryes.NewRAGChunkRepository(client, "")
	if err := repo.EnsureIndex(context.Background(), 3); err != nil {
		t.Fatalf("EnsureIndex error = %v", err)
	}
	vector := createdBody["mappings"].(map[string]any)["properties"].(map[string]any)["vector"].(map[string]any)
	if vector["type"] != "dense_vector" || vector["dims"] != float64(3) || vector["similarity"] != "cosine" {
		t.Fatalf("vector mapping = %#v, want dense_vector dims=3 cosine", vector)
	}
}

func TestRAGChunkRepositoryEnsureIndexSkipsExistingIndex(t *testing.T) {
	// 验证索引已存在时 EnsureIndex 不会重复创建索引。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		if r.Method != http.MethodHead || r.URL.Path != "/chunks" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client, err := infraes.NewClient(infraes.Config{URL: server.URL})
	if err != nil {
		t.Fatalf("NewClient error = %v", err)
	}

	if err := repositoryes.NewRAGChunkRepository(client, "chunks").EnsureIndex(context.Background(), 3); err != nil {
		t.Fatalf("EnsureIndex error = %v", err)
	}
}

func TestRAGChunkRepositoryIndexDocumentChunksWritesParentAndChild(t *testing.T) {
	// 验证文档 chunk 写入会生成 parent/child 文档并带上归属字段、标签和向量。
	userID := uuid.New()
	documentID := uuid.New()
	kbID := uuid.New()
	indexed := map[string]models.RAGChunkDocument{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		if r.Method != http.MethodPut || !strings.HasPrefix(r.URL.Path, "/boxify_chunks/_doc/") {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body models.RAGChunkDocument
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode index body: %v", err)
		}
		indexed[strings.TrimPrefix(r.URL.Path, "/boxify_chunks/_doc/")] = body
		_, _ = w.Write([]byte(`{"result":"created"}`))
	}))
	defer server.Close()
	client, err := infraes.NewClient(infraes.Config{URL: server.URL})
	if err != nil {
		t.Fatalf("NewClient error = %v", err)
	}

	doc := &models.Document{
		ID:         documentID,
		UserID:     userID,
		KBID:       &kbID,
		FileName:   "guide.md",
		SourceType: "file",
		Tags:       []models.Tag{{Name: "重要"}},
	}
	chunks := []*ragchunker.Chunk{{Content: "parent", Children: []string{"child"}}}
	vectors := [][]float64{{0.1, 0.2, 0.3}, {0.4, 0.5, 0.6}}

	if err := repositoryes.NewRAGChunkRepository(client, "").IndexDocumentChunks(context.Background(), doc, chunks, vectors); err != nil {
		t.Fatalf("IndexDocumentChunks error = %v", err)
	}
	if len(indexed) != 2 {
		t.Fatalf("indexed len = %d, want parent and child", len(indexed))
	}
	var gotChild bool
	for _, body := range indexed {
		if body.DocumentID != documentID.String() || body.UserID != userID.String() || body.KBID != kbID.String() || body.DocName != "guide.md" || body.Vector == nil {
			t.Fatalf("indexed body = %#v, want ownership and vector fields", body)
		}
		if body.Level == "child" {
			gotChild = true
			if body.ParentID == "" {
				t.Fatalf("child body = %#v, want parent_id", body)
			}
		}
	}
	if !gotChild {
		t.Fatalf("indexed bodies = %#v, want child chunk", indexed)
	}
}

func TestRAGChunkRepositoryDeleteByDocument(t *testing.T) {
	// 验证删除 query 使用 user_id + document_id。
	userID := uuid.New()
	documentID := uuid.New()
	var deleteBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		if r.Method != http.MethodPost || r.URL.Path != "/boxify_chunks/_delete_by_query" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&deleteBody); err != nil {
			t.Fatalf("decode delete body: %v", err)
		}
		_, _ = w.Write([]byte(`{"deleted":1}`))
	}))
	defer server.Close()
	client, err := infraes.NewClient(infraes.Config{URL: server.URL})
	if err != nil {
		t.Fatalf("NewClient error = %v", err)
	}

	repo := repositoryes.NewRAGChunkRepository(client, "")
	if err := repo.DeleteByDocument(context.Background(), userID, documentID); err != nil {
		t.Fatalf("DeleteByDocument error = %v", err)
	}
	encodedDelete, _ := json.Marshal(deleteBody)
	deleteText := string(encodedDelete)
	if !strings.Contains(deleteText, `"document_id":"`+documentID.String()+`"`) || !strings.Contains(deleteText, `"user_id":"`+userID.String()+`"`) {
		t.Fatalf("delete query = %s, want user_id and document_id", deleteText)
	}

}

func TestRAGChunkRepositoryUpdateKnowledgeBase(t *testing.T) {
	// 验证移动文档时通过 update-by-query 只更新当前用户当前文档的 kb_id。
	userID := uuid.New()
	documentID := uuid.New()
	kbID := uuid.New()
	var updateBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		if r.Method != http.MethodPost || r.URL.Path != "/boxify_chunks/_update_by_query" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&updateBody); err != nil {
			t.Fatalf("decode update body: %v", err)
		}
		_, _ = w.Write([]byte(`{"updated":2}`))
	}))
	defer server.Close()
	client, err := infraes.NewClient(infraes.Config{URL: server.URL})
	if err != nil {
		t.Fatalf("NewClient error = %v", err)
	}

	repo := repositoryes.NewRAGChunkRepository(client, "")
	if err := repo.UpdateKnowledgeBase(context.Background(), userID, documentID, kbID); err != nil {
		t.Fatalf("UpdateKnowledgeBase error = %v", err)
	}
	encoded, _ := json.Marshal(updateBody)
	text := string(encoded)
	if !strings.Contains(text, `"document_id":"`+documentID.String()+`"`) || !strings.Contains(text, `"user_id":"`+userID.String()+`"`) || !strings.Contains(text, `"kb_id":"`+kbID.String()+`"`) {
		t.Fatalf("update query = %s, want user_id document_id and kb_id", text)
	}
	if !strings.Contains(text, "ctx._source.kb_id = params.kb_id") {
		t.Fatalf("update script = %s, want kb_id assignment", text)
	}
}

func TestRAGChunkRepositoryUpdateTags(t *testing.T) {
	// 验证分类完成后通过 update-by-query 只更新当前用户当前文档的 tags。
	userID := uuid.New()
	documentID := uuid.New()
	var updateBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		if r.Method != http.MethodPost || r.URL.Path != "/boxify_chunks/_update_by_query" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&updateBody); err != nil {
			t.Fatalf("decode update tags body: %v", err)
		}
		_, _ = w.Write([]byte(`{"updated":2}`))
	}))
	defer server.Close()
	client, err := infraes.NewClient(infraes.Config{URL: server.URL})
	if err != nil {
		t.Fatalf("NewClient error = %v", err)
	}

	repo := repositoryes.NewRAGChunkRepository(client, "")
	if err := repo.UpdateTags(context.Background(), userID, documentID, []string{"手动", "分类"}); err != nil {
		t.Fatalf("UpdateTags error = %v", err)
	}
	encoded, _ := json.Marshal(updateBody)
	text := string(encoded)
	if !strings.Contains(text, `"document_id":"`+documentID.String()+`"`) || !strings.Contains(text, `"user_id":"`+userID.String()+`"`) {
		t.Fatalf("update tags query = %s, want user_id and document_id", text)
	}
	if !strings.Contains(text, "ctx._source.tags = params.tags") || !strings.Contains(text, `"手动"`) || !strings.Contains(text, `"分类"`) {
		t.Fatalf("update tags script = %s, want tags assignment", text)
	}
}

func TestRAGChunkRepositoryDecodeSourceReturnsModelSource(t *testing.T) {
	// 验证 ES source 解码结果使用 models.RAGChunkSource，避免 repository 暴露业务数据结构。
	userChunkID := uuid.New()
	documentID := uuid.New()
	kbID := uuid.New()
	client, err := infraes.NewClient(infraes.Config{URL: "http://127.0.0.1:9200"})
	if err != nil {
		t.Fatalf("NewClient error = %v", err)
	}
	got, err := repositoryes.NewRAGChunkRepository(client, "").DecodeSource(map[string]any{
		"chunk_id":    userChunkID.String(),
		"document_id": documentID.String(),
		"kb_id":       kbID.String(),
		"doc_name":    "guide.md",
		"source_type": "file",
	})
	if err != nil {
		t.Fatalf("DecodeSource error = %v", err)
	}
	var source models.RAGChunkSource = got
	if source.ChunkID != userChunkID || source.DocumentID != documentID || source.KBID == nil || *source.KBID != kbID || source.DocName != "guide.md" {
		t.Fatalf("DecodeSource source = %#v, want decoded model source", source)
	}
}

// 验证图片 chunk 写入使用 image source_type，并复用 document_id 存图片 ID。
func TestRAGChunkRepositoryIndexImageChunk(t *testing.T) {
	userID := uuid.New()
	imageID := uuid.New()
	kbID := uuid.New()
	var indexed models.RAGChunkDocument
	var docID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		if r.Method != http.MethodPut || !strings.HasPrefix(r.URL.Path, "/boxify_chunks/_doc/") {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		docID = strings.TrimPrefix(r.URL.Path, "/boxify_chunks/_doc/")
		if err := json.NewDecoder(r.Body).Decode(&indexed); err != nil {
			t.Fatalf("decode index body: %v", err)
		}
		_, _ = w.Write([]byte(`{"result":"created"}`))
	}))
	defer server.Close()
	client, err := infraes.NewClient(infraes.Config{URL: server.URL})
	if err != nil {
		t.Fatalf("NewClient error = %v", err)
	}
	image := &models.Image{
		ID: imageID, UserID: userID, KBID: &kbID, FileName: "cat.png",
		Tags: []models.Tag{{Name: "风景"}},
	}
	if err := repositoryes.NewRAGChunkRepository(client, "").IndexImageChunk(context.Background(), image, "一只猫", []float64{0.1, 0.2, 0.3}); err != nil {
		t.Fatalf("IndexImageChunk error = %v", err)
	}
	if indexed.DocumentID != imageID.String() || indexed.UserID != userID.String() || indexed.KBID != kbID.String() {
		t.Fatalf("indexed ownership = %#v", indexed)
	}
	if indexed.SourceType != "image" || indexed.DocName != "cat.png" || indexed.Content != "一只猫" || indexed.Level != "parent" {
		t.Fatalf("indexed body = %#v", indexed)
	}
	if docID == "" || indexed.ChunkID == "" || indexed.ChunkID != docID {
		t.Fatalf("chunk id path=%q body=%q", docID, indexed.ChunkID)
	}
}

