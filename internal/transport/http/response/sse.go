package response

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type SSEEvent interface {
	EventName() string
}

type CommentEvent struct {
	comment string
}

func NewCommentEvent(comment string) *CommentEvent {
	return &CommentEvent{comment: comment}
}

func (e *CommentEvent) EventName() string {
	return ""
}

func (e *CommentEvent) Comment() string {
	if e == nil {
		return ""
	}
	return e.comment
}

func StreamEvents[T SSEEvent](c *gin.Context, events <-chan T) {
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	if events == nil {
		return
	}

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if comment, ok := any(event).(interface{ Comment() string }); ok {
				writeSSEComment(c.Writer, comment.Comment())
				c.Writer.Flush()
				continue
			}
			writeSSE(c.Writer, event.EventName(), event)
			c.Writer.Flush()
		}
	}
}

func writeSSEComment(w gin.ResponseWriter, comment string) {
	fmt.Fprintf(w, ": %s\n\n", comment)
}

func writeSSE(w gin.ResponseWriter, event string, data any) {
	encoded, err := json.Marshal(data)
	if err != nil {
		encoded = []byte(`{"error":"marshal event failed"}`)
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, encoded)
}
