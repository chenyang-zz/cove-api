package auth

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	httpresponse "github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/xerr"
)

type RefreshLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

func NewRefreshLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RefreshLogic {
	return &RefreshLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.auth.refresh"),
	}
}

func (l *RefreshLogic) Refresh(input *request.RefreshRequest) (*httpresponse.AuthResponse, error) {
	raw := strings.TrimSpace(input.RefreshToken)
	if raw == "" {
		return nil, xerr.BadRequest("刷新令牌不能为空")
	}
	token, err := l.svcCtx.RefreshTokenRepo.FindByHash(l.ctx, security.HashRefreshToken(raw))
	if err != nil {
		return nil, xerr.InvalidToken()
	}
	now := time.Now()
	if token.RevokedAt != nil || !token.ExpiresAt.After(now) {
		return nil, xerr.InvalidToken()
	}
	user, err := l.svcCtx.UserRepo.FindByID(l.ctx, token.UserID)
	if err != nil {
		return nil, xerr.InvalidToken()
	}
	if err := l.svcCtx.RefreshTokenRepo.Revoke(l.ctx, token.ID, now); err != nil {
		return nil, err
	}
	return issueAuthResponse(l.ctx, l.svcCtx, user)
}
