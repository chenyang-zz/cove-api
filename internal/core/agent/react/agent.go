package react

import (
	"context"
	"errors"

	coreagent "github.com/boxify/api-go/internal/core/agent"
	"github.com/boxify/api-go/internal/core/llm"
	coretool "github.com/boxify/api-go/internal/core/tool"
)

// Agent 执行模型决策、工具调用和观察结果反馈循环。
type Agent struct {
	base               *coreagent.Base[Decision, Step]
	promptBuilder      PromptBuilder
	parser             Parser
	planner            Planner
	toolCallingEnabled bool
	fallbackToReAct    bool
}

// New 创建 Agent。
//
// 默认会优先检测 client 是否实现 ToolCallingClient；支持时走模型原生工具调用，否则
// 使用 core 内置提示词退回文本 ReAct。registry 为 nil 时会使用空工具注册表。
func New(client llm.Client, registry *coretool.Registry, opts ...Option) *Agent {
	base := coreagent.NewBase[Decision, Step](client, registry)
	base.SetCloneFuncs(cloneDecision, cloneStep)
	a := &Agent{
		base:               base,
		parser:             NewReActParser(),
		toolCallingEnabled: defaultToolCallingEnabled,
		fallbackToReAct:    defaultFallbackToReAct,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(a)
		}
	}
	if a.promptBuilder == nil {
		a.promptBuilder = NewReActPromptBuilder()
	}
	if a.planner == nil {
		a.planner = NewAutoPlanner(a.base.Client(), a.promptBuilder, a.parser, a.toolCallingEnabled, a.fallbackToReAct)
	}
	return a
}

// Run 执行完整 Agent 循环。
//
// 每轮先由 planner 生成决策：默认优先 function calling，必要时退回文本 ReAct。工具调用
// 结果会写入 Step.Observation 并参与下一轮模型决策。
func (a *Agent) Run(ctx context.Context, input Input, opts ...RunOption) (*Result, error) {
	if a == nil {
		return nil, errors.New("agent is nil")
	}
	if err := a.base.Validate(); err != nil {
		return nil, err
	}
	cfg := a.base.RunConfig(opts...)
	state := a.base.NewState(input)
	result := Result{}
	if err := a.base.Hooks().BeforeRun(ctx, a.base.CloneState(state)); err != nil {
		return a.finishWithError(ctx, state, result, err)
	}

	for iteration := 1; iteration <= cfg.MaxIterations; iteration++ {
		state.Iteration = iteration
		if err := a.base.Transition(ctx, &state, PhaseBuildPrompt, "build prompt"); err != nil {
			return a.finishWithError(ctx, state, result, err)
		}

		if err := a.base.Transition(ctx, &state, PhaseModel, "call model"); err != nil {
			return a.finishWithError(ctx, state, result, err)
		}
		messages, err := a.modelMessages(ctx, a.base.CloneState(state))
		if err != nil {
			return a.finishWithError(ctx, state, result, err)
		}
		if err := a.base.Hooks().BeforeModel(ctx, a.base.CloneState(state), messages); err != nil {
			return a.finishWithError(ctx, state, result, err)
		}
		plan, modelErr := a.plan(ctx, a.base.CloneState(state), cfg.ModelOptions...)
		if err := a.base.Hooks().AfterModel(ctx, a.base.CloneState(state), plan.Output, modelErr); err != nil {
			return a.finishWithError(ctx, state, result, err)
		}
		if modelErr != nil {
			return a.finishWithError(ctx, state, result, modelErr)
		}
		if plan.Fallback {
			if err := a.base.Transition(ctx, &state, PhaseFallback, "fallback to react"); err != nil {
				return a.finishWithError(ctx, state, result, err)
			}
			if err := a.base.Transition(ctx, &state, PhaseBuildPrompt, "build react prompt"); err != nil {
				return a.finishWithError(ctx, state, result, err)
			}
		}

		if err := a.base.Transition(ctx, &state, PhaseParse, "parse model output"); err != nil {
			return a.finishWithError(ctx, state, result, err)
		}
		decision := plan.Decision
		if err := a.base.Hooks().AfterParse(ctx, a.base.CloneState(state), decision, nil); err != nil {
			return a.finishWithError(ctx, state, result, err)
		}
		state.LastDecision = decision

		if decision.Kind == DecisionFinal {
			step := Step{
				Iteration:      iteration,
				Thought:        decision.Thought,
				FinalAnswer:    decision.FinalAnswer,
				ToolCallID:     decision.ToolCallID,
				RawModelOutput: plan.Output,
			}
			state.Steps = append(state.Steps, step)
			result = Result{
				Answer:     decision.FinalAnswer,
				Steps:      a.base.CloneSteps(state.Steps),
				Iterations: iteration,
				StoppedBy:  StopFinalAnswer,
			}
			if err := a.base.Transition(ctx, &state, PhaseFinish, "final answer"); err != nil {
				return a.finishWithError(ctx, state, result, err)
			}
			if err := a.base.Hooks().AfterRun(ctx, result, nil); err != nil {
				return &result, err
			}
			return &result, nil
		}

		if err := a.base.Transition(ctx, &state, PhaseTool, "call tool"); err != nil {
			return a.finishWithError(ctx, state, result, err)
		}
		call := ToolCall{Name: decision.Action, Input: cloneInput(decision.ActionInput)}
		if err := a.base.Hooks().BeforeTool(ctx, a.base.CloneState(state), call); err != nil {
			return a.finishWithError(ctx, state, result, err)
		}
		toolOutput, toolErr := a.base.InvokeTool(ctx, decision.Action, decision.ActionInput)
		if err := a.base.Hooks().AfterTool(ctx, a.base.CloneState(state), call, toolOutput, toolErr); err != nil {
			return a.finishWithError(ctx, state, result, err)
		}
		if toolErr != nil {
			return a.finishWithError(ctx, state, result, toolErr)
		}

		if err := a.base.Transition(ctx, &state, PhaseObserve, "record observation"); err != nil {
			return a.finishWithError(ctx, state, result, err)
		}
		step := Step{
			Iteration:      iteration,
			Thought:        decision.Thought,
			Action:         decision.Action,
			ActionInput:    cloneInput(decision.ActionInput),
			ToolCallID:     decision.ToolCallID,
			Observation:    a.base.TruncateObservation(toolOutput.Text),
			RawModelOutput: plan.Output,
		}
		state.Steps = append(state.Steps, step)
		if err := a.base.Hooks().OnStep(ctx, a.base.CloneState(state), step); err != nil {
			return a.finishWithError(ctx, state, result, err)
		}
		result = Result{
			Steps:      a.base.CloneSteps(state.Steps),
			Iterations: iteration,
		}
	}

	result = Result{
		Steps:      a.base.CloneSteps(state.Steps),
		Iterations: cfg.MaxIterations,
		StoppedBy:  StopMaxIterations,
	}
	return a.finishWithError(ctx, state, result, ErrMaxIterations)
}

// plan 调用 planner，并兼容只实现公开 Planner 接口的自定义实现。
func (a *Agent) plan(ctx context.Context, state State, opts ...llm.ModelCallOption) (plannerResult, error) {
	if traced, ok := a.planner.(tracePlanner); ok {
		return traced.planTrace(ctx, state, opts...)
	}
	decision, err := a.planner.Plan(ctx, state, opts...)
	if err != nil {
		return plannerResult{}, err
	}
	return plannerResult{Decision: decision}, nil
}

// modelMessages 返回当前 planner 将要发送给模型的消息快照。
func (a *Agent) modelMessages(ctx context.Context, state State) ([]*llm.Message, error) {
	if planner, ok := a.planner.(modelMessagePlanner); ok {
		return planner.modelMessages(ctx, state)
	}
	return nil, nil
}

// runConfig 应用运行时配置。
// finishWithError 处理错误并完成运行。
func (a *Agent) finishWithError(ctx context.Context, state State, result Result, err error) (*Result, error) {
	return a.base.FinishWithError(ctx, state, result, err)
}
