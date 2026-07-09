package react

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agentprompt "github.com/boxify/api-go/internal/core/agent/prompt"
	"github.com/boxify/api-go/internal/core/llm"
	coreprompt "github.com/boxify/api-go/internal/core/prompt"
)

// ReActPromptBuilder 使用 core 内置模板构造文本 ReAct 模型消息。
type ReActPromptBuilder struct{}

// NewReActPromptBuilder 创建默认 ReAct 提示词构造器。
func NewReActPromptBuilder() *ReActPromptBuilder {
	return &ReActPromptBuilder{}
}

// Build 根据当前状态构造系统消息、历史消息、用户问题和 ReAct scratchpad。
func (b *ReActPromptBuilder) Build(ctx context.Context, state State) ([]*llm.Message, error) {
	_ = ctx
	system, err := b.renderSystemPrompt(state)
	if err != nil {
		return nil, err
	}

	messages := []*llm.Message{llm.SystemMessage(system)}
	messages = append(messages, llm.CloneMessages(state.Input.Messages)...)
	if query := strings.TrimSpace(state.Input.Query); query != "" {
		messages = append(messages, llm.UserMessage(query))
	}
	if len(state.Steps) > 0 {
		messages = append(messages, llm.AssistantMessage(formatScratchpad(state.Steps)))
	}
	return messages, nil
}

func (b *ReActPromptBuilder) renderSystemPrompt(state State) (string, error) {
	tools := make([]agentprompt.ReActToolData, 0, len(state.Tools))
	for _, item := range state.Tools {
		tools = append(tools, agentprompt.ReActToolData{
			Name:        strings.TrimSpace(item.Name),
			Description: strings.TrimSpace(item.Description),
		})
	}
	text, err := coreprompt.TemplateText(agentprompt.Templates, agentprompt.ReActSystemTemplate)
	if err != nil {
		return "", err
	}
	return coreprompt.RenderText(text, agentprompt.ReActSystemData{
		Tools:        tools,
		SystemPrompt: strings.TrimSpace(state.SystemPrompt),
	})
}

func formatScratchpad(steps []Step) string {
	parts := make([]string, 0, len(steps))
	for _, step := range steps {
		if step.FinalAnswer != "" {
			parts = append(parts, fmt.Sprintf("Thought: %s\nFinal Answer: %s", step.Thought, step.FinalAnswer))
			continue
		}
		input, _ := json.Marshal(step.ActionInput)
		parts = append(parts, fmt.Sprintf(
			"Thought: %s\nAction: %s\nAction Input: %s\nObservation: %s",
			step.Thought,
			step.Action,
			string(input),
			step.Observation,
		))
	}
	return strings.Join(parts, "\n\n")
}

var _ PromptBuilder = (*ReActPromptBuilder)(nil)
