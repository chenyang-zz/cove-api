package react

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agentprompt "github.com/boxify/api-go/internal/core/agent/prompt"
	"github.com/boxify/api-go/internal/core/llm"
	coreprompt "github.com/boxify/api-go/internal/core/prompt"
	coretool "github.com/boxify/api-go/internal/core/tool"
)

// PromptBuilder 构造每轮模型输入消息。
type PromptBuilder interface {
	Build(ctx context.Context, state State) ([]*llm.Message, error)
}

// ReActPromptBuilder 是默认 ReAct prompt builder。
type ReActPromptBuilder struct{}

// NewReActPromptBuilder 创建默认 ReAct prompt builder。
func NewReActPromptBuilder() *ReActPromptBuilder {
	return &ReActPromptBuilder{}
}

// Build 根据当前状态构造模型消息。
func (b *ReActPromptBuilder) Build(ctx context.Context, state State) ([]*llm.Message, error) {
	system, err := b.renderSystemPrompt(state)
	if err != nil {
		return nil, err
	}

	messages := []*llm.Message{llm.SystemMessage(system)}
	messages = append(messages, llm.CloneMessages(state.Input.Messages)...)
	userContent := strings.TrimSpace(state.Input.Query)
	if userContent != "" {
		messages = append(messages, llm.UserMessage(userContent))
	}
	if len(state.Steps) > 0 {
		messages = append(messages, llm.AssistantMessage(formatScratchpad(state.Steps)))
	}
	return messages, nil
}

// renderSystemPrompt 渲染系统消息。
func (b *ReActPromptBuilder) renderSystemPrompt(state State) (string, error) {
	data := agentprompt.ReActSystemData{
		Tools:        toolDataFromDescriptors(state.Tools),
		SystemPrompt: strings.TrimSpace(state.SystemPrompt),
	}
	text, err := coreprompt.TemplateText(agentprompt.Templates, agentprompt.ReActSystemTemplate)
	if err != nil {
		return "", err
	}
	return coreprompt.RenderText(text, data)
}

func toolDataFromDescriptors(tools []coretool.Descriptor) []agentprompt.ReActToolData {
	out := make([]agentprompt.ReActToolData, 0, len(tools))
	for _, item := range tools {
		out = append(out, agentprompt.ReActToolData{
			Name:        strings.TrimSpace(item.Name),
			Description: strings.TrimSpace(item.Description),
		})
	}
	return out
}

// formatScratchpad 格式化 scratchpad。
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
