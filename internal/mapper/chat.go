/**
 * @Time   : 2026/6/27 23:39
 * @Author : chenyangzhao542@gmail.com
 * @File   : chat.go
 **/

package mapper

import (
	"context"

	"github.com/boxify/api-go/internal/domain"
	"github.com/boxify/api-go/internal/transport/http/response"
)

func BaseEventToResponse(event *domain.BaseEvent) *response.BaseEvent {
	return &response.BaseEvent{
		Type: event.Type,
	}
}

func EventToResponse(event domain.Event) response.SSEEvent {
	switch e := event.(type) {
	case nil:
		return &response.BaseEvent{Type: domain.EventTypeError, Text: "nil event"}
	case *domain.TextEvent:
		return &response.BaseEvent{
			Type: e.Type,
			Text: e.Text,
		}
	case *domain.MetaEvent:
		return &response.MetaEvent{
			BaseEvent:      response.BaseEvent{Type: e.Type},
			ConversationID: e.ConversationID,
			Title:          e.Title,
		}
	case *domain.ErrorEvent:
		return &response.ErrorEvent{
			BaseEvent: response.BaseEvent{Type: e.Type},
			Message:   e.Message,
		}
	case *domain.BaseEvent:
		return BaseEventToResponse(e)
	default:
		return &response.BaseEvent{Type: event.EventName()}
	}
}

func EventStreamToResponse(ctx context.Context, events <-chan domain.Event) <-chan response.SSEEvent {
	out := make(chan response.SSEEvent)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-events:
				if !ok {
					return
				}
				if event != nil && event.EventName() == domain.EventTypePing {
					select {
					case <-ctx.Done():
						return
					case out <- response.NewCommentEvent("ping"):
					}
					continue
				}
				select {
				case <-ctx.Done():
					return
				case out <- EventToResponse(event):
				}
			}
		}
	}()
	return out
}
