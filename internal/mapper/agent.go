/**
 * @Time   : 2026/6/28 16:08
 * @Author : chenyangzhao542@gmail.com
 * @File   : agent.go
 **/

package mapper

import (
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/transport/http/response"
)

func AgentConfigToResponse(row *models.AgentConfig) *response.AgentConfigResponse {
	return &response.AgentConfigResponse{
		SystemPrompt:       row.SystemPrompt,
		Temperature:        row.Temperature,
		EnableKnowledge:    row.EnableKnowledge,
		EnableMemory:       row.EnableMemory,
		EnableWebSearch:    row.EnableWebSearch,
		EnableActiveRecall: row.EnableActiveRecall,
		EnableCrossSession: row.EnableCrossSession,
		ShowAvatar:         row.ShowAvatar,
		HumanMode:          row.HumanMode,
	}
}

func AgentPersonaToResponse(row *models.AgentPersona, avatarUrl string) *response.AgentPersonaResponse {
	return &response.AgentPersonaResponse{
		ID:          row.ID,
		Name:        row.Name,
		AvatarKey:   row.AvatarKey,
		AvatarUrl:   avatarUrl,
		Identity:    row.Identity,
		Soul:        row.Soul,
		Temperature: row.Temperature,
		IsActive:    row.IsActive,
	}
}
