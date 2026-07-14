package context

import (
	stdcontext "context"
	"errors"

	"github.com/boxify/api-go/internal/core/llm"
	coretool "github.com/boxify/api-go/internal/core/tool"
)

// ErrBudgetExceeded 表示不可裁剪内容已经超过可用上下文预算。
var ErrBudgetExceeded = errors.New("context budget exceeded by pinned messages")

// Policy 描述一次上下文压缩使用的 token 预算与触发策略。
type Policy struct {
	Enabled             bool
	WindowTokens        int     // 可用上下文窗口大小（包含保留和安全边际）
	OutputReserveTokens int     // 输出保留预算（用于模型输出，不能被裁剪）
	SafetyMarginTokens  int     // 安全边际预算（用于模型输入，不能被裁剪）
	TriggerRatio        float64 // 触发压缩的可用预算比例（0 < TriggerRatio <= 1）
	TargetRatio         float64 // 压缩目标的可用预算比例（0 < TargetRatio < TriggerRatio）
	KeepRecentTokens    int     // 保留最近消息的 token 数（用于裁剪，必须小于目标预算）
	SummaryMaxTokens    int64   // 滚动摘要最大 token 数（用于裁剪，必须小于目标预算）
}

// Validate 校验策略是否能形成有效的触发预算和目标预算。
func (p *Policy) Validate() error {
	if p == nil {
		return errors.New("context policy is nil")
	}
	if p.WindowTokens <= 0 {
		return errors.New("context window tokens must be positive")
	}
	if p.OutputReserveTokens < 0 || p.SafetyMarginTokens < 0 {
		return errors.New("context reserved tokens must not be negative")
	}
	if p.OutputReserveTokens+p.SafetyMarginTokens >= p.WindowTokens {
		return errors.New("context reserved tokens must be smaller than window tokens")
	}
	if p.TargetRatio <= 0 || p.TriggerRatio <= 0 || p.TargetRatio >= p.TriggerRatio || p.TriggerRatio > 1 {
		return errors.New("context ratios must satisfy 0 < target < trigger <= 1")
	}
	if p.KeepRecentTokens <= 0 || p.KeepRecentTokens > p.targetTokens() {
		return errors.New("context recent tokens must fit target budget")
	}
	if p.SummaryMaxTokens <= 0 || int(p.SummaryMaxTokens) >= p.targetTokens() {
		return errors.New("context summary max tokens must fit target budget")
	}
	return nil
}

func (p *Policy) usableTokens() int {
	return p.WindowTokens - p.OutputReserveTokens - p.SafetyMarginTokens
}

func (p *Policy) triggerTokens() int {
	return int(float64(p.usableTokens()) * p.TriggerRatio)
}

func (p *Policy) targetTokens() int {
	return int(float64(p.usableTokens()) * p.TargetRatio)
}

// Entry 为一条模型消息补充调用方可持久化的稳定标识。
type Entry struct {
	ID      string
	Message *llm.Message
}

// Input 描述需要压缩的持久化历史以及不可裁剪的固定输入。
//
// Entries 必须按时间从旧到新排列。Pinned 只参与预算计算，不会出现在 Result.Messages 中。
type Input struct {
	Key     string
	Entries []*Entry
	Pinned  []*llm.Message
	Tools   []coretool.Descriptor
}

// Result 描述压缩后的消息和本次压缩统计。
type Result struct {
	Messages            []*llm.Message
	BeforeTokens        int
	AfterTokens         int
	Compacted           bool
	UsedFallback        bool
	SummarizedThroughID string
}

// State 表示可跨请求复用的滚动摘要状态。
type State struct {
	Summary           string
	ThroughID         string // 最后一条被摘要的消息 ID
	Version           int64
	FormatVersion     int
	PolicyFingerprint string
}

// Counter 统计消息与工具描述占用的 token 数。
type Counter interface {
	CountMessages(messages []*llm.Message) int
	CountTools(tools []coretool.Descriptor) int
}

// Summarizer 把已有摘要与一段较旧消息合并为新的滚动摘要。
type Summarizer interface {
	Summarize(ctx stdcontext.Context, previousSummary string, messages []*llm.Message, maxTokens int64) (string, error)
}

// Store 读取摘要状态，并通过乐观锁比较版本后写入新状态。
//
// Load 在状态不存在时返回 nil, nil。CompareAndSwap 返回 false, nil 表示版本冲突。
type Store interface {
	Load(ctx stdcontext.Context, key string) (*State, error)
	CompareAndSwap(ctx stdcontext.Context, key string, expectedVersion int64, next *State) (bool, error)
}
