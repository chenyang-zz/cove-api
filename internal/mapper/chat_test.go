package mapper_test

import (
	"context"
	"testing"

	"github.com/boxify/api-go/internal/domain"
	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/google/uuid"
)

func TestEventToResponseMapsTextEvent(t *testing.T) {
	got := mapper.EventToResponse(domain.NewTokenEvent("hello"))

	event, ok := got.(*response.BaseEvent)
	if !ok {
		t.Fatalf("EventToResponse type = %T, want *response.BaseEvent", got)
	}
	if event.Type != domain.EventTypeToken || event.Text != "hello" || event.EventName() != domain.EventTypeToken {
		t.Fatalf("event = %+v, want token hello", event)
	}
}

func TestEventToResponseMapsMetaEvent(t *testing.T) {
	conversationID := uuid.New()
	got := mapper.EventToResponse(domain.NewMetaEvent(conversationID, "New Chat"))

	event, ok := got.(*response.MetaEvent)
	if !ok {
		t.Fatalf("EventToResponse type = %T, want *response.MetaEvent", got)
	}
	if event.Type != domain.EventTypeMeta || event.ConversationID != conversationID || event.Title != "New Chat" || event.EventName() != domain.EventTypeMeta {
		t.Fatalf("event = %+v, want meta payload", event)
	}
}

func TestEventToResponseMapsErrorEvent(t *testing.T) {
	got := mapper.EventToResponse(domain.NewErrorEvent("boom"))

	event, ok := got.(*response.ErrorEvent)
	if !ok {
		t.Fatalf("EventToResponse type = %T, want *response.ErrorEvent", got)
	}
	if event.Type != domain.EventTypeError || event.Message != "boom" || event.EventName() != domain.EventTypeError {
		t.Fatalf("event = %+v, want error payload", event)
	}
}

func TestEventStreamToResponseMapsAndCloses(t *testing.T) {
	events := make(chan domain.Event, 2)
	events <- domain.NewTokenEvent("hello")
	events <- domain.NewDoneEvent("ok")
	close(events)

	out := mapper.EventStreamToResponse(context.Background(), events)
	first, ok := <-out
	if !ok {
		t.Fatal("first response missing")
	}
	second, ok := <-out
	if !ok {
		t.Fatal("second response missing")
	}
	if _, ok := <-out; ok {
		t.Fatal("response channel remained open")
	}

	if first.EventName() != domain.EventTypeToken || second.EventName() != domain.EventTypeDone {
		t.Fatalf("events = %q/%q, want token/done", first.EventName(), second.EventName())
	}
}

func TestEventStreamToResponseMapsPingToComment(t *testing.T) {
	events := make(chan domain.Event, 1)
	events <- domain.NewPingEvent()
	close(events)

	out := mapper.EventStreamToResponse(context.Background(), events)
	got, ok := <-out
	if !ok {
		t.Fatal("response missing")
	}
	comment, ok := got.(*response.CommentEvent)
	if !ok {
		t.Fatalf("response type = %T, want *response.CommentEvent", got)
	}
	if comment.Comment() != "ping" {
		t.Fatalf("comment = %q, want ping", comment.Comment())
	}
}

func TestEventStreamToResponseMapsNilEventToError(t *testing.T) {
	events := make(chan domain.Event, 1)
	events <- nil
	close(events)

	out := mapper.EventStreamToResponse(context.Background(), events)
	got, ok := <-out
	if !ok {
		t.Fatal("response missing")
	}
	event, ok := got.(*response.BaseEvent)
	if !ok {
		t.Fatalf("response type = %T, want *response.BaseEvent", got)
	}
	if event.EventName() != domain.EventTypeError {
		t.Fatalf("event = %q, want error", event.EventName())
	}
}
