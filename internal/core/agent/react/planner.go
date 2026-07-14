package react

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/boxify/api-go/internal/core/llm"
	coretool "github.com/boxify/api-go/internal/core/tool"
)

type plannerResult struct {
	Decision Decision
	Output   string
	Fallback bool
}

type tracePlanner interface {
	planTrace(ctx context.Context, state State, opts ...llm.ModelCallOption) (plannerResult, error)
}

type tokenEmitter func(ctx context.Context, text string) error

type streamTracePlanner interface {
	planStreamTrace(ctx context.Context, state State, emit tokenEmitter, opts ...llm.ModelCallOption) (plannerResult, error)
}

type modelMessagePlanner interface {
	modelMessages(ctx context.Context, state State) ([]*llm.Message, error)
}

// ReActTextPlanner 使用文本 ReAct prompt 和 parser 生成决策。
type ReActTextPlanner struct {
	client        llm.Client
	promptBuilder PromptBuilder
	parser        Parser
	preparer      MessagePreparer
}

// NewReActTextPlanner 创建文本 ReAct planner。
//
// builder 或 parser 为 nil 时会使用 core 内置默认实现。client 为 nil 时，Plan 会返回错误。
func NewReActTextPlanner(client llm.Client, builder PromptBuilder, parser Parser) *ReActTextPlanner {
	if builder == nil {
		builder = NewReActPromptBuilder()
	}
	if parser == nil {
		parser = NewReActParser()
	}
	return &ReActTextPlanner{
		client:        client,
		promptBuilder: builder,
		parser:        parser,
	}
}

// Plan 调用文本模型并解析 ReAct 决策。
func (p *ReActTextPlanner) Plan(ctx context.Context, state State, opts ...llm.ModelCallOption) (Decision, error) {
	result, err := p.planTrace(ctx, state, opts...)
	if err != nil {
		return Decision{}, err
	}
	return result.Decision, nil
}

// planTrace 通过同步文本调用生成并解析 ReAct 决策。
//
// 此路径不依赖结构化流接口，供不支持流式响应的模型使用。
func (p *ReActTextPlanner) planTrace(ctx context.Context, state State, opts ...llm.ModelCallOption) (plannerResult, error) {
	if p == nil {
		return plannerResult{}, errors.New("react text planner is nil")
	}
	if p.client == nil {
		return plannerResult{}, errors.New("agent model client is nil")
	}
	messages, err := p.modelMessages(ctx, state)
	if err != nil {
		return plannerResult{}, err
	}
	text, err := p.client.Invoke(ctx, messages, opts...)
	if err != nil {
		return plannerResult{}, err
	}
	decision, err := p.parser.Parse(ctx, text)
	if err != nil {
		return plannerResult{Output: text}, err
	}
	return plannerResult{
		Decision: decision,
		Output:   text,
	}, nil
}

// planStreamTrace 使用结构化流读取 ReAct 原文，并且只转发 Final Answer 之后的可展示内容。
//
// 模型不支持结构化流时会退回 planTrace，不会伪造 token 回调。
func (p *ReActTextPlanner) planStreamTrace(ctx context.Context, state State, emit tokenEmitter, opts ...llm.ModelCallOption) (plannerResult, error) {
	if p == nil {
		return plannerResult{}, errors.New("react text planner is nil")
	}
	if p.client == nil {
		return plannerResult{}, errors.New("agent model client is nil")
	}
	streamClient, ok := p.client.(llm.StreamEventClient)
	if !ok {
		return p.planTrace(ctx, state, opts...)
	}
	if p.promptBuilder == nil {
		return plannerResult{}, errors.New("react prompt builder is nil")
	}
	messages, err := p.modelMessages(ctx, state)
	if err != nil {
		return plannerResult{}, err
	}
	events, err := streamClient.StreamEvents(ctx, messages, opts...)
	if err != nil {
		return plannerResult{}, err
	}
	var output strings.Builder
	visible := newFinalAnswerEmitter(emit)
	finished := false
	for event := range events {
		switch event.Kind {
		case llm.StreamEventTextDelta:
			output.WriteString(event.Text)
			if err := visible.Feed(ctx, event.Text); err != nil {
				return plannerResult{Output: output.String()}, err
			}
		case llm.StreamEventError:
			if event.Err == nil {
				event.Err = errors.New("model stream failed")
			}
			return plannerResult{Output: output.String()}, event.Err
		case llm.StreamEventDone:
			finished = true
		}
	}
	text := output.String()
	if !finished {
		if err := ctx.Err(); err != nil {
			return plannerResult{Output: text}, err
		}
		return plannerResult{Output: text}, errors.New("model stream ended without terminal event")
	}
	decision, err := p.parser.Parse(ctx, text)
	if err != nil {
		return plannerResult{Output: text}, err
	}
	return plannerResult{
		Decision: decision,
		Output:   text,
	}, nil
}

func (p *ReActTextPlanner) modelMessages(ctx context.Context, state State) ([]*llm.Message, error) {
	if p == nil {
		return nil, errors.New("react text planner is nil")
	}
	if p.promptBuilder == nil {
		return nil, errors.New("react prompt builder is nil")
	}
	messages, err := p.promptBuilder.Build(ctx, cloneState(state))
	if err != nil || p.preparer == nil {
		return messages, err
	}
	return p.preparer.PrepareMessages(ctx, messages, state.Tools)
}

// FunctionCallingPlanner 使用模型原生工具调用能力生成决策。
type FunctionCallingPlanner struct {
	client   llm.ToolCallingClient
	preparer MessagePreparer
}

// NewFunctionCallingPlanner 创建 function calling planner。
func NewFunctionCallingPlanner(client llm.ToolCallingClient) *FunctionCallingPlanner {
	return &FunctionCallingPlanner{client: client}
}

// SupportsToolCalling reports whether planner 持有可用的 llm.ToolCallingClient。
func (p *FunctionCallingPlanner) SupportsToolCalling() bool {
	return p != nil && p.client != nil
}

// Plan 调用支持工具调用的模型，并把输出规整为统一 Decision。
func (p *FunctionCallingPlanner) Plan(ctx context.Context, state State, opts ...llm.ModelCallOption) (Decision, error) {
	result, err := p.planTrace(ctx, state, opts...)
	if err != nil {
		return Decision{}, err
	}
	return result.Decision, nil
}

// planTrace 通过同步原生工具调用生成决策，并把输出规整为统一 Decision。
//
// 此路径不依赖工具调用流接口，供不支持流式响应的模型使用。
func (p *FunctionCallingPlanner) planTrace(ctx context.Context, state State, opts ...llm.ModelCallOption) (plannerResult, error) {
	if !p.SupportsToolCalling() {
		return plannerResult{}, ErrToolCallingUnsupported
	}
	messages, err := p.modelMessages(ctx, state)
	if err != nil {
		return plannerResult{}, err
	}
	callOpts := make([]llm.ModelCallOption, 0, len(opts)+1)
	callOpts = append(callOpts, opts...)
	if len(state.Tools) > 0 {
		callOpts = append(callOpts, llm.WithTools(state.Tools...))
	}
	output, err := p.client.InvokeWithTools(ctx, messages, callOpts...)
	if err != nil {
		return plannerResult{}, err
	}
	decision, err := decisionFromToolCallingOutput(output)
	if err != nil {
		return plannerResult{Output: outputSummary(output)}, err
	}
	return plannerResult{
		Decision: decision,
		Output:   outputSummary(output),
	}, nil
}

// planStreamTrace 使用原生工具调用流聚合工具参数，并即时转发供应商给出的文本增量。
//
// 模型不支持工具调用流时会退回 planTrace，不会伪造 token 回调。
func (p *FunctionCallingPlanner) planStreamTrace(ctx context.Context, state State, emit tokenEmitter, opts ...llm.ModelCallOption) (plannerResult, error) {
	if !p.SupportsToolCalling() {
		return plannerResult{}, ErrToolCallingUnsupported
	}
	streamClient, ok := p.client.(llm.ToolStreamEventClient)
	if !ok {
		return p.planTrace(ctx, state, opts...)
	}
	messages, err := p.modelMessages(ctx, state)
	if err != nil {
		return plannerResult{}, err
	}
	callOpts := make([]llm.ModelCallOption, 0, len(opts)+1)
	callOpts = append(callOpts, opts...)
	if len(state.Tools) > 0 {
		callOpts = append(callOpts, llm.WithTools(state.Tools...))
	}
	events, err := streamClient.StreamWithTools(ctx, messages, callOpts...)
	if err != nil {
		return plannerResult{}, err
	}
	output := &llm.LLMResult{}
	finished := false
	for event := range events {
		switch event.Kind {
		case llm.StreamEventTextDelta:
			output.Text += event.Text
			if emit != nil && event.Text != "" {
				if err := emit(ctx, event.Text); err != nil {
					return plannerResult{Output: outputSummary(output)}, err
				}
			}
		case llm.StreamEventToolCall:
			if event.ToolCall != nil {
				output.ToolCalls = append(output.ToolCalls, *event.ToolCall)
			}
		case llm.StreamEventError:
			if event.Err == nil {
				event.Err = errors.New("model stream failed")
			}
			return plannerResult{Output: outputSummary(output)}, event.Err
		case llm.StreamEventDone:
			finished = true
		}
	}
	if !finished {
		if err := ctx.Err(); err != nil {
			return plannerResult{Output: outputSummary(output)}, err
		}
		return plannerResult{Output: outputSummary(output)}, errors.New("model stream ended without terminal event")
	}
	decision, err := decisionFromToolCallingOutput(output)
	if err != nil {
		return plannerResult{Output: outputSummary(output)}, err
	}
	return plannerResult{
		Decision: decision,
		Output:   outputSummary(output),
	}, nil
}

func (p *FunctionCallingPlanner) modelMessages(ctx context.Context, state State) ([]*llm.Message, error) {
	messages := toolCallingMessages(state)
	if p.preparer == nil {
		return messages, nil
	}
	return p.preparer.PrepareMessages(ctx, messages, state.Tools)
}

// AutoPlanner 按配置在 function calling 和文本 ReAct 之间选择执行路径。
type AutoPlanner struct {
	toolPlanner        *FunctionCallingPlanner
	reactPlanner       *ReActTextPlanner
	toolCallingEnabled bool
	fallbackToReAct    bool
}

// NewAutoPlanner 创建自动选择 planner。
//
// 当 client 实现 llm.ToolCallingClient 且 enabled 为 true 时优先使用 function calling；
// 否则直接使用文本 ReAct。function calling 返回 ErrToolCallingUnsupported 时，fallback
// 为 true 会自动退回文本 ReAct。
func NewAutoPlanner(client llm.Client, builder PromptBuilder, parser Parser, enabled bool, fallback bool) *AutoPlanner {
	var toolPlanner *FunctionCallingPlanner
	if toolClient, ok := client.(llm.ToolCallingClient); ok {
		toolPlanner = NewFunctionCallingPlanner(toolClient)
	}
	return &AutoPlanner{
		toolPlanner:        toolPlanner,
		reactPlanner:       NewReActTextPlanner(client, builder, parser),
		toolCallingEnabled: enabled,
		fallbackToReAct:    fallback,
	}
}

func (p *AutoPlanner) setMessagePreparer(preparer MessagePreparer) {
	if p == nil || preparer == nil {
		return
	}
	if p.toolPlanner != nil {
		p.toolPlanner.preparer = preparer
	}
	if p.reactPlanner != nil {
		p.reactPlanner.preparer = preparer
	}
}

// Plan 根据当前模型能力和配置生成决策。
func (p *AutoPlanner) Plan(ctx context.Context, state State, opts ...llm.ModelCallOption) (Decision, error) {
	result, err := p.planTrace(ctx, state, opts...)
	if err != nil {
		return Decision{}, err
	}
	return result.Decision, nil
}

func (p *AutoPlanner) planTrace(ctx context.Context, state State, opts ...llm.ModelCallOption) (plannerResult, error) {
	return p.planStreamTrace(ctx, state, nil, opts...)
}

// planStreamTrace 根据模型能力以流式方式生成下一步决策。
func (p *AutoPlanner) planStreamTrace(ctx context.Context, state State, emit tokenEmitter, opts ...llm.ModelCallOption) (plannerResult, error) {
	if p == nil {
		return plannerResult{}, errors.New("auto planner is nil")
	}
	if p.toolCallingEnabled && p.toolPlanner != nil && p.toolPlanner.SupportsToolCalling() {
		result, err := p.toolPlanner.planStreamTrace(ctx, state, emit, opts...)
		if err == nil {
			return result, nil
		}
		// 只有明确“不支持工具调用”才自动退回文本 ReAct，避免吞掉供应商调用错误。
		if (!errors.Is(err, ErrToolCallingUnsupported) && !errors.Is(err, ErrStreamingUnsupported)) || !p.fallbackToReAct {
			return result, err
		}
		fallbackResult, fallbackErr := p.reactPlanner.planStreamTrace(ctx, state, emit, opts...)
		fallbackResult.Fallback = true
		return fallbackResult, fallbackErr
	}
	return p.reactPlanner.planStreamTrace(ctx, state, emit, opts...)
}

type finalAnswerEmitter struct {
	emit    tokenEmitter
	pending string
	found   bool
}

// newFinalAnswerEmitter 创建一个只输出 ReAct 最终回答区域的增量过滤器。
func newFinalAnswerEmitter(emit tokenEmitter) *finalAnswerEmitter {
	return &finalAnswerEmitter{emit: emit}
}

// Feed 记录完整 ReAct 原文，并在识别到 Final Answer: 后才把增量交给调用方。
func (e *finalAnswerEmitter) Feed(ctx context.Context, text string) error {
	if e == nil || text == "" {
		return nil
	}
	if e.found {
		return e.send(ctx, text)
	}
	e.pending += text
	const marker = "final answer:"
	index := strings.Index(strings.ToLower(e.pending), marker)
	if index < 0 {
		// 保留 marker 长度的尾部，避免 marker 恰好被拆在两个流分片之间。
		if len(e.pending) > len(marker)-1 {
			e.pending = e.pending[len(e.pending)-(len(marker)-1):]
		}
		return nil
	}
	e.found = true
	answer := e.pending[index+len(marker):]
	e.pending = ""
	return e.send(ctx, answer)
}

func (e *finalAnswerEmitter) send(ctx context.Context, text string) error {
	if e == nil || e.emit == nil || text == "" {
		return nil
	}
	return e.emit(ctx, text)
}

func (p *AutoPlanner) modelMessages(ctx context.Context, state State) ([]*llm.Message, error) {
	if p != nil && p.toolCallingEnabled && p.toolPlanner != nil && p.toolPlanner.SupportsToolCalling() {
		return p.toolPlanner.modelMessages(ctx, state)
	}
	return p.reactPlanner.modelMessages(ctx, state)
}

// toolCallingMessages 把当前对话历史规整为工具调用模型的输入。
// 非空时在最前注入 state.SystemPrompt（业务人设），避免 FC 路径丢失角色。
func toolCallingMessages(state State) []*llm.Message {
	messages := make([]*llm.Message, 0, len(state.Input.Messages)+len(state.Steps)*2+2)
	if sp := strings.TrimSpace(state.SystemPrompt); sp != "" {
		messages = append(messages, llm.SystemMessage(sp))
	}
	messages = append(messages, llm.CloneMessages(state.Input.Messages)...)
	if query := strings.TrimSpace(state.Input.Query); query != "" {
		messages = append(messages, &llm.Message{
			Role:    llm.UserRole,
			Content: query,
		})
	}
	for _, step := range state.Steps {
		if strings.TrimSpace(step.Action) == "" || strings.TrimSpace(step.ToolCallID) == "" {
			continue
		}
		// 原生工具调用模型需要看到上一轮 assistant tool_call 以及对应的 tool result。
		messages = append(messages,
			&llm.Message{
				Role: llm.AssistantRole,
				ToolCalls: []llm.LLMToolCall{{
					ID:       step.ToolCallID,
					Name:     strings.TrimSpace(step.Action),
					Input:    cloneInput(step.ActionInput),
					RawInput: rawToolInput(step.ActionInput),
				}},
			},
			&llm.Message{
				Role:       llm.ToolRole,
				Content:    step.Observation,
				ToolCallID: step.ToolCallID,
				ToolName:   strings.TrimSpace(step.Action),
			},
		)
	}
	return messages
}

// decisionFromToolCallingOutput 把供应商无关的 LLMResult 规整成 ReAct 主循环决策。
func decisionFromToolCallingOutput(output *llm.LLMResult) (Decision, error) {
	if output == nil {
		return Decision{}, fmt.Errorf("%w: empty tool calling output", ErrParseDecision)
	}
	if len(output.ToolCalls) > 0 {
		call := output.ToolCalls[0]
		return Decision{
			Kind:        DecisionToolCall,
			Action:      strings.TrimSpace(call.Name),
			ActionInput: cloneInput(call.Input),
			ToolCallID:  call.ID,
		}, nil
	}
	if strings.TrimSpace(output.Text) == "" {
		return Decision{}, fmt.Errorf("%w: empty tool calling output", ErrParseDecision)
	}
	return Decision{
		Kind:        DecisionFinal,
		FinalAnswer: strings.TrimSpace(output.Text),
	}, nil
}

// outputSummary 返回 hook 和调试链路可记录的简短模型输出摘要。
func outputSummary(output *llm.LLMResult) string {
	if output == nil {
		return ""
	}
	if len(output.ToolCalls) == 0 {
		return output.Text
	}
	names := make([]string, 0, len(output.ToolCalls))
	for _, call := range output.ToolCalls {
		names = append(names, call.Name)
	}
	return "tool_call:" + strings.Join(names, ",")
}

// rawToolInput 返回可回放给 provider 的工具调用 JSON 参数。
func rawToolInput(input coretool.Input) string {
	if len(input) == 0 {
		return "{}"
	}
	data, err := json.Marshal(input)
	if err != nil {
		return "{}"
	}
	return string(data)
}

var _ Planner = (*ReActTextPlanner)(nil)
var _ Planner = (*FunctionCallingPlanner)(nil)
var _ ToolCallingPlanner = (*FunctionCallingPlanner)(nil)
var _ Planner = (*AutoPlanner)(nil)
var _ tracePlanner = (*ReActTextPlanner)(nil)
var _ tracePlanner = (*FunctionCallingPlanner)(nil)
var _ tracePlanner = (*AutoPlanner)(nil)
var _ streamTracePlanner = (*ReActTextPlanner)(nil)
var _ streamTracePlanner = (*FunctionCallingPlanner)(nil)
var _ streamTracePlanner = (*AutoPlanner)(nil)
var _ modelMessagePlanner = (*ReActTextPlanner)(nil)
var _ modelMessagePlanner = (*FunctionCallingPlanner)(nil)
var _ modelMessagePlanner = (*AutoPlanner)(nil)
