package codec

import (
	"encoding/json"

	"github.com/boxify/api-go/internal/domain/types"
	"github.com/google/uuid"
)

type eventEnvelope struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

type textEventData struct {
	Text string `json:"text"`
}

type metaEventData struct {
	ConversationID uuid.UUID `json:"conversation_id"`
	Title          string    `json:"title"`
}

type errorEventData struct {
	Message string `json:"message"`
}

type toolEventData struct {
	Tool        string         `json:"tool"`
	Input       map[string]any `json:"input,omitempty"`
	Observation string         `json:"observation,omitempty"`
	Error       string         `json:"error,omitempty"`
	Iteration   int            `json:"iteration,omitempty"`
	ToolCallID  string         `json:"tool_call_id,omitempty"`
}

func MarshalEvent(event types.Event) ([]byte, error) {
	var data any = map[string]any{}
	switch e := event.(type) {
	case *types.TextEvent:
		data = textEventData{Text: e.Text}
	case *types.MetaEvent:
		data = metaEventData{ConversationID: e.ConversationID, Title: e.Title}
	case *types.ErrorEvent:
		data = errorEventData{Message: e.Message}
	case *types.ToolEvent:
		data = toolEventData{
			Tool:        e.Tool,
			Input:       e.Input,
			Observation: e.Observation,
			Error:       e.Error,
			Iteration:   e.Iteration,
			ToolCallID:  e.ToolCallID,
		}
	}

	return json.Marshal(struct {
		Event string `json:"event"`
		Data  any    `json:"data"`
	}{
		Event: event.EventName(),
		Data:  data,
	})
}

func UnmarshalEvent(payload []byte) (types.Event, error) {
	var envelope eventEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, err
	}

	switch envelope.Event {
	case types.EventTypeToken:
		var data textEventData
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			return nil, err
		}
		return types.NewTokenEvent(data.Text), nil
	case types.EventTypeDone:
		var data textEventData
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			return nil, err
		}
		return types.NewDoneEvent(data.Text), nil
	case types.EventTypeMeta:
		var data metaEventData
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			return nil, err
		}
		return types.NewMetaEvent(data.ConversationID, data.Title), nil
	case types.EventTypeError:
		var data errorEventData
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			return nil, err
		}
		return types.NewErrorEvent(data.Message), nil
	case types.EventTypeToolCall:
		var data toolEventData
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			return nil, err
		}
		return types.NewToolCallEvent(data.Tool, data.Input, data.Iteration, data.ToolCallID), nil
	case types.EventTypeToolResult:
		var data toolEventData
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			return nil, err
		}
		return types.NewToolResultEvent(data.Tool, data.Input, data.Observation, data.Error, data.Iteration, data.ToolCallID), nil
	case types.EventTypePing:
		return types.NewPingEvent(), nil
	default:
		return types.NewBaseEvent(envelope.Event), nil
	}
}
