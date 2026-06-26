package handler

import (
	"encoding/json"
	"fmt"

	"github.com/boxify/api-go/internal/domain"
	chatlogic "github.com/boxify/api-go/internal/logic/chat"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/middleware"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ChatHandler struct {
	svc *svc.ServiceContext
}

func NewChatHandler(svcCtx *svc.ServiceContext) ChatHandler {
	return ChatHandler{svc: svcCtx}
}

func (h ChatHandler) Stream(c *gin.Context) {
	var body request.ChatStreamRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		response.FromError(c, xerr.Validation(err))
		return
	}
	userID, _ := c.MustGet(middleware.UserIDKey).(uuid.UUID)
	events, err := chatlogic.NewStreamLogic(c.Request.Context(), h.svc).Stream(domain.ChatStreamInput{
		UserID:  userID,
		Message: body.Message,
	}, &body)
	if err != nil {
		response.FromError(c, err)
		return
	}

	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(200)

	for event := range events {
		name := event.Type
		payload := map[string]any{}
		switch event.Type {
		case "meta":
			payload["conversation_id"] = event.Text
			if title, ok := event.Stats["title"]; ok {
				payload["title"] = title
			}
		case "token":
			payload["text"] = event.Text
		case "done":
			payload["conversation_id"] = event.Text
		default:
			payload["text"] = event.Text
		}
		writeSSE(c.Writer, name, payload)
		c.Writer.Flush()
	}
}

func writeSSE(w gin.ResponseWriter, event string, data any) {
	encoded, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, encoded)
}
