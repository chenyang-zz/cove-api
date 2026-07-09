package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"

	coreskill "github.com/boxify/api-go/internal/core/skill"
	"github.com/google/uuid"
)

//go:embed builtin/*/SKILL.md
var builtinFS embed.FS

const (
	// KeyKBStudy 是知识库学习内置技能模板 key。
	KeyKBStudy = "kb_study"
	// KeyStockAnalysis 是股票分析内置技能模板 key。
	KeyStockAnalysis = "stock_analysis"
	// KeyTranslatePolish 是翻译润色内置技能模板 key。
	KeyTranslatePolish = "translate_polish"
	// ToolKnowledgeSearch 是知识库检索内置工具 key。
	ToolKnowledgeSearch = "knowledge_search"
	// ToolWebSearch 是联网搜索工具 key，当前作为模板白名单保留。
	ToolWebSearch = "web_search"
)

var (
	// IDKBStudy 是知识库学习内置技能模板的固定 ID。
	IDKBStudy = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	// IDStockAnalysis 是股票分析内置技能模板的固定 ID。
	IDStockAnalysis = uuid.MustParse("22222222-2222-4222-8222-222222222222")
	// IDTranslatePolish 是翻译润色内置技能模板的固定 ID。
	IDTranslatePolish = uuid.MustParse("33333333-3333-4333-8333-333333333333")
)

// templateSpec 描述内置技能模板的业务配置。
//
// SKILL.md 负责提供名称、说明、图标、工具和提示词正文；templateSpec 负责提供系统内
// 稳定 ID、列表行为和技能配置等业务字段。
type templateSpec struct {
	ID      uuid.UUID
	Enabled bool
	Sort    int
	Config  Config
}

// Config 描述内置技能模板的业务配置。
//
// Config 是 domain 层自己的结构，不绑定数据库模型；落库时由 logic 层显式转换。
type Config struct {
	QuickPrompt []string
	FewShots    []FewShot
}

// FewShot 描述内置技能模板中的一组示例输入输出。
type FewShot struct {
	Input  string
	Output string
}

// Template 描述一个业务内置技能模板。
//
// Template 只存在于代码注册表中，不直接写入数据库；用户复制后会生成自己的持久化记录。
type Template struct {
	ID          uuid.UUID
	Key         string
	Name        string
	Description string
	Icon        string
	Prompt      string
	ToolKeys    []string
	Config      Config
	Enabled     bool
	Sort        int
}

// Registry 保存业务内置技能模板，并提供 UUID 友好的查询方法。
type Registry struct {
	byID  map[uuid.UUID]Template
	byKey map[string]uuid.UUID
}

// NewRegistry 创建并返回业务内置技能模板注册表。
func NewRegistry() (*Registry, error) {
	return newRegistryFromFS(builtinFS)
}

// LookupByID 按固定模板 UUID 查找内置技能模板。
//
// 第二个返回值 reports whether 模板存在。返回的 Template 是副本，调用方可以安全修改。
func (r *Registry) LookupByID(id uuid.UUID) (Template, bool) {
	if r == nil || id == uuid.Nil {
		return Template{}, false
	}
	template, ok := r.byID[id]
	if !ok {
		return Template{}, false
	}
	return cloneTemplate(template), true
}

// LookupByKey 按模板 key 查找内置技能模板。
//
// 第二个返回值 reports whether 模板存在。
func (r *Registry) LookupByKey(key string) (Template, bool) {
	if r == nil || key == "" {
		return Template{}, false
	}
	id, ok := r.byKey[key]
	if !ok {
		return Template{}, false
	}
	return r.LookupByID(id)
}

// List 返回全部内置技能模板。
//
// 返回结果按 Sort 升序、Key 升序排序；每个 Template 都是独立副本。
func (r *Registry) List() []Template {
	if r == nil {
		return nil
	}
	out := make([]Template, 0, len(r.byID))
	for _, template := range r.byID {
		out = append(out, cloneTemplate(template))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Sort != out[j].Sort {
			return out[i].Sort < out[j].Sort
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func newRegistryFromFS(fsys fs.FS) (*Registry, error) {
	paths, err := fs.Glob(fsys, "builtin/*/SKILL.md")
	if err != nil {
		return nil, fmt.Errorf("glob builtin skills: %w", err)
	}
	sort.Strings(paths)
	registry := &Registry{
		byID:  map[uuid.UUID]Template{},
		byKey: map[string]uuid.UUID{},
	}
	for _, path := range paths {
		document, err := coreskill.Read(fsys, path)
		if err != nil {
			return nil, err
		}
		template, err := templateFromDocument(document)
		if err != nil {
			return nil, fmt.Errorf("build builtin skill %s: %w", path, err)
		}
		if err := registry.register(template); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *Registry) register(template Template) error {
	if template.ID == uuid.Nil {
		return fmt.Errorf("builtin skill %q id is empty", template.Key)
	}
	if template.Key == "" {
		return fmt.Errorf("builtin skill key is empty")
	}
	if _, ok := r.byID[template.ID]; ok {
		return fmt.Errorf("builtin skill id %s already registered", template.ID)
	}
	if _, ok := r.byKey[template.Key]; ok {
		return fmt.Errorf("builtin skill key %q already registered", template.Key)
	}
	r.byID[template.ID] = cloneTemplate(template)
	r.byKey[template.Key] = template.ID
	return nil
}

func templateFromDocument(document *coreskill.Document) (Template, error) {
	if document == nil {
		return Template{}, fmt.Errorf("document is nil")
	}
	key := document.Metadata.Name
	spec, ok := templateSpecs()[key]
	if !ok {
		return Template{}, fmt.Errorf("template data for %q not found", key)
	}
	name, ok := document.Metadata.Annotations["title"].(string)
	if !ok || name == "" {
		name = key
	}
	return Template{
		ID:          spec.ID,
		Key:         key,
		Name:        name,
		Description: document.Metadata.Description,
		Icon:        document.Metadata.Icon,
		Prompt:      document.Body,
		ToolKeys:    append([]string(nil), document.Metadata.Tools...),
		Config:      cloneConfig(spec.Config),
		Enabled:     spec.Enabled,
		Sort:        spec.Sort,
	}, nil
}

func templateSpecs() map[string]templateSpec {
	return map[string]templateSpec{
		KeyKBStudy: {
			ID:      IDKBStudy,
			Enabled: true,
			Sort:    10,
			Config: Config{
				QuickPrompt: []string{
					"帮我梳理知识库里的核心知识点",
					"基于知识库给我出几道测验题：",
					"用通俗的话讲讲这个概念：",
					"这部分资料的重点是什么？",
				},
				FewShots: []FewShot{},
			},
		},
		KeyStockAnalysis: {
			ID:      IDStockAnalysis,
			Enabled: true,
			Sort:    20,
			Config: Config{
				QuickPrompt: []string{
					"分析一下今天的大盘走势",
					"帮我分析这只股票（代码/名称）：",
					"最近有哪些影响市场的重要消息？",
					"从基本面看这家公司怎么样：",
				},
				FewShots: []FewShot{},
			},
		},
		KeyTranslatePolish: {
			ID:      IDTranslatePolish,
			Enabled: true,
			Sort:    30,
			Config: Config{
				QuickPrompt: []string{
					"把这段中文翻译成地道英文：",
					"帮我润色这段英文，让它更自然：",
					"这句话还有没有更好的表达：",
				},
				FewShots: []FewShot{},
			},
		},
	}
}

func cloneTemplate(template Template) Template {
	template.ToolKeys = append([]string(nil), template.ToolKeys...)
	template.Config = cloneConfig(template.Config)
	return template
}

func cloneConfig(config Config) Config {
	return Config{
		QuickPrompt: cloneStrings(config.QuickPrompt),
		FewShots:    cloneFewShots(config.FewShots),
	}
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneFewShots(values []FewShot) []FewShot {
	if values == nil {
		return nil
	}
	cloned := make([]FewShot, len(values))
	copy(cloned, values)
	return cloned
}
