package repository

import (
	"context"

	"github.com/boxify/api-go/internal/models"
	"github.com/google/uuid"
)

type UserRepository interface {
	Create(ctx context.Context, user *models.User) (*models.User, error)
	Update(ctx context.Context, user *models.User) (*models.User, error)
	FindByLogin(ctx context.Context, login string) (*models.User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*models.User, error)
}
