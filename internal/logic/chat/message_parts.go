package chat

import (
	"strings"
	"unicode/utf8"

	"github.com/boxify/api-go/internal/models"
)

// 工具 observation 写入上限，避免 jsonb / 列表响应过大。
const maxObservationRunes = 32 * 1024

// appendTextPart 追加或合并相邻 text 片段。
func appendTextPart(parts []models.MessagePart, text string) []models.MessagePart {
	if text == "" {
		return parts
	}
	if n := len(parts); n > 0 && parts[n-1].Type == models.MessagePartTypeText {
		parts[n-1].Text += text
		return parts
	}
	return append(parts, models.MessagePart{
		Type: models.MessagePartTypeText,
		Text: text,
	})
}

// appendToolCallPart 追加 tool_call 片段。
func appendToolCallPart(parts []models.MessagePart, tool string, input map[string]any, iteration int, toolCallID string) []models.MessagePart {
	return append(parts, models.MessagePart{
		Type:       models.MessagePartTypeToolCall,
		Tool:       tool,
		Input:      cloneFlowInput(input),
		Iteration:  iteration,
		ToolCallID: toolCallID,
	})
}

// appendToolResultPart 追加 tool_result 片段。
func appendToolResultPart(parts []models.MessagePart, tool string, input map[string]any, observation, errMessage string, iteration int, toolCallID string) []models.MessagePart {
	return append(parts, models.MessagePart{
		Type:        models.MessagePartTypeToolResult,
		Tool:        tool,
		Input:       cloneFlowInput(input),
		Observation: truncateObservation(observation),
		Error:       errMessage,
		Iteration:   iteration,
		ToolCallID:  toolCallID,
	})
}

// finalizePartsWithAnswer 用最终 answer 对齐 parts 中的正文展示。
// content 列仍以 answer 为准；有工具时保留 tool 前片段，工具后正文统一为 answer。
func finalizePartsWithAnswer(parts []models.MessagePart, answer string) []models.MessagePart {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		if len(parts) == 0 {
			return nil
		}
		return parts
	}

	joined := joinTextParts(parts)
	if joined == answer {
		return parts
	}

	lastToolIdx := -1
	for i, part := range parts {
		if part.Type == models.MessagePartTypeToolCall || part.Type == models.MessagePartTypeToolResult {
			lastToolIdx = i
		}
	}

	// 无工具：整条消息正文以 answer 为唯一 text part
	if lastToolIdx < 0 {
		return []models.MessagePart{{
			Type: models.MessagePartTypeText,
			Text: answer,
		}}
	}

	// 保留最后一个工具及之前的片段，工具后的 text 用 answer 覆盖
	kept := make([]models.MessagePart, 0, lastToolIdx+2)
	kept = append(kept, parts[:lastToolIdx+1]...)
	return append(kept, models.MessagePart{
		Type: models.MessagePartTypeText,
		Text: answer,
	})
}

// joinTextParts 将 parts 中的 text 片段拼接为完整正文。
func joinTextParts(parts []models.MessagePart) string {
	var b strings.Builder
	for _, part := range parts {
		if part.Type == models.MessagePartTypeText {
			b.WriteString(part.Text)
		}
	}
	return strings.TrimSpace(b.String())
}

// truncateObservation 截断 observation 字段，避免过大。
func truncateObservation(observation string) string {
	if observation == "" {
		return ""
	}
	if utf8.RuneCountInString(observation) <= maxObservationRunes {
		return observation
	}
	runes := []rune(observation)
	return string(runes[:maxObservationRunes])
}

// buildAssistantMeta 组装 assistant 落库元数据。
func buildAssistantMeta(parts []models.MessagePart, interrupted bool) *models.MessageMetaData {
	meta := &models.MessageMetaData{
		Interrupted: interrupted,
	}
	if len(parts) > 0 {
		meta.Parts = parts
	}
	if len(meta.Parts) == 0 && !interrupted {
		return nil
	}
	return meta
}
