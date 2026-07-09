package skill

// Document 表示一个 Markdown skill 文档。
//
// Metadata 来自文档 YAML front matter；Body 是去掉 front matter 后的 Markdown 正文；
// Raw 保留原始文档文本，便于调用方做审计、缓存或重新渲染。
type Document struct {
	Metadata Metadata
	Body     string
	Raw      string
}

// Metadata 描述 Markdown skill 文档的通用元数据。
//
// Name 和 Description 用于发现、路由和展示。Icon、Version、Tags、Tools 和
// Requirements 都是通用文档属性；业务特有字段会保存在 Annotations 中。
type Metadata struct {
	Name         string
	Description  string
	Version      string
	Icon         string
	Tags         []string
	Tools        []string
	Requirements Requirements
	Annotations  map[string]any
}

// Requirements 描述 skill 文档声明的通用运行要求。
//
// core/skill 只负责解析这些声明，不检查本机是否满足要求；具体执行环境由调用方决定。
type Requirements struct {
	Env         []string
	Binaries    []string
	OS          []string
	Annotations map[string]any
}
