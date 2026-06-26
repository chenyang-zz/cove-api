package svc

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/boxify/api-go/internal/config"
	dbneo4j "github.com/boxify/api-go/internal/infrastructure/db/neo4j"
	dbpostgres "github.com/boxify/api-go/internal/infrastructure/db/postgres"
	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/repository/graph"
	repositorypostgres "github.com/boxify/api-go/internal/repository/postgres"
	"github.com/boxify/api-go/internal/xerr"
	"gorm.io/gorm"
)

type ServiceContext struct {
	Config config.Config

	GormDB *gorm.DB
	Neo4j  *dbneo4j.Client

	UserRepo         repository.UserRepository
	RefreshTokenRepo repository.RefreshTokenRepository
	MemoryGraphRepo  repository.MemoryGraphRepository
	ModelConfigRepo  repository.ModelConfigRepository

	SecretCipher *security.SecretCipher
	TokenIssuer  *security.TokenIssuer

	closeOnce sync.Once
	closeErr  error
}

func New(ctx context.Context, cfg config.Config) (*ServiceContext, error) {
	cipher, err := security.NewSecretCipher(cfg.SecretKey)
	if err != nil {
		return nil, xerr.Wrapf(err, "创建密钥加密器失败")
	}
	accessTokenTTL, err := time.ParseDuration(cfg.JWT.AccessTokenTTL)
	if err != nil {
		return nil, xerr.BadRequest("JWT access token TTL 配置无效")
	}

	db, err := dbpostgres.NewGormDB(ctx, dbpostgres.Config{URL: cfg.Database.URL})
	if err != nil {
		return nil, err
	}

	userRepo := repositorypostgres.NewUserRepository(db)
	refreshTokenRepo := repositorypostgres.NewRefreshTokenRepository(db)
	modelConfigRepo := repositorypostgres.NewModelConfigRepository(db)

	svcCtx := &ServiceContext{
		Config: cfg,

		GormDB: db,

		UserRepo:         userRepo,
		RefreshTokenRepo: refreshTokenRepo,
		ModelConfigRepo:  modelConfigRepo,

		SecretCipher: cipher,
		TokenIssuer:  security.NewTokenIssuer(cfg.JWT.Secret, accessTokenTTL),
	}

	if shouldInitNeo4j(cfg.Neo4j) {
		neo4jClient, err := dbneo4j.NewClient(ctx, dbneo4j.Config{
			URI:      cfg.Neo4j.URI,
			Username: cfg.Neo4j.Username,
			Password: cfg.Neo4j.Password,
			Database: cfg.Neo4j.Database,
		})
		if err != nil {
			_ = svcCtx.Close(ctx)
			return nil, err
		}
		svcCtx.Neo4j = neo4jClient
		svcCtx.MemoryGraphRepo = graph.NewMemoryGraphRepository(neo4jClient)
	}

	return svcCtx, nil
}

func (s *ServiceContext) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		var errs []error
		if s.Neo4j != nil {
			if err := s.Neo4j.Close(ctx); err != nil {
				errs = append(errs, err)
			}
		}
		if s.GormDB != nil {
			sqlDB, err := s.GormDB.DB()
			if err != nil {
				errs = append(errs, xerr.Wrapf(err, "获取 Postgres 底层连接失败"))
			} else if err := sqlDB.Close(); err != nil {
				errs = append(errs, xerr.Wrapf(err, "关闭 Postgres 连接失败"))
			}
		}
		s.closeErr = errors.Join(errs...)
	})
	return s.closeErr
}

func shouldInitNeo4j(cfg config.Neo4jConfig) bool {
	return cfg.URI != "" && cfg.Username != "" && cfg.Password != ""
}
