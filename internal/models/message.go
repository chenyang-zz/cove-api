/**
 * @Time   : 2026/6/27 19:45
 * @Author : chenyangzhao542@gmail.com
 * @File   : message.go
 **/

package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MessagePartType 消息可展示片段类型，与流式事件对齐。
const (
	MessagePartTypeText       = "text"
	MessagePartTypeToolCall   = "tool_call"
	MessagePartTypeToolResult = "tool_result"
)

// MessagePart 一回合内可展示片段，顺序即 UI 顺序。
type MessagePart struct {
	Type        string         `json:"type"` // text | tool_call | tool_result
	Text        string         `json:"text,omitempty"`
	Tool        string         `json:"tool,omitempty"`
	Input       map[string]any `json:"input,omitempty"`
	Observation string         `json:"observation,omitempty"`
	Error       string         `json:"error,omitempty"`
	Iteration   int            `json:"iteration,omitempty"`
	ToolCallID  string         `json:"tool_call_id,omitempty"`
}

type MessageMetaData struct {
	ImageKeys   []string      `json:"image_keys"`
	SenderName  string        `json:"sender_name"`
	Parts       []MessagePart `json:"parts,omitempty"`
	Interrupted bool          `json:"interrupted,omitempty"`
}

func (m MessageMetaData) Value() (driver.Value, error) {
	return json.Marshal(m)
}

func (m *MessageMetaData) Scan(value any) error {
	if value == nil {
		*m = MessageMetaData{}
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to scan MessageMetaData")
	}

	return json.Unmarshal(bytes, m)
}

type Message struct {
	ID             uuid.UUID    `gorm:"column:id;type:uuid;primaryKey"`
	ConversationID uuid.UUID    `gorm:"column:conversation_id;type:uuid;not null;index"`
	Conversation   Conversation `gorm:"foreignKey:ConversationID;references:ID;constraint:OnDelete:CASCADE"`
	Role           string       `gorm:"column:role;size:16;not null"`
	Content        string       `gorm:"column:content;type:text;not null"`
	// 群聊中该消息由哪个角色卡发出（user 消息为空；单聊 assistant 也为空）
	SenderPersonaID *uuid.UUID `gorm:"column:sender_persona_id;type:uuid;"`
	// 多人实时群聊中该 user 消息由哪个真人发出（单人会话/AI 消息为空）
	SenderUserID *uuid.UUID `gorm:"column:sender_user_id;type:uuid;"`
	// 附加信息：parts 时间线 / 图片 / 中断标记等
	MetaData  *MessageMetaData `gorm:"column:meta_data;type:jsonb"`
	CreatedAt time.Time        `gorm:"column:created_at;autoCreateTime"`
}

func (Message) TableName() string {
	return "messages"
}
