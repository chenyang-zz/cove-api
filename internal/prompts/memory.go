package prompts

import (
	"strconv"

	"github.com/boxify/api-go/internal/core/memory"
	"github.com/boxify/api-go/internal/prompts/promptsgen"
)

var _ memory.Prompter = (*MemoryPrompter)(nil)

// MemoryPrompter 把记忆领域输入转换为生成的模板参数。
type MemoryPrompter struct {
	client *promptsgen.Client
}

// NewMemoryPrompter 创建记忆流程使用的提示词适配器。
func NewMemoryPrompter(client *promptsgen.Client) *MemoryPrompter {
	return &MemoryPrompter{client: client}
}

// StatementExtract 渲染原子陈述抽取提示词。
func (p *MemoryPrompter) StatementExtract(input *memory.StatementPromptInput) (string, error) {
	params := &promptsgen.MemoryStatementExtractParams{}
	if input != nil {
		params.Content = input.Content
		params.Context = input.Context
	}
	return p.client.MemoryStatementExtract(params)
}

// TripletExtract 渲染实体和三元组抽取提示词。
func (p *MemoryPrompter) TripletExtract(input *memory.TripletPromptInput) (string, error) {
	params := &promptsgen.MemoryTripletExtractParams{}
	if input != nil {
		params.Statement = input.Statement
		params.Context = input.Context
		params.EntityTypes = append([]string(nil), input.EntityTypes...)
		params.Predicates = append([]string(nil), input.Predicates...)
		params.ValidAt = input.ValidAt
		params.InvalidAt = input.InvalidAt
		params.DialogAt = input.DialogAt
	}
	return p.client.MemoryTripletExtract(params)
}

// DedupEntity 渲染实体去重判断提示词。
func (p *MemoryPrompter) DedupEntity(input *memory.DedupPromptInput) (string, error) {
	params := &promptsgen.MemoryDedupEntityParams{}
	if input != nil {
		params.EntityA = &promptsgen.MemoryDedupEntityEntityA{
			Name:        input.EntityA.Name,
			Type:        input.EntityA.Type,
			Description: input.EntityA.Description,
			Aliases:     append([]string(nil), input.EntityA.Aliases...),
		}
		params.EntityB = &promptsgen.MemoryDedupEntityEntityB{
			Name:        input.EntityB.Name,
			Type:        input.EntityB.Type,
			Description: input.EntityB.Description,
			Aliases:     append([]string(nil), input.EntityB.Aliases...),
		}
		params.Context = &promptsgen.MemoryDedupEntityContext{
			NameTextSim:  strconv.FormatFloat(input.Context.NameTextSim, 'f', -1, 64),
			NameEmbedSim: strconv.FormatFloat(input.Context.NameEmbedSim, 'f', -1, 64),
			NameContains: strconv.FormatBool(input.Context.NameContains),
		}
	}
	return p.client.MemoryDedupEntity(params)
}

// GenerateCommunityMetadata 渲染社区名称和摘要生成提示词。
func (p *MemoryPrompter) GenerateCommunityMetadata(input *memory.CommunityMetadataPromptInput) (string, error) {
	params := &promptsgen.MemoryGenerateCommunityMetadataParams{}
	if input != nil {
		params.Members = make([]*promptsgen.MemoryGenerateCommunityMetadataMember, 0, len(input.Members))
		for _, member := range input.Members {
			if member == nil {
				continue
			}
			params.Members = append(params.Members, &promptsgen.MemoryGenerateCommunityMetadataMember{
				Name:        member.Name,
				Description: member.Description,
			})
		}
	}
	return p.client.MemoryGenerateCommunityMetadata(params)
}
