/**
 * @Time   : 2026/6/27 15:44
 * @Author : chenyangzhao542@gmail.com
 * @File   : conversation.go
 **/

package response

import (
	"time"

	"github.com/google/uuid"
)

type ConversationResponse struct {
	ID               uuid.UUID `json:"id"`
	Title            string    `json:"title"`
	IsGroup          bool      `json:"is_group"`
	MemberPersonaIDs []string  `json:"member_persona_ids"`
	EnableTools      bool      `json:"enable_tools"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// MessagePart 历史消息可展示片段（与流式 tool/token 时间线对齐）。
// 可选字段用指针且不加 omitempty，缺省时 JSON 为 null。
type MessagePart struct {
	Type        string         `json:"type"`
	Text        *string        `json:"text"`
	Tool        *string        `json:"tool"`
	Input       map[string]any `json:"input"`
	Observation *string        `json:"observation"`
	Error       *string        `json:"error"`
	Iteration   *int           `json:"iteration"`
	ToolCallID  *string        `json:"tool_call_id"`
}

type MessageMetaData struct {
	ImageKeys   []string      `json:"image_keys"`
	SenderName  *string       `json:"sender_name"`
	Parts       []MessagePart `json:"parts"`
	Interrupted bool          `json:"interrupted"`
}

type MessageResponse struct {
	ID              uuid.UUID        `json:"id"`
	Role            string           `json:"role"`
	Content         string           `json:"content"`
	MetaData        *MessageMetaData `json:"meta_data"`
	Images          []string         `json:"images"`
	SenderPersonaID *uuid.UUID       `json:"sender_persona_id"`
	Feedback        *string          `json:"feedback"`
	CreatedAt       time.Time        `json:"created_at"`
}

// MessageListResponse 会话消息列表（游标分页）。
type MessageListResponse struct {
	List    []*MessageResponse `json:"list"`
	HasMore bool               `json:"has_more"`
}
