package postgres_test

import (
	"context"
	"testing"

	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	repositorypostgres "github.com/boxify/api-go/internal/repository/postgres"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

func TestConversationRepositoryUserScopedOperationsWhenPostgresEnvIsConfigured(t *testing.T) {
	db := newAuthTestDB(t)
	ctx := context.Background()
	userRepo := repositorypostgres.NewUserRepository(db)
	conversationRepo := repositorypostgres.NewConversationRepository(db)

	userA, err := userRepo.Create(ctx, &models.User{
		Username:     "conversation-a-" + uuid.NewString(),
		PasswordHash: "hash",
	})
	if err != nil {
		t.Fatalf("Create userA error = %v", err)
	}
	userB, err := userRepo.Create(ctx, &models.User{
		Username:     "conversation-b-" + uuid.NewString(),
		PasswordHash: "hash",
	})
	if err != nil {
		t.Fatalf("Create userB error = %v", err)
	}
	t.Cleanup(func() {
		db.WithContext(context.Background()).Exec("DELETE FROM conversations WHERE user_id IN ?", []uuid.UUID{userA.ID, userB.ID})
		db.WithContext(context.Background()).Exec("DELETE FROM users WHERE id IN ?", []uuid.UUID{userA.ID, userB.ID})
	})

	created, err := conversationRepo.Create(ctx, userA.ID, &models.Conversation{Title: "private"})
	if err != nil {
		t.Fatalf("Create conversation error = %v", err)
	}
	if created.UserID != userA.ID {
		t.Fatalf("created userID = %s, want %s", created.UserID, userA.ID)
	}

	if _, err := conversationRepo.FindByID(ctx, userB.ID, created.ID); xerr.From(err).Kind != xerr.KindNotFound {
		t.Fatalf("FindByID as another user error = %v, want not found", err)
	}

	created.Title = "renamed by other user"
	if _, err := conversationRepo.Update(ctx, userB.ID, created); xerr.From(err).Kind != xerr.KindNotFound {
		t.Fatalf("Update as another user error = %v, want not found", err)
	}

	if err := conversationRepo.Delete(ctx, userB.ID, created.ID); xerr.From(err).Kind != xerr.KindNotFound {
		t.Fatalf("Delete as another user error = %v, want not found", err)
	}

	found, err := conversationRepo.FindByID(ctx, userA.ID, created.ID)
	if err != nil {
		t.Fatalf("FindByID as owner error = %v", err)
	}
	if found.Title != "private" {
		t.Fatalf("title after cross-user update = %q, want unchanged", found.Title)
	}

	found.Title = "renamed"
	updated, err := conversationRepo.Update(ctx, userA.ID, found)
	if err != nil {
		t.Fatalf("Update as owner error = %v", err)
	}
	if updated.Title != "renamed" {
		t.Fatalf("updated title = %q, want renamed", updated.Title)
	}

	if err := conversationRepo.Delete(ctx, userA.ID, updated.ID); err != nil {
		t.Fatalf("Delete as owner error = %v", err)
	}
	if _, err := conversationRepo.FindByID(ctx, userA.ID, updated.ID); xerr.From(err).Kind != xerr.KindNotFound {
		t.Fatalf("FindByID deleted error = %v, want not found", err)
	}
}

func TestConversationRepositoryUpdateFieldsWhenPostgresEnvIsConfigured(t *testing.T) {
	db := newAuthTestDB(t)
	ctx := context.Background()
	userRepo := repositorypostgres.NewUserRepository(db)
	conversationRepo := repositorypostgres.NewConversationRepository(db)

	userA, err := userRepo.Create(ctx, &models.User{
		Username:     "conversation-patch-a-" + uuid.NewString(),
		PasswordHash: "hash",
	})
	if err != nil {
		t.Fatalf("Create userA error = %v", err)
	}
	userB, err := userRepo.Create(ctx, &models.User{
		Username:     "conversation-patch-b-" + uuid.NewString(),
		PasswordHash: "hash",
	})
	if err != nil {
		t.Fatalf("Create userB error = %v", err)
	}
	t.Cleanup(func() {
		db.WithContext(context.Background()).Exec("DELETE FROM conversations WHERE user_id IN ?", []uuid.UUID{userA.ID, userB.ID})
		db.WithContext(context.Background()).Exec("DELETE FROM users WHERE id IN ?", []uuid.UUID{userA.ID, userB.ID})
	})

	created, err := conversationRepo.Create(ctx, userA.ID, &models.Conversation{
		Title:       "private",
		EnableTools: true,
		IsGroup:     true,
	})
	if err != nil {
		t.Fatalf("Create conversation error = %v", err)
	}

	patch := &models.Conversation{Title: "renamed", EnableTools: false, IsGroup: false}
	if _, err := conversationRepo.UpdateFields(ctx, userB.ID, created.ID, patch, repository.NewConversationUpdateFields().Title()); xerr.From(err).Kind != xerr.KindNotFound {
		t.Fatalf("UpdateFields as another user error = %v, want not found", err)
	}

	updated, err := conversationRepo.UpdateFields(ctx, userA.ID, created.ID, patch, repository.NewConversationUpdateFields().Title())
	if err != nil {
		t.Fatalf("UpdateFields as owner error = %v", err)
	}
	if updated.Title != "renamed" {
		t.Fatalf("updated title = %q, want renamed", updated.Title)
	}
	if !updated.EnableTools || !updated.IsGroup {
		t.Fatalf("unselected fields changed: enable_tools=%v is_group=%v", updated.EnableTools, updated.IsGroup)
	}
}

// TestConversationContextStateRepositoryUsesCASAndUserScope 验证摘要状态按用户隔离、使用乐观锁更新并随会话级联删除。
func TestConversationContextStateRepositoryUsesCASAndUserScope(t *testing.T) {
	db := newAuthTestDB(t)
	ctx := context.Background()
	userRepo := repositorypostgres.NewUserRepository(db)
	conversationRepo := repositorypostgres.NewConversationRepository(db)
	stateRepo := repositorypostgres.NewConversationContextStateRepository(db)

	owner, err := userRepo.Create(ctx, &models.User{Username: "context-owner-" + uuid.NewString(), PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("Create owner error = %v", err)
	}
	other, err := userRepo.Create(ctx, &models.User{Username: "context-other-" + uuid.NewString(), PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("Create other error = %v", err)
	}
	t.Cleanup(func() {
		db.WithContext(context.Background()).Exec("DELETE FROM conversations WHERE user_id = ?", owner.ID)
		db.WithContext(context.Background()).Exec("DELETE FROM users WHERE id IN ?", []uuid.UUID{owner.ID, other.ID})
	})
	conversation, err := conversationRepo.Create(ctx, owner.ID, &models.Conversation{Title: "context"})
	if err != nil {
		t.Fatalf("Create conversation error = %v", err)
	}
	throughID := uuid.New()
	state := &models.ConversationContextState{
		ConversationID: conversation.ID, Summary: "summary-1", ThroughMessageID: &throughID,
		Version: 1, FormatVersion: 1, PolicyFingerprint: "fingerprint",
	}

	saved, err := stateRepo.CompareAndSwapContextState(ctx, owner.ID, 0, state)
	if err != nil || !saved {
		t.Fatalf("CompareAndSwapContextState(create) saved=%v error=%v, want true nil", saved, err)
	}
	if saved, err = stateRepo.CompareAndSwapContextState(ctx, owner.ID, 0, state); err != nil || saved {
		t.Fatalf("CompareAndSwapContextState(stale create) saved=%v error=%v, want false nil", saved, err)
	}
	if got, err := stateRepo.LoadContextState(ctx, other.ID, conversation.ID); err != nil || got != nil {
		t.Fatalf("LoadContextState(other user) = %#v, error=%v, want nil nil", got, err)
	}

	state.Summary = "summary-2"
	state.Version = 2
	saved, err = stateRepo.CompareAndSwapContextState(ctx, owner.ID, 1, state)
	if err != nil || !saved {
		t.Fatalf("CompareAndSwapContextState(update) saved=%v error=%v, want true nil", saved, err)
	}
	loaded, err := stateRepo.LoadContextState(ctx, owner.ID, conversation.ID)
	if err != nil || loaded == nil || loaded.Summary != "summary-2" || loaded.Version != 2 {
		t.Fatalf("LoadContextState(owner) = %#v, error=%v, want summary-2 version 2", loaded, err)
	}

	if err := conversationRepo.Delete(ctx, owner.ID, conversation.ID); err != nil {
		t.Fatalf("Delete conversation error = %v", err)
	}
	var count int64
	if err := db.Model(&models.ConversationContextState{}).Where("conversation_id = ?", conversation.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("context state count after cascade = %d, error=%v, want 0 nil", count, err)
	}
}
