package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RefreshTokenRepository struct {
	db *gorm.DB
}

func NewRefreshTokenRepository(db *gorm.DB) repository.RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

func (r *RefreshTokenRepository) Create(ctx context.Context, token *models.RefreshToken) (*models.RefreshToken, error) {
	if err := r.db.WithContext(ctx).Create(token).Error; err != nil {
		if isUniqueViolation(err) {
			return nil, xerr.InvalidToken()
		}
		return nil, xerr.Wrapf(err, "创建刷新令牌失败")
	}
	return token, nil
}

func (r *RefreshTokenRepository) FindByHash(ctx context.Context, hash string) (*models.RefreshToken, error) {
	token := &models.RefreshToken{}
	err := r.db.WithContext(ctx).Where("token_hash = ?", hash).First(token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, xerr.InvalidToken()
	}
	if err != nil {
		return nil, xerr.Wrapf(err, "查询刷新令牌失败")
	}
	return token, nil
}

func (r *RefreshTokenRepository) Revoke(ctx context.Context, id uuid.UUID, revokedAt time.Time) error {
	result := r.db.WithContext(ctx).
		Model(&models.RefreshToken{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Update("revoked_at", revokedAt)
	if result.Error != nil {
		return xerr.Wrapf(result.Error, "撤销刷新令牌失败")
	}
	if result.RowsAffected == 0 {
		return xerr.InvalidToken()
	}
	return nil
}
