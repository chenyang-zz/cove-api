package react

import (
	"context"
	"errors"

	coreagent "github.com/boxify/api-go/internal/core/agent"
	"github.com/boxify/api-go/internal/core/llm"
	coretool "github.com/boxify/api-go/internal/core/tool"
)

// ErrMaxIterations 表示 Agent 在达到最大迭代次数后仍未得到最终答案。
var ErrMaxIterations = coreagent.ErrMaxIterations

// ErrParseDecision 表示模型输出不符合 ReAct 决策格式。
var ErrParseDecision = errors.New("parse react decision")

// ErrInvalidActionInput 表示 Action Input 不是合法 JSON object 或纯文本输入。
var ErrInvalidActionInput = errors.New("invalid action input")

// ErrToolCallingUnsupported 表示模型客户端当前不支持原生工具调用。
var ErrToolCallingUnsupported = errors.New("tool calling unsupported")

// Input 表示一次 Agent 运行的输入。
type Input = coreagent.Input

// Result 表示一次 ReAct Agent 运行的结果。
type Result = coreagent.Result[Step]

// StopReason 表示 Agent 停止运行的原因。
type StopReason = coreagent.StopReason

const (
	// StopFinalAnswer 表示 Agent 得到了最终答案。
	StopFinalAnswer = coreagent.StopFinalAnswer
	// StopMaxIterations 表示 Agent 达到最大迭代次数。
	StopMaxIterations = coreagent.StopMaxIterations
	// StopError 表示 Agent 因错误停止。
	StopError = coreagent.StopError
)

// Phase 表示 Agent 内部状态机阶段。
type Phase = coreagent.Phase

const (
	// PhaseStart 表示运行刚初始化。
	PhaseStart = coreagent.PhaseStart
	// PhaseBuildPrompt 表示正在构造模型输入。
	PhaseBuildPrompt = coreagent.PhaseBuildPrompt
	// PhaseModel 表示正在调用模型。
	PhaseModel = coreagent.PhaseModel
	// PhaseFallback 表示 function calling 路径退回文本 ReAct。
	PhaseFallback = coreagent.PhaseFallback
	// PhaseParse 表示正在解析模型输出。
	PhaseParse = coreagent.PhaseParse
	// PhaseTool 表示正在调用工具。
	PhaseTool = coreagent.PhaseTool
	// PhaseObserve 表示正在记录工具观察结果。
	PhaseObserve = coreagent.PhaseObserve
	// PhaseFinish 表示运行正常结束。
	PhaseFinish = coreagent.PhaseFinish
	// PhaseError 表示运行因错误结束。
	PhaseError = coreagent.PhaseError
)

// Transition 表示一次状态阶段迁移。
type Transition = coreagent.Transition

// State 表示 ReAct Agent 当前运行状态。
type State = coreagent.State[Decision, Step]

// Step 表示一次 Agent 迭代中的模型决策、工具调用和观察结果。
type Step struct {
	Iteration      int
	Thought        string
	Action         string
	ActionInput    coretool.Input
	ToolCallID     string
	Observation    string
	FinalAnswer    string
	RawModelOutput string
}

// DecisionKind 表示模型输出被解析后的决策类型。
type DecisionKind string

const (
	// DecisionFinal 表示模型给出了最终答案。
	DecisionFinal DecisionKind = "final"
	// DecisionToolCall 表示模型要求调用工具。
	DecisionToolCall DecisionKind = "tool_call"
)

// Decision 表示模型输出被标准化后的结构化决策。
type Decision struct {
	Kind        DecisionKind
	Thought     string
	FinalAnswer string
	Action      string
	ActionInput coretool.Input
	ToolCallID  string
}

// ToolCall 表示 Agent 准备执行的一次工具调用。
type ToolCall = coreagent.ToolCall

// Hooks 定义 Agent 关键生命周期和状态迁移钩子。
type Hooks = coreagent.Hooks[Decision, Step]

// NoopHooks 是不执行任何副作用的默认 hooks。
type NoopHooks = coreagent.NoopHooks[Decision, Step]

// PromptBuilder 构造文本 ReAct 路径每轮发送给模型的消息。
//
// 实现通常由应用层注入，以便模板注册、品牌身份和业务提示词不进入 core 包。
type PromptBuilder interface {
	Build(ctx context.Context, state State) ([]*llm.Message, error)
}

// Planner 负责把当前状态转成下一步标准化决策。
type Planner interface {
	Plan(ctx context.Context, state State, opts ...llm.ModelCallOption) (Decision, error)
}

// ToolCallingPlanner 表示支持模型原生工具调用的 planner。
type ToolCallingPlanner interface {
	Planner
	SupportsToolCalling() bool
}
