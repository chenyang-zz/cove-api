package postgres

import (
	"context"
	"errors"

	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) repository.UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user *models.User) (*models.User, error) {
	if err := r.db.WithContext(ctx).Create(user).Error; err != nil {
		if isUniqueViolation(err) {
			return nil, xerr.UserExists()
		}
		return nil, xerr.Wrapf(err, "创建用户失败")
	}
	return user, nil
}

func (r *UserRepository) FindByLogin(ctx context.Context, login string) (*models.User, error) {
	user := &models.User{}
	err := r.db.WithContext(ctx).
		Where("username = ? OR email = ?", login, login).
		First(user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, xerr.NotFound("用户不存在")
	}
	if err != nil {
		return nil, xerr.Wrapf(err, "查询用户失败")
	}
	return user, nil
}

func (r *UserRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	user := &models.User{}
	err := r.db.WithContext(ctx).Where("id = ?", id).First(user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, xerr.NotFound("用户不存在")
	}
	if err != nil {
		return nil, xerr.Wrapf(err, "查询用户失败")
	}
	return user, nil
}

func (r *UserRepository) Update(ctx context.Context, user *models.User) (*models.User, error) {
	if err := r.db.WithContext(ctx).Save(user).Error; err != nil {
		return nil, xerr.Wrapf(err, "更新用户信息失败")
	}
	return user, nil
}
