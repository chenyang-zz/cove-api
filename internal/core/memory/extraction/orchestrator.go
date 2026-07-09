/**
 * @author: chenyang/904852749@qq.com
 * @doc:
 **/

package extraction

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/boxify/api-go/internal/config"
	"github.com/boxify/api-go/internal/core/id"
	"github.com/boxify/api-go/internal/core/jsonx"
	"github.com/boxify/api-go/internal/core/llm"
	"github.com/boxify/api-go/internal/core/memory"
	"github.com/boxify/api-go/internal/core/memory/preprocessing"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/xerr"
)

type MemoryOrchestrator struct {
	log                *slog.Logger
	c                  *config.Config
	userId             string
	id                 id.Generator
	llm                llm.Client
	prompt             memory.Prompter
	jsonParser         jsonx.Parser
	chunker            *preprocessing.TextChunker
	statementExtractor *preprocessing.StatementExtractor
	tripletExtractor   *TripletExtractor
	memoryRepo         repository.MemoryGraphRepository
}

func NewMemoryOrchestrator(config *config.Config, userId string, idGenerator id.Generator, llm llm.Client, prompt memory.Prompter,
	jsonParser jsonx.Parser, memoryGraphRepository repository.MemoryGraphRepository) *MemoryOrchestrator {
	return &MemoryOrchestrator{
		log:                xlog.Component("memory_extract_orchestractor"),
		c:                  config,
		userId:             userId,
		id:                 idGenerator,
		llm:                llm,
		prompt:             prompt,
		jsonParser:         jsonParser,
		chunker:            preprocessing.NewTextChunker(),
		statementExtractor: preprocessing.NewStatementExtractor(llm, prompt, jsonParser),
		tripletExtractor:   NewTripletExtractor(llm, prompt, jsonParser),
		memoryRepo:         memoryGraphRepository,
	}
}

// RunExtraction 对一段文本执行完整萃取并写入图谱
func (o *MemoryOrchestrator) RunExtraction(ctx context.Context, text string, source memory.DialogueSourceType, sourceMessageID string,
	dialogAt time.Time) (*memory.ExtractionStats, error) {
	stats := &memory.ExtractionStats{}

	if source == "" {
		source = memory.ManualDialogueSource
	}

	if dialogAt.IsZero() {
		dialogAt = time.Now()
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return stats, nil
	}

	// 来源根节点
	dialogue := &memory.DialogueNode{
		ID:              o.id.New(),
		UserID:          o.userId,
		Content:         text,
		Source:          source,
		SourceMessageID: sourceMessageID,
		DialogAt:        dialogAt,
		CreatedAt:       time.Now(),
	}
	stats.DialogueID = dialogue.ID

	// 分块
	chunkTexts := o.chunker.Split(text)
	chunks := make([]*memory.ChunkNode, 0)
	for i, chunkText := range chunkTexts {
		chunks = append(chunks, &memory.ChunkNode{
			ID:        o.id.New(),
			UserID:    o.userId,
			DialogID:  dialogue.ID,
			Content:   chunkText,
			Sequence:  i,
			CreatedAt: time.Now(),
		})
	}
	stats.ChunkCount = len(chunks)

	statements := make([]*memory.StatementNode, 0)
	// 收集所有实体（带局部 idx -> EntityNode 映射，按 chunk 隔离）与三元组
	entityPool := make([]*memory.EntityNode, 0)
	mentions := make([]*memory.MentionEdge, 0)
	// 记录（statement_id， local_idx）-> EntityNode，用于三元组连边
	pendingTriplets := make([]struct {
		stmtID  string
		triplet *memory.ExtractedTriplet
		dict    map[int]*memory.EntityNode
	}, 0)
	// 收集事件: (ExtractedEvent, 本块实体名->EntityNode 映射)，去重后按参与边连边
	pendingEvents := make([]struct {
		event *memory.ExtractedEvent
		dict  map[string]*memory.EntityNode
	}, 0)

	// 逐块：陈述抽取 -> 三元组萃取
	var extractContext string
	if len(chunks) > 1 {
		extractContext = text
	}
	for _, chunk := range chunks {
		extractedStmts, err := o.statementExtractor.Extract(ctx, chunk.Content, extractContext)
		if err != nil {
			return nil, err
		}

		if len(extractedStmts) == 0 {
			continue
		}

		extractedTriplets, err := o.tripletExtractor.BatchExtract(ctx, extractedStmts, extractContext, dialogAt, 4)
		if err != nil {
			return nil, err
		}

		// 本块内 实体名 → EntityNode (供事件按 participants 名字关联)
		entityNameMap := make(map[string]*memory.EntityNode)
		for i := range extractedStmts {
			stmt := extractedStmts[i]
			triplets := extractedTriplets[i]

			stmtNode := &memory.StatementNode{
				ID:                o.id.New(),
				UserID:            o.userId,
				ChunkID:           chunk.ID,
				Statement:         stmt.Statement,
				StmtType:          stmt.StatementType,
				TemporalType:      stmt.TemporalType,
				Speaker:           memory.UserSpeaker,
				ValidAt:           time.Time{},
				InvalidAt:         time.Time{},
				DialogAt:          dialogAt,
				Embedding:         nil,
				Importance:        stmt.Importance,
				Confidence:        stmt.Confidence,
				MemoryLayer:       memory.LayerShortTerm,
				AccessCount:       0,
				LastAccessAt:      time.Time{},
				HasEmotionalState: stmt.HasEmotionalState,
				EmotionType:       stmt.EmotionType,
				EmotionIntensity:  stmt.EmotionIntensity,
				EmotionKeywords:   stmt.EmotionKeywords,
				CreatedAt:         time.Now(),
			}
			statements = append(statements, stmtNode)

			// 该陈述内 局部 entity_idx → EntityNod
			idxMap := make(map[int]*memory.EntityNode)
			for _, entity := range triplets.Entities {
				node := &memory.EntityNode{
					ID:                 o.id.New(),
					UserID:             o.userId,
					Name:               strings.TrimSpace(entity.Name),
					Type:               memory.NormalizeEntityType(entity.Type),
					Description:        entity.Description,
					Aliases:            nil,
					NameEmbedding:      nil,
					CommunityID:        "",
					Importance:         entity.Importance,
					Confidence:         entity.Confidence,
					MemoryLayer:        memory.LayerShortTerm,
					AccessCount:        0,
					LastAccessAt:       time.Time{},
					MentionCount:       1,
					ConnectStrength:    memory.StrongConnectStrength,
					CoreFacts:          nil,
					Traits:             nil,
					LastConsolidatedAt: time.Time{},
					CreatedAt:          time.Now(),
				}
				idxMap[entity.EntityIdx] = node
				entityPool = append(entityPool, node)
				entityNameMap[node.Name] = node
				mentions = append(mentions, &memory.MentionEdge{
					UserID:          o.userId,
					StatementID:     stmtNode.ID,
					EntityID:        node.ID,
					ConnectStrength: memory.StrongConnectStrength,
					CreatedAt:       time.Now(),
				})
			}

			for _, triplet := range triplets.Triplets {
				pendingTriplets = append(pendingTriplets, struct {
					stmtID  string
					triplet *memory.ExtractedTriplet
					dict    map[int]*memory.EntityNode
				}{
					stmtID:  stmtNode.ID,
					triplet: triplet,
					dict:    idxMap,
				})
			}

			for _, event := range triplets.Events {
				pendingEvents = append(pendingEvents, struct {
					event *memory.ExtractedEvent
					dict  map[string]*memory.EntityNode
				}{event: event, dict: entityNameMap})
			}
		}

	}

	stats.StatementCount = len(statements)
	if len(entityPool) == 0 {
		// 没抽到实体也写来源 + 陈述 (保留溯源), 关系/事件为空
		err := o.persist(ctx, dialogue, chunks, statements, nil, nil, mentions, nil, nil)
		if err != nil {
			return nil, err
		}

		return stats, nil
	}

	// 实体 name 向量化
	embeddingTexts := make([]string, len(entityPool))
	for _, entity := range entityPool {
		embeddingTexts = append(embeddingTexts, entity.Name)
	}

	vectors, err := o.llm.Embed(ctx, embeddingTexts, o.c.Rag.EmbeddingDim)
	if err != nil {
		return nil, xerr.Wrapf(err, "embed text failed: %v", err)
	}
	for i := range entityPool {
		entity := entityPool[i]
		vector := vectors[i]
		entity.NameEmbedding = vector
	}

	// 去重
	finalEntities, resolve, err := o.dedup(ctx, entityPool)

	finalById := make(map[string]*memory.EntityNode, len(finalEntities))
	finalEntityIDs := make([]string, 0, len(finalEntities))
	for _, entity := range finalEntities {
		finalById[entity.ID] = entity
		finalEntityIDs = append(finalEntityIDs, entity.ID)
	}
	stats.EntityCount = len(finalEntities)
	stats.EntityIDs = finalEntityIDs

	// mention 边重定向到最终实体 id
	finalMentions := make([]*memory.MentionEdge, 0, len(mentions))
	for _, mention := range mentions {
		mention.EntityID = resolve(mention.EntityID)
		// 去掉指向不存在实体的 mention (被合并的已重定向，正常都存在)
		if _, ok := finalById[mention.EntityID]; ok {
			finalMentions = append(finalMentions, mention)
		}
	}

	// 三元组 → RELATION 边
	relations := make([]*memory.RelationEdge, 0)
	for _, pending := range pendingTriplets {
		subj := pending.dict[pending.triplet.SubjectID]
		obj := pending.dict[pending.triplet.ObjectID]
		if subj == nil || obj == nil {
			continue
		}
		sId := resolve(subj.ID)
		oId := resolve(obj.ID)
		_, sExist := finalById[sId]
		_, oExist := finalById[oId]

		// 过滤不存在的以及自环
		if !sExist || !oExist || sId == oId {
			continue
		}

		relations = append(relations, &memory.RelationEdge{
			ID:               o.id.New(),
			UserID:           o.userId,
			SourceID:         sId,
			TargetID:         oId,
			Predicate:        memory.NormalizePredicate(pending.triplet.Predicate),
			PredicateSurface: pending.triplet.PredicateSurface,
			SourceText:       "",
			StatementID:      pending.stmtID,
			Value:            pending.triplet.Value,
			ValidAt:          time.Time{},
			InvalidAt:        time.Time{},
			Importance:       0.5,
			Confidence:       0.8,
			AccessCount:      0,
			CreatedAt:        time.Now(),
		})
	}
	stats.RelationCount = len(relations)

	// 事件 → Event 节点 + INVOLVES 边 (按 participants 名字匹配到最终实体)
	events := make([]*memory.EventNode, 0)
	involves := make([]*memory.InvolvesEdge, 0)
	for _, pending := range pendingEvents {
		title := strings.TrimSpace(pending.event.Title)
		if title == "" {
			continue
		}

		eventNode := &memory.EventNode{
			ID:          o.id.New(),
			UserID:      o.userId,
			Title:       title,
			Description: pending.event.Description,
			EventTime:   pending.event.EventTime,
			Embedding:   nil,
			CreatedAt:   time.Now(),
		}

		// 参与者名字 → 本块 EntityNode → 重定向到最终实体 id
		linkedSet := make(map[string]struct{})
		for _, pName := range pending.event.Participants {
			entity := pending.dict[pName]
			if entity == nil {
				continue
			}
			entityId := resolve(entity.ID)
			_, exist := finalById[entityId]
			_, linked := linkedSet[entityId]
			if exist && !linked {
				linkedSet[entityId] = struct{}{}
				involves = append(involves, &memory.InvolvesEdge{
					UserID:    o.userId,
					EventID:   eventNode.ID,
					EntityID:  entityId,
					CreatedAt: time.Now(),
				})
			}
		}
		events = append(events, eventNode)
	}
	stats.EventCount = len(events)

	// 写图 (单事务原子落库)
	err = o.persist(ctx, dialogue, chunks, statements, finalEntities, events, mentions, relations, involves)
	if err != nil {
		return nil, err
	}

	// TODO: 增量社区聚类 (新实体归入社区; 失败不影响萃取结果)

	// TODO: 反思增量触发: 累计新增实体数达阈值则派发一次单用户反思 (失败不影响萃取)

	return stats, nil
}

// persist 持久化图到仓库中
func (o *MemoryOrchestrator) persist(
	ctx context.Context,
	dialogue *memory.DialogueNode,
	chunks []*memory.ChunkNode,
	statements []*memory.StatementNode,
	entities []*memory.EntityNode,
	events []*memory.EventNode,
	mentions []*memory.MentionEdge,
	relations []*memory.RelationEdge,
	involves []*memory.InvolvesEdge,
) error {
	return o.memoryRepo.SaveGraph(ctx, []*memory.DialogueNode{dialogue},
		chunks, statements, entities, events, mentions, relations, involves)
}
