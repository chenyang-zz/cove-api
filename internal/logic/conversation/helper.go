package conversation

import (
	"strings"

	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

const (
	defaultMessagePageLimit = 30
	maxMessagePageLimit     = 100
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

// normalizeMessageLimit 规范化消息分页 limit：省略时默认 30，上限 100。
func normalizeMessageLimit(limit int64) int {
	if limit < 1 {
		return defaultMessagePageLimit
	}
	if limit > maxMessagePageLimit {
		return maxMessagePageLimit
	}
	return int(limit)
}

// parseOptionalBeforeID 解析可选 before 游标；空字符串表示最新页。
func parseOptionalBeforeID(before string) (*uuid.UUID, error) {
	before = strings.TrimSpace(before)
	if before == "" {
		return nil, nil
	}
	id, err := uuid.Parse(before)
	if err != nil {
		return nil, xerr.BadRequest("before 消息 ID 无效")
	}
	return &id, nil
}
