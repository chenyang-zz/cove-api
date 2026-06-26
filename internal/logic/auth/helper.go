package auth

import (
	"context"
	"time"

	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/svc"
	httpresponse "github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

const refreshTokenTTL = 30 * 24 * time.Hour

func issueAuthResponse(ctx context.Context, svcCtx *svc.ServiceContext, user *models.User) (*httpresponse.AuthResponse, error) {
	accessToken, err := svcCtx.TokenIssuer.IssueAccessToken(user.ID)
	if err != nil {
		return nil, xerr.Internal("令牌签发失败", err)
	}
	refreshToken, err := security.GenerateRefreshToken()
	if err != nil {
		return nil, xerr.Internal("刷新令牌生成失败", err)
	}
	if _, err := svcCtx.RefreshTokenRepo.Create(ctx, &models.RefreshToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: security.HashRefreshToken(refreshToken),
		ExpiresAt: time.Now().Add(refreshTokenTTL),
	}); err != nil {
		return nil, err
	}
	return &httpresponse.AuthResponse{
		UserID:       user.ID,
		Username:     user.Username,
		Email:        user.Email,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}
