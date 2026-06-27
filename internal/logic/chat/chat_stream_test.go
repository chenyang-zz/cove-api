package chat

import (
	"context"
	"testing"
	"time"

	"github.com/boxify/api-go/internal/domain"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

func TestChatStreamUsesRealtimeBrokerPathWithoutRedis(t *testing.T) {
	userID := uuid.New()
	logic := NewChatStreamLogic(context.Background(), &svc.ServiceContext{
		ConversationRepo: &chatTestConversationRepository{},
		MessageRepo:      &chatTestMessageRepository{},
	})

	events, err := logic.ChatStream(userID, &request.ChatStreamRequest{Message: "hello"})
	if err != nil {
		t.Fatalf("ChatStream error = %v", err)
	}

	got := collectChatEvents(t, events)
	want := []string{domain.EventTypeMeta, domain.EventTypeToken, domain.EventTypeDone}
	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events = %v, want %v", got, want)
		}
	}
}

func collectChatEvents(t *testing.T, events <-chan domain.Event) []string {
	t.Helper()

	var got []string
	timeout := time.After(time.Second)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return got
			}
			got = append(got, event.EventName())
		case <-timeout:
			t.Fatalf("timed out collecting events: %v", got)
		}
	}
}

type chatTestConversationRepository struct {
	rows []*models.Conversation
}

func (r *chatTestConversationRepository) Create(ctx context.Context, userID uuid.UUID, row *models.Conversation) (*models.Conversation, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	row.UserID = userID
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *chatTestConversationRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.Conversation, error) {
	return nil, nil
}

func (r *chatTestConversationRepository) FindByID(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) (*models.Conversation, error) {
	for _, row := range r.rows {
		if row.ID == conversationID && row.UserID == userID {
			return row, nil
		}
	}
	return nil, xerr.NotFound("会话不存在")
}

func (r *chatTestConversationRepository) Update(ctx context.Context, userID uuid.UUID, row *models.Conversation) (*models.Conversation, error) {
	return row, nil
}

func (r *chatTestConversationRepository) UpdateFields(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID, row *models.Conversation, fields *repository.ConversationUpdateFields) (*models.Conversation, error) {
	return row, nil
}

func (r *chatTestConversationRepository) Delete(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) error {
	return nil
}

type chatTestMessageRepository struct {
	rows []*models.Message
}

func (r *chatTestMessageRepository) Create(ctx context.Context, userID uuid.UUID, row *models.Message) (*models.Message, error) {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	r.rows = append(r.rows, row)
	return row, nil
}

func (r *chatTestMessageRepository) List(ctx context.Context, userID uuid.UUID) ([]*models.Message, error) {
	return nil, nil
}

func (r *chatTestMessageRepository) FindByID(ctx context.Context, userID uuid.UUID, messageID uuid.UUID) (*models.Message, error) {
	return nil, xerr.NotFound("消息不存在")
}

func (r *chatTestMessageRepository) Update(ctx context.Context, userID uuid.UUID, row *models.Message) (*models.Message, error) {
	return row, nil
}

func (r *chatTestMessageRepository) UpdateFields(ctx context.Context, userID uuid.UUID, messageID uuid.UUID, row *models.Message, fields *repository.MessageUpdateFields) (*models.Message, error) {
	return row, nil
}

func (r *chatTestMessageRepository) Delete(ctx context.Context, userID uuid.UUID, messageID uuid.UUID) error {
	return nil
}

func (r *chatTestMessageRepository) Count(ctx context.Context, conversationID uuid.UUID) (int64, error) {
	return 0, nil
}
