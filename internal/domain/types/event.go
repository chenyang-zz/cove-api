/**
 * @Time   : 2026/6/27 23:32
 * @Author : chenyangzhao542@gmail.com
 * @File   : event.go
 **/

package types

import "github.com/google/uuid"

const (
	EventTypeToken      = "token"
	EventTypeDone       = "done"
	EventTypeMeta       = "meta"
	EventTypeError      = "error"
	EventTypeToolCall   = "tool_call"
	EventTypeToolResult = "tool_result"
	EventTypePing       = "_ping"
)

type Event interface {
	EventName() string
}

type BaseEvent struct {
	Type string `json:"type"`
}

type TextEvent struct {
	BaseEvent
	Text string
}

type ErrorEvent struct {
	BaseEvent
	Message string
}

type MetaEvent struct {
	BaseEvent
	ConversationID uuid.UUID
	Title          string
}

type ToolEvent struct {
	BaseEvent
	Tool        string
	Input       map[string]any
	Observation string
	Error       string
	Iteration   int
	ToolCallID  string
}

func NewBaseEvent(eventType string) *BaseEvent {
	return &BaseEvent{Type: eventType}
}

func NewTextEvent(eventType, text string) *TextEvent {
	return &TextEvent{
		BaseEvent: BaseEvent{Type: eventType},
		Text:      text,
	}
}

func NewTokenEvent(text string) *TextEvent {
	return NewTextEvent(EventTypeToken, text)
}

func NewDoneEvent(text string) *TextEvent {
	return NewTextEvent(EventTypeDone, text)
}

func NewMetaEvent(conversationID uuid.UUID, title string) *MetaEvent {
	return &MetaEvent{
		BaseEvent:      BaseEvent{Type: EventTypeMeta},
		ConversationID: conversationID,
		Title:          title,
	}
}

func NewErrorEvent(message string) *ErrorEvent {
	return &ErrorEvent{
		BaseEvent: BaseEvent{Type: EventTypeError},
		Message:   message,
	}
}

func NewToolCallEvent(tool string, input map[string]any, iteration int, toolCallID string) *ToolEvent {
	return &ToolEvent{
		BaseEvent:  BaseEvent{Type: EventTypeToolCall},
		Tool:       tool,
		Input:      input,
		Iteration:  iteration,
		ToolCallID: toolCallID,
	}
}

func NewToolResultEvent(tool string, input map[string]any, observation string, errMessage string, iteration int, toolCallID string) *ToolEvent {
	return &ToolEvent{
		BaseEvent:   BaseEvent{Type: EventTypeToolResult},
		Tool:        tool,
		Input:       input,
		Observation: observation,
		Error:       errMessage,
		Iteration:   iteration,
		ToolCallID:  toolCallID,
	}
}

func NewPingEvent() *BaseEvent {
	return NewBaseEvent(EventTypePing)
}

func (e *BaseEvent) EventName() string {
	return e.Type
}
