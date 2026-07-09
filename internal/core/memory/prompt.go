package memory

// Prompter 定义记忆处理流程所需的提示词能力。
//
// 实现由 core 外部注入，因此记忆流程不依赖模板注册位置、模板名称或具体模板引擎。
type Prompter interface {
	StatementExtract(input *StatementPromptInput) (string, error)
	TripletExtract(input *TripletPromptInput) (string, error)
	DedupEntity(input *DedupPromptInput) (string, error)
	GenerateCommunityMetadata(input *CommunityMetadataPromptInput) (string, error)
}

// StatementPromptInput 表示原子陈述抽取所需的业务输入。
type StatementPromptInput struct {
	Content string
	Context string
}

// TripletPromptInput 表示实体和三元组抽取所需的业务输入。
type TripletPromptInput struct {
	Statement   string
	Context     string
	EntityTypes []string
	Predicates  []string
	ValidAt     string
	InvalidAt   string
	DialogAt    string
}

// DedupEntityPromptInput 表示实体去重中的一个候选实体。
type DedupEntityPromptInput struct {
	Name        string
	Type        string
	Description string
	Aliases     []string
}

// DedupPromptContext 表示实体去重判断所需的相似度上下文。
type DedupPromptContext struct {
	NameTextSim  float64
	NameEmbedSim float64
	NameContains bool
}

// DedupPromptInput 表示实体去重提示词所需的业务输入。
type DedupPromptInput struct {
	EntityA DedupEntityPromptInput
	EntityB DedupEntityPromptInput
	Context DedupPromptContext
}

// CommunityMemberPromptInput 表示社区摘要中的一个成员。
type CommunityMemberPromptInput struct {
	Name        string
	Description string
}

// CommunityMetadataPromptInput 表示生成社区名称和摘要所需的业务输入。
type CommunityMetadataPromptInput struct {
	Members []*CommunityMemberPromptInput
}
