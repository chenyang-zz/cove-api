package migration

import (
	"context"
	"database/sql"
	"strings"

	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/xerr"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Config struct {
	DatabaseURL string
}

type Runner struct {
	db *gorm.DB
}

func NewRunner(cfg Config) (*Runner, error) {
	databaseURL := strings.TrimSpace(cfg.DatabaseURL)
	if databaseURL == "" {
		return nil, xerr.BadRequest("Postgres 连接 URL 不能为空")
	}
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		return nil, xerr.Wrapf(err, "打开 GORM Postgres 连接失败")
	}
	return &Runner{db: db}, nil
}

func (r *Runner) Up(ctx context.Context) error {
	if r == nil || r.db == nil {
		return xerr.BadRequest("GORM migration runner 未初始化")
	}
	if err := r.db.WithContext(ctx).AutoMigrate(&models.User{}, &models.RefreshToken{}, &models.ModelConfig{}); err != nil {
		return xerr.Wrapf(err, "执行 GORM migration 失败")
	}
	return nil
}

func (r *Runner) DB() (*sql.DB, error) {
	if r == nil || r.db == nil {
		return nil, xerr.BadRequest("GORM migration runner 未初始化")
	}
	db, err := r.db.DB()
	if err != nil {
		return nil, xerr.Wrapf(err, "获取 GORM 底层数据库连接失败")
	}
	return db, nil
}

func (r *Runner) Close() error {
	db, err := r.DB()
	if err != nil {
		return err
	}
	if err := db.Close(); err != nil {
		return xerr.Wrapf(err, "关闭 GORM Postgres 连接失败")
	}
	return nil
}
