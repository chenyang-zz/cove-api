package react

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/boxify/api-go/internal/core/llm"
	coretool "github.com/boxify/api-go/internal/core/tool"
)

// 验证点：ReAct parser 应优先识别 Final Answer，并返回 final 决策。
func TestReActParserParsesFinalAnswer(t *testing.T) {
	parser := NewReActParser()

	decision, err := parser.Parse(context.Background(), "Thought: enough\nFinal Answer: done")
	if err != nil {
		t.Fatalf("ReActParser.Parse(final) error = %v, want nil", err)
	}
	if decision.Kind != DecisionFinal || decision.FinalAnswer != "done" {
		t.Fatalf("ReActParser.Parse(final) = %#v, want final answer done", decision)
	}
}

// 验证点：ReAct parser 应解析 Action 和 JSON object Action Input。
func TestReActParserParsesToolCallWithJSONInput(t *testing.T) {
	parser := NewReActParser()

	decision, err := parser.Parse(context.Background(), `Thought: need search
Action: knowledge_search
Action Input: {"query":"golang","top_k":3}`)
	if err != nil {
		t.Fatalf("ReActParser.Parse(tool call) error = %v, want nil", err)
	}
	if decision.Kind != DecisionToolCall || decision.Action != "knowledge_search" {
		t.Fatalf("ReActParser.Parse(tool call) = %#v, want knowledge_search call", decision)
	}
	if decision.ActionInput["query"] != "golang" || decision.ActionInput["top_k"] != float64(3) {
		t.Fatalf("ReActParser.Parse(tool call).ActionInput = %#v, want query/top_k", decision.ActionInput)
	}
}

// 验证点：ReAct parser 应把纯文本 Action Input 映射为默认 query 字段。
func TestReActParserParsesPlainTextActionInputAsQuery(t *testing.T) {
	parser := NewReActParser()

	decision, err := parser.Parse(context.Background(), `Thought: need search
Action: knowledge_search
Action Input: golang rag`)
	if err != nil {
		t.Fatalf("ReActParser.Parse(plain text input) error = %v, want nil", err)
	}
	if decision.Kind != DecisionToolCall || decision.ActionInput["query"] != "golang rag" {
		t.Fatalf("ReActParser.Parse(plain text input) = %#v, want query golang rag", decision)
	}
}

// 验证点：ReAct parser 应支持调用方配置纯文本 Action Input 的目标字段名。
func TestReActParserParsesPlainTextActionInputWithCustomKey(t *testing.T) {
	parser := NewReActParser(WithTextActionInputKey("input"))

	decision, err := parser.Parse(context.Background(), `Thought: need search
Action: web_search
Action Input: golang rag`)
	if err != nil {
		t.Fatalf("ReActParser.Parse(custom plain text input) error = %v, want nil", err)
	}
	if decision.ActionInput["input"] != "golang rag" {
		t.Fatalf("ReActParser.Parse(custom plain text input).ActionInput = %#v, want input golang rag", decision.ActionInput)
	}
}

// 验证点：ReAct parser 对缺失 Action Input 应返回空输入，支持最小 ReAct 兜底格式。
func TestReActParserAllowsMissingActionInput(t *testing.T) {
	parser := NewReActParser()

	decision, err := parser.Parse(context.Background(), "Thought: check\nAction: current_time")
	if err != nil {
		t.Fatalf("ReActParser.Parse(missing input) error = %v, want nil", err)
	}
	if decision.Kind != DecisionToolCall || len(decision.ActionInput) != 0 {
		t.Fatalf("ReActParser.Parse(missing input) = %#v, want empty tool input", decision)
	}
}

// 验证点：ReAct parser 应拒绝非法或非 object 的 Action Input。
func TestReActParserRejectsInvalidActionInput(t *testing.T) {
	parser := NewReActParser()

	_, err := parser.Parse(context.Background(), "Thought: bad\nAction: search\nAction Input: [1,2]")
	if !errors.Is(err, ErrInvalidActionInput) {
		t.Fatalf("ReActParser.Parse(array input) error = %v, want ErrInvalidActionInput", err)
	}
}

// 验证点：默认 ReAct prompt builder 应渲染通过 SystemPrompt 注入的业务信息、中文模板和工具列表。
func TestReActPromptBuilderRendersDefaultChineseTemplate(t *testing.T) {
	builder := NewReActPromptBuilder()

	messages, err := builder.Build(context.Background(), State{
		SystemPrompt: "你是「Cove」的智能助手。\n\n回答要简洁。",
		Tools: []coretool.Descriptor{
			{Name: "knowledge_search", Description: "检索知识库。"},
			{Name: "current_time", Description: "获取当前时间。"},
		},
		Input: Input{Query: "你好"},
	})
	if err != nil {
		t.Fatalf("ReActPromptBuilder.Build() error = %v, want nil", err)
	}
	if len(messages) < 2 {
		t.Fatalf("ReActPromptBuilder.Build() messages len = %d, want at least 2", len(messages))
	}
	system := messages[0].Content
	for _, want := range []string{"Cove", "- knowledge_search：检索知识库。", "Action Input: 传给工具的查询关键词（一行纯文本）", "回答要简洁。"} {
		if !strings.Contains(system, want) {
			t.Fatalf("ReActPromptBuilder.Build() system prompt = %q, want to contain %q", system, want)
		}
	}
	if strings.Contains(system, "parameters:") {
		t.Fatalf("ReActPromptBuilder.Build() system prompt = %q, want no schema parameters in text prompt", system)
	}
}

// 验证点：默认 ReAct prompt builder 不配置 intro 时仍生成协议文本且不包含业务品牌。
func TestReActPromptBuilderOmitsBusinessIdentityByDefault(t *testing.T) {
	builder := NewReActPromptBuilder()

	messages, err := builder.Build(context.Background(), State{
		Tools: []coretool.Descriptor{{Name: "current_time", Description: "获取当前时间。"}},
		Input: Input{Query: "你好"},
	})
	if err != nil {
		t.Fatalf("ReActPromptBuilder.Build(default) error = %v, want nil", err)
	}
	system := messages[0].Content
	if !strings.Contains(system, "请按以下格式逐步推理") || !strings.Contains(system, "- current_time：获取当前时间。") {
		t.Fatalf("ReActPromptBuilder.Build(default) system prompt = %q, want react protocol and tool list", system)
	}
	for _, forbidden := range []string{"彗记", "Cove", "你是「"} {
		if strings.Contains(system, forbidden) {
			t.Fatalf("ReActPromptBuilder.Build(default) system prompt = %q, want no business identity containing %q", system, forbidden)
		}
	}
}

// 验证点：WithSystemPrompt 应通过默认 builder 注入业务身份到文本 ReAct 模型消息。
func TestAgentRunInjectsBusinessIdentityThroughSystemPrompt(t *testing.T) {
	ctx := context.Background()
	model := &fakeAgentLLM{outputs: []string{"Thought: done\nFinal Answer: ok"}}

	_, err := New(model, coretool.NewRegistry(),
		WithToolCallingEnabled(false),
		WithSystemPrompt("你是「Cove」的智能助手。"),
	).Run(ctx, Input{Query: "你好"})
	if err != nil {
		t.Fatalf("Agent.Run(system prompt business identity) error = %v, want nil", err)
	}
	if len(model.messages) != 1 || len(model.messages[0]) == 0 {
		t.Fatalf("model messages = %#v, want one call with system message", model.messages)
	}
	system := model.messages[0][0].Content
	if !strings.Contains(system, "Cove") {
		t.Fatalf("system prompt = %q, want Cove business identity", system)
	}
}

// 验证点：模型直接返回 Final Answer 时 Agent 不调用工具并正常结束。
func TestAgentRunReturnsDirectFinalAnswer(t *testing.T) {
	ctx := context.Background()
	model := &fakeAgentLLM{outputs: []string{"Thought: done\nFinal Answer: hello"}}
	registry := coretool.NewRegistry()

	result, err := New(model, registry).Run(ctx, Input{Query: "say hello"})
	if err != nil {
		t.Fatalf("Agent.Run(final) error = %v, want nil", err)
	}
	if result.Answer != "hello" || result.StoppedBy != StopFinalAnswer {
		t.Fatalf("Agent.Run(final) result = %#v, want answer hello and final stop", result)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("Agent.Run(final) steps len = %d, want 1", len(result.Steps))
	}
}

// 验证点：Agent 应执行工具调用，把 Observation 写回下一轮 prompt，并最终返回答案。
func TestAgentRunExecutesToolAndFeedsObservation(t *testing.T) {
	ctx := context.Background()
	model := &fakeAgentLLM{outputs: []string{
		`Thought: need time
Action: current_time
Action Input: now`,
		"Thought: observed\nFinal Answer: It is noon.",
	}}
	registry := coretool.NewRegistry()
	err := registry.Register(ctx, coretool.NewFuncTool(coretool.Descriptor{
		Name:        "current_time",
		Description: "Get current time.",
	}, func(ctx context.Context, input coretool.Input) (coretool.Output, error) {
		return coretool.Output{Text: "12:00"}, nil
	}))
	if err != nil {
		t.Fatalf("Registry.Register(current_time) error = %v, want nil", err)
	}

	result, err := New(model, registry).Run(ctx, Input{Query: "time?"})
	if err != nil {
		t.Fatalf("Agent.Run(tool call) error = %v, want nil", err)
	}
	if result.Answer != "It is noon." {
		t.Fatalf("Agent.Run(tool call).Answer = %q, want It is noon.", result.Answer)
	}
	if len(result.Steps) != 2 || result.Steps[0].Observation != "12:00" {
		t.Fatalf("Agent.Run(tool call).Steps = %#v, want first observation 12:00", result.Steps)
	}
	if len(model.messages) != 2 || !strings.Contains(joinMessages(model.messages[1]), "Observation: 12:00") {
		t.Fatalf("second model prompt = %q, want Observation: 12:00", joinMessages(model.messages[1]))
	}
}

// 验证点：Agent 达到最大迭代次数时应返回 ErrMaxIterations 和部分步骤。
func TestAgentRunStopsAtMaxIterations(t *testing.T) {
	ctx := context.Background()
	model := &fakeAgentLLM{outputs: []string{
		"Thought: again\nAction: current_time\nAction Input: {}",
		"Thought: again\nAction: current_time\nAction Input: {}",
	}}
	registry := coretool.NewRegistry()
	err := registry.Register(ctx, coretool.NewFuncTool(coretool.Descriptor{Name: "current_time"}, func(ctx context.Context, input coretool.Input) (coretool.Output, error) {
		return coretool.Output{Text: "tick"}, nil
	}))
	if err != nil {
		t.Fatalf("Registry.Register(current_time) error = %v, want nil", err)
	}

	result, err := New(model, registry, WithMaxIterations(2)).Run(ctx, Input{Query: "loop"})
	if !errors.Is(err, ErrMaxIterations) {
		t.Fatalf("Agent.Run(max iterations) error = %v, want ErrMaxIterations", err)
	}
	if result == nil || result.StoppedBy != StopMaxIterations || len(result.Steps) != 2 {
		t.Fatalf("Agent.Run(max iterations) result = %#v, want partial result with 2 steps", result)
	}
}

// 验证点：Agent 应触发状态迁移 hook，保留后续接入状态机的 phase 边界。
func TestAgentRunEmitsTransitionHooks(t *testing.T) {
	ctx := context.Background()
	model := &fakeAgentLLM{outputs: []string{"Thought: done\nFinal Answer: ok"}}
	hooks := &recordingHooks{}

	_, err := New(model, coretool.NewRegistry(), WithHooks(hooks)).Run(ctx, Input{Query: "ok"})
	if err != nil {
		t.Fatalf("Agent.Run(hooks) error = %v, want nil", err)
	}

	want := []string{
		"before_run",
		"before_transition:start->build_prompt",
		"after_transition:start->build_prompt",
		"before_transition:build_prompt->model",
		"before_model",
		"after_model",
		"before_transition:model->parse",
		"after_parse",
		"before_transition:parse->finish",
		"after_run",
	}
	if !containsInOrder(hooks.events, want) {
		t.Fatalf("hook events = %#v, want ordered subsequence %#v", hooks.events, want)
	}
}

// 验证点：模型实现 ToolCallingClient 时，Agent 默认优先使用 function calling 并直接返回最终答案。
func TestAgentRunUsesToolCallingClientByDefault(t *testing.T) {
	ctx := context.Background()
	model := &fakeToolCallingLLM{
		toolOutputs: []ToolCallingOutput{{Content: "tool calling final"}},
	}

	result, err := New(model, coretool.NewRegistry()).Run(ctx, Input{Query: "answer"})
	if err != nil {
		t.Fatalf("Agent.Run(tool calling final) error = %v, want nil", err)
	}
	if result.Answer != "tool calling final" {
		t.Fatalf("Agent.Run(tool calling final).Answer = %q, want tool calling final", result.Answer)
	}
	if len(model.toolInputs) != 1 {
		t.Fatalf("ToolCallingClient.InvokeWithTools calls = %d, want 1", len(model.toolInputs))
	}
	if len(model.messages) != 0 {
		t.Fatalf("llm.Client.Invoke calls = %d, want 0", len(model.messages))
	}
}

// 验证点：function calling 工具调用应执行工具，并把 observation 和 tool call ID 传入下一轮。
func TestAgentRunExecutesFunctionToolCallAndFeedsSteps(t *testing.T) {
	ctx := context.Background()
	model := &fakeToolCallingLLM{
		toolOutputs: []ToolCallingOutput{
			{ToolCalls: []ModelToolCall{{ID: "call_1", Name: "current_time", Input: coretool.Input{"zone": "UTC"}}}},
			{Content: "It is noon."},
		},
	}
	registry := coretool.NewRegistry()
	err := registry.Register(ctx, coretool.NewFuncTool(coretool.Descriptor{Name: "current_time"}, func(ctx context.Context, input coretool.Input) (coretool.Output, error) {
		return coretool.Output{Text: "12:00"}, nil
	}))
	if err != nil {
		t.Fatalf("Registry.Register(current_time) error = %v, want nil", err)
	}

	result, err := New(model, registry).Run(ctx, Input{Query: "time?"})
	if err != nil {
		t.Fatalf("Agent.Run(function tool call) error = %v, want nil", err)
	}
	if result.Answer != "It is noon." {
		t.Fatalf("Agent.Run(function tool call).Answer = %q, want It is noon.", result.Answer)
	}
	if len(result.Steps) != 2 || result.Steps[0].Observation != "12:00" || result.Steps[0].ToolCallID != "call_1" {
		t.Fatalf("Agent.Run(function tool call).Steps = %#v, want observation 12:00 and tool call id call_1", result.Steps)
	}
	if len(model.toolInputs) != 2 || len(model.toolInputs[1].Steps) != 1 || model.toolInputs[1].Steps[0].Observation != "12:00" {
		t.Fatalf("second ToolCallingInput = %#v, want previous observation 12:00", model.toolInputs)
	}
}

// 验证点：显式关闭 function calling 后，即使模型支持 ToolCallingClient 也应走文本 ReAct。
func TestAgentRunDisablesToolCalling(t *testing.T) {
	ctx := context.Background()
	model := &fakeToolCallingLLM{
		fakeAgentLLM: fakeAgentLLM{outputs: []string{"Thought: done\nFinal Answer: react final"}},
		toolOutputs:  []ToolCallingOutput{{Content: "tool calling final"}},
	}

	result, err := New(model, coretool.NewRegistry(), WithToolCallingEnabled(false)).Run(ctx, Input{Query: "answer"})
	if err != nil {
		t.Fatalf("Agent.Run(disable tool calling) error = %v, want nil", err)
	}
	if result.Answer != "react final" {
		t.Fatalf("Agent.Run(disable tool calling).Answer = %q, want react final", result.Answer)
	}
	if len(model.toolInputs) != 0 {
		t.Fatalf("ToolCallingClient.InvokeWithTools calls = %d, want 0", len(model.toolInputs))
	}
	if len(model.messages) != 1 {
		t.Fatalf("llm.Client.Invoke calls = %d, want 1", len(model.messages))
	}
}

// 验证点：function calling 明确不支持且允许 fallback 时，应切换到文本 ReAct 并触发 fallback 状态迁移。
func TestAgentRunFallsBackToReActWhenToolCallingUnsupported(t *testing.T) {
	ctx := context.Background()
	model := &fakeToolCallingLLM{
		fakeAgentLLM: fakeAgentLLM{outputs: []string{"Thought: fallback\nFinal Answer: react final"}},
		toolErr:      ErrToolCallingUnsupported,
	}
	hooks := &recordingHooks{}

	result, err := New(model, coretool.NewRegistry(), WithHooks(hooks)).Run(ctx, Input{Query: "answer"})
	if err != nil {
		t.Fatalf("Agent.Run(fallback) error = %v, want nil", err)
	}
	if result.Answer != "react final" {
		t.Fatalf("Agent.Run(fallback).Answer = %q, want react final", result.Answer)
	}
	want := []string{
		"before_transition:build_prompt->model",
		"before_transition:model->fallback",
		"before_transition:fallback->build_prompt",
		"before_transition:build_prompt->parse",
	}
	if !containsInOrder(hooks.events, want) {
		t.Fatalf("hook events = %#v, want ordered fallback transitions %#v", hooks.events, want)
	}
}

// 验证点：关闭 fallback 后，function calling 不支持错误应直接返回，不再调用文本 ReAct。
func TestAgentRunReturnsToolCallingErrorWhenFallbackDisabled(t *testing.T) {
	ctx := context.Background()
	model := &fakeToolCallingLLM{
		fakeAgentLLM: fakeAgentLLM{outputs: []string{"Thought: fallback\nFinal Answer: react final"}},
		toolErr:      ErrToolCallingUnsupported,
	}

	result, err := New(model, coretool.NewRegistry(), WithFallbackToReAct(false)).Run(ctx, Input{Query: "answer"})
	if !errors.Is(err, ErrToolCallingUnsupported) {
		t.Fatalf("Agent.Run(fallback disabled) error = %v, want ErrToolCallingUnsupported", err)
	}
	if result == nil || result.StoppedBy != StopError {
		t.Fatalf("Agent.Run(fallback disabled) result = %#v, want StopError", result)
	}
	if len(model.messages) != 0 {
		t.Fatalf("llm.Client.Invoke calls = %d, want 0", len(model.messages))
	}
}

// 验证点：react 公开的 State、Result、NoopHooks 等 alias 仍能作为调用方 hooks 类型使用。
func TestReactAliasesExposeBaseTypes(t *testing.T) {
	var hooks Hooks = NoopHooks{}
	state := State{Iteration: 1, LastDecision: Decision{ActionInput: coretool.Input{"query": "hello"}}}
	result := Result{Steps: []Step{{ActionInput: coretool.Input{"query": "hello"}}}, StoppedBy: StopFinalAnswer}

	if err := hooks.BeforeRun(context.Background(), state); err != nil {
		t.Fatalf("NoopHooks.BeforeRun() error = %v, want nil", err)
	}
	if err := hooks.AfterRun(context.Background(), result, nil); err != nil {
		t.Fatalf("NoopHooks.AfterRun() error = %v, want nil", err)
	}
	if state.LastDecision.ActionInput["query"] != "hello" || result.Steps[0].ActionInput["query"] != "hello" {
		t.Fatalf("aliases state/result = %#v/%#v, want preserved values", state, result)
	}
}

type fakeAgentLLM struct {
	outputs  []string
	err      error
	messages [][]*llm.Message
}

func (f *fakeAgentLLM) Invoke(ctx context.Context, messages []*llm.Message, opts ...llm.ModelCallOption) (string, error) {
	f.messages = append(f.messages, cloneMessages(messages))
	if f.err != nil {
		return "", f.err
	}
	if len(f.outputs) == 0 {
		return "", errors.New("no model output")
	}
	out := f.outputs[0]
	f.outputs = f.outputs[1:]
	return out, nil
}

func (f *fakeAgentLLM) InvokeResult(ctx context.Context, messages []*llm.Message, opts ...llm.ModelCallOption) (*llm.LLMResult, error) {
	text, err := f.Invoke(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	return &llm.LLMResult{Text: text}, nil
}

func (f *fakeAgentLLM) Stream(ctx context.Context, messages []*llm.Message, opts ...llm.ModelCallOption) (<-chan string, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgentLLM) Embed(ctx context.Context, texts []string, dimensions int, opts ...llm.EmbeddingOption) ([][]float64, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgentLLM) EmbedOne(ctx context.Context, text string, dimensions int) ([]float64, error) {
	return nil, errors.New("not implemented")
}

type fakeToolCallingLLM struct {
	fakeAgentLLM
	toolOutputs []ToolCallingOutput
	toolErr     error
	toolInputs  []ToolCallingInput
}

func (f *fakeToolCallingLLM) InvokeWithTools(ctx context.Context, input ToolCallingInput, opts ...llm.ModelCallOption) (ToolCallingOutput, error) {
	f.toolInputs = append(f.toolInputs, cloneToolCallingInput(input))
	if f.toolErr != nil {
		return ToolCallingOutput{}, f.toolErr
	}
	if len(f.toolOutputs) == 0 {
		return ToolCallingOutput{}, errors.New("no tool calling output")
	}
	out := f.toolOutputs[0]
	f.toolOutputs = f.toolOutputs[1:]
	return out, nil
}

type recordingHooks struct {
	events []string
}

func (h *recordingHooks) BeforeRun(ctx context.Context, state State) error {
	h.events = append(h.events, "before_run")
	return nil
}

func (h *recordingHooks) AfterRun(ctx context.Context, result Result, runErr error) error {
	h.events = append(h.events, "after_run")
	return nil
}

func (h *recordingHooks) BeforeTransition(ctx context.Context, state State, transition Transition) error {
	h.events = append(h.events, "before_transition:"+string(transition.From)+"->"+string(transition.To))
	return nil
}

func (h *recordingHooks) AfterTransition(ctx context.Context, state State, transition Transition) error {
	h.events = append(h.events, "after_transition:"+string(transition.From)+"->"+string(transition.To))
	return nil
}

func (h *recordingHooks) BeforeModel(ctx context.Context, state State, messages []*llm.Message) error {
	h.events = append(h.events, "before_model")
	return nil
}

func (h *recordingHooks) AfterModel(ctx context.Context, state State, output string, modelErr error) error {
	h.events = append(h.events, "after_model")
	return nil
}

func (h *recordingHooks) AfterParse(ctx context.Context, state State, decision Decision, parseErr error) error {
	h.events = append(h.events, "after_parse")
	return nil
}

func (h *recordingHooks) BeforeTool(ctx context.Context, state State, call ToolCall) error {
	h.events = append(h.events, "before_tool")
	return nil
}

func (h *recordingHooks) AfterTool(ctx context.Context, state State, call ToolCall, output coretool.Output, toolErr error) error {
	h.events = append(h.events, "after_tool")
	return nil
}

func (h *recordingHooks) OnStep(ctx context.Context, state State, step Step) error {
	h.events = append(h.events, "on_step")
	return nil
}

func (h *recordingHooks) OnError(ctx context.Context, state State, err error) error {
	h.events = append(h.events, "on_error")
	return nil
}

func cloneMessages(messages []*llm.Message) []*llm.Message {
	return llm.CloneMessages(messages)
}

func cloneToolCallingInput(input ToolCallingInput) ToolCallingInput {
	return ToolCallingInput{
		Messages: cloneMessages(input.Messages),
		Tools:    cloneToolDescriptors(input.Tools),
		Steps:    cloneSteps(input.Steps),
	}
}

func joinMessages(messages []*llm.Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		parts = append(parts, message.Content)
	}
	return strings.Join(parts, "\n")
}

func containsInOrder(got []string, want []string) bool {
	position := 0
	for _, event := range got {
		if position < len(want) && reflect.DeepEqual(event, want[position]) {
			position++
		}
	}
	return position == len(want)
}
