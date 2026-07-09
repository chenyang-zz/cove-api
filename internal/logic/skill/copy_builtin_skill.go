package skill

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

// CopyBuiltinSkillLogic contains the copyBuiltinSkill use case.
type CopyBuiltinSkillLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewCopyBuiltinSkillLogic creates a CopyBuiltinSkillLogic.
func NewCopyBuiltinSkillLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CopyBuiltinSkillLogic {
	return &CopyBuiltinSkillLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.skill.copybuiltinskill"),
	}
}

// CopyBuiltinSkill 把内置技能复制为用户技能
func (l *CopyBuiltinSkillLogic) CopyBuiltinSkill(userID uuid.UUID, input *request.UriSkillIDRequest) (*response.SkillResponse, error) {
	skillID, err := skillIDFromInput(input)
	if err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.SkillRegistry == nil || l.svcCtx.SkillRepo == nil {
		return nil, xerr.Internal("内置技能依赖未初始化", nil)
	}
	template, ok := l.svcCtx.SkillRegistry.LookupByID(skillID)
	if !ok {
		return nil, xerr.NotFound("内置技能不存在")
	}

	row := mapper.BuiltinSkillToModel(template, userID, uuid.New())
	row, err = l.svcCtx.SkillRepo.Create(l.ctx, userID, row)
	if err != nil {
		return nil, err
	}
	l.log.InfoContext(l.ctx, "复制内置技能",
		slog.String("user_id", userID.String()),
		slog.String("builtin_skill_id", skillID.String()),
		slog.String("skill_id", row.ID.String()),
	)
	return mapper.SkillToResponse(row), nil
}
