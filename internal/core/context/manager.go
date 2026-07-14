package context

import (
	stdcontext "context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/boxify/api-go/internal/core/llm"
	coretool "github.com/boxify/api-go/internal/core/tool"
)

// Manager 负责评估上下文预算、更新滚动摘要并执行最终裁剪。
type Manager struct {
	policy     *Policy
	counter    Counter
	llmClient  llm.Client
	summarizer Summarizer
	store      Store
}

// NewManager 创建具备完整默认依赖的上下文管理器。
//
// 默认计数器使用 cl100k_base。调用方可通过 WithLLMClient 创建默认
// *LLMSummarizer，或通过 WithSummarizer 完整替换摘要器；两者均未提供时不设置
// 摘要器，压缩直接使用确定性裁剪。无效策略或默认 tokenizer 初始化失败会返回错误。
func NewManager(opts ...Option) (*Manager, error) {
	manager := &Manager{
		policy: DefaultPolicy(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(manager)
		}
	}
	// 仅在调用方没有注入计数器时加载默认 tokenizer，保持替换实现完全独立。
	if manager.counter == nil {
		counter, err := defaultCounter()
		if err != nil {
			return nil, err
		}
		manager.counter = counter
	}
	// 自定义摘要器优先；只有提供 LLM client 时才构造默认模型摘要器。
	if manager.summarizer == nil && manager.llmClient != nil {
		manager.summarizer = defaultSummarizer(manager.llmClient)
	}
	if err := manager.policy.Validate(); err != nil {
		return nil, err
	}
	return manager, nil
}

// defaultCounter 创建 Manager 使用的默认 token 计数器。
func defaultCounter() (Counter, error) {
	counter, err := NewTiktokenCounter("")
	if err != nil {
		return nil, fmt.Errorf("create context token counter: %w", err)
	}
	return counter, nil
}

// Prepare 压缩带稳定消息 ID 的持久化历史。
//
// Pinned 和 Tools 参与预算但不出现在结果中。摘要或存储失败时返回裁剪后的历史和 nil error；
// 只有策略无效或 pinned 内容已超过预算时返回错误。
func (m *Manager) Prepare(ctx stdcontext.Context, input *Input) (*Result, error) {
	if m == nil || m.counter == nil || m.policy == nil {
		return nil, errors.New("context manager is nil")
	}
	if input == nil {
		return nil, errors.New("context input is nil")
	}
	entries := cloneEntries(input.Entries)
	history := entryMessages(entries)

	// 计算固定 token 数量（Pinned + Tools），如果超过可用预算直接返回错误。
	fixedTokens := m.counter.CountMessages(input.Pinned) + m.counter.CountTools(input.Tools)
	if fixedTokens > m.policy.usableTokens() {
		return nil, ErrBudgetExceeded
	}

	// 计算历史消息总 token 数，如果未超过触发预算直接返回原始历史。
	before := fixedTokens + m.counter.CountMessages(history)
	if !m.policy.Enabled || before <= m.policy.triggerTokens() {
		return &Result{Messages: history, BeforeTokens: before, AfterTokens: before}, nil
	}

	// 如果没有摘要器或存储器，或者 key 为空，直接返回裁剪后的历史。
	if m.summarizer == nil || m.store == nil || strings.TrimSpace(input.Key) == "" {
		return m.fallback(history, fixedTokens, before)
	}

	// CAS 冲突时重新读取一次最新摘要，避免并发回合覆盖已经推进的游标。
	//
	// Compare-And-Swap（CAS）是一种原子操作，用于在多线程或分布式系统中安全地更新共享数据。
	// 它的基本思想是：在更新数据之前，先检查当前数据的值是否与预期值相同。如果相同，则执行更新；
	// 如果不同，则不执行更新。这种机制可以防止多个线程同时修改同一数据而导致的数据不一致问题。
	for range 2 {
		state, err := m.store.Load(ctx, input.Key)
		if err != nil {
			return m.fallback(history, fixedTokens, before)
		}

		// 如果摘要状态存在但不兼容当前策略，直接返回裁剪后的历史。
		result, saved, err := m.prepareWithState(ctx, input.Key, entries, state, fixedTokens, before)
		if err != nil {
			return m.fallback(history, fixedTokens, before)
		}
		if saved {
			return result, nil
		}
	}

	// 如果两次尝试都未成功保存，说明存在并发冲突，直接返回裁剪后的历史。
	return m.fallback(history, fixedTokens, before)
}

// PrepareMessages 对一次即将发送给模型的完整消息执行无副作用限窗。
//
// 该方法不会读取或写入 Store，适合 Agent 每轮生成完 system、tool call 和 tool result
// 消息后做最后检查。
func (m *Manager) PrepareMessages(ctx stdcontext.Context, messages []*llm.Message, tools []coretool.Descriptor) ([]*llm.Message, error) {
	_ = ctx
	if m == nil || m.counter == nil || m.policy == nil {
		return nil, errors.New("context manager is nil")
	}
	cloned := llm.CloneMessages(messages)
	if !m.policy.Enabled {
		return cloned, nil
	}
	toolTokens := m.counter.CountTools(tools)
	if toolTokens > m.policy.usableTokens() {
		return nil, ErrBudgetExceeded
	}
	before := toolTokens + m.counter.CountMessages(cloned)
	if before <= m.policy.triggerTokens() {
		return cloned, nil
	}
	fitted, _ := m.fitMessages(cloned, toolTokens, m.policy.targetTokens())
	if toolTokens+m.counter.CountMessages(fitted) > m.policy.usableTokens() {
		return nil, ErrBudgetExceeded
	}
	return fitted, nil
}

// prepareWithState 在给定摘要状态的情况下尝试更新滚动摘要并返回裁剪后的消息。
func (m *Manager) prepareWithState(ctx stdcontext.Context, key string, entries []*Entry, state *State, fixedTokens int, before int) (*Result, bool, error) {
	state, cursor := normalizeState(state, entries, m.policyFingerprint())
	remaining := entries[cursor:]
	keepStart := recentStart(m.counter, remaining, m.policy.KeepRecentTokens)
	toSummarize := remaining[:keepStart]

	// 如果没有消息需要摘要，但已有摘要存在，则直接返回摘要和近期历史；否则返回错误。
	if len(toSummarize) == 0 {
		if strings.TrimSpace(state.Summary) != "" {
			messages := append([]*llm.Message{summaryMessage(state.Summary)}, entryMessages(remaining)...)
			fitted, compacted := m.fitMessages(messages, fixedTokens, m.policy.targetTokens())
			after := fixedTokens + m.counter.CountMessages(fitted)
			return &Result{
				Messages:            fitted,
				BeforeTokens:        before,
				AfterTokens:         after,
				Compacted:           compacted || after < before,
				SummarizedThroughID: state.ThroughID,
			}, true, nil
		}
		return nil, false, errors.New("context has no messages eligible for summary")
	}

	// 如果有消息需要摘要，则调用 Summarizer 生成新的摘要，并尝试通过 CAS 更新状态。
	newSummary, err := m.summarizer.Summarize(ctx, state.Summary, entryMessages(toSummarize), m.policy.SummaryMaxTokens)
	if err != nil {
		return nil, false, err
	}
	next := &State{
		Summary:           newSummary,
		ThroughID:         toSummarize[len(toSummarize)-1].ID,
		Version:           state.Version + 1,
		FormatVersion:     CurrentFormatVersion,
		PolicyFingerprint: m.policyFingerprint(),
	}
	saved, err := m.store.CompareAndSwap(ctx, key, state.Version, next)
	if err != nil || !saved {
		return nil, saved, err
	}

	recent := entryMessages(remaining[keepStart:])
	messages := append([]*llm.Message{summaryMessage(newSummary)}, recent...)
	fitted, compacted := m.fitMessages(messages, fixedTokens, m.policy.targetTokens())
	after := fixedTokens + m.counter.CountMessages(fitted)
	return &Result{
		Messages:            fitted,
		BeforeTokens:        before,
		AfterTokens:         after,
		Compacted:           compacted || after < before,
		SummarizedThroughID: next.ThroughID,
	}, true, nil
}

// fallback 在无法使用滚动摘要时执行确定性裁剪。
func (m *Manager) fallback(messages []*llm.Message, fixedTokens int, before int) (*Result, error) {
	fitted, _ := m.fitMessages(messages, fixedTokens, m.policy.targetTokens())
	after := fixedTokens + m.counter.CountMessages(fitted)
	if after > m.policy.usableTokens() {
		return nil, ErrBudgetExceeded
	}
	return &Result{
		Messages:     fitted,
		BeforeTokens: before,
		AfterTokens:  after,
		Compacted:    after < before,
		UsedFallback: true,
	}, nil
}

// policyFingerprint 返回当前策略的 SHA256 摘要，用于判断摘要状态是否兼容。
func (m *Manager) policyFingerprint() string {
	data, _ := json.Marshal(m.policy)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// normalizeState 检查摘要状态是否与当前策略兼容，并返回可用的游标。
func normalizeState(state *State, entries []*Entry, fingerprint string) (*State, int) {
	if state == nil {
		return &State{}, 0
	}
	if state.FormatVersion != CurrentFormatVersion || state.PolicyFingerprint != fingerprint {
		return &State{Version: state.Version}, 0
	}

	if state.ThroughID == "" {
		return state, 0
	}
	for index, entry := range entries {
		if entry.ID == state.ThroughID {
			return state, index + 1
		}
	}
	return &State{Version: state.Version}, 0
}

// recentStart 返回在给定 token 限额下，尽量保留的近期历史的起始索引。
func recentStart(counter Counter, entries []*Entry, limit int) int {
	tokens := 0
	start := len(entries)
	for start > 0 {
		itemTokens := counter.CountMessages([]*llm.Message{entries[start-1].Message})
		if tokens > 0 && tokens+itemTokens > limit {
			break
		}
		tokens += itemTokens
		start--
	}
	return start
}

// summaryMessage 返回一条系统消息，内容为给定的摘要文本。
func summaryMessage(summary string) *llm.Message {
	return llm.SystemMessage("先前对话摘要：\n" + strings.TrimSpace(summary))
}

func cloneEntries(entries []*Entry) []*Entry {
	out := make([]*Entry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || entry.Message == nil {
			continue
		}
		out = append(out, &Entry{ID: entry.ID, Message: llm.CloneMessages([]*llm.Message{entry.Message})[0]})
	}
	return out
}

// entryMessages 提取 Entry 列表中的消息。
func entryMessages(entries []*Entry) []*llm.Message {
	messages := make([]*llm.Message, 0, len(entries))
	for _, entry := range entries {
		messages = append(messages, entry.Message)
	}
	return messages
}
