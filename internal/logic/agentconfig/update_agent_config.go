package agentconfig

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

// UpdateAgentConfigLogic contains the updateAgentConfig use case.
type UpdateAgentConfigLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewUpdateAgentConfigLogic creates a UpdateAgentConfigLogic.
func NewUpdateAgentConfigLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateAgentConfigLogic {
	return &UpdateAgentConfigLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.agentconfig.updateagentconfig"),
	}
}

// UpdateAgentConfig 更新智能体配置
func (l *UpdateAgentConfigLogic) UpdateAgentConfig(userID uuid.UUID, input *request.UpdateAgentConfigRequest) (*response.AgentConfigResponse, error) {

	config, err := l.svcCtx.AgentConfigRepo.FindByUserID(l.ctx, userID)
	if err != nil {
		if xerr.From(err).Kind != xerr.KindNotFound {
			return nil, err
		}
		config, err = l.svcCtx.AgentConfigRepo.Create(l.ctx, userID, &models.AgentConfig{})
		if err != nil {
			return nil, err
		}
	}

	patch := &models.AgentConfig{}
	candidate := *config
	fields := repository.NewAgentConfigUpdateFields()
	if input.SystemPrompt != nil {
		patch.SystemPrompt = *input.SystemPrompt
		fields.SystemPrompt()
	}
	if input.Temperature != nil {
		patch.Temperature = *input.Temperature
		fields.Temperature()
	}
	if input.EnableKnowledge != nil {
		patch.EnableKnowledge = *input.EnableKnowledge
		fields.EnableKnowledge()
	}
	if input.EnableMemory != nil {
		patch.EnableMemory = *input.EnableMemory
		fields.EnableMemory()
	}
	if input.EnableWebSearch != nil {
		patch.EnableWebSearch = *input.EnableWebSearch
		fields.EnableWebSearch()
	}
	if input.EnableActiveRecall != nil {
		patch.EnableActiveRecall = *input.EnableActiveRecall
		fields.EnableActiveRecall()
	}
	if input.EnableCrossSession != nil {
		patch.EnableCrossSession = *input.EnableCrossSession
		fields.EnableCrossSession()
	}
	if input.ShowAvatar != nil {
		patch.ShowAvatar = *input.ShowAvatar
		fields.ShowAvatar()
	}
	if input.HumanMode != nil {
		patch.HumanMode = *input.HumanMode
		fields.HumanMode()
	}
	if input.ContextEnabled != nil {
		patch.ContextEnabled, candidate.ContextEnabled = *input.ContextEnabled, *input.ContextEnabled
		fields.ContextEnabled()
	}
	if input.ContextWindowTokens != nil {
		patch.ContextWindowTokens, candidate.ContextWindowTokens = *input.ContextWindowTokens, *input.ContextWindowTokens
		fields.ContextWindowTokens()
	}
	if input.ContextOutputReserveTokens != nil {
		patch.ContextOutputReserveTokens, candidate.ContextOutputReserveTokens = *input.ContextOutputReserveTokens, *input.ContextOutputReserveTokens
		fields.ContextOutputReserveTokens()
	}
	if input.ContextSafetyMarginTokens != nil {
		patch.ContextSafetyMarginTokens, candidate.ContextSafetyMarginTokens = *input.ContextSafetyMarginTokens, *input.ContextSafetyMarginTokens
		fields.ContextSafetyMarginTokens()
	}
	if input.ContextTriggerRatio != nil {
		patch.ContextTriggerRatio, candidate.ContextTriggerRatio = *input.ContextTriggerRatio, *input.ContextTriggerRatio
		fields.ContextTriggerRatio()
	}
	if input.ContextTargetRatio != nil {
		patch.ContextTargetRatio, candidate.ContextTargetRatio = *input.ContextTargetRatio, *input.ContextTargetRatio
		fields.ContextTargetRatio()
	}
	if input.ContextKeepRecentTokens != nil {
		patch.ContextKeepRecentTokens, candidate.ContextKeepRecentTokens = *input.ContextKeepRecentTokens, *input.ContextKeepRecentTokens
		fields.ContextKeepRecentTokens()
	}
	if input.ContextSummaryMaxTokens != nil {
		patch.ContextSummaryMaxTokens, candidate.ContextSummaryMaxTokens = *input.ContextSummaryMaxTokens, *input.ContextSummaryMaxTokens
		fields.ContextSummaryMaxTokens()
	}
	if err := mapper.AgentConfigToContextPolicy(&candidate).Validate(); err != nil {
		return nil, xerr.BadRequest(err.Error())
	}

	config, err = l.svcCtx.AgentConfigRepo.UpdateFields(l.ctx, userID, config.ID, patch, fields)
	if err != nil {
		return nil, err
	}

	return mapper.AgentConfigToResponse(config), nil
}
