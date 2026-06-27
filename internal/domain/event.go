/**
 * @Time   : 2026/6/27 23:32
 * @Author : chenyangzhao542@gmail.com
 * @File   : event.go
 **/

package domain

import "github.com/google/uuid"

const (
	EventTypeToken = "token"
	EventTypeDone  = "done"
	EventTypeMeta  = "meta"
	EventTypeError = "error"
	EventTypePing  = "_ping"
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

func NewPingEvent() *BaseEvent {
	return NewBaseEvent(EventTypePing)
}

func (e *BaseEvent) EventName() string {
	return e.Type
}
