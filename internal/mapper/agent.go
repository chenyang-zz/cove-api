/**
 * @Time   : 2026/6/28 16:08
 * @Author : chenyangzhao542@gmail.com
 * @File   : agent.go
 **/

package mapper

import (
	corecontext "github.com/boxify/api-go/internal/core/context"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/transport/http/response"
)

func AgentConfigToResponse(row *models.AgentConfig) *response.AgentConfigResponse {
	return &response.AgentConfigResponse{
		SystemPrompt:               row.SystemPrompt,
		Temperature:                row.Temperature,
		EnableKnowledge:            row.EnableKnowledge,
		EnableMemory:               row.EnableMemory,
		EnableWebSearch:            row.EnableWebSearch,
		EnableActiveRecall:         row.EnableActiveRecall,
		EnableCrossSession:         row.EnableCrossSession,
		ShowAvatar:                 row.ShowAvatar,
		HumanMode:                  row.HumanMode,
		ContextEnabled:             row.ContextEnabled,
		ContextWindowTokens:        row.ContextWindowTokens,
		ContextOutputReserveTokens: row.ContextOutputReserveTokens,
		ContextSafetyMarginTokens:  row.ContextSafetyMarginTokens,
		ContextTriggerRatio:        row.ContextTriggerRatio,
		ContextTargetRatio:         row.ContextTargetRatio,
		ContextKeepRecentTokens:    row.ContextKeepRecentTokens,
		ContextSummaryMaxTokens:    row.ContextSummaryMaxTokens,
	}
}

// AgentConfigToContextPolicy 将数据库 Agent 配置映射为 core 上下文策略。
func AgentConfigToContextPolicy(row *models.AgentConfig) *corecontext.Policy {
	if row == nil {
		return corecontext.DefaultPolicy()
	}
	if row.ContextWindowTokens == 0 && row.ContextOutputReserveTokens == 0 && row.ContextSafetyMarginTokens == 0 &&
		row.ContextTriggerRatio == 0 && row.ContextTargetRatio == 0 && row.ContextKeepRecentTokens == 0 && row.ContextSummaryMaxTokens == 0 {
		return corecontext.DefaultPolicy()
	}
	return &corecontext.Policy{
		Enabled:             row.ContextEnabled,
		WindowTokens:        row.ContextWindowTokens,
		OutputReserveTokens: row.ContextOutputReserveTokens,
		SafetyMarginTokens:  row.ContextSafetyMarginTokens,
		TriggerRatio:        row.ContextTriggerRatio,
		TargetRatio:         row.ContextTargetRatio,
		KeepRecentTokens:    row.ContextKeepRecentTokens,
		SummaryMaxTokens:    row.ContextSummaryMaxTokens,
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
