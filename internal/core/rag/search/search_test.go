package search

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/boxify/api-go/internal/core/valuex"
)

type fakeESClient struct {
	calls []esCall
	err   error
	resps []map[string]any
}

type esCall struct {
	index string
	query any
}

func (c *fakeESClient) Search(ctx context.Context, index string, query any) (map[string]any, error) {
	c.calls = append(c.calls, esCall{index: index, query: query})
	if c.err != nil {
		return nil, c.err
	}
	if len(c.resps) == 0 {
		return hitsResponse(), nil
	}
	resp := c.resps[0]
	c.resps = c.resps[1:]
	return resp, nil
}

type fakeEmbedder struct {
	calls int
	vec   []float64
	err   error
	dim   int
}

func (e *fakeEmbedder) EmbedOne(ctx context.Context, text string, dimensions int) ([]float64, error) {
	e.calls++
	e.dim = dimensions
	if e.err != nil {
		return nil, e.err
	}
	return e.vec, nil
}

type fakeReranker struct {
	calls     int
	documents []string
	results   []RerankResult
	err       error
}

func (r *fakeReranker) Rerank(ctx context.Context, query string, documents []string, topN int) ([]RerankResult, error) {
	r.calls++
	r.documents = append([]string(nil), documents...)
	if r.err != nil {
		return nil, r.err
	}
	return r.results, nil
}

type sourceMeta struct {
	DocName string
	KBID    string
}

func decodeSourceMeta(src map[string]any) (sourceMeta, error) {
	return sourceMeta{
		DocName: valuex.String(src["doc_name"]),
		KBID:    valuex.String(src["kb_id"]),
	}, nil
}

func TestNewSearcherAppliesOptions(t *testing.T) {
	// 验证 NewSearcher 使用默认配置，并且 WithOption 能覆盖默认值。
	reranker := &fakeReranker{}
	esClient := &fakeESClient{}
	embedder := &fakeEmbedder{}
	filterBuilder := func(ctx context.Context, req Request) ([]any, error) {
		return []any{map[string]any{"term": map[string]any{"tenant": "u-1"}}}, nil
	}

	searcher := NewSearcher[sourceMeta](
		esClient,
		embedder,
		WithIndex("custom_chunks"),
		WithEmbeddingDim(2048),
		WithRecallSize(30),
		WithVectorWeight(0.7),
		WithBM25Weight(0.3),
		WithKnnOversample(5),
		WithReranker(reranker),
		WithFilterBuilder(filterBuilder),
		WithSourceDecoder[sourceMeta](decodeSourceMeta),
	)

	if searcher.es != esClient || searcher.embedder != embedder {
		t.Fatalf("dependencies were not assigned")
	}
	if searcher.Index != "custom_chunks" {
		t.Fatalf("Index = %q, want custom_chunks", searcher.Index)
	}
	if searcher.EmbeddingDim != 2048 {
		t.Fatalf("EmbeddingDim = %d, want 2048", searcher.EmbeddingDim)
	}
	if searcher.RecallSize != 30 {
		t.Fatalf("RecallSize = %d, want 30", searcher.RecallSize)
	}
	if searcher.VectorWeight != 0.7 || searcher.BM25Weight != 0.3 {
		t.Fatalf("weights = %v/%v, want 0.7/0.3", searcher.VectorWeight, searcher.BM25Weight)
	}
	if searcher.KnnOversample != 5 {
		t.Fatalf("KnnOversample = %d, want 5", searcher.KnnOversample)
	}
	if searcher.Reranker != reranker {
		t.Fatalf("Reranker = %#v, want fake reranker", searcher.Reranker)
	}
	if searcher.FilterBuilder == nil || searcher.sourceDecoder == nil {
		t.Fatal("FilterBuilder or sourceDecoder is nil")
	}
}

func TestNormalizeScores(t *testing.T) {
	// 验证分数归一化能处理空输入、同分输入和普通区间输入。
	if got := Normalize(nil); len(got) != 0 {
		t.Fatalf("Normalize(nil) = %#v, want empty", got)
	}
	if got := Normalize(map[string]float64{"a": 2, "b": 2}); !reflect.DeepEqual(got, map[string]float64{"a": 1, "b": 1}) {
		t.Fatalf("Normalize(equal) = %#v", got)
	}
	got := Normalize(map[string]float64{"low": 10, "mid": 15, "high": 20})
	want := map[string]float64{"low": 0, "mid": 0.5, "high": 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Normalize() = %#v, want %#v", got, want)
	}
}

func TestSearcherReturnsFilterBuilderErrorBeforeDependencies(t *testing.T) {
	// 验证 filter builder 出错时会直接返回错误，不访问 embedding 和 ES。
	wantErr := errors.New("build filter failed")
	esClient := &fakeESClient{}
	embedder := &fakeEmbedder{vec: []float64{0.1}}

	_, err := NewSearcher[sourceMeta](esClient, embedder, WithFilterBuilder(func(ctx context.Context, req Request) ([]any, error) {
		return nil, wantErr
	})).Search(context.Background(), "hello")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Search() error = %v, want %v", err, wantErr)
	}
	if embedder.calls != 0 || len(esClient.calls) != 0 {
		t.Fatalf("dependency calls = embedder %d es %d, want zero", embedder.calls, len(esClient.calls))
	}
}

func TestSearcherUsesRequestOptionsInVectorAndBM25Queries(t *testing.T) {
	// 验证 Search 的 RequestOption 会同时影响向量召回和 BM25 召回。
	filter := []any{
		map[string]any{"term": map[string]any{"tenant": "u-1"}},
		map[string]any{"terms": map[string]any{"kb_id": []string{"kb-1", "kb-2"}}},
	}
	esClient := &fakeESClient{resps: []map[string]any{hitsResponse(), hitsResponse()}}
	embedder := &fakeEmbedder{vec: []float64{0.1, 0.2}}

	_, err := NewSearcher[sourceMeta](esClient, embedder, WithIndex("chunks"), WithEmbeddingDim(512)).
		Search(context.Background(), "search text", WithFilters(filter), WithTopK(3), WithRequestRecallSize(7))
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if embedder.calls != 1 || embedder.dim != 512 {
		t.Fatalf("embedder calls/dim = %d/%d, want 1/512", embedder.calls, embedder.dim)
	}
	if len(esClient.calls) != 2 {
		t.Fatalf("es calls = %d, want 2", len(esClient.calls))
	}
	vectorQuery := esClient.calls[0].query.(map[string]any)
	if vectorQuery["size"] != 7 {
		t.Fatalf("vector size = %#v, want 7", vectorQuery["size"])
	}
	knn := vectorQuery["knn"].(map[string]any)
	if knn["k"] != 7 {
		t.Fatalf("knn = %#v, want k=7", knn)
	}
	if _, ok := knn["num_candidates"]; ok {
		t.Fatalf("num_candidates = %#v, want omitted by default", knn["num_candidates"])
	}
	gotVectorFilter := mustFilter(t, knn["filter"])
	if !reflect.DeepEqual(gotVectorFilter, filter) {
		t.Fatalf("vector filter = %#v, want %#v", gotVectorFilter, filter)
	}

	bm25Query := esClient.calls[1].query.(map[string]any)
	gotBM25Filter := bm25Query["query"].(map[string]any)["bool"].(map[string]any)["filter"].([]any)
	if !reflect.DeepEqual(gotBM25Filter, filter) {
		t.Fatalf("bm25 filter = %#v, want %#v", gotBM25Filter, filter)
	}
	must := bm25Query["query"].(map[string]any)["bool"].(map[string]any)["must"].([]any)
	if !reflect.DeepEqual(must, []any{map[string]any{"match": map[string]any{"content": "search text"}}}) {
		t.Fatalf("bm25 must = %#v", must)
	}
}

func TestSearcherUsesFilterBuilderAndOptionRecallSize(t *testing.T) {
	// 验证自定义 filter builder 会收到内部 Request，并且 Request 未设置 recallSize 时使用 searcher option 默认值。
	filter := []any{map[string]any{"term": map[string]any{"tenant": "from-builder"}}}
	esClient := &fakeESClient{resps: []map[string]any{hitsResponse(), hitsResponse()}}
	embedder := &fakeEmbedder{vec: []float64{0.1}}
	calls := 0

	_, err := NewSearcher[sourceMeta](
		esClient,
		embedder,
		WithRecallSize(11),
		WithFilterBuilder(func(ctx context.Context, req Request) ([]any, error) {
			calls++
			if req.Query != "query" || req.TopK != 2 {
				t.Fatalf("request passed to filter builder = %#v", req)
			}
			return filter, nil
		}),
	).Search(context.Background(), "query", WithTopK(2))
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("filter builder calls = %d, want 1", calls)
	}
	if len(esClient.calls) != 2 {
		t.Fatalf("es calls = %d, want 2", len(esClient.calls))
	}
	vectorQuery := esClient.calls[0].query.(map[string]any)
	if vectorQuery["size"] != 11 {
		t.Fatalf("vector size = %#v, want 11", vectorQuery["size"])
	}
	if got := mustFilter(t, vectorQuery["knn"].(map[string]any)["filter"]); !reflect.DeepEqual(got, filter) {
		t.Fatalf("vector filter = %#v, want %#v", got, filter)
	}
}

func TestSearcherUsesConfiguredKnnOversample(t *testing.T) {
	// 验证 knn oversample 显式配置后写入 num_candidates。
	esClient := &fakeESClient{resps: []map[string]any{hitsResponse(), hitsResponse()}}
	embedder := &fakeEmbedder{vec: []float64{0.1}}

	_, err := NewSearcher[sourceMeta](esClient, embedder, WithKnnOversample(3)).
		Search(context.Background(), "query", WithRequestRecallSize(8))
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	vectorQuery := esClient.calls[0].query.(map[string]any)
	knn := vectorQuery["knn"].(map[string]any)
	if knn["num_candidates"] != 24 {
		t.Fatalf("num_candidates = %#v, want 24", knn["num_candidates"])
	}
}

func TestSearcherFusesScoresAndDecodesSource(t *testing.T) {
	// 验证结果只包含通用字段，业务元数据通过 decoder 放入 Source，parent 内容不影响 Source 来源。
	childSrc := source("both child", "parent-both")
	childSrc["doc_name"] = "ChildDoc"
	esClient := &fakeESClient{resps: []map[string]any{
		hitsResponse(
			hit("vec-only", 0.9, source("vec only", "")),
			hit("both", 0.8, childSrc),
		),
		hitsResponse(
			hit("bm-only", 20, source("bm only", "")),
			hit("both", 10, childSrc),
		),
		hitsResponse(hit("parent-both-hit", 1, map[string]any{"content": "parent content", "doc_name": "ParentDoc"})),
	}}
	embedder := &fakeEmbedder{vec: []float64{0.1}}

	got, err := NewSearcher[sourceMeta](esClient, embedder, WithSourceDecoder[sourceMeta](decodeSourceMeta)).
		Search(context.Background(), "query", WithTopK(3))
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	gotIDs := resultIDs(got)
	wantIDs := []string{"vec-only", "bm-only", "both"}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("result ids = %#v, want %#v; results=%#v", gotIDs, wantIDs, got)
	}
	if got[0].Score != 0.6 || got[1].Score != 0.4 || got[2].Content != "parent content" {
		t.Fatalf("results = %#v, want fused scores and parent content", got)
	}
	if got[2].Source.DocName != "ChildDoc" {
		t.Fatalf("decoded source = %#v, want child source metadata", got[2].Source)
	}
}

func TestSearcherReturnsDecoderError(t *testing.T) {
	// 验证 source decoder 出错时 Search 会返回错误，避免静默丢失业务元数据。
	wantErr := errors.New("decode failed")
	esClient := &fakeESClient{resps: []map[string]any{
		hitsResponse(hit("a", 1, source("doc a", ""))),
		hitsResponse(),
	}}
	embedder := &fakeEmbedder{vec: []float64{0.1}}

	_, err := NewSearcher[sourceMeta](esClient, embedder, WithSourceDecoder[sourceMeta](func(src map[string]any) (sourceMeta, error) {
		return sourceMeta{}, wantErr
	})).Search(context.Background(), "query", WithTopK(1))
	if !errors.Is(err, wantErr) {
		t.Fatalf("Search() error = %v, want %v", err, wantErr)
	}
}

func TestSearcherUsesZeroSourceWithoutDecoder(t *testing.T) {
	// 验证未配置 source decoder 时，Result.Source 使用类型零值。
	esClient := &fakeESClient{resps: []map[string]any{
		hitsResponse(hit("a", 1, source("doc a", ""))),
		hitsResponse(),
	}}
	embedder := &fakeEmbedder{vec: []float64{0.1}}

	got, err := NewSearcher[sourceMeta](esClient, embedder).Search(context.Background(), "query", WithTopK(1))
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) != 1 || got[0].Source != (sourceMeta{}) {
		t.Fatalf("results = %#v, want zero source", got)
	}
}

func TestSearcherFiltersByMinVectorScore(t *testing.T) {
	// 验证 MinVectorScore 只过滤向量不达标的候选，保留候选继续参与 BM25 融合和 rerank。
	threshold := 0.5
	reranker := &fakeReranker{results: []RerankResult{{Index: 0, Score: 1}, {Index: 1, Score: 0.9}}}
	esClient := &fakeESClient{resps: []map[string]any{
		hitsResponse(
			hit("semantic", 0.9, source("semantic", "")),
			hit("lexical", 0.8, source("lexical", "")),
			hit("low", 0.7, source("low", "")),
		),
		hitsResponse(
			hit("bm-only", 100, source("bm only", "")),
			hit("lexical", 20, source("lexical", "")),
			hit("semantic", 10, source("semantic", "")),
		),
	}}
	embedder := &fakeEmbedder{vec: []float64{0.1}}

	got, err := NewSearcher[sourceMeta](
		esClient,
		embedder,
		WithVectorWeight(0.4),
		WithBM25Weight(0.6),
		WithReranker(reranker),
	).Search(context.Background(), "query", WithTopK(5), WithMinVectorScore(threshold))
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if gotIDs := resultIDs(got); !slices.Equal(gotIDs, []string{"lexical", "semantic"}) {
		t.Fatalf("result ids = %#v, want lexical,semantic", gotIDs)
	}
	if reranker.calls != 1 || !slices.Equal(reranker.documents, []string{"lexical", "semantic"}) {
		t.Fatalf("reranker calls/documents = %d/%#v", reranker.calls, reranker.documents)
	}
	if got[0].Score != 0.6 || got[1].Score != 0.4 {
		t.Fatalf("scores = %#v, want fused scores after vector gate", got)
	}
}

func TestSearcherFallsBackToChildContentWhenParentMissing(t *testing.T) {
	// 验证 child 指向的 parent 查不到时，会回退返回 child 自身内容。
	esClient := &fakeESClient{resps: []map[string]any{
		hitsResponse(hit("child", 1, source("child content", "missing-parent"))),
		hitsResponse(),
		hitsResponse(),
	}}
	embedder := &fakeEmbedder{vec: []float64{0.1}}

	got, err := NewSearcher[sourceMeta](esClient, embedder).Search(context.Background(), "query", WithTopK(1))
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) != 1 || got[0].Content != "child content" {
		t.Fatalf("results = %#v, want child content fallback", got)
	}
}

func TestSearcherUsesRerankerAndFallsBackOnError(t *testing.T) {
	// 验证 reranker 成功时按重排顺序返回，失败时回退融合排序。
	reranker := &fakeReranker{results: []RerankResult{{Index: 1, Score: 0.99}, {Index: 0, Score: 0.5}}}
	esClient := &fakeESClient{resps: []map[string]any{
		hitsResponse(hit("a", 2, source("doc a", "")), hit("b", 1, source("doc b", ""))),
		hitsResponse(),
	}}
	embedder := &fakeEmbedder{vec: []float64{0.1}}

	got, err := NewSearcher[sourceMeta](esClient, embedder, WithReranker(reranker)).Search(context.Background(), "query", WithTopK(2))
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if gotIDs := resultIDs(got); !slices.Equal(gotIDs, []string{"b", "a"}) {
		t.Fatalf("reranked result ids = %#v, want b,a", gotIDs)
	}
	if reranker.calls != 1 || !slices.Equal(reranker.documents, []string{"doc a", "doc b"}) {
		t.Fatalf("reranker calls/documents = %d/%#v", reranker.calls, reranker.documents)
	}

	fallbackES := &fakeESClient{resps: []map[string]any{
		hitsResponse(hit("a", 2, source("doc a", "")), hit("b", 1, source("doc b", ""))),
		hitsResponse(),
	}}
	fallback, err := NewSearcher[sourceMeta](fallbackES, embedder, WithReranker(&fakeReranker{err: errors.New("rerank failed")})).
		Search(context.Background(), "query", WithTopK(2))
	if err != nil {
		t.Fatalf("fallback Search() error = %v", err)
	}
	if gotIDs := resultIDs(fallback); !slices.Equal(gotIDs, []string{"a", "b"}) {
		t.Fatalf("fallback result ids = %#v, want fused order a,b", gotIDs)
	}
}

func TestNoopSearcherReturnsEmptyResult(t *testing.T) {
	// 验证默认空检索器不返回结果，也不产生错误。
	got, err := NoopSearcher[sourceMeta]{}.Search(context.Background(), "query")
	if err != nil {
		t.Fatalf("Search() error = %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("Search() = %#v, want nil", got)
	}
}

func hitsResponse(hits ...map[string]any) map[string]any {
	values := make([]any, 0, len(hits))
	for _, h := range hits {
		values = append(values, h)
	}
	return map[string]any{"hits": map[string]any{"hits": values}}
}

func hit(id string, score float64, src map[string]any) map[string]any {
	return map[string]any{"_id": id, "_score": score, "_source": src}
}

func source(content string, parentID string) map[string]any {
	src := map[string]any{
		"content":     content,
		"doc_name":    "Doc",
		"source_id":   "source-1",
		"source_type": "document",
		"kb_id":       "kb-1",
	}
	if parentID != "" {
		src["parent_id"] = parentID
	}
	return src
}

func resultIDs(results []Result[sourceMeta]) []string {
	ids := make([]string, 0, len(results))
	for _, result := range results {
		ids = append(ids, result.ID)
	}
	return ids
}

func mustFilter(t *testing.T, value any) []any {
	t.Helper()
	filter := value.(map[string]any)["bool"].(map[string]any)["filter"].([]any)
	return filter
}
