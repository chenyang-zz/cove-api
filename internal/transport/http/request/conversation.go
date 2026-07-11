/**
 * @Time   : 2026/6/27 15:41
 * @Author : chenyangzhao542@gmail.com
 * @File   : conversation.go
 **/

package request

type CreateConversationRequest struct {
	Title *string `json:"title" binding:"omitempty,min=1,max=256"`
}

// ListConversationsRequest 分页获取会话列表。
type ListConversationsRequest struct {
	PageRequest
}

type UriConversationIDRequest struct {
	ConversationID string `uri:"conversation_id" binding:"required"`
}

type RenameConversationRequest struct {
	UriConversationIDRequest
	Title string `json:"title" binding:"required,min=1,max=256"`
}

// ListMessagesRequest 获取会话消息列表（支持 before 游标滚动加载）。
type ListMessagesRequest struct {
	UriConversationIDRequest
	Limit  int64  `form:"limit" json:"limit" binding:"omitempty,gte=1,lte=100"`
	Before string `form:"before" json:"before" binding:"omitempty,uuid"`
}
