package repository

import (
	"context"
	"time"

	"github.com/boxify/api-go/internal/models"
	"github.com/google/uuid"
)

type RefreshTokenRepository interface {
	Create(ctx context.Context, token *models.RefreshToken) (*models.RefreshToken, error)
	FindByHash(ctx context.Context, hash string) (*models.RefreshToken, error)
	Revoke(ctx context.Context, id uuid.UUID, revokedAt time.Time) error
}
