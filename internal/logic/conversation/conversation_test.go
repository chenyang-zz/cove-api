package conversation

import (
	"context"
	"testing"

	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

type fakeConversationRepository struct {
	rows       map[uuid.UUID]*models.Conversation
	created    *models.Conversation
	listUserID uuid.UUID
	updated    *models.Conversation
	partial    *models.Conversation
	fields     []string
	deletedID  uuid.UUID
}

func newFakeConversationRepository(rows ...*models.Conversation) *fakeConversationRepository {
	repo := &fakeConversationRepository{rows: map[uuid.UUID]*models.Conversation{}}
	for _, row := range rows {
		repo.rows[row.ID] = row
	}
	return repo
}

func (r *fakeConversationRepository) Create(ctx context.Context, userID uuid.UUID, row *models.Conversation) (*models.Conversation, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	row.UserID = userID
	r.created = row
	r.rows[row.ID] = row
	return row, nil
}

func (r *fakeConversationRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.Conversation, error) {
	r.listUserID = userID
	out := make([]*models.Conversation, 0, len(r.rows))
	for _, row := range r.rows {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeConversationRepository) FindByID(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) (*models.Conversation, error) {
	row, ok := r.rows[conversationID]
	if !ok || row.UserID != userID {
		return nil, xerr.NotFound("会话不存在")
	}
	return row, nil
}

func (r *fakeConversationRepository) Update(ctx context.Context, userID uuid.UUID, row *models.Conversation) (*models.Conversation, error) {
	if existing, ok := r.rows[row.ID]; !ok || existing.UserID != userID {
		return nil, xerr.NotFound("会话不存在")
	}
	row.UserID = userID
	r.updated = row
	r.rows[row.ID] = row
	return row, nil
}

func (r *fakeConversationRepository) UpdateFields(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID, row *models.Conversation, fields *repository.ConversationUpdateFields) (*models.Conversation, error) {
	existing, ok := r.rows[conversationID]
	if !ok || existing.UserID != userID {
		return nil, xerr.NotFound("会话不存在")
	}
	r.partial = row
	r.fields = fields.Columns()
	existing.Title = row.Title
	return existing, nil
}

func (r *fakeConversationRepository) Delete(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) error {
	if row, ok := r.rows[conversationID]; !ok || row.UserID != userID {
		return xerr.NotFound("会话不存在")
	}
	r.deletedID = conversationID
	delete(r.rows, conversationID)
	return nil
}

func TestCreateConversationUsesAuthenticatedUserAndDefaultTitle(t *testing.T) {
	userID := uuid.New()
	repo := newFakeConversationRepository()
	logic := NewCreateConversationLogic(context.Background(), &svc.ServiceContext{ConversationRepo: repo})

	out, err := logic.CreateConversation(userID, &request.CreateConversationRequest{})
	if err != nil {
		t.Fatalf("CreateConversation error = %v", err)
	}
	if repo.created == nil {
		t.Fatal("repository Create was not called")
	}
	if repo.created.UserID != userID {
		t.Fatalf("created userID = %s, want %s", repo.created.UserID, userID)
	}
	if out.Title != "新对话" {
		t.Fatalf("title = %q, want default", out.Title)
	}
}

func TestListConversationsUsesAuthenticatedUser(t *testing.T) {
	userID := uuid.New()
	otherUserID := uuid.New()
	repo := newFakeConversationRepository(
		&models.Conversation{ID: uuid.New(), UserID: userID, Title: "mine"},
		&models.Conversation{ID: uuid.New(), UserID: otherUserID, Title: "other"},
	)
	logic := NewListConversationsLogic(context.Background(), &svc.ServiceContext{ConversationRepo: repo})

	out, err := logic.ListConversations(userID)
	if err != nil {
		t.Fatalf("ListConversations error = %v", err)
	}
	if repo.listUserID != userID {
		t.Fatalf("list userID = %s, want %s", repo.listUserID, userID)
	}
	if len(out.List) != 1 || out.List[0].Title != "mine" {
		t.Fatalf("list = %+v, want only authenticated user's conversations", out.List)
	}
}

func TestRenameConversationRejectsInvalidConversationID(t *testing.T) {
	repo := newFakeConversationRepository()
	logic := NewRenameConversationLogic(context.Background(), &svc.ServiceContext{ConversationRepo: repo})

	var err error
	if _, err = logic.RenameConversation(uuid.New(), nil); xerr.From(err).Kind != xerr.KindBadRequest {
		t.Fatalf("RenameConversation nil input error = %v, want bad request", err)
	}

	_, err = logic.RenameConversation(uuid.New(), &request.RenameConversationRequest{
		Title: "new title",
	})
	if xerr.From(err).Kind != xerr.KindBadRequest {
		t.Fatalf("RenameConversation missing ID error = %v, want bad request", err)
	}

	_, err = logic.RenameConversation(uuid.New(), &request.RenameConversationRequest{
		UriConversationIDRequest: request.UriConversationIDRequest{ConversationID: "not-a-uuid"},
		Title:                    "new title",
	})
	if xerr.From(err).Kind != xerr.KindBadRequest {
		t.Fatalf("RenameConversation error = %v, want bad request", err)
	}
}

func TestRenameConversationCannotUpdateAnotherUsersConversation(t *testing.T) {
	ownerID := uuid.New()
	otherUserID := uuid.New()
	conversationID := uuid.New()
	repo := newFakeConversationRepository(&models.Conversation{
		ID:     conversationID,
		UserID: ownerID,
		Title:  "private",
	})
	logic := NewRenameConversationLogic(context.Background(), &svc.ServiceContext{ConversationRepo: repo})

	_, err := logic.RenameConversation(otherUserID, &request.RenameConversationRequest{
		UriConversationIDRequest: request.UriConversationIDRequest{ConversationID: conversationID.String()},
		Title:                    "stolen",
	})
	if xerr.From(err).Kind != xerr.KindNotFound {
		t.Fatalf("RenameConversation error = %v, want not found", err)
	}
	if repo.updated != nil {
		t.Fatalf("cross-user rename updated row: %+v", repo.updated)
	}
}

func TestRenameConversationUpdatesOwnedConversation(t *testing.T) {
	userID := uuid.New()
	conversationID := uuid.New()
	repo := newFakeConversationRepository(&models.Conversation{
		ID:     conversationID,
		UserID: userID,
		Title:  "old",
	})
	logic := NewRenameConversationLogic(context.Background(), &svc.ServiceContext{ConversationRepo: repo})

	out, err := logic.RenameConversation(userID, &request.RenameConversationRequest{
		UriConversationIDRequest: request.UriConversationIDRequest{ConversationID: conversationID.String()},
		Title:                    "new",
	})
	if err != nil {
		t.Fatalf("RenameConversation error = %v", err)
	}
	if repo.updated != nil {
		t.Fatalf("rename used full update path: %+v", repo.updated)
	}
	if repo.partial == nil || repo.partial.Title != "new" {
		t.Fatalf("partial update = %+v, want title new", repo.partial)
	}
	if len(repo.fields) != 1 || repo.fields[0] != "title" {
		t.Fatalf("partial fields = %v, want [title]", repo.fields)
	}
	if out.Title != "new" || out.ID != conversationID {
		t.Fatalf("response = %+v, want renamed conversation", out)
	}
}
