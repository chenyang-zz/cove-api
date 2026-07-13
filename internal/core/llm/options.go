package llm

import (
	"strings"

	coretool "github.com/boxify/api-go/internal/core/tool"
)

const (
	// DefaultTemperature 是聊天模型调用在缺少上层配置时使用的安全默认温度。
	DefaultTemperature = 0.7
	// DefaultVisionMaxTokens 是多模态看图调用在未显式设置 MaxTokens 时的默认最大输出 token 数。
	DefaultVisionMaxTokens int64 = 1024
)

// ModelCallOptions 表示一次模型生成调用的可选参数。
//
// 聊天补全与多模态看图（Vision）共用这组参数：Temperature、TopP、MaxTokens，
// 以及 Tools / ToolChoice（若 provider 支持）。
type ModelCallOptions struct {
	Temperature *float64
	TopP        *float64
	MaxTokens   *int64
	Tools       []coretool.Descriptor
	ToolChoice  *ToolChoice
}

// EmbeddingOptions 表示一次批量向量化调用的可选参数。
type EmbeddingOptions struct {
	BatchSize int
}

// ModelCallOption 修改一次模型生成调用的可选参数。
type ModelCallOption func(*ModelCallOptions)

// EmbeddingOption 修改一次批量向量化调用的可选参数。
type EmbeddingOption func(*EmbeddingOptions)

// NewChatOptions 返回合并后的模型生成调用参数。
func NewChatOptions(opts ...ModelCallOption) ModelCallOptions {
	temperature := DefaultTemperature
	out := ModelCallOptions{Temperature: &temperature}
	for _, opt := range opts {
		if opt != nil {
			opt(&out)
		}
	}
	return out
}

// NewVisionOptions 返回合并后的多模态看图调用参数。
//
// 复用 NewChatOptions 的温度等默认值；未显式设置 MaxTokens 时回落到 DefaultVisionMaxTokens。
func NewVisionOptions(opts ...ModelCallOption) ModelCallOptions {
	out := NewChatOptions(opts...)
	if out.MaxTokens == nil {
		maxTokens := DefaultVisionMaxTokens
		out.MaxTokens = &maxTokens
	}
	return out
}

// WithTemperature 设置模型采样温度。
func WithTemperature(value float64) ModelCallOption {
	return func(opts *ModelCallOptions) {
		opts.Temperature = &value
	}
}

// WithTopP 设置 nucleus sampling 参数，只有 0 < value <= 1 时生效。
func WithTopP(value float64) ModelCallOption {
	return func(opts *ModelCallOptions) {
		if value > 0 && value <= 1 {
			opts.TopP = &value
		}
	}
}

// NewEmbeddingOptions 返回合并后的批量向量化调用参数。
func NewEmbeddingOptions(opts ...EmbeddingOption) EmbeddingOptions {
	var out EmbeddingOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&out)
		}
	}
	return out
}

// WithMaxTokens 设置模型最大输出 token 数。
func WithMaxTokens(value int64) ModelCallOption {
	return func(opts *ModelCallOptions) {
		opts.MaxTokens = &value
	}
}

// ToolChoiceMode 表示模型工具选择策略。
type ToolChoiceMode string

const (
	// ToolChoiceAuto 表示模型可以自行决定是否调用工具。
	ToolChoiceAuto ToolChoiceMode = "auto"
	// ToolChoiceNone 表示模型不应调用工具。
	ToolChoiceNone ToolChoiceMode = "none"
	// ToolChoiceRequired 表示模型必须调用至少一个工具。
	ToolChoiceRequired ToolChoiceMode = "required"
	// ToolChoiceTool 表示模型必须调用指定工具。
	ToolChoiceTool ToolChoiceMode = "tool"
)

// ToolChoice 表示一次模型调用的工具选择策略。
type ToolChoice struct {
	Mode ToolChoiceMode
	Name string
}

// WithTools 设置本次模型调用可用的工具描述。
func WithTools(tools ...coretool.Descriptor) ModelCallOption {
	return func(opts *ModelCallOptions) {
		opts.Tools = coretool.CloneDescriptors(tools)
	}
}

// WithToolChoice 设置本次模型调用的工具选择策略。
func WithToolChoice(choice ToolChoice) ModelCallOption {
	return func(opts *ModelCallOptions) {
		switch choice.Mode {
		case ToolChoiceAuto, ToolChoiceNone, ToolChoiceRequired:
			opts.ToolChoice = &ToolChoice{Mode: choice.Mode}
		case ToolChoiceTool:
			name := strings.TrimSpace(choice.Name)
			if name != "" {
				opts.ToolChoice = &ToolChoice{Mode: ToolChoiceTool, Name: name}
			}
		}
	}
}

// WithToolChoiceAuto 允许模型自行决定是否调用工具。
func WithToolChoiceAuto() ModelCallOption {
	return WithToolChoice(ToolChoice{Mode: ToolChoiceAuto})
}

// WithToolChoiceNone 禁止模型调用工具。
func WithToolChoiceNone() ModelCallOption {
	return WithToolChoice(ToolChoice{Mode: ToolChoiceNone})
}

// WithToolChoiceRequired 要求模型调用至少一个工具。
func WithToolChoiceRequired() ModelCallOption {
	return WithToolChoice(ToolChoice{Mode: ToolChoiceRequired})
}

// WithRequiredTool 要求模型调用指定名称的工具。
func WithRequiredTool(name string) ModelCallOption {
	return WithToolChoice(ToolChoice{Mode: ToolChoiceTool, Name: name})
}

// WithEmbeddingBatchSize 设置批量向量化请求的单批文本数量，非正数会被忽略。
func WithEmbeddingBatchSize(batchSize int) EmbeddingOption {
	return func(opts *EmbeddingOptions) {
		if batchSize > 0 {
			opts.BatchSize = batchSize
		}
	}
}
