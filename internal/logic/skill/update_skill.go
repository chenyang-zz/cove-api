package skill

import (
	"context"
	"log/slog"
	"strings"

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

// UpdateSkillLogic contains the updateSkill use case.
type UpdateSkillLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewUpdateSkillLogic creates a UpdateSkillLogic.
func NewUpdateSkillLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateSkillLogic {
	return &UpdateSkillLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.skill.updateskill"),
	}
}

// UpdateSkill 更新skill
func (l *UpdateSkillLogic) UpdateSkill(userID uuid.UUID, input *request.UpdateSkillRequest) (*response.SkillResponse, error) {
	skillID, err := skillIDFromInput(&input.UriSkillIDRequest)
	if err != nil {
		return nil, err
	}
	patch := &models.Skill{}
	fields := repository.NewSkillUpdateFields()
	changed := make([]string, 0, 8)
	if input.Name != nil {
		patch.Name = strings.TrimSpace(*input.Name)
		fields.Name()
		changed = append(changed, "name")
	}
	if input.Description != nil {
		patch.Description = strings.TrimSpace(*input.Description)
		fields.Description()
		changed = append(changed, "description")
	}
	if input.Icon != nil {
		patch.Icon = strings.TrimSpace(*input.Icon)
		if patch.Icon == "" {
			patch.Icon = defaultSkillIcon
		}
		fields.Icon()
		changed = append(changed, "icon")
	}
	if input.Prompt != nil {
		patch.Prompt = strings.TrimSpace(*input.Prompt)
		fields.Prompt()
		changed = append(changed, "prompt")
	}
	if input.ToolKeys != nil {
		patch.ToolKeys = mapper.SkillToolKeysFromRequest(input.ToolKeys)
		fields.ToolKeys()
		changed = append(changed, "tool_keys")
	}
	if input.KBID != nil {
		patch.KBID, err = resolveSkillKnowledgeBaseID(l.ctx, l.svcCtx, userID, *input.KBID)
		if err != nil {
			return nil, err
		}
		fields.KBID()
		changed = append(changed, "kb_id")
	}
	if input.Config != nil {
		patch.Config = mapper.SkillConfigFromRequest(input.Config)
		fields.Config()
		changed = append(changed, "config")
	}
	if input.Enabled != nil {
		patch.Enabled = *input.Enabled
		fields.Enabled()
		changed = append(changed, "enabled")
	}
	if len(changed) == 0 {
		return nil, xerr.BadRequest("更新字段不能为空")
	}
	row, err := l.svcCtx.SkillRepo.UpdateFields(l.ctx, userID, skillID, patch, fields)
	if err != nil {
		return nil, err
	}
	l.log.InfoContext(l.ctx, "更新技能",
		slog.String("user_id", userID.String()),
		slog.String("skill_id", skillID.String()),
		slog.Any("fields", changed),
	)
	return mapper.SkillToResponse(row), nil
}
