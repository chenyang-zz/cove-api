/**
 * @Time   : 2026/6/27 15:50
 * @Author : chenyangzhao542@gmail.com
 * @File   : conversation.go
 **/

package postgres

import (
	"context"
	"errors"

	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ConversationRepository struct {
	db *gorm.DB
}

func NewConversationRepository(db *gorm.DB) repository.ConversationRepository {
	return &ConversationRepository{db: db}
}

func (r *ConversationRepository) Create(ctx context.Context, userID uuid.UUID, conversation *models.Conversation) (*models.Conversation, error) {
	conversation.UserID = userID
	if err := r.db.WithContext(ctx).Create(conversation).Error; err != nil {
		return nil, xerr.Wrapf(err, "创建会话失败")
	}
	return conversation, nil
}

func (r *ConversationRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.Conversation, error) {
	var rows []*models.Conversation

	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("updated_at DESC").
		Find(&rows).Error
	if err != nil {
		return nil, xerr.Wrapf(err, "查询会话列表失败")
	}

	return rows, nil
}

func (r *ConversationRepository) FindByID(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) (*models.Conversation, error) {
	conversation := &models.Conversation{}
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", conversationID, userID).
		First(conversation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, xerr.NotFound("会话不存在")
	}
	if err != nil {
		return nil, xerr.Wrapf(err, "查询会话失败")
	}
	return conversation, nil
}

func (r *ConversationRepository) Update(ctx context.Context, userID uuid.UUID, conversation *models.Conversation) (*models.Conversation, error) {
	result := r.db.WithContext(ctx).
		Model(&models.Conversation{}).
		Where("id = ? AND user_id = ?", conversation.ID, userID).
		Omit("id", "user_id", "user", "created_at", "updated_at").
		Updates(conversation)
	if result.Error != nil {
		return nil, xerr.Wrapf(result.Error, "更新会话失败")
	}
	if result.RowsAffected == 0 {
		return nil, xerr.NotFound("会话不存在")
	}
	return r.FindByID(ctx, userID, conversation.ID)
}

func (r *ConversationRepository) UpdateFields(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID, conversation *models.Conversation, fields *repository.ConversationUpdateFields) (*models.Conversation, error) {
	columns := fields.Columns()
	if len(columns) == 0 {
		return nil, xerr.BadRequest("更新字段不能为空")
	}
	result := r.db.WithContext(ctx).
		Model(&models.Conversation{}).
		Where("id = ? AND user_id = ?", conversationID, userID).
		Select(columns).
		Updates(conversation)
	if result.Error != nil {
		return nil, xerr.Wrapf(result.Error, "更新会话失败")
	}
	if result.RowsAffected == 0 {
		return nil, xerr.NotFound("会话不存在")
	}
	return r.FindByID(ctx, userID, conversationID)
}

func (r *ConversationRepository) Delete(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", conversationID, userID).
		Delete(&models.Conversation{})
	if result.Error != nil {
		return xerr.Wrapf(result.Error, "删除会话失败")
	}
	if result.RowsAffected == 0 {
		return xerr.NotFound("会话不存在")
	}
	return nil
}
