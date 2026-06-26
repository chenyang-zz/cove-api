package auth

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

type PasswordLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

func NewPasswordLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PasswordLogic {
	return &PasswordLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.auth.password"),
	}
}

func (l *PasswordLogic) Password(userID uuid.UUID, input *request.PasswordRequest) error {
	user, err := l.svcCtx.UserRepo.FindByID(l.ctx, userID)
	if err != nil {
		return err
	}

	if !security.CheckPassword(user.PasswordHash, input.OldPassword) {
		return xerr.Wrapf(err, "原密码错误")
	}

	if input.OldPassword == input.NewPassword {
		return xerr.BadRequestf("原密码与新密码不能相同")
	}

	hash, err := security.HashPassword(input.NewPassword)
	if err != nil {
		return xerr.Wrap(err, "密码处理失败")
	}

	user.PasswordHash = hash
	_, err = l.svcCtx.UserRepo.Update(l.ctx, user)
	if err != nil {
		return err
	}

	return nil
}
