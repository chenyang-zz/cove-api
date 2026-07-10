/**
 * @Time   : 2026/6/27 23:39
 * @Author : chenyangzhao542@gmail.com
 * @File   : chat.go
 **/

package mapper

import (
	"context"

	"github.com/boxify/api-go/internal/domain/types"
	"github.com/boxify/api-go/internal/transport/http/response"
)

func BaseEventToResponse(event *types.BaseEvent) *response.BaseEvent {
	return &response.BaseEvent{
		Type: event.Type,
	}
}

func EventToResponse(event types.Event) response.SSEEvent {
	switch e := event.(type) {
	case nil:
		return &response.BaseEvent{Type: types.EventTypeError, Text: "nil event"}
	case *types.TextEvent:
		return &response.BaseEvent{
			Type: e.Type,
			Text: e.Text,
		}
	case *types.MetaEvent:
		return &response.MetaEvent{
			BaseEvent:      response.BaseEvent{Type: e.Type},
			ConversationID: e.ConversationID,
			Title:          e.Title,
		}
	case *types.ErrorEvent:
		return &response.ErrorEvent{
			BaseEvent: response.BaseEvent{Type: e.Type},
			Message:   e.Message,
		}
	case *types.BaseEvent:
		return BaseEventToResponse(e)
	case *types.ToolEvent:
		return &response.ToolEvent{
			BaseEvent:   response.BaseEvent{Type: e.Type},
			Tool:        e.Tool,
			Input:       e.Input,
			Observation: e.Observation,
			Error:       e.Error,
			Iteration:   e.Iteration,
			ToolCallID:  e.ToolCallID,
		}
	default:
		return &response.BaseEvent{Type: event.EventName()}
	}
}

func EventStreamToResponse(ctx context.Context, events <-chan types.Event) <-chan response.SSEEvent {
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
				if event != nil && event.EventName() == types.EventTypePing {
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
