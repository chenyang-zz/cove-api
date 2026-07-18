package postgres

import (
	"context"
	"errors"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type KnowledgeBaseRepository struct {
	db *gorm.DB
}

func NewKnowledgeBaseRepository(db *gorm.DB) repository.KnowledgeBaseRepository {
	return &KnowledgeBaseRepository{db: db}
}

func (r *KnowledgeBaseRepository) Create(ctx context.Context, userID uuid.UUID, knowledgeBase *models.KnowledgeBase) (*models.KnowledgeBase, error) {
	knowledgeBase.UserID = userID
	if err := r.db.WithContext(ctx).Create(knowledgeBase).Error; err != nil {
		return nil, xerr.Wrapf(err, "创建知识库失败")
	}
	return knowledgeBase, nil
}

func (r *KnowledgeBaseRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.KnowledgeBase, error) {
	var rows []*models.KnowledgeBase

	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("updated_at DESC").
		Find(&rows).Error
	if err != nil {
		return nil, xerr.Wrapf(err, "查询知识库列表失败")
	}

	return rows, nil
}

func (r *KnowledgeBaseRepository) FindDefault(ctx context.Context, userID uuid.UUID) (*models.KnowledgeBase, error) {
	knowledgeBase := &models.KnowledgeBase{}
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND is_default = ?", userID, true).
		First(knowledgeBase).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, xerr.NotFound("默认知识库不存在")
	}
	if err != nil {
		return nil, xerr.Wrapf(err, "查询默认知识库失败")
	}
	return knowledgeBase, nil
}

func (r *KnowledgeBaseRepository) FindByID(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID) (*models.KnowledgeBase, error) {
	knowledgeBase := &models.KnowledgeBase{}
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", knowledgeBaseID, userID).
		First(knowledgeBase).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, xerr.NotFound("知识库不存在")
	}
	if err != nil {
		return nil, xerr.Wrapf(err, "查询知识库失败")
	}
	return knowledgeBase, nil
}

// SetDefault 原子地将指定知识库设为当前用户的唯一默认知识库。
func (r *KnowledgeBaseRepository) SetDefault(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID) (*models.KnowledgeBase, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 锁定用户行，将同一用户的并发默认项切换串行化。
		user := &models.User{}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").
			Where("id = ?", userID).
			First(user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return xerr.NotFound("用户不存在")
			}
			return xerr.Wrapf(err, "锁定用户默认知识库失败")
		}

		target := &models.KnowledgeBase{}
		if err := tx.Where("id = ? AND user_id = ?", knowledgeBaseID, userID).First(target).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return xerr.NotFound("知识库不存在")
			}
			return xerr.Wrapf(err, "查询知识库失败")
		}

		// 先清除其他默认项，再设置目标项，保证事务提交后只有一个默认知识库。
		if err := tx.Model(&models.KnowledgeBase{}).
			Where("user_id = ? AND id <> ? AND is_default = ?", userID, knowledgeBaseID, true).
			Update("is_default", false).Error; err != nil {
			return xerr.Wrapf(err, "清除默认知识库失败")
		}
		if err := tx.Model(&models.KnowledgeBase{}).
			Where("id = ? AND user_id = ?", knowledgeBaseID, userID).
			Update("is_default", true).Error; err != nil {
			return xerr.Wrapf(err, "设置默认知识库失败")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.FindByID(ctx, userID, knowledgeBaseID)
}

func (r *KnowledgeBaseRepository) Update(ctx context.Context, userID uuid.UUID, knowledgeBase *models.KnowledgeBase) (*models.KnowledgeBase, error) {
	result := r.db.WithContext(ctx).
		Model(&models.KnowledgeBase{}).
		Where("id = ? AND user_id = ?", knowledgeBase.ID, userID).
		Omit("id", "user_id", "user", "created_at", "updated_at").
		Updates(knowledgeBase)
	if result.Error != nil {
		return nil, xerr.Wrapf(result.Error, "更新知识库失败")
	}
	if result.RowsAffected == 0 {
		return nil, xerr.NotFound("知识库不存在")
	}
	return r.FindByID(ctx, userID, knowledgeBase.ID)
}

func (r *KnowledgeBaseRepository) UpdateFields(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID, knowledgeBase *models.KnowledgeBase, fields *repository.KnowledgeBaseUpdateFields) (*models.KnowledgeBase, error) {
	columns := fields.Columns()
	if len(columns) == 0 {
		return nil, xerr.BadRequest("更新字段不能为空")
	}
	result := r.db.WithContext(ctx).
		Model(&models.KnowledgeBase{}).
		Where("id = ? AND user_id = ?", knowledgeBaseID, userID).
		Select(columns).
		Updates(knowledgeBase)
	if result.Error != nil {
		return nil, xerr.Wrapf(result.Error, "更新知识库失败")
	}
	if result.RowsAffected == 0 {
		return nil, xerr.NotFound("知识库不存在")
	}
	return r.FindByID(ctx, userID, knowledgeBaseID)
}

func (r *KnowledgeBaseRepository) Delete(ctx context.Context, userID uuid.UUID, knowledgeBaseID uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", knowledgeBaseID, userID).
		Delete(&models.KnowledgeBase{})
	if result.Error != nil {
		return xerr.Wrapf(result.Error, "删除知识库失败")
	}
	if result.RowsAffected == 0 {
		return xerr.NotFound("知识库不存在")
	}
	return nil
}
