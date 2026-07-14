package postgres

import (
	"context"
	"errors"

	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ConversationContextStateRepository 使用 Postgres 持久化会话滚动摘要状态。
type ConversationContextStateRepository struct {
	db *gorm.DB
}

// NewConversationContextStateRepository 创建独立的会话上下文状态仓储。
func NewConversationContextStateRepository(db *gorm.DB) repository.ConversationContextStateRepository {
	return &ConversationContextStateRepository{db: db}
}

// LoadContextState 读取用户所属会话的滚动摘要；尚未生成摘要时返回 nil, nil。
func (r *ConversationContextStateRepository) LoadContextState(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) (*models.ConversationContextState, error) {
	state := &models.ConversationContextState{}
	err := r.db.WithContext(ctx).
		Joins("JOIN conversations ON conversation_context_states.conversation_id = conversations.id").
		Where("conversation_context_states.conversation_id = ?", conversationID).
		Where("conversations.user_id = ?", userID).
		First(state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, xerr.Wrapf(err, "读取会话上下文摘要失败")
	}
	return state, nil
}

// CompareAndSwapContextState 仅在版本匹配时写入滚动摘要。
//
// 返回 false, nil 表示另一请求已经推进版本，调用方应重新加载后重算。
func (r *ConversationContextStateRepository) CompareAndSwapContextState(ctx context.Context, userID uuid.UUID, expectedVersion int64, state *models.ConversationContextState) (bool, error) {
	if state == nil || state.ConversationID == uuid.Nil {
		return false, xerr.BadRequest("会话上下文摘要不能为空")
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.Conversation{}).
		Where("id = ? AND user_id = ?", state.ConversationID, userID).
		Count(&count).Error; err != nil {
		return false, xerr.Wrapf(err, "校验会话上下文归属失败")
	}
	if count == 0 {
		return false, xerr.NotFound("会话不存在")
	}

	if expectedVersion == 0 {
		candidate := *state
		result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate)
		if result.Error != nil {
			return false, xerr.Wrapf(result.Error, "创建会话上下文摘要失败")
		}
		return result.RowsAffected == 1, nil
	}

	result := r.db.WithContext(ctx).
		Model(&models.ConversationContextState{}).
		Where("conversation_id = ? AND version = ?", state.ConversationID, expectedVersion).
		Where("EXISTS (SELECT 1 FROM conversations WHERE conversations.id = conversation_context_states.conversation_id AND conversations.user_id = ?)", userID).
		Select("summary", "through_message_id", "version", "format_version", "policy_fingerprint").
		Updates(state)
	if result.Error != nil {
		return false, xerr.Wrapf(result.Error, "更新会话上下文摘要失败")
	}
	return result.RowsAffected == 1, nil
}

var _ repository.ConversationContextStateRepository = (*ConversationContextStateRepository)(nil)
