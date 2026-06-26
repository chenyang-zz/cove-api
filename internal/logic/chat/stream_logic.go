package chat

import (
	"context"
	"log/slog"
	"strings"

	"github.com/boxify/api-go/internal/domain"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/google/uuid"
)

type StreamLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

func NewStreamLogic(ctx context.Context, svcCtx *svc.ServiceContext) *StreamLogic {
	return &StreamLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.chat.stream"),
	}
}

func (l *StreamLogic) Stream(input domain.ChatStreamInput, req *request.ChatStreamRequest) (<-chan domain.AgentEvent, error) {
	if req != nil {
		input.Message = req.Message
	}
	events := make(chan domain.AgentEvent, 4)
	go func() {
		defer close(events)
		conversationID := uuid.New()
		title := strings.TrimSpace(input.Message)
		if title == "" {
			title = "新对话"
		}
		if len([]rune(title)) > 20 {
			title = string([]rune(title)[:20])
		}
		select {
		case <-l.ctx.Done():
			return
		case events <- domain.AgentEvent{Type: "meta", Text: conversationID.String(), Stats: map[string]any{"title": title}}:
		}
		select {
		case <-l.ctx.Done():
			return
		case events <- domain.AgentEvent{Type: "token", Text: "hello"}:
		}
		select {
		case <-l.ctx.Done():
			return
		case events <- domain.AgentEvent{Type: "done", Text: conversationID.String()}:
		}
	}()
	return events, nil
}
