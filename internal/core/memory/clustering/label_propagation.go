/**
 * @Time   : 2026/6/24 23:36
 * @Author : chenyangzhao542@gmail.com
 * @File   : label_propagation.go
 **/

package clustering

import (
	"context"
	"log/slog"
	"strings"

	"github.com/boxify/api-go/internal/config"
	"github.com/boxify/api-go/internal/core/id"
	"github.com/boxify/api-go/internal/core/jsonx"
	"github.com/boxify/api-go/internal/core/llm"
	"github.com/boxify/api-go/internal/core/memory"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/util"
	"github.com/boxify/api-go/internal/xerr"
)

// 标签传播聚类引擎 (LPA)
//
// 对 Neo4j 中某用户的 Entity 节点做社区聚类:
// - 全量初始化: 所有实体初始各自一个标签, 按邻居加权投票迭代传播至收敛
// - 增量更新: 新实体按邻居投票归入已有社区或新建
//
// 加权投票权重 = 语义相似度(name_embedding 余弦) * 0.6 + 关系连接强度 * 0.4
// 迭代收敛后做社区合并 (平均向量余弦 > 阈值), 再为每个社区用 LLM 生成名称+摘要

// LabelPropagationEngine 标签传播聚类引擎
type LabelPropagationEngine struct {
	log           *slog.Logger
	c             *config.Config
	userId        string
	id            id.Generator
	llm           llm.Client
	prompt        memory.Prompter
	jsonParser    jsonx.Parser
	communityRepo repository.MemoryCommunityRepository
	memoryRepo    repository.MemoryGraphRepository
}

func NewLabelPropagationEngine(config *config.Config, idGenerator id.Generator, llm llm.Client, prompt memory.Prompter,
	jsonParser jsonx.Parser, memoryCommunityRepo repository.MemoryCommunityRepository,
	memoryGraphRepo repository.MemoryGraphRepository) *LabelPropagationEngine {
	return &LabelPropagationEngine{
		log:           xlog.Component("community_label_propagation"),
		c:             config,
		id:            idGenerator,
		llm:           llm,
		prompt:        prompt,
		jsonParser:    jsonParser,
		communityRepo: memoryCommunityRepo,
		memoryRepo:    memoryGraphRepo,
	}
}

// Invoke 无社区 → 全量
// 有社区且给了新实体 → 增量
func (e *LabelPropagationEngine) Invoke(ctx context.Context, userId string) error {
	e.log.InfoContext(ctx, "开始标签传播")

	hasCommunities, err := e.communityRepo.HasCommunities(ctx, e.userId)
	if err != nil {
		return xerr.Wrapf(err, "查询用户是否存在社区失败")
	}

	if hasCommunities {
		return e.incrementalUpdate(ctx, userId)
	} else {
		return e.fullClustering(ctx, userId)
	}

}

// fullClustering 权量聚类
func (e *LabelPropagationEngine) fullClustering(ctx context.Context, userId string) error {
	e.log.InfoContext(ctx, "开始全量标签传播")

	entities, err := e.communityRepo.ListEntityEmbedding(ctx, userId)
	if err != nil {
		return err
	}
	if len(entities) == 0 {
		return nil
	}

	entityIds := make([]string, 0, len(entities))
	embeddingById := make(map[string][]float64, len(entities))
	labelById := make(map[string]string, len(entities))
	for _, entity := range entities {
		entityIds = append(entityIds, entity.ID)
		embeddingById[entity.ID] = entity.NameEmbedding
		labelById[entity.ID] = entity.ID
	}

	neighborsCache, err := e.communityRepo.ListNeighborsForVote(ctx, userId, entityIds)
	if err != nil {
		return err
	}

	for _ = range e.c.Memory.CommunityClusteringMaxIterations {
		changed := 0
		for _, eid := range entityIds {
			neighbors := neighborsCache[eid]
			newLabel := e.weightVote(neighbors, embeddingById[eid], labelById)
			if newLabel != "" && newLabel != labelById[eid] {
				labelById[eid] = newLabel
				changed += 1
			}
		}
		if changed == 0 {
			break
		}
	}

	// 对社区标签进行去重
	communityUniqueIds := make([]string, 0)
	seen := make(map[string]struct{})
	for _, cid := range labelById {
		if _, exist := seen[cid]; !exist {
			seen[cid] = struct{}{}
			communityUniqueIds = append(communityUniqueIds, cid)
		}
	}

	// 写标签 → 社区节点 + 归属
	err = e.flushLabels(ctx, userId, communityUniqueIds, labelById)
	if err != nil {
		return err
	}

	// 合并相似社区
	finalCommunityIds, err := e.mergeCommunities(ctx, userId, communityUniqueIds)
	if err != nil {
		return err
	}
	// 清理孤立社区
	err = e.communityRepo.PruneEmptyCommunity(ctx, userId)
	if err != nil {
		return err
	}

	// 生成元数据
	err = e.generateMetadata(ctx, userId, finalCommunityIds)
	if err != nil {
		return err
	}

	e.log.InfoContext(ctx, "全量聚类完成",
		slog.String("user", userId),
		slog.Int("社区数", len(finalCommunityIds)),
	)
	return nil
}

// incrementalUpdate 增量更新
func (e *LabelPropagationEngine) incrementalUpdate(ctx context.Context, userId string) error {
	e.log.InfoContext(ctx, "开始增量标签传播")
	// TODO
	return nil
}

// weightVote
// 邻居按 语义相似度*0.6 + 关系连接*0.4 加权投票, 返回得票最高的社区标签
func (e *LabelPropagationEngine) weightVote(neighbors []*memory.EntityNeighborForVote, embedding []float64, labelById map[string]string) string {
	votes := make(map[string]float64, len(neighbors))

	// 计算每一个标签的投票得分
	for _, nb := range neighbors {
		communityId := labelById[nb.ID]
		if communityId == "" {
			communityId = nb.CommunityID
		}

		if communityId == "" {
			continue
		}

		sem := util.Cosine(embedding, nb.NameEmbedding)
		weight := e.c.Memory.CommunityVoteSemWeight*sem + e.c.Memory.CommunityVoteRelWeight*1.0 // 关系边即记一次连接强度
		votes[communityId] += weight
	}

	// 获取最大投票的标签
	var res string
	curMax := 0.0
	for label, vote := range votes {
		if vote > curMax {
			curMax = vote
			res = label
		}
	}

	return res
}

// flushLabels 将标签持久化到图谱中
// 写标签 → 社区节点 + 归属
func (e *LabelPropagationEngine) flushLabels(ctx context.Context, userId string, communityUniqueIds []string, labelById map[string]string) error {
	// 保存/更新社区
	err := e.communityRepo.UpsertCommunities(ctx, userId, communityUniqueIds)
	if err != nil {
		return err
	}

	// 更新实体归属社区
	entityIds := make([]string, 0, len(labelById))
	communityIds := make([]string, 0, len(labelById))
	for entityID, communityID := range labelById {
		entityIds = append(entityIds, entityID)
		communityIds = append(communityIds, communityID)
	}
	err = e.communityRepo.AssignEntityToCommunity(ctx, userId, entityIds, communityIds)
	if err != nil {
		return err
	}

	// 刷新社区成员
	_, err = e.communityRepo.RefreshCommunityMemberCount(ctx, userId, communityUniqueIds)
	if err != nil {
		return err
	}

	return nil
}

// mergeCommunities 合并相似社区
// 平均向量余弦 > 阈值的社区合并, 保留成员多的一方. 返回存活社区 id
func (e *LabelPropagationEngine) mergeCommunities(ctx context.Context, userId string, communityIds []string) ([]string, error) {
	avgEmbedding := make(map[string][]float64)
	sizes := make(map[string]int)
	membersByCommunityId := make(map[string][]*memory.CommunityMember)

	members, err := e.communityRepo.GetCommunityMembers(ctx, userId, communityIds)
	if err != nil {
		return nil, err
	}

	for i, cid := range communityIds {
		curMembers := members[i]

		membersByCommunityId[cid] = curMembers
		sizes[cid] = len(curMembers)
		embeddings := make([][]float64, 0, sizes[cid])

		for _, member := range curMembers {
			embeddings = append(embeddings, member.NameEmbedding)
		}

		if len(embeddings) > 0 {
			avgEmbedding[cid] = util.MeanVector(embeddings)
		}
	}

	mergeInfo := make(map[string]string)

	root := func(x string) string {
		for v, ok := mergeInfo[x]; ok; {
			x = v
		}
		return x
	}

	needRefreshCommunities := make([]string, 0)
	needUpdateEntities := make([]string, 0)
	needUpdateCommunities := make([]string, 0)
	for i := 0; i < len(communityIds)-1; i++ {
		for j := i + 1; j < len(communityIds); j++ {
			r1, r2 := root(communityIds[i]), root(communityIds[j])
			if r1 == r2 {
				continue
			}

			// 是否到达可合并的阈值
			if util.Cosine(avgEmbedding[r1], avgEmbedding[r2]) <= e.c.Memory.CommunityMergeThreshold {
				continue
			}

			keep := r1
			dissolve := r2
			if sizes[r1] < sizes[r2] {
				keep, dissolve = dissolve, keep
			}

			mergeInfo[dissolve] = keep

			dissolveMembers := membersByCommunityId[dissolve]
			for _, m := range dissolveMembers {
				needUpdateEntities = append(needUpdateEntities, m.ID)
				needUpdateCommunities = append(needUpdateCommunities, keep)
			}

			sizes[keep] = sizes[keep] + sizes[dissolve]
			sizes[dissolve] = 0

			needRefreshCommunities = append(needRefreshCommunities, keep, dissolve)
		}
	}

	// 更新实体社区
	err = e.communityRepo.AssignEntityToCommunity(ctx, userId, needUpdateEntities, needUpdateCommunities)
	if err != nil {
		return nil, err
	}

	// 刷新实现成员数量
	_, err = e.communityRepo.RefreshCommunityMemberCount(ctx, userId, needRefreshCommunities)
	if err != nil {
		return nil, err
	}

	finalCommunities := make([]string, 0)
	for _, cid := range mergeInfo {
		if _, exist := mergeInfo[cid]; !exist {
			finalCommunities = append(finalCommunities, cid)
		}
	}

	return finalCommunities, nil
}

// 生成元数据并持久化
func (e *LabelPropagationEngine) generateMetadata(ctx context.Context, userId string, communityIds []string) error {
	members, err := e.communityRepo.GetCommunityMembers(ctx, userId, communityIds)
	if err != nil {
		return err
	}

	updateInfo := make([]*memory.UpdateCommunityMetaItem, 0)

	// 没有 llm 兜底
	if e.llm == nil {
		for i, cid := range communityIds {
			curMembers := members[i]
			names := make([]string, 0, len(curMembers))
			for _, member := range curMembers {
				if member.Name != "" {
					names = append(names, member.Name)
				}
			}
			name := "未命名社区"
			if len(names) > 0 {
				name = strings.Join(util.Head(names, 3), "、")
			}
			summary := "包含实体: " + strings.Join(util.Head(names, 10), "、")
			updateInfo = append(updateInfo, &memory.UpdateCommunityMetaItem{
				CommunityId: cid,
				Name:        name,
				Summary:     summary,
			})
		}
	} else {
		for i, cid := range communityIds {
			curMembers := util.Head(members[i], 20)

			promptMembers := make([]*memory.CommunityMemberPromptInput, 0, len(curMembers))
			for _, mem := range curMembers {
				promptMembers = append(promptMembers, &memory.CommunityMemberPromptInput{
					Name:        mem.Name,
					Description: mem.Description,
				})
			}

			promptText, err := e.prompt.GenerateCommunityMetadata(&memory.CommunityMetadataPromptInput{
				Members: promptMembers,
			})
			if err != nil {
				e.log.InfoContext(ctx, "解析抽取三元组提示词失败")
				continue
			}

			response, err := e.llm.Invoke(ctx, []*llm.Message{
				llm.UserMessage(promptText),
			})
			if err != nil {
				return xerr.Wrap(err, "调用大语言模型失败")
			}

			metadata, err := jsonx.Parse[*memory.CommunityMetadata](e.jsonParser, response)
			if err != nil {
				return xerr.Wrap(err, "解析llm输出失败")
			}

			updateInfo = append(updateInfo, &memory.UpdateCommunityMetaItem{
				CommunityId: cid,
				Name:        metadata.Name,
				Summary:     metadata.Summary,
			})
		}
	}

	err = e.communityRepo.UpdateCommunityMetadata(ctx, userId, updateInfo)
	if err != nil {
		return err
	}

	return nil
}
