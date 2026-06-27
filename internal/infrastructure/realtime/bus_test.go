package realtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/boxify/api-go/internal/domain"
	"github.com/google/uuid"
)

func TestConversationTopic(t *testing.T) {
	conversationID := uuid.New()

	if got, want := ConversationTopic(conversationID), "conversation:"+conversationID.String(); got != want {
		t.Fatalf("ConversationTopic() = %q, want %q", got, want)
	}
}

func TestMemoryBrokerBroadcastsToSubscribers(t *testing.T) {
	ctx := context.Background()
	broker := NewMemoryBroker()

	first, err := broker.Subscribe(ctx, "topic")
	if err != nil {
		t.Fatalf("first subscribe error = %v", err)
	}
	defer first.Close(ctx)
	second, err := broker.Subscribe(ctx, "topic")
	if err != nil {
		t.Fatalf("second subscribe error = %v", err)
	}
	defer second.Close(ctx)

	if err := broker.Publish(ctx, "topic", domain.NewTokenEvent("hello")); err != nil {
		t.Fatalf("publish error = %v", err)
	}

	for name, sub := range map[string]Subscription{"first": first, "second": second} {
		select {
		case event := <-sub.Events():
			if event.EventName() != domain.EventTypeToken {
				t.Fatalf("%s event = %q, want token", name, event.EventName())
			}
		case <-time.After(time.Second):
			t.Fatalf("%s subscriber did not receive event", name)
		}
	}
}

func TestForwardRelaysEventsAndStopsOnDone(t *testing.T) {
	ctx := context.Background()
	broker := NewMemoryBroker()
	sub, err := broker.Subscribe(ctx, "topic")
	if err != nil {
		t.Fatalf("subscribe error = %v", err)
	}

	if err := broker.Publish(ctx, "topic", domain.NewTokenEvent("hello")); err != nil {
		t.Fatalf("publish token error = %v", err)
	}
	if err := broker.Publish(ctx, "topic", domain.NewDoneEvent("ok")); err != nil {
		t.Fatalf("publish done error = %v", err)
	}

	out := make(chan domain.Event, 2)
	if err := Forward(ctx, sub, out, ForwardOptions{}); err != nil {
		t.Fatalf("Forward error = %v", err)
	}

	first, ok := <-out
	if !ok {
		t.Fatal("first forwarded event missing")
	}
	second, ok := <-out
	if !ok {
		t.Fatal("second forwarded event missing")
	}
	if _, ok := <-out; ok {
		t.Fatal("out channel remained open")
	}
	if first.EventName() != domain.EventTypeToken || second.EventName() != domain.EventTypeDone {
		t.Fatalf("forwarded events = %q/%q, want token/done", first.EventName(), second.EventName())
	}
}

func TestForwardStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	broker := NewMemoryBroker()
	sub, err := broker.Subscribe(context.Background(), "topic")
	if err != nil {
		t.Fatalf("subscribe error = %v", err)
	}

	out := make(chan domain.Event)
	if err := Forward(ctx, sub, out, ForwardOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Forward error = %v, want context.Canceled", err)
	}
	if _, ok := <-out; ok {
		t.Fatal("out channel remained open")
	}
}

func TestEventCodecRoundTripsKnownEvents(t *testing.T) {
	conversationID := uuid.New()
	tests := []domain.Event{
		domain.NewTokenEvent("tok"),
		domain.NewDoneEvent("done"),
		domain.NewMetaEvent(conversationID, "Title"),
		domain.NewErrorEvent("boom"),
	}

	for _, event := range tests {
		payload, err := MarshalEvent(event)
		if err != nil {
			t.Fatalf("MarshalEvent(%q) error = %v", event.EventName(), err)
		}
		got, err := UnmarshalEvent(payload)
		if err != nil {
			t.Fatalf("UnmarshalEvent(%q) error = %v", event.EventName(), err)
		}
		if got.EventName() != event.EventName() {
			t.Fatalf("round trip event = %q, want %q", got.EventName(), event.EventName())
		}
	}
}

func TestEventCodecRejectsInvalidJSON(t *testing.T) {
	if _, err := UnmarshalEvent([]byte(`{"event":`)); err == nil {
		t.Fatal("UnmarshalEvent returned nil error for invalid JSON")
	}
}
