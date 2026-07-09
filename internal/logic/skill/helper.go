package skill

import (
	"context"

	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

const defaultSkillIcon = "🧩"

func skillIDFromInput(input *request.UriSkillIDRequest) (uuid.UUID, error) {
	if input == nil {
		return uuid.Nil, xerr.BadRequest("技能 ID 无效")
	}
	id, err := uuid.Parse(input.ID)
	if err != nil {
		return uuid.Nil, xerr.BadRequest("技能 ID 无效")
	}
	return id, nil
}

func resolveSkillKnowledgeBaseID(ctx context.Context, svcCtx *svc.ServiceContext, userID uuid.UUID, raw string) (*uuid.UUID, error) {
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, xerr.BadRequest("知识库 ID 无效")
	}
	if _, err := svcCtx.KnowledgeBaseRepo.FindByID(ctx, userID, id); err != nil {
		return nil, err
	}
	return &id, nil
}
