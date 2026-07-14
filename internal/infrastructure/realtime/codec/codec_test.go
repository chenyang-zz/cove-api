package codec

import (
	"strings"
	"testing"

	"github.com/boxify/api-go/internal/domain/types"
	"github.com/google/uuid"
)

// 验证事件编解码器可以往返处理所有内置事件类型。
func TestEventCodecRoundTripsKnownEvents(t *testing.T) {
	conversationID := uuid.New()
	tests := []types.Event{
		types.NewTokenEvent("tok"),
		types.NewDoneEvent("done"),
		types.NewMetaEvent(conversationID, "Title"),
		types.NewErrorEvent("boom"),
		types.NewToolCallEvent("current_time", map[string]any{"zone": "UTC"}, 1, "call_1"),
		types.NewToolResultEvent("current_time", map[string]any{"zone": "UTC"}, "12:00", "", 1, "call_1"),
		types.NewThinkEvent(types.ThinkStatusThinking, 1),
		types.NewThinkEvent(types.ThinkStatusDone, 2),
		types.NewPingEvent(),
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

// 验证工具事件编解码会保留工具名称、参数、观察结果和调用编号。
func TestEventCodecRoundTripsToolEventFields(t *testing.T) {
	event := types.NewToolResultEvent("current_time", map[string]any{"zone": "UTC"}, "12:00", "", 2, "call_2")

	payload, err := MarshalEvent(event)
	if err != nil {
		t.Fatalf("MarshalEvent(tool_result) error = %v, want nil", err)
	}
	gotEvent, err := UnmarshalEvent(payload)
	if err != nil {
		t.Fatalf("UnmarshalEvent(tool_result) error = %v, want nil", err)
	}
	got, ok := gotEvent.(*types.ToolEvent)
	if !ok {
		t.Fatalf("decoded event type = %T, want *types.ToolEvent", gotEvent)
	}
	if got.Tool != "current_time" || got.Input["zone"] != "UTC" || got.Observation != "12:00" || got.Iteration != 2 || got.ToolCallID != "call_2" {
		t.Fatalf("decoded tool event = %#v, want preserved fields", got)
	}
}

// 验证 think 事件编解码保留 status 与 iteration。
func TestEventCodecRoundTripsThinkEventFields(t *testing.T) {
	event := types.NewThinkEvent(types.ThinkStatusThinking, 3)
	payload, err := MarshalEvent(event)
	if err != nil {
		t.Fatalf("MarshalEvent(think) error = %v", err)
	}
	raw := string(payload)
	if !strings.Contains(raw, `"status":"thinking"`) || !strings.Contains(raw, `"iteration":3`) {
		t.Fatalf("payload = %s, want status and iteration fields", raw)
	}
	gotEvent, err := UnmarshalEvent(payload)
	if err != nil {
		t.Fatalf("UnmarshalEvent(think) error = %v", err)
	}
	got, ok := gotEvent.(*types.ThinkEvent)
	if !ok {
		t.Fatalf("decoded event type = %T, want *types.ThinkEvent", gotEvent)
	}
	if got.Status != types.ThinkStatusThinking || got.Iteration != 3 {
		t.Fatalf("decoded think event = %#v, want status=thinking iteration=3", got)
	}
}

// 验证未知事件会退回 BaseEvent，兼容新增但本端尚不认识的事件。
func TestEventCodecMapsUnknownEventsToBaseEvent(t *testing.T) {
	got, err := UnmarshalEvent([]byte(`{"event":"custom","data":{"x":1}}`))
	if err != nil {
		t.Fatalf("UnmarshalEvent error = %v", err)
	}
	if got.EventName() != "custom" {
		t.Fatalf("EventName() = %q, want custom", got.EventName())
	}
	if _, ok := got.(*types.BaseEvent); !ok {
		t.Fatalf("event type = %T, want *types.BaseEvent", got)
	}
}

// 验证非法 JSON 会返回解析错误。
func TestEventCodecRejectsInvalidJSON(t *testing.T) {
	if _, err := UnmarshalEvent([]byte(`{"event":`)); err == nil {
		t.Fatal("UnmarshalEvent returned nil error for invalid JSON")
	}
}
