package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/boxify/api-go/internal/infrastructure/db/migration"
	dbpostgres "github.com/boxify/api-go/internal/infrastructure/db/postgres"
	"github.com/boxify/api-go/internal/models"
	repositorypostgres "github.com/boxify/api-go/internal/repository/postgres"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestUserRepositoryIntegrationWhenPostgresEnvIsConfigured(t *testing.T) {
	db := newAuthTestDB(t)
	ctx := context.Background()
	repo := repositorypostgres.NewUserRepository(db)
	email := "alice-" + uuid.NewString() + "@example.com"
	username := "alice-" + uuid.NewString()

	created, err := repo.Create(ctx, &models.User{
		ID:           uuid.New(),
		Username:     username,
		Email:        &email,
		PasswordHash: "hash",
	})
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	t.Cleanup(func() {
		db.WithContext(context.Background()).Exec("DELETE FROM refresh_tokens WHERE user_id = ?", created.ID)
		db.WithContext(context.Background()).Exec("DELETE FROM users WHERE id = ?", created.ID)
	})

	for _, login := range []string{username, email} {
		found, err := repo.FindByLogin(ctx, login)
		if err != nil {
			t.Fatalf("FindByLogin(%q) error = %v", login, err)
		}
		if found.ID != created.ID || found.Username != username {
			t.Fatalf("FindByLogin(%q) = %+v, want created user", login, found)
		}
	}

	_, err = repo.Create(ctx, &models.User{
		ID:           uuid.New(),
		Username:     username,
		PasswordHash: "hash",
	})
	if xerr.From(err).Kind != xerr.KindConflict {
		t.Fatalf("duplicate create error = %v, want conflict", err)
	}
}

func TestRefreshTokenRepositoryIntegrationWhenPostgresEnvIsConfigured(t *testing.T) {
	db := newAuthTestDB(t)
	ctx := context.Background()
	userRepo := repositorypostgres.NewUserRepository(db)
	tokenRepo := repositorypostgres.NewRefreshTokenRepository(db)
	username := "token-" + uuid.NewString()
	user, err := userRepo.Create(ctx, &models.User{
		ID:           uuid.New(),
		Username:     username,
		PasswordHash: "hash",
	})
	if err != nil {
		t.Fatalf("Create user error = %v", err)
	}
	t.Cleanup(func() {
		db.WithContext(context.Background()).Exec("DELETE FROM refresh_tokens WHERE user_id = ?", user.ID)
		db.WithContext(context.Background()).Exec("DELETE FROM users WHERE id = ?", user.ID)
	})

	tokenHash := "hash-" + uuid.NewString()
	created, err := tokenRepo.Create(ctx, &models.RefreshToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("Create token error = %v", err)
	}

	found, err := tokenRepo.FindByHash(ctx, tokenHash)
	if err != nil {
		t.Fatalf("FindByHash error = %v", err)
	}
	if found.ID != created.ID || found.UserID != user.ID {
		t.Fatalf("FindByHash = %+v, want created token", found)
	}

	revokedAt := time.Now()
	if err := tokenRepo.Revoke(ctx, created.ID, revokedAt); err != nil {
		t.Fatalf("Revoke error = %v", err)
	}
	revoked, err := tokenRepo.FindByHash(ctx, tokenHash)
	if err != nil {
		t.Fatalf("FindByHash revoked error = %v", err)
	}
	if revoked.RevokedAt == nil {
		t.Fatal("revoked token has nil RevokedAt")
	}
	if err := tokenRepo.Revoke(ctx, created.ID, time.Now()); xerr.From(err).Kind != xerr.KindUnauthorized {
		t.Fatalf("second revoke error = %v, want unauthorized", err)
	}
}

func newAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	url := os.Getenv("POSTGRES_AUTH_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_AUTH_TEST_URL is required")
	}
	runner, err := migration.NewRunner(migration.Config{DatabaseURL: url})
	if err != nil {
		t.Fatalf("NewRunner error = %v", err)
	}
	if err := runner.Up(context.Background()); err != nil {
		t.Fatalf("migration Up error = %v", err)
	}
	if err := runner.Close(); err != nil {
		t.Fatalf("migration Close error = %v", err)
	}
	db, err := dbpostgres.NewGormDB(context.Background(), dbpostgres.Config{URL: url})
	if err != nil {
		t.Fatalf("NewGormDB error = %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("DB error = %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Fatalf("Close DB error = %v", err)
		}
	})
	return db
}
