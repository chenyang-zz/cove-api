/**
 * @Time   : 2026/6/27 15:48
 * @Author : chenyangzhao542@gmail.com
 * @File   : conversation.go
 **/

package repository

import (
	"context"

	"github.com/boxify/api-go/internal/models"
	"github.com/google/uuid"
)

type ConversationRepository interface {
	Create(ctx context.Context, userID uuid.UUID, conversation *models.Conversation) (*models.Conversation, error)
	List(ctx context.Context, userID uuid.UUID) ([]*models.Conversation, error)
	// PageList 按用户分页查询会话，返回当前页与总数。
	PageList(ctx context.Context, userID uuid.UUID, query ConversationListQuery) (rows []*models.Conversation, total int64, err error)
	FindByID(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) (*models.Conversation, error)
	Update(ctx context.Context, userID uuid.UUID, conversation *models.Conversation) (*models.Conversation, error)
	UpdateFields(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID, conversation *models.Conversation, fields *ConversationUpdateFields) (*models.Conversation, error)
	Delete(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) error
}

// ConversationListQuery 会话列表分页查询条件。
type ConversationListQuery struct {
	PageQuery
}

type ConversationUpdateFields struct {
	columns []string
	seen    map[string]struct{}
}

func NewConversationUpdateFields() *ConversationUpdateFields {
	return &ConversationUpdateFields{
		seen: map[string]struct{}{},
	}
}

func (f *ConversationUpdateFields) Title() *ConversationUpdateFields {
	return f.add("title")
}

func (f *ConversationUpdateFields) IsGroup() *ConversationUpdateFields {
	return f.add("is_group")
}

func (f *ConversationUpdateFields) MemberPersonaIDs() *ConversationUpdateFields {
	return f.add("member_persona_ids")
}

func (f *ConversationUpdateFields) EnableTools() *ConversationUpdateFields {
	return f.add("enable_tools")
}

func (f *ConversationUpdateFields) JoinCode() *ConversationUpdateFields {
	return f.add("join_code")
}

func (f *ConversationUpdateFields) Columns() []string {
	if f == nil || len(f.columns) == 0 {
		return nil
	}
	out := make([]string, len(f.columns))
	copy(out, f.columns)
	return out
}

func (f *ConversationUpdateFields) add(column string) *ConversationUpdateFields {
	if f == nil {
		f = NewConversationUpdateFields()
	}
	if f.seen == nil {
		f.seen = map[string]struct{}{}
	}
	if _, ok := f.seen[column]; ok {
		return f
	}
	f.seen[column] = struct{}{}
	f.columns = append(f.columns, column)
	return f
}
