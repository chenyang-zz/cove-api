package conversation

import (
	"context"
	"testing"
	"time"

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

func (r *fakeConversationRepository) PageList(ctx context.Context, userID uuid.UUID, query repository.ConversationListQuery) ([]*models.Conversation, int64, error) {
	r.listUserID = userID
	all, err := r.List(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	total := int64(len(all))
	limit, offset := query.LimitOffset(20)
	if offset >= len(all) {
		return []*models.Conversation{}, total, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], total, nil
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

// 验证 ListConversations 分页只返回当前用户会话并带 total/page 元信息。
func TestListConversationsUsesAuthenticatedUser(t *testing.T) {
	userID := uuid.New()
	otherUserID := uuid.New()
	repo := newFakeConversationRepository(
		&models.Conversation{ID: uuid.New(), UserID: userID, Title: "mine"},
		&models.Conversation{ID: uuid.New(), UserID: otherUserID, Title: "other"},
	)
	logic := NewListConversationsLogic(context.Background(), &svc.ServiceContext{ConversationRepo: repo})

	out, err := logic.ListConversations(userID, &request.ListConversationsRequest{
		PageRequest: request.PageRequest{Page: 1, PageSize: 20},
	})
	if err != nil {
		t.Fatalf("ListConversations error = %v", err)
	}
	if repo.listUserID != userID {
		t.Fatalf("list userID = %s, want %s", repo.listUserID, userID)
	}
	if out.Total != 1 || out.Page != 1 || out.PageSize != 20 {
		t.Fatalf("page meta = total:%d page:%d page_size:%d, want 1/1/20", out.Total, out.Page, out.PageSize)
	}
	if len(out.List) != 1 || out.List[0].Title != "mine" {
		t.Fatalf("list = %+v, want only authenticated user's conversations", out.List)
	}
}

// 验证 ListConversations 分页截断。
func TestListConversationsPaginates(t *testing.T) {
	userID := uuid.New()
	repo := newFakeConversationRepository(
		&models.Conversation{ID: uuid.New(), UserID: userID, Title: "a"},
		&models.Conversation{ID: uuid.New(), UserID: userID, Title: "b"},
		&models.Conversation{ID: uuid.New(), UserID: userID, Title: "c"},
	)
	logic := NewListConversationsLogic(context.Background(), &svc.ServiceContext{ConversationRepo: repo})

	out, err := logic.ListConversations(userID, &request.ListConversationsRequest{
		PageRequest: request.PageRequest{Page: 1, PageSize: 2},
	})
	if err != nil {
		t.Fatalf("ListConversations error = %v", err)
	}
	if out.Total != 3 || len(out.List) != 2 {
		t.Fatalf("ListConversations total=%d len=%d, want total 3 len 2", out.Total, len(out.List))
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

type fakeMessageRepository struct {
	rows []*models.Message
}

func (r *fakeMessageRepository) Create(ctx context.Context, userID uuid.UUID, row *models.Message) (*models.Message, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *fakeMessageRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.Message, error) {
	return append([]*models.Message(nil), r.rows...), nil
}

func (r *fakeMessageRepository) ListByConversationID(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) ([]*models.Message, error) {
	out := make([]*models.Message, 0)
	for _, row := range r.rows {
		if row.ConversationID == conversationID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeMessageRepository) ListPage(ctx context.Context, userID uuid.UUID, query repository.MessageListQuery) ([]*models.Message, bool, error) {
	rows, err := r.ListByConversationID(ctx, userID, query.ConversationID)
	if err != nil {
		return nil, false, err
	}
	// 按 created_at ASC 再取尾部最新页
	sortByCreatedAtAsc(rows)
	limit := query.Limit
	if limit < 1 {
		limit = 30
	}
	if query.BeforeID != nil {
		var cursor *models.Message
		for _, row := range rows {
			if row.ID == *query.BeforeID {
				cursor = row
				break
			}
		}
		if cursor == nil {
			return nil, false, xerr.BadRequest("before 消息不存在或不属于该会话")
		}
		filtered := make([]*models.Message, 0, len(rows))
		for _, row := range rows {
			if row.CreatedAt.Before(cursor.CreatedAt) || (row.CreatedAt.Equal(cursor.CreatedAt) && row.ID.String() < cursor.ID.String()) {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	if len(rows) <= limit {
		return rows, false, nil
	}
	return rows[len(rows)-limit:], true, nil
}

func (r *fakeMessageRepository) FindByID(ctx context.Context, userID uuid.UUID, messageID uuid.UUID) (*models.Message, error) {
	for _, row := range r.rows {
		if row.ID == messageID {
			return row, nil
		}
	}
	return nil, xerr.NotFound("消息不存在")
}

func (r *fakeMessageRepository) Update(ctx context.Context, userID uuid.UUID, row *models.Message) (*models.Message, error) {
	return row, nil
}

func (r *fakeMessageRepository) UpdateFields(ctx context.Context, userID uuid.UUID, messageID uuid.UUID, row *models.Message, fields *repository.MessageUpdateFields) (*models.Message, error) {
	return row, nil
}

func (r *fakeMessageRepository) Delete(ctx context.Context, userID uuid.UUID, messageID uuid.UUID) error {
	return nil
}

func (r *fakeMessageRepository) Count(ctx context.Context, conversationID uuid.UUID) (int64, error) {
	var n int64
	for _, row := range r.rows {
		if row.ConversationID == conversationID {
			n++
		}
	}
	return n, nil
}

type fakeMessageFeedbackRepository struct {
	rows []*models.MessageFeedback
}

func (r *fakeMessageFeedbackRepository) Create(ctx context.Context, userID uuid.UUID, row *models.MessageFeedback) (*models.MessageFeedback, error) {
	return row, nil
}

func (r *fakeMessageFeedbackRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.MessageFeedback, error) {
	return r.rows, nil
}

func (r *fakeMessageFeedbackRepository) ListByConversationID(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) ([]*models.MessageFeedback, error) {
	out := make([]*models.MessageFeedback, 0)
	for _, row := range r.rows {
		if row.ConversationID == conversationID && row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeMessageFeedbackRepository) FindByID(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*models.MessageFeedback, error) {
	return nil, xerr.NotFound("消息反馈不存在")
}

func (r *fakeMessageFeedbackRepository) Update(ctx context.Context, userID uuid.UUID, row *models.MessageFeedback) (*models.MessageFeedback, error) {
	return row, nil
}

func (r *fakeMessageFeedbackRepository) UpdateFields(ctx context.Context, userID uuid.UUID, id uuid.UUID, row *models.MessageFeedback, fields *repository.MessageFeedbackUpdateFields) (*models.MessageFeedback, error) {
	return row, nil
}

func (r *fakeMessageFeedbackRepository) Delete(ctx context.Context, userID uuid.UUID, id uuid.UUID) error {
	return nil
}

func sortByCreatedAtAsc(rows []*models.Message) {
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			if rows[j].CreatedAt.Before(rows[i].CreatedAt) {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
}

// 验证 ListMessages 按会话分页返回最近消息并报告 has_more。
func TestListMessagesReturnsLatestPageWithHasMore(t *testing.T) {
	userID := uuid.New()
	conversationID := uuid.New()
	otherConversationID := uuid.New()
	base := time.Now().Add(-time.Hour)

	msgRepo := &fakeMessageRepository{rows: []*models.Message{
		{ID: uuid.New(), ConversationID: conversationID, Role: "user", Content: "m1", CreatedAt: base},
		{ID: uuid.New(), ConversationID: conversationID, Role: "assistant", Content: "m2", CreatedAt: base.Add(time.Minute)},
		{ID: uuid.New(), ConversationID: conversationID, Role: "user", Content: "m3", CreatedAt: base.Add(2 * time.Minute)},
		{ID: uuid.New(), ConversationID: otherConversationID, Role: "user", Content: "other", CreatedAt: base.Add(3 * time.Minute)},
	}}
	convRepo := newFakeConversationRepository(&models.Conversation{
		ID:     conversationID,
		UserID: userID,
		Title:  "chat",
	})
	feedbackRepo := &fakeMessageFeedbackRepository{rows: []*models.MessageFeedback{
		{MessageID: msgRepo.rows[2].ID, ConversationID: conversationID, UserID: userID, Rating: "up"},
		{MessageID: msgRepo.rows[0].ID, ConversationID: conversationID, UserID: userID, Rating: "down"},
	}}

	logic := NewListMessagesLogic(context.Background(), &svc.ServiceContext{
		ConversationRepo:    convRepo,
		MessageRepo:         msgRepo,
		MessageFeedbackRepo: feedbackRepo,
	})

	out, err := logic.ListMessages(userID, &request.ListMessagesRequest{
		UriConversationIDRequest: request.UriConversationIDRequest{ConversationID: conversationID.String()},
		Limit:                    2,
	})
	if err != nil {
		t.Fatalf("ListMessages error = %v, want nil", err)
	}
	if out == nil || !out.HasMore {
		t.Fatalf("ListMessages has_more = %+v, want true", out)
	}
	if len(out.List) != 2 || out.List[0].Content != "m2" || out.List[1].Content != "m3" {
		t.Fatalf("ListMessages list = %+v, want m2,m3", out.List)
	}
	if out.List[1].Feedback == nil || *out.List[1].Feedback != "up" {
		t.Fatalf("ListMessages feedback = %+v, want up on latest message", out.List[1].Feedback)
	}

	// 向上滚动加载更早消息
	older, err := logic.ListMessages(userID, &request.ListMessagesRequest{
		UriConversationIDRequest: request.UriConversationIDRequest{ConversationID: conversationID.String()},
		Limit:                    2,
		Before:                   out.List[0].ID.String(),
	})
	if err != nil {
		t.Fatalf("ListMessages before error = %v, want nil", err)
	}
	if older.HasMore {
		t.Fatalf("ListMessages before has_more = true, want false")
	}
	if len(older.List) != 1 || older.List[0].Content != "m1" {
		t.Fatalf("ListMessages before list = %+v, want m1", older.List)
	}
}

// 验证 ListMessages 对不存在的会话返回 not found。
func TestListMessagesRejectsMissingConversation(t *testing.T) {
	logic := NewListMessagesLogic(context.Background(), &svc.ServiceContext{
		ConversationRepo:    newFakeConversationRepository(),
		MessageRepo:         &fakeMessageRepository{},
		MessageFeedbackRepo: &fakeMessageFeedbackRepository{},
	})

	_, err := logic.ListMessages(uuid.New(), &request.ListMessagesRequest{
		UriConversationIDRequest: request.UriConversationIDRequest{ConversationID: uuid.New().String()},
	})
	if xerr.From(err).Kind != xerr.KindNotFound {
		t.Fatalf("ListMessages error = %v, want not found", err)
	}
}
