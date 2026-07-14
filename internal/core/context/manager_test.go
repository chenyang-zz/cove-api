package context

import (
	stdcontext "context"
	"errors"
	"strings"
	"testing"

	"github.com/boxify/api-go/internal/core/llm"
	coretool "github.com/boxify/api-go/internal/core/tool"
)

// TestPolicyValidateChecksCrossFieldConstraints 验证策略会拒绝无效窗口、比例和近期消息预算。
func TestPolicyValidateChecksCrossFieldConstraints(t *testing.T) {
	policy := DefaultPolicy()
	if err := policy.Validate(); err != nil {
		t.Fatalf("DefaultPolicy().Validate() error = %v, want nil", err)
	}
	policy.TargetRatio = policy.TriggerRatio
	if err := policy.Validate(); err == nil {
		t.Fatal("Policy.Validate() error = nil, want ratio validation error")
	}
}

// TestNewManagerUsesDefaultDependencies 验证未传入依赖选项时只创建默认计数器且不设置摘要器。
func TestNewManagerUsesDefaultDependencies(t *testing.T) {
	manager, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager() error = %v, want nil", err)
	}
	if _, ok := manager.counter.(*TiktokenCounter); !ok {
		t.Fatalf("NewManager() counter type = %T, want *TiktokenCounter", manager.counter)
	}
	if manager.summarizer != nil {
		t.Fatalf("NewManager() summarizer = %T, want nil", manager.summarizer)
	}
}

// TestNewManagerAppliesDependencyOptions 验证计数器和摘要器选项会替换默认依赖且忽略 nil。
func TestNewManagerAppliesDependencyOptions(t *testing.T) {
	counter := runeCounter{}
	summarizer := &fakeSummarizer{text: "summary"}
	manager, err := NewManager(
		WithCounter(counter),
		WithSummarizer(summarizer),
		WithCounter(nil),
		WithSummarizer(nil),
	)
	if err != nil {
		t.Fatalf("NewManager() error = %v, want nil", err)
	}
	if _, ok := manager.counter.(runeCounter); !ok {
		t.Fatalf("NewManager() counter type = %T, want runeCounter", manager.counter)
	}
	if manager.summarizer != summarizer {
		t.Fatalf("NewManager() summarizer = %T, want injected fake", manager.summarizer)
	}
}

// TestNewManagerAppliesLLMClientOption 验证模型客户端选项会创建持有该客户端的 LLM 摘要器。
func TestNewManagerAppliesLLMClientOption(t *testing.T) {
	client := &fakeLLMClient{}
	manager, err := NewManager(WithLLMClient(client), WithLLMClient(nil))
	if err != nil {
		t.Fatalf("NewManager() error = %v, want nil", err)
	}
	summarizer, ok := manager.summarizer.(*LLMSummarizer)
	if !ok {
		t.Fatalf("NewManager() summarizer type = %T, want *LLMSummarizer", manager.summarizer)
	}
	if summarizer.client != client {
		t.Fatalf("NewManager() summarizer client = %T, want injected client", summarizer.client)
	}

	custom := &fakeSummarizer{text: "custom"}
	manager, err = NewManager(WithSummarizer(custom), WithLLMClient(client))
	if err != nil {
		t.Fatalf("NewManager() with custom summarizer error = %v, want nil", err)
	}
	if manager.summarizer != custom {
		t.Fatalf("NewManager() summarizer = %T, want custom summarizer", manager.summarizer)
	}
}

// TestManagerPrepareReturnsHistoryBeforeTrigger 验证未达到触发阈值时返回独立历史副本且不访问摘要依赖。
func TestManagerPrepareReturnsHistoryBeforeTrigger(t *testing.T) {
	store := &fakeStore{}
	manager := newTestManager(t, DefaultPolicy(), &fakeSummarizer{}, store)
	input := &Input{Entries: []*Entry{{ID: "1", Message: llm.UserMessage("short")}}}

	result, err := manager.Prepare(stdcontext.Background(), input)
	if err != nil {
		t.Fatalf("Manager.Prepare() error = %v, want nil", err)
	}
	if result.Compacted || result.UsedFallback || store.loads != 0 {
		t.Fatalf("Manager.Prepare() result = %#v, store loads = %d, want unchanged history", result, store.loads)
	}
	result.Messages[0].Content = "changed"
	if input.Entries[0].Message.Content != "short" {
		t.Fatalf("Manager.Prepare() mutated input content = %q, want short", input.Entries[0].Message.Content)
	}
}

// TestManagerPrepareSummarizesOldEntriesAndPersistsCursor 验证超限后只摘要旧消息、保留近期消息并推进持久化游标。
func TestManagerPrepareSummarizesOldEntriesAndPersistsCursor(t *testing.T) {
	policy := smallPolicy()
	store := &fakeStore{}
	summarizer := &fakeSummarizer{text: "summary"}
	manager := newTestManager(t, policy, summarizer, store)
	input := &Input{Key: "conversation", Entries: []*Entry{
		{ID: "1", Message: llm.UserMessage(strings.Repeat("a", 35))},
		{ID: "2", Message: llm.AssistantMessage(strings.Repeat("b", 35))},
		{ID: "3", Message: llm.UserMessage(strings.Repeat("c", 25))},
	}}

	result, err := manager.Prepare(stdcontext.Background(), input)
	if err != nil {
		t.Fatalf("Manager.Prepare() error = %v, want nil", err)
	}
	if !result.Compacted || result.SummarizedThroughID == "" || store.state == nil {
		t.Fatalf("Manager.Prepare() result = %#v, state = %#v, want persisted summary", result, store.state)
	}
	if summarizer.calls != 1 || !strings.Contains(result.Messages[0].Content, "summary") {
		t.Fatalf("summarizer calls = %d, messages = %#v, want summary message", summarizer.calls, result.Messages)
	}
}

// TestManagerPrepareContinuesFromPersistedCursor 验证已有摘要只合并游标之后且不属于近期窗口的消息。
func TestManagerPrepareContinuesFromPersistedCursor(t *testing.T) {
	policy := smallPolicy()
	manager := newTestManager(t, policy, &fakeSummarizer{text: "next"}, &fakeStore{state: &State{
		Summary: "old", ThroughID: "1", Version: 3, FormatVersion: CurrentFormatVersion,
		PolicyFingerprint: policyFingerprintForTest(t, policy),
	}})
	result, err := manager.Prepare(stdcontext.Background(), &Input{Key: "conversation", Entries: []*Entry{
		{ID: "1", Message: llm.UserMessage(strings.Repeat("a", 35))},
		{ID: "2", Message: llm.AssistantMessage(strings.Repeat("b", 35))},
		{ID: "3", Message: llm.UserMessage(strings.Repeat("c", 25))},
	}})
	if err != nil {
		t.Fatalf("Manager.Prepare() error = %v, want nil", err)
	}
	if result.SummarizedThroughID != "2" {
		t.Fatalf("Manager.Prepare().SummarizedThroughID = %q, want 2", result.SummarizedThroughID)
	}
}

// TestManagerPrepareRetriesAfterCASConflict 验证并发版本冲突后会重新读取状态并只重试一次。
func TestManagerPrepareRetriesAfterCASConflict(t *testing.T) {
	store := &fakeStore{conflicts: 1}
	manager := newTestManager(t, smallPolicy(), &fakeSummarizer{text: "summary"}, store)
	result, err := manager.Prepare(stdcontext.Background(), &Input{Key: "conversation", Entries: []*Entry{
		{ID: "1", Message: llm.UserMessage(strings.Repeat("a", 35))},
		{ID: "2", Message: llm.AssistantMessage(strings.Repeat("b", 35))},
		{ID: "3", Message: llm.UserMessage(strings.Repeat("c", 25))},
	}})
	if err != nil {
		t.Fatalf("Manager.Prepare(CAS conflict) error = %v, want nil", err)
	}
	if store.loads != 2 || result.SummarizedThroughID == "" {
		t.Fatalf("store loads = %d, result = %#v, want one reload and persisted cursor", store.loads, result)
	}
}

// TestManagerPrepareReusesSummaryWithoutNewEligiblePrefix 验证没有新旧消息可摘要时会复用已有摘要而不是恢复全历史。
func TestManagerPrepareReusesSummaryWithoutNewEligiblePrefix(t *testing.T) {
	policy := smallPolicy()
	store := &fakeStore{state: &State{
		Summary: "old summary", ThroughID: "2", Version: 2, FormatVersion: CurrentFormatVersion,
		PolicyFingerprint: policyFingerprintForTest(t, policy),
	}}
	manager := newTestManager(t, policy, &fakeSummarizer{text: "unused"}, store)
	result, err := manager.Prepare(stdcontext.Background(), &Input{Key: "conversation", Entries: []*Entry{
		{ID: "1", Message: llm.UserMessage(strings.Repeat("a", 35))},
		{ID: "2", Message: llm.AssistantMessage(strings.Repeat("b", 35))},
		{ID: "3", Message: llm.UserMessage(strings.Repeat("c", 25))},
	}, Pinned: []*llm.Message{llm.SystemMessage(strings.Repeat("p", 10))}})
	if err != nil {
		t.Fatalf("Manager.Prepare(existing summary) error = %v, want nil", err)
	}
	if result.SummarizedThroughID != "2" || !strings.Contains(result.Messages[0].Content, "old summary") {
		t.Fatalf("Manager.Prepare(existing summary) result = %#v, want cursor 2 and old summary", result)
	}
}

// TestManagerPrepareFallsBackWhenSummarizerFails 验证摘要失败时会裁剪旧消息并继续返回可用上下文。
func TestManagerPrepareFallsBackWhenSummarizerFails(t *testing.T) {
	manager := newTestManager(t, smallPolicy(), &fakeSummarizer{err: errors.New("boom")}, &fakeStore{})
	result, err := manager.Prepare(stdcontext.Background(), &Input{Key: "conversation", Entries: []*Entry{
		{ID: "1", Message: llm.UserMessage(strings.Repeat("a", 40))},
		{ID: "2", Message: llm.AssistantMessage(strings.Repeat("b", 40))},
		{ID: "3", Message: llm.UserMessage("latest")},
	}})
	if err != nil {
		t.Fatalf("Manager.Prepare() error = %v, want nil fallback", err)
	}
	if !result.UsedFallback || len(result.Messages) == 0 || result.Messages[len(result.Messages)-1].Content != "latest" {
		t.Fatalf("Manager.Prepare() result = %#v, want fallback preserving latest message", result)
	}
}

// TestManagerPrepareMessagesKeepsToolCallAndResultTogether 验证最终裁剪不会拆散 assistant tool call 与对应工具结果。
func TestManagerPrepareMessagesKeepsToolCallAndResultTogether(t *testing.T) {
	manager := newTestManager(t, smallPolicy(), nil, nil)
	messages := []*llm.Message{
		llm.SystemMessage("system"),
		llm.UserMessage(strings.Repeat("old", 20)),
		{Role: llm.AssistantRole, ToolCalls: []llm.LLMToolCall{{ID: "call", Name: "search"}}},
		{Role: llm.ToolRole, ToolCallID: "call", Content: strings.Repeat("result", 20)},
		llm.UserMessage("latest"),
	}
	got, err := manager.PrepareMessages(stdcontext.Background(), messages, nil)
	if err != nil {
		t.Fatalf("Manager.PrepareMessages() error = %v, want nil", err)
	}
	hasCall, hasResult := false, false
	for _, message := range got {
		hasCall = hasCall || len(message.ToolCalls) > 0
		hasResult = hasResult || message.Role == llm.ToolRole
	}
	if hasCall != hasResult {
		t.Fatalf("Manager.PrepareMessages() tool call/result presence = %v/%v, want equal", hasCall, hasResult)
	}
}

// TestManagerPrepareRejectsPinnedContentOverBudget 验证固定输入本身超窗时返回明确错误而不裁剪当前问题。
func TestManagerPrepareRejectsPinnedContentOverBudget(t *testing.T) {
	manager := newTestManager(t, smallPolicy(), nil, nil)
	_, err := manager.Prepare(stdcontext.Background(), &Input{Pinned: []*llm.Message{llm.UserMessage(strings.Repeat("x", 200))}})
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("Manager.Prepare() error = %v, want ErrBudgetExceeded", err)
	}
}

// TestTiktokenCounterCountsMessagesAndToolSchemas 验证默认 tokenizer 能统计消息正文和完整工具 schema。
func TestTiktokenCounterCountsMessagesAndToolSchemas(t *testing.T) {
	counter, err := NewTiktokenCounter("")
	if err != nil {
		t.Fatalf("NewTiktokenCounter() error = %v", err)
	}
	tools := []coretool.Descriptor{{Name: "search", Schema: coretool.Schema{Parameters: coretool.NewParametersSchema(map[string]any{
		"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}},
	})}}}
	if counter.CountMessages([]*llm.Message{llm.UserMessage("hello")}) <= 0 || counter.CountTools(tools) <= 0 {
		t.Fatal("TiktokenCounter returned non-positive count for non-empty input")
	}
}

type runeCounter struct{}

func (runeCounter) CountMessages(messages []*llm.Message) int {
	total := 0
	for _, message := range messages {
		if message != nil {
			total += len([]rune(message.Content)) + len(message.ToolCalls)*5
		}
	}
	return total
}

func (runeCounter) CountTools(tools []coretool.Descriptor) int { return len(tools) * 5 }

type fakeSummarizer struct {
	text  string
	err   error
	calls int
}

type fakeLLMClient struct{}

func (*fakeLLMClient) Invoke(stdcontext.Context, []*llm.Message, ...llm.ModelCallOption) (string, error) {
	return "summary", nil
}

func (*fakeLLMClient) InvokeResult(stdcontext.Context, []*llm.Message, ...llm.ModelCallOption) (*llm.LLMResult, error) {
	return &llm.LLMResult{Text: "summary"}, nil
}

func (*fakeLLMClient) Stream(stdcontext.Context, []*llm.Message, ...llm.ModelCallOption) (<-chan string, error) {
	return nil, errors.New("not implemented")
}

func (*fakeLLMClient) Embed(stdcontext.Context, []string, int, ...llm.EmbeddingOption) ([][]float64, error) {
	return nil, errors.New("not implemented")
}

func (*fakeLLMClient) EmbedOne(stdcontext.Context, string, int) ([]float64, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeSummarizer) Summarize(_ stdcontext.Context, _ string, _ []*llm.Message, _ int64) (string, error) {
	f.calls++
	return f.text, f.err
}

type fakeStore struct {
	state     *State
	loads     int
	conflicts int
}

func (f *fakeStore) Load(_ stdcontext.Context, _ string) (*State, error) {
	f.loads++
	if f.state == nil {
		return nil, nil
	}
	cloned := *f.state
	return &cloned, nil
}

func (f *fakeStore) CompareAndSwap(_ stdcontext.Context, _ string, expectedVersion int64, next *State) (bool, error) {
	if f.conflicts > 0 {
		f.conflicts--
		return false, nil
	}
	current := int64(0)
	if f.state != nil {
		current = f.state.Version
	}
	if current != expectedVersion {
		return false, nil
	}
	cloned := *next
	f.state = &cloned
	return true, nil
}

func smallPolicy() *Policy {
	return &Policy{Enabled: true, WindowTokens: 120, OutputReserveTokens: 10, SafetyMarginTokens: 10, TriggerRatio: 0.8, TargetRatio: 0.6, KeepRecentTokens: 25, SummaryMaxTokens: 10}
}

func newTestManager(t *testing.T, policy *Policy, summarizer Summarizer, store Store) *Manager {
	t.Helper()
	options := []Option{WithPolicy(policy), WithCounter(runeCounter{})}
	if summarizer != nil {
		options = append(options, WithSummarizer(summarizer))
	}
	if store != nil {
		options = append(options, WithStore(store))
	}
	manager, err := NewManager(options...)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return manager
}

func policyFingerprintForTest(t *testing.T, policy *Policy) string {
	t.Helper()
	manager := newTestManager(t, policy, nil, nil)
	return manager.policyFingerprint()
}
