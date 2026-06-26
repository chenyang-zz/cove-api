package postgres

import (
	"context"

	"github.com/boxify/api-go/internal/domain"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ModelConfigRepository struct {
	db *gorm.DB
}

func NewModelConfigRepository(db *gorm.DB) repository.ModelConfigRepository {
	return &ModelConfigRepository{db: db}
}

func (r *ModelConfigRepository) Create(ctx context.Context, row *models.ModelConfig) (*models.ModelConfig, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if row.IsDefault {
			if err := tx.Model(&models.ModelConfig{}).
				Where("user_id = ? AND type = ?", row.UserID, row.Type).
				Update("is_default", false).Error; err != nil {
				return xerr.Wrapf(err, "更新默认模型配置失败")
			}
		}
		if err := tx.Create(&row).Error; err != nil {
			if isUniqueViolation(err) {
				return xerr.Wrap(err, "当前模型已存在")
			}
			return xerr.Wrapf(err, "创建模型配置失败")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (r *ModelConfigRepository) List(ctx context.Context, userID uuid.UUID, modelType *domain.ModelType) ([]*models.ModelConfig, error) {
	var rows []*models.ModelConfig

	query := r.db.WithContext(ctx).
		Where("user_id = ?", userID)
	if modelType != nil {
		query = query.Where("type = ?", *modelType)
	}
	if err := query.Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil, xerr.Wrapf(err, "查询模型配置失败")
	}
	return rows, nil
}
