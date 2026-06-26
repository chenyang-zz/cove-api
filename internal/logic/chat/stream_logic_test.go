package chat_test

import (
	"testing"

	"github.com/boxify/api-go/internal/domain"
	"github.com/boxify/api-go/internal/logic/chat"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/google/uuid"
)

func TestStreamLogicReturnsMetaTokenAndDoneEvents(t *testing.T) {
	events, err := chat.NewStreamLogic(t.Context(), &svc.ServiceContext{}).Stream(domain.ChatStreamInput{
		UserID:  uuid.New(),
		Message: "hello",
	}, &request.ChatStreamRequest{Message: "hello"})
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}

	seen := map[string]bool{}
	for event := range events {
		seen[event.Type] = true
	}
	for _, eventType := range []string{"meta", "token", "done"} {
		if !seen[eventType] {
			t.Fatalf("missing event %q", eventType)
		}
	}
}
