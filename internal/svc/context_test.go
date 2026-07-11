package svc_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/boxify/api-go/internal/config"
	"github.com/boxify/api-go/internal/domain/types"
	"github.com/boxify/api-go/internal/infrastructure/db/migration"
	dbpostgres "github.com/boxify/api-go/internal/infrastructure/db/postgres"
	infraredis "github.com/boxify/api-go/internal/infrastructure/db/redis"
	"github.com/boxify/api-go/internal/infrastructure/queue"
	"github.com/boxify/api-go/internal/infrastructure/storage"
	"github.com/boxify/api-go/internal/models"
	repositorypostgres "github.com/boxify/api-go/internal/repository/postgres"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestNewReturnsErrorForInvalidPostgresURL(t *testing.T) {
	cfg := config.Config{}
	cfg.Database.URL = "   "
	cfg.JWT.Secret = "test-secret"
	cfg.JWT.AccessTokenTTL = "168h"
	cfg.SecretKey = "0123456789abcdef0123456789abcdef"

	if _, err := svc.New(context.Background(), cfg); err == nil {
		t.Fatal("svc.New error = nil, want error")
	}
}

func TestNewReturnsErrorForInvalidAccessTokenTTL(t *testing.T) {
	cfg := config.Config{}
	cfg.Database.URL = "   "
	cfg.JWT.Secret = "test-secret"
	cfg.JWT.AccessTokenTTL = "not-a-duration"
	cfg.SecretKey = "0123456789abcdef0123456789abcdef"

	_, err := svc.New(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "JWT access token TTL 配置无效") {
		t.Fatalf("svc.New error = %v, want invalid ttl error", err)
	}
}

// TestNewReturnsErrorForInvalidMCPDiscoverTimeout 验证 MCP 时长配置非法时启动失败。
func TestNewReturnsErrorForInvalidMCPDiscoverTimeout(t *testing.T) {
	cfg := config.Config{}
	cfg.Database.URL = "   "
	cfg.JWT.Secret = "test-secret"
	cfg.JWT.AccessTokenTTL = "168h"
	cfg.SecretKey = "0123456789abcdef0123456789abcdef"
	cfg.MCP.DiscoverTimeout = "not-a-duration"

	_, err := svc.New(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "MCP discover_timeout 配置无效") {
		t.Fatalf("svc.New error = %v, want invalid mcp discover_timeout error", err)
	}
}

func TestCloseCanBeCalledRepeatedly(t *testing.T) {
	svcCtx := &svc.ServiceContext{Storage: storage.NewLocalStore(t.TempDir())}
	ctx := context.Background()

	if err := svcCtx.Close(ctx); err != nil {
		t.Fatalf("first Close error = %v", err)
	}
	if err := svcCtx.Close(ctx); err != nil {
		t.Fatalf("second Close error = %v", err)
	}
}

func TestServiceContextCanHoldLocalStorage(t *testing.T) {
	svcCtx := &svc.ServiceContext{Storage: storage.NewLocalStore(t.TempDir())}

	if svcCtx.Storage == nil {
		t.Fatal("storage = nil, want local store")
	}
}

func TestBuildStorageReturnsErrorForIncompleteCOSConfig(t *testing.T) {
	_, _, err := svc.BuildStorage(config.StorageConfig{Backend: "cos"})
	if err == nil || !strings.Contains(err.Error(), "COS 存储配置无效") {
		t.Fatalf("BuildStorage error = %v, want cos config error", err)
	}
}

func TestBuildStorageReturnsErrorForUnknownBackend(t *testing.T) {
	_, _, err := svc.BuildStorage(config.StorageConfig{Backend: "unknown"})
	if err == nil || !strings.Contains(err.Error(), "存储 backend 配置无效") {
		t.Fatalf("BuildStorage error = %v, want backend error", err)
	}
}

func TestBuildStorageReturnsLocalStoreByDefault(t *testing.T) {
	store, signer, err := svc.BuildStorage(config.StorageConfig{Backend: "local", Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("BuildStorage error = %v", err)
	}
	if store == nil {
		t.Fatal("store = nil")
	}
	if signer != nil {
		t.Fatalf("signer = %T, want nil for local storage", signer)
	}
}

func TestBuildRealtimeReturnsRedisBrokerForRedisClient(t *testing.T) {
	redisClient, err := infraredis.NewClient(context.Background(), infraredis.Config{Addr: "localhost:6379"})
	if err != nil {
		t.Fatalf("NewClient error = %v", err)
	}
	defer redisClient.Close()

	broker := svc.BuildRealtime(redisClient)
	if broker == nil {
		t.Fatal("BuildRealtime returned nil broker")
	}
}

func TestWithTxRequiresGormDB(t *testing.T) {
	ctx := context.Background()

	var nilSvc *svc.ServiceContext
	if err := nilSvc.WithTx(ctx, func(*svc.ServiceContext) error { return nil }); xerr.From(err).Kind != xerr.KindInternal {
		t.Fatalf("nil ServiceContext WithTx error = %v, want internal", err)
	}

	if err := (&svc.ServiceContext{}).WithTx(ctx, func(*svc.ServiceContext) error { return nil }); xerr.From(err).Kind != xerr.KindInternal {
		t.Fatalf("nil GormDB WithTx error = %v, want internal", err)
	}
}

// TestWithTxRollsBackAndCommitsWhenPostgresEnvIsConfigured 验证事务上下文会重新绑定所有 Postgres 仓储并正确提交或回滚。
func TestWithTxRollsBackAndCommitsWhenPostgresEnvIsConfigured(t *testing.T) {
	db := newSvcTxTestDB(t)
	ctx := context.Background()
	svcCtx := &svc.ServiceContext{GormDB: db}
	userRepo := repositorypostgres.NewUserRepository(db)

	rollbackUsername := "tx-rollback-" + uuid.NewString()
	rollbackErr := errors.New("rollback")
	err := svcCtx.WithTx(ctx, func(txSvc *svc.ServiceContext) error {
		if txSvc.GormDB == nil || txSvc.UserRepo == nil || txSvc.ConversationRepo == nil || txSvc.SkillRepo == nil || txSvc.ToolConfigRepo == nil {
			return errors.New("transaction repositories were not rebound")
		}
		user, err := txSvc.UserRepo.Create(ctx, &models.User{
			Username:     rollbackUsername,
			PasswordHash: "hash",
		})
		if err != nil {
			return err
		}
		if _, err := txSvc.ConversationRepo.Create(ctx, user.ID, &models.Conversation{Title: "rollback"}); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("WithTx rollback error = %v, want %v", err, rollbackErr)
	}
	if _, err := userRepo.FindByLogin(ctx, rollbackUsername); xerr.From(err).Kind != xerr.KindNotFound {
		t.Fatalf("FindByLogin after rollback error = %v, want not found", err)
	}

	commitUsername := "tx-commit-" + uuid.NewString()
	var committedUserID uuid.UUID
	err = svcCtx.WithTx(ctx, func(txSvc *svc.ServiceContext) error {
		if txSvc.GormDB == nil || txSvc.UserRepo == nil || txSvc.ConversationRepo == nil || txSvc.SkillRepo == nil || txSvc.ToolConfigRepo == nil {
			return errors.New("transaction repositories were not rebound")
		}
		user, err := txSvc.UserRepo.Create(ctx, &models.User{
			Username:     commitUsername,
			PasswordHash: "hash",
		})
		if err != nil {
			return err
		}
		committedUserID = user.ID
		_, err = txSvc.ConversationRepo.Create(ctx, user.ID, &models.Conversation{Title: "commit"})
		return err
	})
	if err != nil {
		t.Fatalf("WithTx commit error = %v", err)
	}
	t.Cleanup(func() {
		db.WithContext(context.Background()).Exec("DELETE FROM conversations WHERE user_id = ?", committedUserID)
		db.WithContext(context.Background()).Exec("DELETE FROM users WHERE id = ?", committedUserID)
	})
	if _, err := userRepo.FindByLogin(ctx, commitUsername); err != nil {
		t.Fatalf("FindByLogin after commit error = %v", err)
	}
}

func TestCloseReturnsStorageCloseError(t *testing.T) {
	want := errors.New("close storage")
	svcCtx := &svc.ServiceContext{Storage: closeStore{err: want}}

	if err := svcCtx.Close(context.Background()); !errors.Is(err, want) {
		t.Fatalf("Close error = %v, want %v", err, want)
	}
}

func TestCloseClosesTaskProducer(t *testing.T) {
	// 验证 ServiceContext 关闭时会释放任务 producer，避免 Redis 连接泄漏。
	producer := &closeTaskProducer{}
	svcCtx := &svc.ServiceContext{TaskProducer: producer}

	if err := svcCtx.Close(context.Background()); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	if !producer.closed {
		t.Fatal("task producer was not closed")
	}
}

type closeStore struct {
	err error
}

func (s closeStore) Put(context.Context, string, []byte) error   { return nil }
func (s closeStore) Get(context.Context, string) ([]byte, error) { return nil, nil }
func (s closeStore) Delete(context.Context, string) error        { return nil }
func (s closeStore) Ping(context.Context) error                  { return nil }
func (s closeStore) Close() error                                { return s.err }

type closeTaskProducer struct {
	closed bool
}

func (p *closeTaskProducer) Enqueue(context.Context, *types.Task, ...queue.EnqueueOption) (*queue.TaskInfo, error) {
	return &queue.TaskInfo{ID: "task-id", Queue: types.QueueParse}, nil
}

func (p *closeTaskProducer) Close() error {
	p.closed = true
	return nil
}

func newSvcTxTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	url := os.Getenv("POSTGRES_AUTH_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_AUTH_TEST_URL is required")
	}
	runner, err := migration.NewRunner(migration.Config{DatabaseURL: url}, models.MigrationModels()...)
	if err != nil {
		t.Fatalf("NewRunner error = %v", err)
	}
	if err := runner.Up(context.Background()); err != nil {
		_ = runner.Close()
		t.Fatalf("migration Up error = %v", err)
	}
	if err := runner.Close(); err != nil {
		t.Fatalf("migration Close error = %v", err)
	}
	db, err := dbpostgres.NewGormDB(context.Background(), dbpostgres.Config{URL: url})
	if err != nil {
		t.Fatalf("NewGormDB error = %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatalf("db.DB error = %v", err)
		}
		if err := sqlDB.Close(); err != nil {
			t.Fatalf("db Close error = %v", err)
		}
	})
	return db
}
