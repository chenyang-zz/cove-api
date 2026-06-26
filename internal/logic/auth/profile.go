package auth

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/util"
	"github.com/google/uuid"
)

type ProfileLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

func NewProfileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProfileLogic {
	return &ProfileLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.auth.profile"),
	}
}

func (l *ProfileLogic) Profile(userID uuid.UUID, input *request.ProfileRequest) (*response.UserResponse, error) {
	user, err := l.svcCtx.UserRepo.FindByID(l.ctx, userID)
	if err != nil {
		return nil, err
	}
	if input.Nickname != nil {
		user.Nickname = util.NormalizeOptional(input.Nickname, false)
	}
	if input.Email != nil {
		user.Email = util.NormalizeOptional(input.Email, true)
	}

	user, err = l.svcCtx.UserRepo.Update(l.ctx, user)
	if err != nil {
		return nil, err
	}
	return &response.UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		Nickname:  user.Nickname,
		Email:     user.Email,
		Avatar:    user.Avatar,
		CreatedAt: user.CreatedAt,
	}, nil
}
