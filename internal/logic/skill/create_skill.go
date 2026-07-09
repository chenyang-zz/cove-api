package skill

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/google/uuid"
)

// CreateSkillLogic contains the createSkill use case.
type CreateSkillLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewCreateSkillLogic creates a CreateSkillLogic.
func NewCreateSkillLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateSkillLogic {
	return &CreateSkillLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.skill.createskill"),
	}
}

// CreateSkill 创建skill
func (l *CreateSkillLogic) CreateSkill(userID uuid.UUID, input *request.CreateSkillRequest) (*response.SkillResponse, error) {
	var kbID *uuid.UUID
	var err error
	if input.KBID != nil {
		kbID, err = resolveSkillKnowledgeBaseID(l.ctx, l.svcCtx, userID, *input.KBID)
		if err != nil {
			return nil, err
		}
	}
	row := mapper.SkillFromCreateRequest(input, userID, uuid.New(), kbID, defaultSkillIcon)
	row, err = l.svcCtx.SkillRepo.Create(l.ctx, userID, row)
	if err != nil {
		return nil, err
	}
	l.log.InfoContext(l.ctx, "创建技能",
		slog.String("user_id", userID.String()),
		slog.String("skill_id", row.ID.String()),
	)
	return mapper.SkillToResponse(row), nil
}
