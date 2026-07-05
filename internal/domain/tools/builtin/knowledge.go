package builtin

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	corellm "github.com/boxify/api-go/internal/core/llm"
	ragsearch "github.com/boxify/api-go/internal/core/rag/search"
	coretool "github.com/boxify/api-go/internal/core/tool"
	"github.com/boxify/api-go/internal/domain/types"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/util"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

const (
	defaultKnowledgeSearchTopK = 5
	maxKnowledgeSearchTopK     = 20
	tagSimilarityThreshold     = 0.78
	maxMatchesPerRequestedTag  = 3
)

type knowledgeSearchRequest struct {
	Query string
	TopK  int
	Tags  []string
}

type knowledgeSearchOutput struct {
	KBIDs         []uuid.UUID
	Results       []knowledgeSearchResult
	TagResolution knowledgeTagResolution
}

type knowledgeSearchResult struct {
	ChunkID     uuid.UUID
	Content     string
	DocName     string
	SourceID    uuid.UUID
	SourceType  string
	KBID        *uuid.UUID
	Score       float64
	RerankScore *float64
}

type knowledgeTagResolution struct {
	Requested     []string
	Matched       []string
	Unmatched     []string
	FilterApplied bool
	Error         string
}

func newKnowledgeSearchTool(svcCtx *svc.ServiceContext) coretool.Tool {
	return coretool.NewFuncTool(coretool.Descriptor{
		Name:        ToolKnowledgeSearch,
		Description: "在当前上下文授权的知识库范围内检索相关内容。",
		Schema: coretool.Schema{
			Parameters: coretool.ParametersSchema{
				Type: "object",
				Properties: map[string]coretool.PropertySchema{
					"query": {
						"type":        "string",
						"description": "检索关键词。",
					},
					"top_k": {
						"type":        "integer",
						"description": "可选的返回结果数量，默认 5，取值范围 1 到 20。",
						"minimum":     1,
						"maximum":     20,
					},
					"tags": {
						"type":        "array",
						"description": "可选的文档标签名称列表。",
						"items": map[string]any{
							"type": "string",
						},
					},
				},
				Required:             []string{"query"},
				AdditionalProperties: false,
			},
		},
	}, func(ctx context.Context, input coretool.Input) (coretool.Output, error) {
		req, err := knowledgeSearchRequestFromInput(input)
		if err != nil {
			return coretool.Output{}, err
		}
		out, err := searchKnowledge(ctx, svcCtx, req)
		if err != nil {
			return coretool.Output{}, err
		}
		return knowledgeSearchToolOutput(req, out), nil
	})
}

// searchKnowledge 检索知识库
func searchKnowledge(ctx context.Context, svcCtx *svc.ServiceContext, req *knowledgeSearchRequest) (*knowledgeSearchOutput, error) {
	if req == nil || strings.TrimSpace(req.Query) == "" {
		return nil, xerr.BadRequest("检索关键词不能为空")
	}
	if svcCtx == nil || svcCtx.RAGSearcher == nil || svcCtx.KnowledgeBaseRepo == nil {
		return nil, xerr.Internal("知识库检索依赖未初始化", nil)
	}
	userID, err := util.UserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	kbIDs, err := util.KnowledgeBaseIDsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateKnowledgeBases(ctx, svcCtx, userID, kbIDs); err != nil {
		return nil, err
	}
	llmClient, err := svc.EmbeddingClient(ctx, svcCtx, userID)
	if err != nil {
		return nil, err
	}
	tagResolution := resolveKnowledgeTags(ctx, svcCtx, llmClient, userID, req.Tags)
	results, err := svcCtx.RAGSearcher.Search(ctx, req.Query,
		ragsearch.WithTopK(req.TopK),
		ragsearch.WithFilters(knowledgeSearchFilters(userID, kbIDs, tagResolution.Matched)),
		ragsearch.WithInputEmbedder(llmClient),
	)
	if err != nil {
		return nil, err
	}
	out := make([]knowledgeSearchResult, 0, len(results))
	for _, item := range results {
		out = append(out, knowledgeSearchResult{
			ChunkID:     item.Source.ChunkID,
			Content:     item.Content,
			DocName:     item.Source.DocName,
			SourceID:    item.Source.DocumentID,
			SourceType:  item.Source.SourceType,
			KBID:        item.Source.KBID,
			Score:       item.Score,
			RerankScore: item.RerankScore,
		})
	}
	return &knowledgeSearchOutput{KBIDs: kbIDs, Results: out, TagResolution: tagResolution}, nil
}

// validateKnowledgeBases 验证知识库
func validateKnowledgeBases(ctx context.Context, svcCtx *svc.ServiceContext, userID uuid.UUID, kbIDs []uuid.UUID) error {
	for _, kbID := range kbIDs {
		if _, err := svcCtx.KnowledgeBaseRepo.FindByID(ctx, userID, kbID); err != nil {
			return err
		}
	}
	return nil
}

// knowledgeSearchRequestFromInput 提取工具输入中的检索请求
func knowledgeSearchRequestFromInput(input coretool.Input) (*knowledgeSearchRequest, error) {
	query, err := requiredToolString(input, "query")
	if err != nil {
		return nil, err
	}
	topK, err := optionalKnowledgeTopK(input)
	if err != nil {
		return nil, err
	}
	tags, err := optionalToolStringList(input, "tags")
	if err != nil {
		return nil, err
	}
	return &knowledgeSearchRequest{Query: query, TopK: topK, Tags: tags}, nil
}

func requiredToolString(input coretool.Input, key string) (string, error) {
	if input == nil {
		return "", fmt.Errorf("%s is required", key)
	}
	raw, ok := input[key]
	if !ok || raw == nil {
		return "", fmt.Errorf("%s is required", key)
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func optionalKnowledgeTopK(input coretool.Input) (int, error) {
	if input == nil {
		return defaultKnowledgeSearchTopK, nil
	}
	raw, ok := input["top_k"]
	if !ok || raw == nil {
		return defaultKnowledgeSearchTopK, nil
	}
	value, err := intFromAny(raw)
	if err != nil {
		return 0, fmt.Errorf("top_k must be an integer")
	}
	if value < 1 || value > maxKnowledgeSearchTopK {
		return 0, fmt.Errorf("top_k must be between 1 and %d", maxKnowledgeSearchTopK)
	}
	return value, nil
}

func intFromAny(raw any) (int, error) {
	switch value := raw.(type) {
	case int:
		return value, nil
	case int64:
		return int(value), nil
	case float64:
		if math.Trunc(value) != value {
			return 0, fmt.Errorf("not integer")
		}
		return int(value), nil
	case interface{ String() string }:
		parsed, err := strconv.Atoi(value.String())
		if err != nil {
			return 0, err
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unsupported number type %T", raw)
	}
}

func optionalToolStringList(input coretool.Input, key string) ([]string, error) {
	if input == nil {
		return nil, nil
	}
	raw, ok := input[key]
	if !ok || raw == nil {
		return nil, nil
	}
	switch values := raw.(type) {
	case []string:
		return cleanSearchTags(values), nil
	case []any:
		out := make([]string, 0, len(values))
		for _, item := range values {
			value, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s must contain only strings", key)
			}
			if value = strings.TrimSpace(value); value != "" {
				out = append(out, value)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%s must be an array of strings", key)
	}
}

func knowledgeSearchToolOutput(req *knowledgeSearchRequest, out *knowledgeSearchOutput) coretool.Output {
	if out == nil {
		out = &knowledgeSearchOutput{}
	}
	metadata := map[string]any{
		"query":              req.Query,
		"top_k":              req.TopK,
		"kb_ids":             uuidStrings(out.KBIDs),
		"count":              len(out.Results),
		"results":            knowledgeSearchResultMetadata(out.Results),
		"requested_tags":     out.TagResolution.Requested,
		"matched_tags":       out.TagResolution.Matched,
		"unmatched_tags":     out.TagResolution.Unmatched,
		"tag_filter_applied": out.TagResolution.FilterApplied,
	}
	if out.TagResolution.Error != "" {
		metadata["tag_resolution_error"] = out.TagResolution.Error
	}
	return coretool.Output{
		Text:     knowledgeSearchText(out.Results),
		Metadata: metadata,
	}
}

func knowledgeSearchText(results []knowledgeSearchResult) string {
	if len(results) == 0 {
		return "No knowledge base results found."
	}
	var builder strings.Builder
	for index, result := range results {
		if index > 0 {
			builder.WriteString("\n\n")
		}
		title := strings.TrimSpace(result.DocName)
		if title == "" {
			title = result.SourceID.String()
		}
		builder.WriteString(fmt.Sprintf("%d. %s\n%s", index+1, title, strings.TrimSpace(result.Content)))
	}
	return builder.String()
}

func knowledgeSearchResultMetadata(results []knowledgeSearchResult) []map[string]any {
	out := make([]map[string]any, 0, len(results))
	for _, result := range results {
		item := map[string]any{
			"chunk_id":    result.ChunkID.String(),
			"content":     result.Content,
			"doc_name":    result.DocName,
			"source_id":   result.SourceID.String(),
			"source_type": result.SourceType,
			"score":       result.Score,
		}
		if result.KBID != nil {
			item["kb_id"] = result.KBID.String()
		}
		if result.RerankScore != nil {
			item["rerank_score"] = *result.RerankScore
		}
		out = append(out, item)
	}
	return out
}

func knowledgeSearchFilters(userID uuid.UUID, kbIDs []uuid.UUID, tags []string) []any {
	filters := []any{
		map[string]any{"term": map[string]any{"user_id": userID.String()}},
		map[string]any{"terms": map[string]any{"kb_id": uuidStrings(kbIDs)}},
	}
	cleanTags := cleanSearchTags(tags)
	if len(cleanTags) != 0 {
		filters = append(filters, map[string]any{"terms": map[string]any{"tags": cleanTags}})
	}
	return filters
}

// resolveKnowledgeTags 解析知识库标签
func resolveKnowledgeTags(ctx context.Context, svcCtx *svc.ServiceContext, llmClient corellm.Client, userID uuid.UUID, tags []string) knowledgeTagResolution {
	requested := cleanSearchTags(tags)
	resolution := knowledgeTagResolution{Requested: requested}
	if len(requested) == 0 {
		return resolution
	}
	if svcCtx == nil || svcCtx.TagRepo == nil {
		resolution.Unmatched = append([]string(nil), requested...)
		resolution.Error = "标签仓储未初始化"
		return resolution
	}
	// 获取用户标签
	rows, err := svcCtx.TagRepo.ListByScope(ctx, userID, string(types.TagScopeDocument))
	if err != nil {
		resolution.Unmatched = append([]string(nil), requested...)
		resolution.Error = err.Error()
		return resolution
	}
	// 提取标签名称
	existing := tagNamesFromRows(rows)
	if len(existing) == 0 {
		resolution.Unmatched = append([]string(nil), requested...)
		return resolution
	}

	// 匹配标签
	matched := map[string]struct{}{}
	remaining := make([]string, 0, len(requested))
	normalizedExisting := normalizedTagNames(existing)
	for _, tag := range requested {
		if existingTag, ok := normalizedExisting[normalizeTagName(tag)]; ok {
			addMatchedTag(matched, &resolution, existingTag)
			continue
		}
		remaining = append(remaining, tag)
	}
	if len(remaining) != 0 {
		resolveApproximateKnowledgeTags(ctx, llmClient, svcCtx.RAGSearcher.EmbeddingDim, remaining, existing, matched, &resolution)
	}
	resolution.FilterApplied = len(resolution.Matched) != 0
	return resolution
}

// resolveApproximateKnowledgeTags 解析近似知识库标签
func resolveApproximateKnowledgeTags(ctx context.Context, llmClient corellm.Client, dimensions int, requested []string, existing []string, seen map[string]struct{}, resolution *knowledgeTagResolution) {
	if llmClient == nil || resolution == nil {
		if resolution != nil {
			resolution.Unmatched = append(resolution.Unmatched, requested...)
		}
		return
	}
	texts := make([]string, 0, len(requested)+len(existing))
	texts = append(texts, requested...)
	texts = append(texts, existing...)
	vectors, err := llmClient.Embed(ctx, texts, dimensions)
	if err != nil {
		resolution.Unmatched = append(resolution.Unmatched, requested...)
		resolution.Error = err.Error()
		return
	}
	if len(vectors) < len(texts) {
		resolution.Unmatched = append(resolution.Unmatched, requested...)
		resolution.Error = "标签向量数量不匹配"
		return
	}
	// 提取已知标签的向量
	existingVectors := vectors[len(requested):]
	// 遍历请求的标签
	for index, tag := range requested {
		matches := topSimilarTags(vectors[index], existing, existingVectors, seen)
		if len(matches) == 0 {
			resolution.Unmatched = append(resolution.Unmatched, tag)
			continue
		}
		for _, match := range matches {
			addMatchedTag(seen, resolution, match)
		}
	}
}

// topSimilarTags 找出最相似的标签
func topSimilarTags(queryVector []float64, existing []string, existingVectors [][]float64, seen map[string]struct{}) []string {
	type candidate struct {
		name  string
		score float64
	}
	candidates := make([]candidate, 0, len(existing))
	for index, name := range existing {
		if _, ok := seen[name]; ok {
			continue
		}
		if index >= len(existingVectors) {
			break
		}
		score := cosineSimilarity(queryVector, existingVectors[index])
		if score >= tagSimilarityThreshold {
			candidates = append(candidates, candidate{name: name, score: score})
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	for i := 0; i < len(candidates)-1; i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[i].score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}
	limit := maxMatchesPerRequestedTag
	if len(candidates) < limit {
		limit = len(candidates)
	}
	out := make([]string, 0, limit)
	for _, item := range candidates[:limit] {
		out = append(out, item.name)
	}
	return out
}

func cosineSimilarity(left []float64, right []float64) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	var dot float64
	var leftNorm float64
	var rightNorm float64
	for i := 0; i < limit; i++ {
		dot += left[i] * right[i]
		leftNorm += left[i] * left[i]
		rightNorm += right[i] * right[i]
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}

func addMatchedTag(seen map[string]struct{}, resolution *knowledgeTagResolution, tag string) {
	if tag == "" || resolution == nil {
		return
	}
	if _, ok := seen[tag]; ok {
		return
	}
	seen[tag] = struct{}{}
	resolution.Matched = append(resolution.Matched, tag)
}

func tagNamesFromRows(rows []*models.Tag) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		if name := strings.TrimSpace(row.Name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func normalizedTagNames(tags []string) map[string]string {
	out := make(map[string]string, len(tags))
	for _, tag := range tags {
		normalized := normalizeTagName(tag)
		if normalized == "" {
			continue
		}
		if _, ok := out[normalized]; !ok {
			out[normalized] = tag
		}
	}
	return out
}

func normalizeTagName(tag string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(tag)), ""))
}

func uuidStrings(values []uuid.UUID) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != uuid.Nil {
			out = append(out, value.String())
		}
	}
	return out
}

func cleanSearchTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if value := strings.TrimSpace(tag); value != "" {
			out = append(out, value)
		}
	}
	return out
}
