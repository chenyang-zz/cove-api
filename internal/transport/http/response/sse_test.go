package response

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type testSSEEvent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (e testSSEEvent) EventName() string {
	return e.Type
}

func TestStreamEventsWritesSSEEvents(t *testing.T) {
	events := make(chan testSSEEvent, 2)
	events <- testSSEEvent{Type: "token", Text: "hello"}
	events <- testSSEEvent{Type: "done", Text: "ok"}
	close(events)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/events", nil)

	StreamEvents(c, events)

	if got := w.Code; got != http.StatusOK {
		t.Fatalf("status = %d, want %d", got, http.StatusOK)
	}
	if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", got)
	}
	body := w.Body.String()
	for _, want := range []string{
		"event: token",
		`data: {"type":"token","text":"hello"}`,
		"event: done",
		`data: {"type":"done","text":"ok"}`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
}

func TestStreamEventsWithNilChannelReturns(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/events", nil)

	StreamEvents[testSSEEvent](c, nil)

	if got := w.Code; got != http.StatusOK {
		t.Fatalf("status = %d, want %d", got, http.StatusOK)
	}
	if got := w.Body.String(); got != "" {
		t.Fatalf("body = %q, want empty", got)
	}
}

func TestStreamEventsWritesCommentEvents(t *testing.T) {
	events := make(chan *CommentEvent, 1)
	events <- NewCommentEvent("ping")
	close(events)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/events", nil)

	StreamEvents(c, events)

	body := w.Body.String()
	if !strings.Contains(body, ": ping\n\n") {
		t.Fatalf("body missing ping comment:\n%s", body)
	}
	if strings.Contains(body, "event:") {
		t.Fatalf("comment event should not write event field:\n%s", body)
	}
}
