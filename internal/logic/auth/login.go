package auth

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	httpresponse "github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/util"
	"github.com/boxify/api-go/internal/xerr"
)

type LoginLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

func NewLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogic {
	return &LoginLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.auth.login"),
	}
}

func (l *LoginLogic) Login(input *request.LoginRequest) (*httpresponse.AuthResponse, error) {
	login := util.NormalizeRequired(input.Login)
	if login == "" || len(input.Password) < 6 {
		return nil, xerr.BadRequest("账号或密码格式错误")
	}
	user, err := l.svcCtx.UserRepo.FindByLogin(l.ctx, login)
	if err != nil {
		return nil, xerr.InvalidCredential()
	}
	if !security.CheckPassword(user.PasswordHash, input.Password) {
		return nil, xerr.InvalidCredential()
	}
	return issueAuthResponse(l.ctx, l.svcCtx, user)
}
