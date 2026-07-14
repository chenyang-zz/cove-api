package models

import (
	"time"

	"github.com/google/uuid"
)

// ConversationContextState 保存一个会话已经提交的滚动摘要及其乐观锁版本。
//
// ThroughMessageID 是摘要覆盖到的最后一条消息。原始消息仍保存在 messages 表中，
// 本模型只保存发送给模型时使用的派生状态。
type ConversationContextState struct {
	ConversationID    uuid.UUID  `gorm:"column:conversation_id;type:uuid;primaryKey"`
	Summary           string     `gorm:"column:summary;type:text;not null;default:''"`
	ThroughMessageID  *uuid.UUID `gorm:"column:through_message_id;type:uuid"`
	Version           int64      `gorm:"column:version;not null;default:0"`
	FormatVersion     int        `gorm:"column:format_version;not null;default:1"`
	PolicyFingerprint string     `gorm:"column:policy_fingerprint;size:64;not null;default:''"`
	CreatedAt         time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;autoUpdateTime"`

	Conversation Conversation `gorm:"foreignKey:ConversationID;references:ID;constraint:OnDelete:CASCADE"`
}

// TableName 返回滚动摘要状态使用的数据库表名。
func (ConversationContextState) TableName() string {
	return "conversation_context_states"
}
