/**
 * @Time   : 2026/6/27 15:56
 * @Author : chenyangzhao542@gmail.com
 * @File   : conversation.go
 **/

package mapper

import (
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/google/uuid"
)

func ConversationToResponse(row *models.Conversation) *response.ConversationResponse {
	if row == nil {
		return nil
	}
	res := &response.ConversationResponse{
		ID:               row.ID,
		Title:            row.Title,
		IsGroup:          row.IsGroup,
		MemberPersonaIDs: row.MemberPersonaIDs,
		EnableTools:      row.EnableTools,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}

	if res.MemberPersonaIDs == nil {
		res.MemberPersonaIDs = []string{}
	}

	return res
}

func ConversationsToListResponse(rows []*models.Conversation) *response.ListResponse[*response.ConversationResponse] {
	out := make([]*response.ConversationResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, ConversationToResponse(row))
	}
	return &response.ListResponse[*response.ConversationResponse]{List: out}
}

func MessageToResponse(row *models.Message, imagesMap map[uuid.UUID][]string, ratingMap map[uuid.UUID]string) *response.MessageResponse {
	images := make([]string, 0)
	if imgs, exist := imagesMap[row.ID]; exist {
		images = imgs
	}

	metadata := messageMetaToResponse(row.MetaData)

	res := &response.MessageResponse{
		ID:        row.ID,
		Role:      row.Role,
		Content:   row.Content,
		MetaData:  metadata,
		Images:    images,
		CreatedAt: row.CreatedAt,
	}

	// 可选字段保持 null，不加 omitempty；sender_name 仅放在 meta_data 内
	if row.SenderPersonaID != nil && *row.SenderPersonaID != uuid.Nil {
		res.SenderPersonaID = row.SenderPersonaID
	}
	if rating, exist := ratingMap[row.ID]; exist {
		res.Feedback = &rating
	}

	return res
}

// messageMetaToResponse 将模型 meta 转为响应；可选字段用指针，空则为 null。
func messageMetaToResponse(meta *models.MessageMetaData) *response.MessageMetaData {
	if meta == nil {
		return &response.MessageMetaData{}
	}

	out := &response.MessageMetaData{
		ImageKeys:   meta.ImageKeys,
		SenderName:  optionalStringPtr(meta.SenderName),
		Interrupted: meta.Interrupted,
	}
	// 透出有序 parts，供前端还原流式样式
	if len(meta.Parts) > 0 {
		out.Parts = make([]response.MessagePart, 0, len(meta.Parts))
		for _, part := range meta.Parts {
			out.Parts = append(out.Parts, messagePartToResponse(part))
		}
	}
	return out
}

func messagePartToResponse(part models.MessagePart) response.MessagePart {
	out := response.MessagePart{
		Type:  part.Type,
		Input: part.Input, // nil map → JSON null
	}
	out.Text = optionalStringPtr(part.Text)
	out.Tool = optionalStringPtr(part.Tool)
	out.Observation = optionalStringPtr(part.Observation)
	out.Error = optionalStringPtr(part.Error)
	out.ToolCallID = optionalStringPtr(part.ToolCallID)
	// iteration：仅工具相关 part 写出，纯 text 保持 null
	if part.Type == models.MessagePartTypeToolCall || part.Type == models.MessagePartTypeToolResult {
		iteration := part.Iteration
		out.Iteration = &iteration
	}
	return out
}

// optionalStringPtr 空字符串转为 nil，JSON 序列化为 null。
func optionalStringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// MessagesToListResponse 将消息列表映射为带 has_more 的会话消息列表响应。
func MessagesToListResponse(rows []*models.Message, imagesMap map[uuid.UUID][]string, ratingMap map[uuid.UUID]string, hasMore bool) *response.MessageListResponse {
	out := make([]*response.MessageResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, MessageToResponse(row, imagesMap, ratingMap))
	}
	return &response.MessageListResponse{
		List:    out,
		HasMore: hasMore,
	}
}
