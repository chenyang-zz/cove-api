package mapper

import (
	"testing"
	"time"

	"github.com/boxify/api-go/internal/models"
	"github.com/google/uuid"
)

// 验证 MessageToResponse 会透出 parts / interrupted，且可选空字段为 nil（JSON null）。
func TestMessageToResponseExposesParts(t *testing.T) {
	id := uuid.New()
	row := &models.Message{
		ID:        id,
		Role:      "assistant",
		Content:   "最终回答",
		CreatedAt: time.Now(),
		MetaData: &models.MessageMetaData{
			Interrupted: true,
			Parts: []models.MessagePart{
				{Type: models.MessagePartTypeToolCall, Tool: "search", ToolCallID: "c1", Iteration: 1},
				{Type: models.MessagePartTypeToolResult, Tool: "search", ToolCallID: "c1", Observation: "hit", Iteration: 1},
				{Type: models.MessagePartTypeText, Text: "最终回答"},
			},
		},
	}

	got := MessageToResponse(row, nil, nil)
	if got == nil || got.MetaData == nil {
		t.Fatalf("MessageToResponse = %#v, want metadata", got)
	}
	if !got.MetaData.Interrupted {
		t.Fatal("Interrupted = false, want true")
	}
	if len(got.MetaData.Parts) != 3 {
		t.Fatalf("parts len = %d, want 3", len(got.MetaData.Parts))
	}
	call := got.MetaData.Parts[0]
	if call.Type != models.MessagePartTypeToolCall || call.ToolCallID == nil || *call.ToolCallID != "c1" {
		t.Fatalf("parts[0] = %#v", call)
	}
	if call.Text != nil || call.Observation != nil {
		t.Fatalf("parts[0] optional empties should be nil, got text=%v observation=%v", call.Text, call.Observation)
	}
	result := got.MetaData.Parts[1]
	if result.Observation == nil || *result.Observation != "hit" {
		t.Fatalf("parts[1] = %#v, want observation hit", result)
	}
	text := got.MetaData.Parts[2]
	if text.Text == nil || *text.Text != "最终回答" {
		t.Fatalf("parts[2] = %#v, want text 最终回答", text)
	}
	if text.Tool != nil || text.Iteration != nil {
		t.Fatalf("parts[2] tool/iteration should be nil, got %#v", text)
	}
	if got.Feedback != nil || got.SenderPersonaID != nil {
		t.Fatalf("optional top-level fields should stay nil, feedback=%v persona=%v", got.Feedback, got.SenderPersonaID)
	}
	if got.MetaData.SenderName != nil {
		t.Fatalf("sender_name should live only in meta when empty, got %#v", got.MetaData.SenderName)
	}
}
