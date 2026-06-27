package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestEventConstructors(t *testing.T) {
	conversationID := uuid.New()

	tests := []struct {
		name      string
		event     Event
		eventType string
		check     func(t *testing.T, event Event)
	}{
		{
			name:      "base",
			event:     NewBaseEvent("custom"),
			eventType: "custom",
		},
		{
			name:      "text",
			event:     NewTextEvent("typing", "hello"),
			eventType: "typing",
			check: func(t *testing.T, event Event) {
				textEvent := event.(*TextEvent)
				if textEvent.Text != "hello" {
					t.Fatalf("Text = %q, want hello", textEvent.Text)
				}
			},
		},
		{
			name:      "token",
			event:     NewTokenEvent("tok"),
			eventType: EventTypeToken,
			check: func(t *testing.T, event Event) {
				textEvent := event.(*TextEvent)
				if textEvent.Text != "tok" {
					t.Fatalf("Text = %q, want tok", textEvent.Text)
				}
			},
		},
		{
			name:      "done",
			event:     NewDoneEvent("done"),
			eventType: EventTypeDone,
			check: func(t *testing.T, event Event) {
				textEvent := event.(*TextEvent)
				if textEvent.Text != "done" {
					t.Fatalf("Text = %q, want done", textEvent.Text)
				}
			},
		},
		{
			name:      "meta",
			event:     NewMetaEvent(conversationID, "New Chat"),
			eventType: EventTypeMeta,
			check: func(t *testing.T, event Event) {
				metaEvent := event.(*MetaEvent)
				if metaEvent.ConversationID != conversationID || metaEvent.Title != "New Chat" {
					t.Fatalf("meta event = %+v, want conversation/title", metaEvent)
				}
			},
		},
		{
			name:      "error",
			event:     NewErrorEvent("boom"),
			eventType: EventTypeError,
			check: func(t *testing.T, event Event) {
				errorEvent := event.(*ErrorEvent)
				if errorEvent.Message != "boom" {
					t.Fatalf("Message = %q, want boom", errorEvent.Message)
				}
			},
		},
		{
			name:      "ping",
			event:     NewPingEvent(),
			eventType: EventTypePing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.event.EventName() != tt.eventType {
				t.Fatalf("EventName() = %q, want %q", tt.event.EventName(), tt.eventType)
			}
			if tt.check != nil {
				tt.check(t, tt.event)
			}
		})
	}
}
