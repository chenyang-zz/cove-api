package agentpersona

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
	"github.com/google/uuid"
)

// UpdateAgentPersonaLogic contains the updateAgentPersona use case.
type UpdateAgentPersonaLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewUpdateAgentPersonaLogic creates a UpdateAgentPersonaLogic.
func NewUpdateAgentPersonaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateAgentPersonaLogic {
	return &UpdateAgentPersonaLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.agentpersona.updateagentpersona"),
	}
}

// UpdateAgentPersona 更新智能体角色
func (l *UpdateAgentPersonaLogic) UpdateAgentPersona(userID uuid.UUID, input *request.UpdateAgentPersonaRequest) (*response.AgentPersonaResponse, error) {
	personaID, err := personIDFromInput(&input.UriAgentPersonaIDRequest)
	if err != nil {
		return nil, err
	}
	persona, err := l.svcCtx.AgentPersonaRepo.FindByID(l.ctx, userID, personaID)
	if err != nil {
		return nil, err
	}

	patch := &models.AgentPersona{}
	fields := repository.NewAgentPersonaUpdateFields()
	if input.Name != nil {
		patch.Name = *input.Name
		fields.Name()
	}
	if input.AvatarKey != nil {
		patch.AvatarKey = *input.AvatarKey
		fields.AvatarKey()
	}
	if input.Temperature != nil {
		patch.Temperature = *input.Temperature
		fields.Temperature()
	}
	if input.Identity != nil {
		patch.Identity = *input.Identity
		fields.Identity()
	}
	if input.Soul != nil {
		patch.Soul = *input.Soul
		fields.Soul()
	}

	persona, err = l.svcCtx.AgentPersonaRepo.UpdateFields(l.ctx, userID, persona.ID, patch, fields)
	if err != nil {
		return nil, err
	}

	avatarUrl := ""
	if persona.AvatarKey != "" {
		avatarUrl = l.svcCtx.URLSigner.URL(persona.AvatarKey)
	}

	return mapper.AgentPersonaToResponse(persona, avatarUrl), nil
}
