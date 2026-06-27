/**
 * @Time   : 2026/6/27 15:41
 * @Author : chenyangzhao542@gmail.com
 * @File   : conversation.go
 **/

package request

type CreateConversationRequest struct {
	Title *string `json:"title" binding:"omitempty,min=1,max=256"`
}

type UriConversationIDRequest struct {
	ConversationID string `uri:"conversation_id" binding:"required"`
}

type RenameConversationRequest struct {
	UriConversationIDRequest
	Title string `json:"title" binding:"required,min=1,max=256"`
}
