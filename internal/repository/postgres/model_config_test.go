package postgres_test

import (
	"context"
	"testing"

	"github.com/boxify/api-go/internal/models"
	repositorypostgres "github.com/boxify/api-go/internal/repository/postgres"
	"github.com/google/uuid"
)

func TestModelConfigRepositoryIntegrationWhenPostgresEnvIsConfigured(t *testing.T) {
	db := newAuthTestDB(t)
	ctx := context.Background()
	userRepo := repositorypostgres.NewUserRepository(db)
	repo := repositorypostgres.NewModelConfigRepository(db)
	username := "model-config-" + uuid.NewString()
	user, err := userRepo.Create(ctx, &models.User{
		ID:           uuid.New(),
		Username:     username,
		PasswordHash: "hash",
	})
	if err != nil {
		t.Fatalf("Create user error = %v", err)
	}
	t.Cleanup(func() {
		db.WithContext(context.Background()).Exec("DELETE FROM model_configs WHERE user_id = ?", user.ID)
		db.WithContext(context.Background()).Exec("DELETE FROM users WHERE id = ?", user.ID)
	})

	first, err := repo.Create(ctx, &models.ModelConfig{
		ID:              uuid.New(),
		UserID:          user.ID,
		Type:            "chat",
		Provider:        "deepseek",
		Name:            "DeepSeek Chat",
		ModelName:       "deepseek-chat",
		APIKeyEncrypted: "encrypted-1",
		BaseURL:         "https://api.deepseek.com",
		Capability:      models.StringList{"function_call"},
		IsDefault:       true,
	})
	if err != nil {
		t.Fatalf("Create first error = %v", err)
	}
	second, err := repo.Create(ctx, &models.ModelConfig{
		ID:              uuid.New(),
		UserID:          user.ID,
		Type:            "chat",
		Provider:        "openai",
		Name:            "GPT",
		ModelName:       "gpt-4o",
		APIKeyEncrypted: "encrypted-2",
		BaseURL:         "https://api.openai.com/v1",
		Capability:      models.StringList{"vision"},
		IsDefault:       true,
	})
	if err != nil {
		t.Fatalf("Create second error = %v", err)
	}

	rows, err := repo.List(ctx, user.ID, nil)
	if err != nil {
		t.Fatalf("List error = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows len = %d, want 2", len(rows))
	}
	if rows[0].ID != second.ID {
		t.Fatalf("first row id = %s, want newest second %s", rows[0].ID, second.ID)
	}
	if len(rows[0].Capability) != 1 || rows[0].Capability[0] != "vision" {
		t.Fatalf("capability = %#v", rows[0].Capability)
	}
	for _, row := range rows {
		if row.ID == first.ID && row.IsDefault {
			t.Fatalf("first row remained default: %+v", row)
		}
	}
}
