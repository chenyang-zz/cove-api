package auth

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	httpresponse "github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/util"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

type RegisterLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

func NewRegisterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RegisterLogic {
	return &RegisterLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.auth.register"),
	}
}

func (l *RegisterLogic) Register(input *request.RegisterRequest) (*httpresponse.AuthResponse, error) {
	username := util.NormalizeRequired(input.Username)
	email := util.NormalizeOptional(input.Email, true)
	nickname := util.NormalizeOptional(input.Nickname, false)
	avatar := util.NormalizeOptional(input.Avatar, false)
	if username == "" || len(input.Password) < 6 {
		return nil, xerr.BadRequest("用户名或密码格式错误")
	}
	hash, err := security.HashPassword(input.Password)
	if err != nil {
		return nil, xerr.Internal("密码处理失败", err)
	}
	user, err := l.svcCtx.UserRepo.Create(l.ctx, &models.User{
		ID:           uuid.New(),
		Username:     username,
		Nickname:     nickname,
		Email:        email,
		Avatar:       avatar,
		PasswordHash: hash,
	})
	if err != nil {
		return nil, err
	}
	return issueAuthResponse(l.ctx, l.svcCtx, user)
}
