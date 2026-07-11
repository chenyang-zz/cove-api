package chat

import (
	"strings"
	"testing"

	"github.com/boxify/api-go/internal/models"
)

// 验证相邻 text 会合并，工具片段按序追加。
func TestAppendPartsBuildsOrderedTimeline(t *testing.T) {
	var parts []models.MessagePart
	parts = appendTextPart(parts, "先")
	parts = appendTextPart(parts, "查一下")
	parts = appendToolCallPart(parts, "search", map[string]any{"q": "go"}, 1, "call_1")
	parts = appendToolResultPart(parts, "search", map[string]any{"q": "go"}, "ok", "", 1, "call_1")
	parts = finalizePartsWithAnswer(parts, "根据结果回答")

	if len(parts) != 4 {
		t.Fatalf("parts len = %d, want 4: %#v", len(parts), parts)
	}
	if parts[0].Type != models.MessagePartTypeText || parts[0].Text != "先查一下" {
		t.Fatalf("text part = %#v, want merged 先查一下", parts[0])
	}
	if parts[1].Type != models.MessagePartTypeToolCall || parts[1].ToolCallID != "call_1" {
		t.Fatalf("tool_call part = %#v", parts[1])
	}
	if parts[2].Type != models.MessagePartTypeToolResult || parts[2].Observation != "ok" {
		t.Fatalf("tool_result part = %#v", parts[2])
	}
	if parts[3].Type != models.MessagePartTypeText || parts[3].Text != "根据结果回答" {
		t.Fatalf("final text = %#v, want 根据结果回答", parts[3])
	}
}

// 验证无工具时 finalize 只保留最终 answer。
func TestFinalizePartsWithoutToolsUsesAnswerOnly(t *testing.T) {
	parts := appendTextPart(nil, "partial")
	got := finalizePartsWithAnswer(parts, "final answer")
	if len(got) != 1 || got[0].Text != "final answer" {
		t.Fatalf("finalize without tools = %#v, want single final answer", got)
	}
}

// 验证 observation 超长会被截断。
func TestTruncateObservationLimitsRunes(t *testing.T) {
	long := strings.Repeat("测", maxObservationRunes+10)
	got := truncateObservation(long)
	if len([]rune(got)) != maxObservationRunes {
		t.Fatalf("truncate len = %d, want %d", len([]rune(got)), maxObservationRunes)
	}
}
