package context

import "github.com/boxify/api-go/internal/core/llm"

const (
	// DefaultWindowTokens 是未显式提供策略时使用的保守上下文窗口。
	DefaultWindowTokens = 32768 // 32k
	// DefaultOutputReserveTokens 是为模型输出预留的 token 数。
	DefaultOutputReserveTokens = 4096 // 4k
	// DefaultSafetyMarginTokens 是为供应商消息包装和协议文本保留的安全余量。
	DefaultSafetyMarginTokens = 512 // 0.5k
	// DefaultTriggerRatio 是触发压缩的可用预算比例。
	DefaultTriggerRatio = 0.8
	// DefaultTargetRatio 是压缩后的目标预算比例。
	DefaultTargetRatio = 0.6
	// DefaultKeepRecentTokens 是始终尽量保留的近期历史 token 数。
	DefaultKeepRecentTokens = 8192 // 8k
	// DefaultSummaryMaxTokens 是单次摘要允许的最大输出 token 数。
	DefaultSummaryMaxTokens int64 = 1024 // 1k
	// CurrentFormatVersion 标识持久化摘要的当前语义版本。
	CurrentFormatVersion = 1
)

// DefaultPolicy 返回独立的默认上下文策略。
func DefaultPolicy() *Policy {
	return &Policy{
		Enabled:             true,
		WindowTokens:        DefaultWindowTokens,
		OutputReserveTokens: DefaultOutputReserveTokens,
		SafetyMarginTokens:  DefaultSafetyMarginTokens,
		TriggerRatio:        DefaultTriggerRatio,
		TargetRatio:         DefaultTargetRatio,
		KeepRecentTokens:    DefaultKeepRecentTokens,
		SummaryMaxTokens:    DefaultSummaryMaxTokens,
	}
}

// Option 配置 Manager 的长期依赖与默认策略。
type Option func(*Manager)

// WithCounter 设置 Manager 使用的 token 计数器；nil 会被忽略并保留默认计数器。
func WithCounter(counter Counter) Option {
	return func(manager *Manager) {
		if counter != nil {
			manager.counter = counter
		}
	}
}

// WithSummarizer 设置 Manager 使用的滚动摘要器；nil 会被忽略并保留安全默认实现。
func WithSummarizer(summarizer Summarizer) Option {
	return func(manager *Manager) {
		if summarizer != nil {
			manager.summarizer = summarizer
		}
	}
}

// WithLLMClient 设置默认 LLM 摘要器使用的模型客户端；nil 会被忽略。
// 如果同时设置 WithSummarizer，NewManager 会优先使用自定义摘要器。
func WithLLMClient(client llm.Client) Option {
	return func(manager *Manager) {
		if client != nil {
			manager.llmClient = client
		}
	}
}

// WithPolicy 设置 Manager 默认使用的策略；nil 或无效策略会由 NewManager 返回错误。
func WithPolicy(policy *Policy) Option {
	return func(manager *Manager) {
		manager.policy = clonePolicy(policy)
	}
}

// WithStore 设置持久化滚动摘要状态的存储；nil 会被忽略。
func WithStore(store Store) Option {
	return func(manager *Manager) {
		if store != nil {
			manager.store = store
		}
	}
}

func clonePolicy(policy *Policy) *Policy {
	if policy == nil {
		return nil
	}
	cloned := *policy
	return &cloned
}
