/**
 * @Time   : 2026/6/26 16:47
 * @Author : chenyangzhao542@gmail.com
 * @File   : me.go.go
 **/

package auth

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	httpresponse "github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/util"
)

type MeLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

func NewMeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MeLogic {
	return &MeLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.auth.me"),
	}
}

func (l *MeLogic) Me() (*httpresponse.UserResponse, error) {
	userID, err := util.UserIDFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	user, err := l.svcCtx.UserRepo.FindByID(l.ctx, userID)
	if err != nil {
		return nil, err
	}
	return &httpresponse.UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		Nickname:  user.Nickname,
		Email:     user.Email,
		Avatar:    user.Avatar,
		CreatedAt: user.CreatedAt,
	}, nil
}
