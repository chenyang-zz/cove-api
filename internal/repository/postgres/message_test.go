package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	repositorypostgres "github.com/boxify/api-go/internal/repository/postgres"
	"github.com/google/uuid"
)

// 验证 ListByConversationID 会按用户和会话过滤消息，并按创建时间升序返回。
func TestMessageRepositoryListByConversationIDWhenPostgresEnvIsConfigured(t *testing.T) {
	db := newAuthTestDB(t)
	ctx := context.Background()
	userRepo := repositorypostgres.NewUserRepository(db)
	conversationRepo := repositorypostgres.NewConversationRepository(db)
	messageRepo := repositorypostgres.NewMessageRepository(db)

	userA, err := userRepo.Create(ctx, &models.User{
		Username:     "message-history-a-" + uuid.NewString(),
		PasswordHash: "hash",
	})
	if err != nil {
		t.Fatalf("Create userA error = %v", err)
	}
	userB, err := userRepo.Create(ctx, &models.User{
		Username:     "message-history-b-" + uuid.NewString(),
		PasswordHash: "hash",
	})
	if err != nil {
		t.Fatalf("Create userB error = %v", err)
	}
	t.Cleanup(func() {
		db.WithContext(context.Background()).Exec("DELETE FROM conversations WHERE user_id IN ?", []uuid.UUID{userA.ID, userB.ID})
		db.WithContext(context.Background()).Exec("DELETE FROM users WHERE id IN ?", []uuid.UUID{userA.ID, userB.ID})
	})

	convA, err := conversationRepo.Create(ctx, userA.ID, &models.Conversation{Title: "a"})
	if err != nil {
		t.Fatalf("Create convA error = %v", err)
	}
	convOther, err := conversationRepo.Create(ctx, userA.ID, &models.Conversation{Title: "other"})
	if err != nil {
		t.Fatalf("Create convOther error = %v", err)
	}
	convB, err := conversationRepo.Create(ctx, userB.ID, &models.Conversation{Title: "b"})
	if err != nil {
		t.Fatalf("Create convB error = %v", err)
	}

	base := time.Now().Add(-time.Hour)
	rows := []*models.Message{
		{ConversationID: convA.ID, Role: "assistant", Content: "second", CreatedAt: base.Add(2 * time.Minute)},
		{ConversationID: convOther.ID, Role: "user", Content: "other conversation", CreatedAt: base.Add(time.Minute)},
		{ConversationID: convB.ID, Role: "user", Content: "other user", CreatedAt: base.Add(time.Minute)},
		{ConversationID: convA.ID, Role: "user", Content: "first", CreatedAt: base},
	}
	for _, row := range rows {
		if _, err := messageRepo.Create(ctx, rowOwner(row, userA.ID, userB.ID, convB.ID), row); err != nil {
			t.Fatalf("Create message %q error = %v", row.Content, err)
		}
	}

	got, err := messageRepo.ListByConversationID(ctx, userA.ID, convA.ID)
	if err != nil {
		t.Fatalf("ListByConversationID error = %v, want nil", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListByConversationID len = %d, want 2: %#v", len(got), got)
	}
	if got[0].Content != "first" || got[1].Content != "second" {
		t.Fatalf("ListByConversationID order/content = %q,%q; want first,second", got[0].Content, got[1].Content)
	}
}

func rowOwner(row *models.Message, userA uuid.UUID, userB uuid.UUID, convB uuid.UUID) uuid.UUID {
	if row.ConversationID == convB {
		return userB
	}
	return userA
}

// 验证 ListPage 支持最新页与 before 游标分页，并正确报告 has_more。
func TestMessageRepositoryListPageWhenPostgresEnvIsConfigured(t *testing.T) {
	db := newAuthTestDB(t)
	ctx := context.Background()
	userRepo := repositorypostgres.NewUserRepository(db)
	conversationRepo := repositorypostgres.NewConversationRepository(db)
	messageRepo := repositorypostgres.NewMessageRepository(db)

	user, err := userRepo.Create(ctx, &models.User{
		Username:     "message-page-" + uuid.NewString(),
		PasswordHash: "hash",
	})
	if err != nil {
		t.Fatalf("Create user error = %v", err)
	}
	t.Cleanup(func() {
		db.WithContext(context.Background()).Exec("DELETE FROM conversations WHERE user_id = ?", user.ID)
		db.WithContext(context.Background()).Exec("DELETE FROM users WHERE id = ?", user.ID)
	})

	conv, err := conversationRepo.Create(ctx, user.ID, &models.Conversation{Title: "page"})
	if err != nil {
		t.Fatalf("Create conversation error = %v", err)
	}

	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	contents := []string{"m1", "m2", "m3", "m4", "m5"}
	created := make([]*models.Message, 0, len(contents))
	for i, content := range contents {
		row, createErr := messageRepo.Create(ctx, user.ID, &models.Message{
			ConversationID: conv.ID,
			Role:           "user",
			Content:        content,
			CreatedAt:      base.Add(time.Duration(i) * time.Minute),
		})
		if createErr != nil {
			t.Fatalf("Create message %q error = %v", content, createErr)
		}
		created = append(created, row)
	}

	// 最新 2 条
	page1, hasMore, err := messageRepo.ListPage(ctx, user.ID, repository.MessageListQuery{
		ConversationID: conv.ID,
		Limit:          2,
	})
	if err != nil {
		t.Fatalf("ListPage latest error = %v, want nil", err)
	}
	if !hasMore {
		t.Fatal("ListPage latest hasMore = false, want true")
	}
	if len(page1) != 2 || page1[0].Content != "m4" || page1[1].Content != "m5" {
		t.Fatalf("ListPage latest = %#v, want m4,m5", contentsOf(page1))
	}

	// 继续向上加载更早的 2 条
	page2, hasMore, err := messageRepo.ListPage(ctx, user.ID, repository.MessageListQuery{
		ConversationID: conv.ID,
		BeforeID:       &page1[0].ID,
		Limit:          2,
	})
	if err != nil {
		t.Fatalf("ListPage before error = %v, want nil", err)
	}
	if !hasMore {
		t.Fatal("ListPage before hasMore = false, want true")
	}
	if len(page2) != 2 || page2[0].Content != "m2" || page2[1].Content != "m3" {
		t.Fatalf("ListPage before = %#v, want m2,m3", contentsOf(page2))
	}

	// 最后一页
	page3, hasMore, err := messageRepo.ListPage(ctx, user.ID, repository.MessageListQuery{
		ConversationID: conv.ID,
		BeforeID:       &page2[0].ID,
		Limit:          2,
	})
	if err != nil {
		t.Fatalf("ListPage last error = %v, want nil", err)
	}
	if hasMore {
		t.Fatal("ListPage last hasMore = true, want false")
	}
	if len(page3) != 1 || page3[0].Content != "m1" {
		t.Fatalf("ListPage last = %#v, want m1", contentsOf(page3))
	}

	// 非法 before
	missing := uuid.New()
	_, _, err = messageRepo.ListPage(ctx, user.ID, repository.MessageListQuery{
		ConversationID: conv.ID,
		BeforeID:       &missing,
		Limit:          2,
	})
	if err == nil {
		t.Fatal("ListPage invalid before error = nil, want error")
	}

	_ = created
}

func contentsOf(rows []*models.Message) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Content)
	}
	return out
}
