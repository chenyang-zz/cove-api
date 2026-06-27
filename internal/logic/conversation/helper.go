package conversation

import (
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

func conversationIDFromInput(input *request.UriConversationIDRequest) (uuid.UUID, error) {
	if input == nil {
		return uuid.Nil, xerr.BadRequest("会话 ID 无效")
	}
	id, err := uuid.Parse(input.ConversationID)
	if err != nil {
		return uuid.Nil, xerr.BadRequest("会话 ID 无效")
	}
	return id, nil
}
