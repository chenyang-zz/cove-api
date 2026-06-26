package repository

import (
	"context"

	"github.com/boxify/api-go/internal/domain"
	"github.com/boxify/api-go/internal/models"
	"github.com/google/uuid"
)

type ModelConfigRepository interface {
	Create(ctx context.Context, row *models.ModelConfig) (*models.ModelConfig, error)
	List(ctx context.Context, userID uuid.UUID, modelType *domain.ModelType) ([]*models.ModelConfig, error)
}
