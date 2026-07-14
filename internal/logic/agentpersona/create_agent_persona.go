package agentpersona

import (
	"context"
	"log/slog"
	"strings"

	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

// CreateAgentPersonaLogic contains the createAgentPersona use case.
type CreateAgentPersonaLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewCreateAgentPersonaLogic creates a CreateAgentPersonaLogic.
func NewCreateAgentPersonaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateAgentPersonaLogic {
	return &CreateAgentPersonaLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.agentpersona.createagentpersona"),
	}
}

// CreateAgentPersona 创建智能体角色
func (l *CreateAgentPersonaLogic) CreateAgentPersona(userID uuid.UUID, input *request.CreateAgentPersonaRequest) (*response.AgentPersonaResponse, error) {
	count, err := l.svcCtx.AgentPersonaRepo.Count(l.ctx, userID)
	if err != nil {
		return nil, err
	}
	if count >= int64(l.svcCtx.Config.Agent.MaxPersona) {
		return nil, xerr.BadRequestf("角色数量已达上限: %d", l.svcCtx.Config.Agent.MaxPersona)
	}

	persona := &models.AgentPersona{
		UserID:      userID,
		Name:        strings.TrimSpace(input.Name),
		AvatarKey:   input.AvatarKey,
		Identity:    strings.TrimSpace(input.Identity),
		Soul:        strings.TrimSpace(input.Soul),
		Temperature: 0.7,
	}
	if input.Temperature != nil {
		persona.Temperature = *input.Temperature
	}

	persona, err = l.svcCtx.AgentPersonaRepo.Create(l.ctx, userID, persona)
	if err != nil {
		return nil, err
	}

	avatarUrl := ""
	if persona.AvatarKey != "" {
		avatarUrl = l.svcCtx.URLSigner.URL(persona.AvatarKey)
	}

	return mapper.AgentPersonaToResponse(persona, avatarUrl), nil
}
