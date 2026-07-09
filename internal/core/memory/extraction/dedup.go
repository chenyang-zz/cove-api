/**
 * @Time   : 2026/6/23 11:38
 * @Author : chenyangzhao542@gmail.com
 * @File   : dedup.go
 **/

package extraction

import (
	"context"
	"slices"
	"strings"

	"github.com/boxify/api-go/internal/core/jsonx"
	"github.com/boxify/api-go/internal/core/llm"
	"github.com/boxify/api-go/internal/core/memory"
	"github.com/boxify/api-go/internal/util"
	"github.com/boxify/api-go/internal/xerr"
)

// dedup 去重
// 第一层 dedupWithinBatch
// 第二层 mergeWithGraph
func (o *MemoryOrchestrator) dedup(ctx context.Context, entities []*memory.EntityNode) ([]*memory.EntityNode, func(string) string, error) {
	// 批内去重 → id 重定向
	deduped, batchRedirect, err := o.dedupWithinBatch(ctx, entities)
	if err != nil {
		return nil, nil, err
	}

	// 与图谱已有实体二层融合
	finalEntities, graphRedirect, err := o.mergeWithGraph(ctx, deduped)
	if err != nil {
		return nil, nil, err
	}

	// 合并两层重定向: 原始 EntityNode.id → 最终落库 id
	var resolve = func(entityId string) string {
		if batchV, ok := batchRedirect[entityId]; ok {
			if graphV, ok := graphRedirect[batchV]; ok {
				return graphV
			}
			return batchV
		}
		return entityId
	}

	return finalEntities, resolve, nil
}

// dedupWithinBatch 第一层: 批内去重. 返回(去重后实体,, id 重定向表 old_id->canon_id)
func (o *MemoryOrchestrator) dedupWithinBatch(ctx context.Context, entities []*memory.EntityNode) ([]*memory.EntityNode, map[string]string, error) {
	redirect := make(map[string]string)
	if len(entities) < 2 {
		return entities, redirect, nil
	}

	//同名同类型直接合并(不必问 LLM)
	type entityKey struct {
		name string
		t    string
	}
	byKey := make(map[entityKey]*memory.EntityNode)
	survivors := make([]*memory.EntityNode, 0, len(entities))
	for _, entity := range entities {
		key := entityKey{
			name: strings.TrimSpace(strings.ToLower(entity.Name)),
			t:    entity.Type,
		}
		if canon, ok := byKey[key]; ok {
			o.mergeInto(canon, entity)
			redirect[entity.ID] = canon.ID
		} else {
			byKey[key] = entity
			survivors = append(survivors, entity)
		}
	}

	//候选对: 同类型 + 名称相似/向量相似/包含, 交给 LLM 判定
	n := len(survivors)
	for i := 0; i < n-1; i++ {
		left := survivors[i]
		if _, ok := redirect[left.ID]; ok {
			continue
		}
		for j := i + 1; j < n; j++ {
			right := survivors[j]
			if _, ok := redirect[right.ID]; ok || left.Type != right.Type {
				continue
			}

			txt := util.TextSim(left.Name, right.Name)
			emb := util.Cosine(left.NameEmbedding, right.NameEmbedding)
			con := util.Contains(left.Name, right.Name)
			if max(txt, emb) < o.c.Memory.NameSimGate && !con {
				continue
			}

			decision, err := o.judgeSameByLLM(ctx, left, right, txt, emb, con)
			if err != nil {
				return nil, nil, err
			}

			if decision.SameEntity && decision.Confidence >= o.c.Memory.LLMMergeConfidence {
				if decision.CanonicalIdx == 1 {
					o.mergeInto(right, left)
					redirect[left.ID] = right.ID
					break // 左侧已合并 跳出内层循环
				}
				o.mergeInto(left, right)
				redirect[right.ID] = left.ID
			}

		}
	}

	deduped := make([]*memory.EntityNode, 0, len(survivors))
	for _, entity := range survivors {
		if _, ok := redirect[entity.ID]; !ok {
			deduped = append(deduped, entity)
		}
	}

	return deduped, redirect, nil

}

// mergeWithGraph 第二层:与 Neo4j 已有同类型实体融合
// 命中已有实体则把本次实体 id 重定向到已有 id (复用图节点）
// 并把新别名/描述补进该实体. 返回 待写入实体, id 重定向表 new_id->existing_id
func (o *MemoryOrchestrator) mergeWithGraph(ctx context.Context, entities []*memory.EntityNode) ([]*memory.EntityNode, map[string]string, error) {
	redirect := make(map[string]string)
	out := make([]*memory.EntityNode, 0)
	cache := make(map[string][]*memory.EntityNode)

	for _, entity := range entities {
		if _, ok := cache[entity.Type]; !ok {
			res, err := o.memoryRepo.ListEntitiesByType(ctx, o.userId, entity.Type)
			if err != nil {
				return nil, nil, xerr.Wrapf(err, "ListEntitiesByType failed: %v", err)
			}
			cache[entity.Type] = res
		}
		existing := cache[entity.Type]

		// 同名同类型直接复用已有图节点, 不问 LLM
		// 关键: 保证「用户」等稳定自指实体跨多次萃取只有一个图节点
		// 避免 LLM 非确定性判定把同名实体反复判为不同而重复建节点
		normName := strings.ToLower(strings.TrimSpace(entity.Name))
		exactIdx := slices.IndexFunc(existing, func(node *memory.EntityNode) bool {
			return node.Name == normName
		})

		// 同类型同名 直接复用
		if exactIdx != -1 {
			exactNode := existing[exactIdx]
			o.mergeInto(exactNode, entity)
			if entity.NameEmbedding != nil {
				exactNode.NameEmbedding = entity.NameEmbedding
			}
			redirect[entity.ID] = exactNode.ID
			out = append(out, exactNode)
			continue
		}

		// 找到图中最相似的实体
		var bestEntity *memory.EntityNode
		bestScore := 0.0
		for _, existEntity := range existing {
			txt := util.TextSim(entity.Name, existEntity.Name)
			emb := util.Cosine(entity.NameEmbedding, existEntity.NameEmbedding)
			con := util.Contains(entity.Name, existEntity.Name)
			score := max(txt, emb)
			if (score >= o.c.Memory.NameSimGate || con) && score > bestScore {
				bestEntity, bestScore = existEntity, score
			}
		}

		// 没有复合要求的待llm判断项, 直接使用新实体
		if bestEntity == nil {
			out = append(out, entity)
			continue
		}

		txt := util.Round(bestScore, 3)
		emb := util.Round(bestScore, 3)
		con := util.Contains(entity.Name, bestEntity.Name)
		decision, err := o.judgeSameByLLM(ctx, bestEntity, entity, txt, emb, con)
		if err != nil {
			return nil, nil, err
		}

		if decision.SameEntity && decision.Confidence >= o.c.Memory.LLMMergeConfidence {
			o.mergeInto(bestEntity, entity)
			bestEntity.NameEmbedding = entity.NameEmbedding
			redirect[entity.ID] = bestEntity.ID
			out = append(out, bestEntity)
		} else {
			out = append(out, entity)
		}
	}

	return out, redirect, nil
}

// mergeInto 把 other 的别名/描述/动力学属性并入 canon (保留方)
func (o *MemoryOrchestrator) mergeInto(canon *memory.EntityNode, other *memory.EntityNode) {
	names := make(map[string]struct{})
	for _, v := range slices.Concat(canon.Aliases, other.Aliases) {
		names[v] = struct{}{}
	}
	names[other.Name] = struct{}{}
	delete(names, canon.Name)

	canon.Aliases = make([]string, 0, len(names))
	for alias := range names {
		canon.Aliases = append(canon.Aliases, alias)
	}

	if len(other.Description) > len(canon.Description) {
		canon.Description = other.Description
	}

	// 动力学: 重要度/置信度取较大、提及次数累加、连接强度合并
	canon.Importance = max(canon.Importance, other.Importance)
	canon.Confidence = max(canon.Confidence, other.Confidence)
	canon.MentionCount = canon.MentionCount + other.MentionCount
	if canon.ConnectStrength != other.ConnectStrength {
		canon.ConnectStrength = memory.BothConnectStrength
	}
}

// judgeSameByLLM 使用llm判断两个实体是否相同
func (o *MemoryOrchestrator) judgeSameByLLM(ctx context.Context, a *memory.EntityNode, b *memory.EntityNode, txt float64, emb float64,
	con bool) (*memory.DedupDecision,
	error) {
	promptText, err := o.prompt.DedupEntity(&memory.DedupPromptInput{
		EntityA: memory.DedupEntityPromptInput{
			Name:        a.Name,
			Type:        a.Type,
			Description: a.Description,
			Aliases:     a.Aliases,
		},
		EntityB: memory.DedupEntityPromptInput{
			Name:        b.Name,
			Type:        b.Type,
			Description: b.Description,
			Aliases:     b.Aliases,
		},
		Context: memory.DedupPromptContext{
			NameTextSim:  txt,
			NameEmbedSim: emb,
			NameContains: con,
		},
	})

	if err != nil {
		return nil, xerr.Wrapf(err, "parse dedup entity prompt failed: %v", err)
	}

	response, err := o.llm.Invoke(ctx, []*llm.Message{
		llm.UserMessage(promptText),
	}, llm.WithTemperature(0.0), llm.WithMaxTokens(300))
	if err != nil {
		return nil, xerr.Wrapf(err, "call llm failed: %v", err)
	}

	res, err := jsonx.Parse[*memory.DedupDecision](o.jsonParser, response)
	if err != nil {
		return nil, err
	}

	return res, nil
}
