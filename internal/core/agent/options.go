package agent

import (
	"github.com/boxify/api-go/internal/core/llm"
	coretool "github.com/boxify/api-go/internal/core/tool"
)

const (
	defaultMaxIterations       = 6
	defaultObservationMaxRunes = 4000
	defaultToolCallingEnabled  = true
	defaultFallbackToReAct     = true
)

// Option 配置 Agent 的长期行为。
type Option func(*Agent)

// RunOption 配置单次 Run 的行为。
type RunOption func(*runConfig)

type runConfig struct {
	maxIterations int
	modelOptions  []llm.ModelCallOption
}

// WithMaxIterations 设置默认最大 ReAct 迭代次数，非正数会被忽略。
func WithMaxIterations(n int) Option {
	return func(a *Agent) {
		if n > 0 {
			a.maxIterations = n
		}
	}
}

// WithSystemPrompt 设置默认系统提示词。
func WithSystemPrompt(prompt string) Option {
	return func(a *Agent) {
		if prompt != "" {
			a.systemPrompt = prompt
		}
	}
}

// WithPromptBuilder 设置 prompt builder，nil 会被忽略。
func WithPromptBuilder(builder PromptBuilder) Option {
	return func(a *Agent) {
		if builder != nil {
			a.promptBuilder = builder
		}
	}
}

// WithParser 设置 ReAct 输出解析器，nil 会被忽略。
func WithParser(parser Parser) Option {
	return func(a *Agent) {
		if parser != nil {
			a.parser = parser
		}
	}
}

// WithHooks 设置 Agent 生命周期 hooks，nil 会被忽略。
func WithHooks(hooks Hooks) Option {
	return func(a *Agent) {
		if hooks != nil {
			a.hooks = hooks
		}
	}
}

// WithModelOptions 设置默认模型调用参数。
func WithModelOptions(opts ...llm.ModelCallOption) Option {
	return func(a *Agent) {
		a.modelOptions = append(a.modelOptions, opts...)
	}
}

// WithObservationMaxRunes 设置 Observation 最大 rune 数，非正数会被忽略。
func WithObservationMaxRunes(n int) Option {
	return func(a *Agent) {
		if n > 0 {
			a.observationMaxRunes = n
		}
	}
}

// WithToolRunner 设置工具调用器，nil 会被忽略。
func WithToolRunner(runner *coretool.Runner) Option {
	return func(a *Agent) {
		if runner != nil {
			a.toolRunner = runner
		}
	}
}

// WithPlanner 设置自定义 planner，nil 会被忽略。
//
// 自定义 planner 会跳过默认的 function calling 自动检测。调用方需要在 planner 内自行决定
// 是否支持文本 ReAct 兜底。
func WithPlanner(planner Planner) Option {
	return func(a *Agent) {
		if planner != nil {
			a.planner = planner
		}
	}
}

// WithToolCallingEnabled 设置是否优先使用模型原生工具调用。
func WithToolCallingEnabled(enabled bool) Option {
	return func(a *Agent) {
		a.toolCallingEnabled = enabled
	}
}

// WithFallbackToReAct 设置 function calling 不支持时是否退回文本 ReAct。
func WithFallbackToReAct(enabled bool) Option {
	return func(a *Agent) {
		a.fallbackToReAct = enabled
	}
}

// WithRunMaxIterations 设置单次运行的最大 ReAct 迭代次数，非正数会被忽略。
func WithRunMaxIterations(n int) RunOption {
	return func(cfg *runConfig) {
		if n > 0 {
			cfg.maxIterations = n
		}
	}
}

// WithRunModelOptions 设置单次运行的模型调用参数。
func WithRunModelOptions(opts ...llm.ModelCallOption) RunOption {
	return func(cfg *runConfig) {
		cfg.modelOptions = append(cfg.modelOptions, opts...)
	}
}
