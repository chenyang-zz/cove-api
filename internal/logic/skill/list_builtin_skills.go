package skill

import (
	"context"

	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
	"log/slog"
)

// ListBuiltinSkillsLogic contains the listBuiltinSkills use case.
type ListBuiltinSkillsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewListBuiltinSkillsLogic creates a ListBuiltinSkillsLogic.
func NewListBuiltinSkillsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListBuiltinSkillsLogic {
	return &ListBuiltinSkillsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.skill.listbuiltinskills"),
	}
}

// ListBuiltinSkills 查询内置skill
func (l *ListBuiltinSkillsLogic) ListBuiltinSkills(userID uuid.UUID) (*response.ListResponse[*response.SkillResponse], error) {
	if l.svcCtx == nil || l.svcCtx.SkillRegistry == nil {
		return nil, xerr.Internal("内置技能注册表未初始化", nil)
	}
	return mapper.BuiltinSkillsToListResponse(l.svcCtx.SkillRegistry.List()), nil
}
