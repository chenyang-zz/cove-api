/**
 * @Time   : 2026/6/23 01:32
 * @Author : chenyangzhao542@gmail.com
 * @File   : memory.go
 **/

// Package prompt 封装 memory 相关提示词入口。
//
// 本文件只负责把记忆抽取、三元组抽取、实体去重和社区摘要请求映射到对应模板，
// 不直接处理模板解析细节。
package prompt

import "fmt"

// StatementExtractData 表示记忆陈述抽取模板需要的数据。
type StatementExtractData struct {
	Content string
	Context string
}

// TripletExtractData 表示三元组抽取模板需要的数据。
type TripletExtractData struct {
	Statement   string
	Context     string
	EntityTypes []string
	Predicates  []string
	ValidAt     string
	InvalidAt   string
	DialogAt    string
}

// DedupEntity 表示实体去重模板中的单个候选实体。
type DedupEntity struct {
	Name        string
	Type        string
	Description string
	Aliases     []string
}

// DedupContext 表示实体去重模板中的相似度上下文。
type DedupContext struct {
	NameTextSim  float64
	NameEmbedSim float64
	NameContains bool
}

// DedupEntityData 表示实体去重模板需要的数据。
type DedupEntityData struct {
	EntityA DedupEntity
	EntityB DedupEntity
	Context DedupContext
}

// CommunityMember 表示社区摘要模板中的成员信息。
type CommunityMember struct {
	Name        string
	Description string
}

// GenerateCommunityMetadataData 表示社区摘要模板需要的数据。
type GenerateCommunityMetadataData struct {
	Members []*CommunityMember
}

// MemoryPrompts 提供 memory 模板的类型化入口。
type MemoryPrompts struct {
	namespace string
	manager   *Manager
}

// NewMemoryPrompts 创建 memory 提示词入口。
func NewMemoryPrompts(manager *Manager) *MemoryPrompts {
	return &MemoryPrompts{
		namespace: "memory",
		manager:   manager,
	}
}

// StatementExtract 渲染记忆陈述抽取模板。
func (m *MemoryPrompts) StatementExtract(data *StatementExtractData) (string, error) {
	return m._render("statement_extract", data)
}

// TripletExtract 渲染三元组抽取模板。
func (m *MemoryPrompts) TripletExtract(data *TripletExtractData) (string, error) {
	return m._render("triplet_extract", data)
}

// DedupEntity 渲染实体去重模板。
func (m *MemoryPrompts) DedupEntity(data *DedupEntityData) (string, error) {
	return m._render("dedup_entity", data)
}

// GenerateCommunityMetadata 渲染社区摘要模板。
func (m *MemoryPrompts) GenerateCommunityMetadata(data *GenerateCommunityMetadataData) (string, error) {
	return m._render("generate_community_metadata", data)
}

// _render 拼接 memory 模板名称，并交给 Manager 统一查找和渲染。
func (m *MemoryPrompts) _render(promptName string, data any) (string, error) {
	return m.manager.Render(fmt.Sprintf("%s/%s", m.namespace, promptName), data)
}
