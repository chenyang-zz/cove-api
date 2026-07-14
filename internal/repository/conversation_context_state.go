package repository

import (
	"context"

	"github.com/boxify/api-go/internal/models"
	"github.com/google/uuid"
)

// ConversationContextStateRepository 定义会话滚动摘要状态的独立持久化能力。
type ConversationContextStateRepository interface {
	LoadContextState(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) (*models.ConversationContextState, error)
	CompareAndSwapContextState(ctx context.Context, userID uuid.UUID, expectedVersion int64, state *models.ConversationContextState) (bool, error)
}
