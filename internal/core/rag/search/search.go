package search

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/boxify/api-go/internal/core/valuex"
)

type Searcher[T any] struct {
	Options
	es            ESClient
	embedder      Embedder
	sourceDecoder SourceDecoder[T]
}

type NoopSearcher[T any] struct{}

func NewSearcher[T any](esClient ESClient, embedder Embedder, opts ...Option) *Searcher[T] {
	searcher := &Searcher[T]{
		Options: Options{
			Index:         defaultIndex,
			EmbeddingDim:  defaultEmbeddingDim,
			RecallSize:    defaultRecallSize,
			VectorWeight:  defaultVectorWeight,
			BM25Weight:    defaultBM25Weight,
			FilterBuilder: defaultFilterBuilder,
		},
		es:       esClient,
		embedder: embedder,
	}
	for _, opt := range opts {
		opt(&searcher.Options)
	}
	if decoder, ok := searcher.Options.sourceDecoder.(SourceDecoder[T]); ok {
		searcher.sourceDecoder = decoder
	}
	return searcher
}

func (NoopSearcher[T]) Search(ctx context.Context, query string, opts ...RequestOption) ([]Result[T], error) {
	return nil, nil
}

// Search 执行混合检索：向量召回 + BM25 召回，然后按权重融合排序。
// 当 MinVectorScore 存在时，先过滤向量相关度不足的候选，再继续融合和重排。
func (s *Searcher[T]) Search(ctx context.Context, query string, opts ...RequestOption) ([]Result[T], error) {
	if s == nil || s.es == nil {
		return nil, errors.New("rag search ES client is nil")
	}
	if s.embedder == nil {
		return nil, errors.New("rag search embedder is nil")
	}

	req := Request{Query: strings.TrimSpace(query)}
	for _, opt := range opts {
		opt(&req)
	}

	topK := req.TopK
	if topK <= 0 {
		topK = defaultTopK
	}
	recallSize := s.RecallSize
	if req.RecallSize > 0 {
		recallSize = req.RecallSize
	}

	baseFilter, err := s.FilterBuilder(ctx, req)
	if err != nil {
		return nil, err
	}

	queryVector, err := s.embedder.EmbedOne(ctx, req.Query, s.EmbeddingDim)
	if err != nil {
		return nil, err
	}

	knnResp, err := s.es.Search(ctx, s.Index, vectorQuery(queryVector, recallSize, s.KnnOversample, baseFilter))
	if err != nil {
		return nil, err
	}
	bm25Resp, err := s.es.Search(ctx, s.Index, bm25Query(req.Query, recallSize, baseFilter))
	if err != nil {
		return nil, err
	}

	hits := map[string]map[string]any{}
	vecScores := map[string]float64{}
	bmScores := map[string]float64{}
	collectHits(knnResp, hits, vecScores)
	collectHits(bm25Resp, hits, bmScores)

	if req.MinVectorScore != nil {
		hits, vecScores, bmScores = filterByMinVectorScore(hits, vecScores, bmScores, *req.MinVectorScore)
		if len(hits) == 0 {
			return nil, nil
		}
	}

	fused := fuseScores(hits, Normalize(vecScores), Normalize(bmScores), s.VectorWeight, s.BM25Weight)
	candidateIDs := rankedIDs(fused, max(topK, recallSize))
	if s.Reranker != nil && len(candidateIDs) != 0 {
		if reranked, err := s.rerank(ctx, req.Query, candidateIDs, hits, topK); err == nil {
			candidateIDs = reranked
		}
	}

	if len(candidateIDs) > topK {
		candidateIDs = candidateIDs[:topK]
	}
	return s.resultsForIDs(ctx, candidateIDs, hits, fused)
}

// rerank 把候选 chunk id 映射成文档内容交给 reranker，再把返回下标映射回 chunk id。
func (s *Searcher[T]) rerank(ctx context.Context, query string, candidateIDs []string, hits map[string]map[string]any, topK int) ([]string, error) {
	docs := make([]string, 0, len(candidateIDs))
	for _, id := range candidateIDs {
		docs = append(docs, valuex.String(hits[id]["content"]))
	}
	reranked, err := s.Reranker.Rerank(ctx, query, docs, topK)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(reranked))
	seen := map[int]struct{}{}
	for _, item := range reranked {
		if item.Index < 0 || item.Index >= len(candidateIDs) {
			continue
		}
		if _, ok := seen[item.Index]; ok {
			continue
		}
		seen[item.Index] = struct{}{}
		ids = append(ids, candidateIDs[item.Index])
	}
	return ids, nil
}

// resultsForIDs 组装检索结果；命中 child 时优先返回 parent 内容，给上层提供更完整上下文。
func (s *Searcher[T]) resultsForIDs(ctx context.Context, ids []string, hits map[string]map[string]any, scores map[string]float64) ([]Result[T], error) {
	results := make([]Result[T], 0, len(ids))
	for _, id := range ids {
		src := hits[id]
		content, err := s.resolveParentContent(ctx, src)
		if err != nil {
			content = valuex.String(src["content"])
		}
		var source T
		if s.sourceDecoder != nil {
			source, err = s.sourceDecoder(src)
			if err != nil {
				return nil, err
			}
		}
		results = append(results, Result[T]{
			ID:      id,
			Content: content,
			Score:   round4(scores[id]),
			Source:  source,
		})
	}
	return results, nil
}

// resolveParentContent 查询 parent chunk 内容；业务隔离过滤由外层调用方负责提供。
func (s *Searcher[T]) resolveParentContent(ctx context.Context, src map[string]any) (string, error) {
	parentID := valuex.String(src["parent_id"])
	if parentID == "" {
		return valuex.String(src["content"]), nil
	}
	resp, err := s.es.Search(ctx, s.Index, map[string]any{
		"size": 1,
		"query": map[string]any{
			"bool": map[string]any{
				"filter": []any{
					map[string]any{"term": map[string]any{"chunk_id": parentID}},
				},
			},
		},
	})
	if err != nil {
		return "", err
	}
	hits := responseHits(resp)
	if len(hits) == 0 {
		return valuex.String(src["content"]), nil
	}
	parentSrc, _ := hits[0]["_source"].(map[string]any)
	content := valuex.String(parentSrc["content"])
	if content == "" {
		return valuex.String(src["content"]), nil
	}
	return content, nil
}

// filterByMinVectorScore 使用 ES cosine 原始相关度过滤候选。
// ES cosine knn 的 _score = (1 + cos) / 2，这里需要还原成真实 cosine。
func filterByMinVectorScore(hits map[string]map[string]any, vecScores map[string]float64, bmScores map[string]float64, minScore float64) (map[string]map[string]any, map[string]float64, map[string]float64) {
	filteredHits := make(map[string]map[string]any, len(hits))
	filteredVecScores := make(map[string]float64, len(vecScores))
	filteredBMScores := make(map[string]float64, len(bmScores))
	for id, src := range hits {
		score, ok := vecScores[id]
		if !ok {
			continue
		}
		cos := 2*score - 1
		if cos < minScore {
			continue
		}
		filteredHits[id] = src
		filteredVecScores[id] = score
		if bmScore, ok := bmScores[id]; ok {
			filteredBMScores[id] = bmScore
		}
	}
	return filteredHits, filteredVecScores, filteredBMScores
}

// Normalize 把同一路召回分数归一到 0-1，便于向量分数和 BM25 分数融合。
func Normalize(scores map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(scores))
	if len(scores) == 0 {
		return out
	}
	var lo, hi float64
	first := true
	for _, score := range scores {
		if first {
			lo, hi = score, score
			first = false
			continue
		}
		lo = math.Min(lo, score)
		hi = math.Max(hi, score)
	}
	if hi-lo < 1e-9 {
		for id := range scores {
			out[id] = 1
		}
		return out
	}
	for id, score := range scores {
		out[id] = (score - lo) / (hi - lo)
	}
	return out
}

// defaultFilterBuilder 只透传调用方提供的 ES filter，避免核心包绑定业务字段。
func defaultFilterBuilder(ctx context.Context, req Request) ([]any, error) {
	return req.Filters, nil
}

// vectorQuery 构造 ES knn 查询。
// knnOversample 为 0 时不写 num_candidates，交给 ES 默认策略处理。
func vectorQuery(queryVector []float64, recallSize int, knnOversample int, baseFilter []any) map[string]any {
	boolFilter := map[string]any{"bool": map[string]any{"filter": baseFilter}}
	knn := map[string]any{
		"field":        "vector",
		"query_vector": queryVector,
		"k":            recallSize,
		"filter":       boolFilter,
	}
	effectiveCandidates := effectiveKnnCandidates(recallSize, knnOversample)
	if effectiveCandidates > 0 {
		knn["num_candidates"] = effectiveCandidates
	}
	return map[string]any{
		"size":  recallSize,
		"query": boolFilter,
		"knn":   knn,
	}
}

func effectiveKnnCandidates(recallSize int, knnOversample int) int {
	if knnOversample <= 0 {
		return 0
	}
	candidates := recallSize * knnOversample
	if candidates < recallSize {
		return recallSize
	}
	return candidates
}

// bm25Query 构造 ES 文本匹配查询，与向量召回共享同一组 filter。
func bm25Query(query string, recallSize int, baseFilter []any) map[string]any {
	return map[string]any{
		"size": recallSize,
		"query": map[string]any{
			"bool": map[string]any{
				"must":   []any{map[string]any{"match": map[string]any{"content": query}}},
				"filter": baseFilter,
			},
		},
	}
}

// fuseScores 向量分数和 BM25 分数融合。
func fuseScores(hits map[string]map[string]any, vecScores map[string]float64, bmScores map[string]float64, vectorWeight float64, bm25Weight float64) map[string]float64 {
	fused := make(map[string]float64, len(hits))
	for id := range hits {
		fused[id] = vectorWeight*vecScores[id] + bm25Weight*bmScores[id]
	}
	return fused
}

// rankedIDs 按分数从高到低排序，分数相同按 id 升序。
func rankedIDs(scores map[string]float64, limit int) []string {
	ids := make([]string, 0, len(scores))
	for id := range scores {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if scores[ids[i]] == scores[ids[j]] {
			return ids[i] < ids[j]
		}
		return scores[ids[i]] > scores[ids[j]]
	})
	if limit > 0 && len(ids) > limit {
		return ids[:limit]
	}
	return ids
}

// collectHits 收集命中结果。
func collectHits(resp map[string]any, hits map[string]map[string]any, scores map[string]float64) {
	for _, hit := range responseHits(resp) {
		id := valuex.String(hit["_id"])
		if id == "" {
			continue
		}
		src, _ := hit["_source"].(map[string]any)
		if src == nil {
			src = map[string]any{}
		}
		hits[id] = src
		scores[id] = valuex.Float(hit["_score"])
	}
}

// responseHits 提取 ES 查询结果中的 hits 部分。
func responseHits(resp map[string]any) []map[string]any {
	hitsObj, _ := resp["hits"].(map[string]any)
	rawHits, _ := hitsObj["hits"].([]any)
	out := make([]map[string]any, 0, len(rawHits))
	for _, raw := range rawHits {
		hit, ok := raw.(map[string]any)
		if ok {
			out = append(out, hit)
		}
	}
	return out
}

func round4(value float64) float64 {
	return math.Round(value*10000) / 10000
}
