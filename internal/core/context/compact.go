package context

import (
	"github.com/boxify/api-go/internal/core/llm"
)

type messageBlock struct {
	messages []*llm.Message
	pinned   bool
	tool     bool
}

// fitMessages 尝试在固定 token 数量下裁剪消息列表，使其不超过目标 token 数量。
func (m *Manager) fitMessages(messages []*llm.Message, fixedTokens int, targetTokens int) ([]*llm.Message, bool) {
	blocks := groupMessages(messages)
	compacted := false

	// 优先清空较旧工具结果，保留工具调用关系但避免大段 Observation 挤占窗口。
	for index := 0; index < len(blocks)-1 && fixedTokens+m.countBlocks(blocks) > targetTokens; index++ {
		if !blocks[index].tool {
			continue
		}
		for _, message := range blocks[index].messages {
			// 仅清空工具结果消息的内容，保留工具调用消息和工具结果的关系。
			// 内容为空的工具结果消息不会被重复清空。
			if message.Role == llm.ToolRole && message.Content != "" {
				message.Content = "[tool result omitted]"
				compacted = true
			}
		}
	}

	// 再从最旧的可裁剪消息块开始删除；system 与最后一个消息块始终保留。
	for index := 0; index < len(blocks)-1 && fixedTokens+m.countBlocks(blocks) > targetTokens; {
		if blocks[index].pinned {
			index++
			continue
		}
		blocks = append(blocks[:index], blocks[index+1:]...)
		compacted = true
	}
	return flattenBlocks(blocks), compacted
}

// countBlocks 计算消息块列表的总 token 数。
func (m *Manager) countBlocks(blocks []messageBlock) int {
	return m.counter.CountMessages(flattenBlocks(blocks))
}

// groupMessages 将消息列表按 role 和 tool call 分组为 messageBlock 列表，便于裁剪。
//
// 如果一个工具发起工具调用（tool call）消息后紧跟一个或多个工具结果消息（tool result），则
// 这些消息会被归为同一个 messageBlock。
// 一个工具调用后一定是紧跟着它的工具结果消息，直到下一个非工具角色的消息出现。
func groupMessages(messages []*llm.Message) []messageBlock {
	blocks := make([]messageBlock, 0, len(messages))
	for index := 0; index < len(messages); {
		message := messages[index]
		if message == nil {
			index++
			continue
		}
		block := messageBlock{
			messages: []*llm.Message{message},
			pinned:   message.Role == llm.SystemRole,
			tool:     len(message.ToolCalls) > 0 || message.Role == llm.ToolRole,
		}
		index++
		if len(message.ToolCalls) > 0 {
			for index < len(messages) && messages[index] != nil && messages[index].Role == llm.ToolRole {
				block.messages = append(block.messages, messages[index])
				index++
			}
		}
		blocks = append(blocks, block)
	}
	return blocks
}

// flattenBlocks 将 messageBlock 列表展开为消息列表。
func flattenBlocks(blocks []messageBlock) []*llm.Message {
	var messages []*llm.Message
	for _, block := range blocks {
		messages = append(messages, block.messages...)
	}
	return messages
}
